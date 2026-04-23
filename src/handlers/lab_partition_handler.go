package handlers

import (
	"context"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

// LabPartitionHandler は Postgres の RANGE パーティションと partition pruning を
// ブラウザから観察するための lab ハンドラ。events テーブル (月次 RANGE) に対して
// 日付範囲クエリを実行し、EXPLAIN ANALYZE と「実際に触られた partition 一覧」を返す。
type LabPartitionHandler struct {
	pool *pgxpool.Pool
}

// NewLabPartitionHandler は POSTGRES_DSN で Postgres pool を作る。
// DSN 未設定 or 接続不能の場合は nil handler を返し、上位で /api/lab/partition を無効化する。
func NewLabPartitionHandler() (*LabPartitionHandler, error) {
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
	return &LabPartitionHandler{pool: pool}, nil
}

type partitionInfo struct {
	Name    string `json:"name"`
	Scanned bool   `json:"scanned"`
}

type labPartitionResp struct {
	From              string          `json:"from"`
	To                string          `json:"to"`
	Count             int64           `json:"count"`
	ElapsedMs         int64           `json:"elapsedMs"`
	Partitions        []partitionInfo `json:"partitions"`
	ScannedPartitions []string        `json:"scannedPartitions"`
	Explain           string          `json:"explain"`
}

// Query は ?from=...&to=... で events を絞り込み、件数 / EXPLAIN ANALYZE /
// どの partition が scan されたかを返す。partition pruning の効果を可視化するのが狙い。
//
// API の契約: from/to は ISO8601 (YYYY-MM-DD or RFC3339) を想定。パースしたうえで
// パラメータ化クエリに渡すので SQL インジェクションの経路はない。
func (h *LabPartitionHandler) Query(c echo.Context) error {
	from, to, err := parseFromTo(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()

	// 件数は素直に COUNT(*)。小さいテーブルなので充分。
	start := time.Now()
	var count int64
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE event_at >= $1 AND event_at < $2`,
		from, to,
	).Scan(&count); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	elapsed := time.Since(start).Milliseconds()

	// EXPLAIN ANALYZE で実行計画を取得。ANALYZE を付けると実際に実行される点に注意
	// (本番で重いクエリに ANALYZE を付けると同じコストがかかる)。
	explainText, err := h.explain(ctx, from, to)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// 全 partition 名と、EXPLAIN に現れた partition 名の差分を取って
	// UI 側で「prune されたか否か」をハイライトできるようにする。
	allPartitions, err := h.listPartitions(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	scanned := extractScannedPartitions(explainText, allPartitions)

	scannedSet := make(map[string]struct{}, len(scanned))
	for _, name := range scanned {
		scannedSet[name] = struct{}{}
	}
	partitions := make([]partitionInfo, 0, len(allPartitions))
	for _, name := range allPartitions {
		_, hit := scannedSet[name]
		partitions = append(partitions, partitionInfo{Name: name, Scanned: hit})
	}

	return c.JSON(http.StatusOK, labPartitionResp{
		From:              from.Format(time.RFC3339),
		To:                to.Format(time.RFC3339),
		Count:             count,
		ElapsedMs:         elapsed,
		Partitions:        partitions,
		ScannedPartitions: scanned,
		Explain:           explainText,
	})
}

// parseFromTo は ?from=&to= を time.Time に。YYYY-MM-DD 形式を優先で受け付け、
// RFC3339 へのフォールバックも用意しておく (UI 側の値に揺れが出ても落ちないように)。
func parseFromTo(c echo.Context) (time.Time, time.Time, error) {
	fromStr := c.QueryParam("from")
	toStr := c.QueryParam("to")
	if fromStr == "" || toStr == "" {
		return time.Time{}, time.Time{}, echo.NewHTTPError(http.StatusBadRequest, "from and to query params are required (YYYY-MM-DD)")
	}
	from, err := parseDateParam(fromStr)
	if err != nil {
		return time.Time{}, time.Time{}, echo.NewHTTPError(http.StatusBadRequest, "invalid from: "+err.Error())
	}
	to, err := parseDateParam(toStr)
	if err != nil {
		return time.Time{}, time.Time{}, echo.NewHTTPError(http.StatusBadRequest, "invalid to: "+err.Error())
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, echo.NewHTTPError(http.StatusBadRequest, "to must be after from")
	}
	return from, to, nil
}

func parseDateParam(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	return time.Parse(time.RFC3339, s)
}

// explain は EXPLAIN (ANALYZE, BUFFERS) の結果をテキストで取得する。
// pgx は複数行返却されるので行ごとに Scan して結合する。
func (h *LabPartitionHandler) explain(ctx context.Context, from, to time.Time) (string, error) {
	rows, err := h.pool.Query(ctx,
		`EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM events WHERE event_at >= $1 AND event_at < $2`,
		from, to,
	)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return "", err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}

// listPartitions は events テーブルの child partition 名を返す。
// pg_inherits を見れば親子関係が分かる。
func (h *LabPartitionHandler) listPartitions(ctx context.Context) ([]string, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT c.relname
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname = 'events'
		ORDER BY c.relname
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// extractScannedPartitions は EXPLAIN 出力から「実際に scan された partition 名」を拾う。
// "Seq Scan on events_2026_01" や "Index Scan using ... on events_2026_01" の
// ような行に出てくる partition 名を集める。
// 同じ partition が複数ノードに現れても dedup する。
func extractScannedPartitions(explainText string, allPartitions []string) []string {
	if len(allPartitions) == 0 {
		return nil
	}
	known := make(map[string]struct{}, len(allPartitions))
	for _, p := range allPartitions {
		known[p] = struct{}{}
	}

	// 単語境界 + 既知の partition 名のみを拾う正規表現。
	// EXPLAIN 出力中の他の箇所 (例: "never executed") に同名が出ないので
	// これで十分。
	re := regexp.MustCompile(`\bevents_[a-z0-9_]+\b`)
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, m := range re.FindAllString(explainText, -1) {
		if _, ok := known[m]; !ok {
			continue
		}
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out
}
