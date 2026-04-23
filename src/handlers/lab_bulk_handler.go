package handlers

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

// LabBulkHandler は大量レコードを Postgres に投入する際の
// pgx.CopyFrom (COPY FROM STDIN) と 1 件ずつの INSERT ループを比較する lab ハンドラ。
// CSV を受け取り、method に応じて 2 方式を切り替え、所要時間を返す。
type LabBulkHandler struct {
	pool *pgxpool.Pool
}

// NewLabBulkHandler は POSTGRES_DSN から pgxpool を作る。
// DSN 未設定 or 接続不能なら nil を返し、上位で /api/lab/bulk を無効化する。
func NewLabBulkHandler() (*LabBulkHandler, error) {
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
	return &LabBulkHandler{pool: pool}, nil
}

type bulkRow struct {
	name  string
	email string
	score int
}

type labBulkResp struct {
	Method    string `json:"method"`
	Rows      int    `json:"rows"`
	ElapsedMs int64  `json:"elapsedMs"`
	RowsPerMs string `json:"rowsPerMs"`
}

type labBulkCountResp struct {
	Count int64 `json:"count"`
}

// Count は bulk_samples の現在の件数を返す。UI で「投入後の残存」を確認するための補助。
func (h *LabBulkHandler) Count(c echo.Context) error {
	ctx := c.Request().Context()
	var count int64
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM bulk_samples`).Scan(&count); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, labBulkCountResp{Count: count})
}

// Import は CSV を受け取り、method=copy | insert で bulk_samples に投入する。
// 事前に TRUNCATE し、計測区間を「投入方式そのもの」だけに絞ることで比較を意味ある形にする。
//
// CSV の期待フォーマット (ヘッダ必須):
//
//	name,email,score
//	alice,alice@example.com,42
//	...
func (h *LabBulkHandler) Import(c echo.Context) error {
	method := c.FormValue("method")
	if method != "copy" && method != "insert" {
		return echo.NewHTTPError(http.StatusBadRequest, "method must be 'copy' or 'insert'")
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file is required")
	}
	f, err := fh.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	defer f.Close()

	rows, err := parseBulkCSV(f)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if len(rows) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "CSV has no data rows")
	}

	ctx := c.Request().Context()

	// 毎リクエスト TRUNCATE。直前の投入件数が混ざると所要時間の比較が狂う。
	// 本番ではこんなことはしないが、lab のベンチ目的なら単純化を優先する。
	if _, err := h.pool.Exec(ctx, `TRUNCATE TABLE bulk_samples RESTART IDENTITY`); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "truncate: "+err.Error())
	}

	start := time.Now()
	switch method {
	case "copy":
		err = h.importCopy(ctx, rows)
	case "insert":
		err = h.importInsertLoop(ctx, rows)
	}
	elapsed := time.Since(start)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	elapsedMs := elapsed.Milliseconds()
	rowsPerMs := "-"
	if elapsedMs > 0 {
		rowsPerMs = strconv.FormatFloat(float64(len(rows))/float64(elapsedMs), 'f', 2, 64)
	}
	return c.JSON(http.StatusOK, labBulkResp{
		Method:    method,
		Rows:      len(rows),
		ElapsedMs: elapsedMs,
		RowsPerMs: rowsPerMs,
	})
}

// importCopy は pgx.CopyFrom で一括投入する。COPY プロトコルはクライアントから
// バイナリに近い形でストリームするので、1 件ずつの往復が発生せず大量投入に圧倒的に強い。
func (h *LabBulkHandler) importCopy(ctx context.Context, rows []bulkRow) error {
	src := pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
		r := rows[i]
		return []any{r.name, r.email, r.score}, nil
	})
	_, err := h.pool.CopyFrom(
		ctx,
		pgx.Identifier{"bulk_samples"},
		[]string{"name", "email", "score"},
		src,
	)
	return err
}

// importInsertLoop は 1 件ずつ INSERT を送る愚直な方式。
// 1 トランザクションに包むことで COMMIT 回数は 1 回に抑えているが、ネットワーク往復は行数ぶん発生する。
// あえて prepared statement や batch も使わず、COPY との差を際立たせる。
func (h *LabBulkHandler) importInsertLoop(ctx context.Context, rows []bulkRow) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, r := range rows {
		if _, err := tx.Exec(ctx,
			`INSERT INTO bulk_samples (name, email, score) VALUES ($1, $2, $3)`,
			r.name, r.email, r.score,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// parseBulkCSV はヘッダ付き CSV を bulkRow の slice に。列順は name,email,score で固定。
// スキーマが揺れると COPY 側の列順とズレてハマるので、寛容にせず固定で扱う。
func parseBulkCSV(r io.Reader) ([]bulkRow, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = 3

	header, err := reader.Read()
	if err != nil {
		return nil, errors.New("failed to read header: " + err.Error())
	}
	if len(header) != 3 || header[0] != "name" || header[1] != "email" || header[2] != "score" {
		return nil, errors.New("CSV header must be: name,email,score")
	}

	out := make([]bulkRow, 0, 1024)
	for i := 2; ; i++ {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("row " + strconv.Itoa(i) + ": " + err.Error())
		}
		score, convErr := strconv.Atoi(rec[2])
		if convErr != nil {
			return nil, errors.New("row " + strconv.Itoa(i) + ": score must be int")
		}
		out = append(out, bulkRow{name: rec[0], email: rec[1], score: score})
	}
	return out, nil
}
