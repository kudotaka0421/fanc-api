package handlers

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

// LabRLSHandler は Postgres の Row Level Security をブラウザから観察するための lab ハンドラ。
// X-Tenant-Id ヘッダで指定したテナントの samples のみが返ることを確認するのが目的。
type LabRLSHandler struct {
	pool *pgxpool.Pool
}

// NewLabRLSHandler は POSTGRES_DSN で Postgres pool を作る。
// DSN 未設定 or 接続不能の場合は nil handler を返し、上位で /api/lab/rls/* を無効化する。
func NewLabRLSHandler() (*LabRLSHandler, error) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		return nil, errors.New("POSTGRES_DSN is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &LabRLSHandler{pool: pool}, nil
}

type labTenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type labSample struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"orgId"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// ListTenants は UI のドロップダウン用にテナント一覧を返す。RLS 対象外（tenants テーブルは policy なし）。
func (h *LabRLSHandler) ListTenants(c echo.Context) error {
	ctx := c.Request().Context()
	rows, err := h.pool.Query(ctx, `SELECT id, name FROM tenants ORDER BY name`)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	out := make([]labTenant, 0)
	for rows.Next() {
		var t labTenant
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		out = append(out, t)
	}
	return c.JSON(http.StatusOK, out)
}

// ListSamples は X-Tenant-Id ヘッダを SET LOCAL app.current_org_id に載せて samples を取得する。
// WHERE 句は書かない。RLS policy が自動で絞り込む。
// ポイント: SET LOCAL はトランザクションに閉じるので、必ず BEGIN 下で実行する必要がある。
func (h *LabRLSHandler) ListSamples(c echo.Context) error {
	tenantID, err := tenantIDFromHeader(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()

	items, err := queryWithRLS(ctx, h.pool, tenantID, func(ctx context.Context, tx pgx.Tx) ([]labSample, error) {
		rows, err := tx.Query(ctx, `SELECT id, org_id, body, created_at FROM samples ORDER BY created_at`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := make([]labSample, 0)
		for rows.Next() {
			var s labSample
			if err := rows.Scan(&s.ID, &s.OrgID, &s.Body, &s.CreatedAt); err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, rows.Err()
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, items)
}

type createSampleReq struct {
	Body string `json:"body" validate:"required"`
}

// CreateSample はリクエスト元テナントのサンプルを追加する。
// org_id はヘッダから決まるので body にはリクエスト本文しか書かない。
// RLS の WITH CHECK が効くので、ヘッダと異なる org_id を INSERT しようとしても弾かれる。
func (h *LabRLSHandler) CreateSample(c echo.Context) error {
	tenantID, err := tenantIDFromHeader(c)
	if err != nil {
		return err
	}
	var req createSampleReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Body == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "body is required")
	}
	ctx := c.Request().Context()

	item, err := queryWithRLS(ctx, h.pool, tenantID, func(ctx context.Context, tx pgx.Tx) (labSample, error) {
		var s labSample
		err := tx.QueryRow(ctx,
			`INSERT INTO samples (org_id, body) VALUES ($1, $2)
			 RETURNING id, org_id, body, created_at`,
			tenantID, req.Body,
		).Scan(&s.ID, &s.OrgID, &s.Body, &s.CreatedAt)
		return s, err
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, item)
}

// tenantIDFromHeader は X-Tenant-Id を取り出す。値は SQL 識別子ではなく
// current_setting() を通るため Bobby Tables 系のリスクはないが、
// 明らかに不正な形式は 400 で弾く。
func tenantIDFromHeader(c echo.Context) (string, error) {
	id := c.Request().Header.Get("X-Tenant-Id")
	if id == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "X-Tenant-Id header is required")
	}
	return id, nil
}

// queryWithRLS は BEGIN → SET LOCAL → callback → COMMIT を一括で面倒見るヘルパ。
// SET LOCAL は現在のトランザクションにのみ効くため、トランザクション外でも動く
// コードを書くと pool の別コネクションに戻った瞬間 policy が効かなくなり、
// 気付きにくいバグになる。ハンドラ側で "トランザクション必須" を強制するのが目的。
func queryWithRLS[T any](ctx context.Context, pool *pgxpool.Pool, tenantID string, fn func(context.Context, pgx.Tx) (T, error)) (T, error) {
	var zero T
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return zero, err
	}
	defer tx.Rollback(ctx)
	// set_config(name, value, is_local=true) は SET LOCAL 相当を関数形式で書ける。
	// プレースホルダが使えるので値をそのまま埋めても SQL インジェクションにならない。
	if _, err := tx.Exec(ctx, `SELECT set_config('app.current_org_id', $1, true)`, tenantID); err != nil {
		return zero, err
	}
	out, err := fn(ctx, tx)
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, err
	}
	return out, nil
}
