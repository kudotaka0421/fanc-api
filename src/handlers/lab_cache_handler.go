package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"fanc-api/src/models"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// LabCacheHandler は Redis による Cache-Aside パターンを体験するための lab 用ハンドラ。
// GET は HIT なら Redis、MISS なら MySQL から読み Redis に書き戻す。
// DB 側には擬似遅延を入れてキャッシュ効果を視覚化する。
type LabCacheHandler struct {
	db    *gorm.DB
	rdb   *redis.Client
	key   string
	ttl   time.Duration
	delay time.Duration
}

const (
	labCacheKey        = "lab:schools:v1"
	labCacheTTL        = 60 * time.Second
	labCacheDBFakeWait = 300 * time.Millisecond
)

// NewLabCacheHandler は Redis クライアントと DB を受け取って初期化する。
// 既存 lab ハンドラ群にならい、接続不能でも init 自体は成功させる（実リクエスト時にエラー）。
func NewLabCacheHandler(db *gorm.DB) (*LabCacheHandler, error) {
	addr := getenv("REDIS_ADDR", "redis:6379")
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	return &LabCacheHandler{
		db:    db,
		rdb:   rdb,
		key:   labCacheKey,
		ttl:   labCacheTTL,
		delay: labCacheDBFakeWait,
	}, nil
}

type labSchool struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	MonthlyFee int    `json:"monthlyFee"`
}

type cacheGetRes struct {
	Source    string      `json:"source"`
	ElapsedMs int64       `json:"elapsedMs"`
	Schools   []labSchool `json:"schools"`
}

// GetSchools は Cache-Aside で schools 一覧を返す。
// HIT → Redis からそのまま、MISS → DB 取得後に Redis へ保存。
func (h *LabCacheHandler) GetSchools(c echo.Context) error {
	ctx := c.Request().Context()
	start := time.Now()

	if val, err := h.rdb.Get(ctx, h.key).Bytes(); err == nil {
		var items []labSchool
		if jerr := json.Unmarshal(val, &items); jerr == nil {
			return c.JSON(http.StatusOK, cacheGetRes{
				Source:    "cache",
				ElapsedMs: time.Since(start).Milliseconds(),
				Schools:   items,
			})
		}
	} else if !errors.Is(err, redis.Nil) {
		// Redis 障害でも DB にフォールバックする（キャッシュは可用性を下げない設計が基本）
		c.Logger().Warnf("cache get failed, fallback to db: %s", err.Error())
	}

	items, err := h.loadSchoolsFromDB(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if payload, jerr := json.Marshal(items); jerr == nil {
		if serr := h.rdb.Set(ctx, h.key, payload, h.ttl).Err(); serr != nil {
			c.Logger().Warnf("cache set failed: %s", serr.Error())
		}
	}

	return c.JSON(http.StatusOK, cacheGetRes{
		Source:    "db",
		ElapsedMs: time.Since(start).Milliseconds(),
		Schools:   items,
	})
}

// DeleteCache はキャッシュ key を削除する。UI の「キャッシュ削除」ボタンから呼ばれる。
func (h *LabCacheHandler) DeleteCache(c echo.Context) error {
	if err := h.rdb.Del(c.Request().Context(), h.key).Err(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"deleted": h.key})
}

// loadSchoolsFromDB は schools を id/name/monthly_fee のみ読む。
// DB 呼び出しに見立てた固定遅延を入れてキャッシュ効果を可視化する。
func (h *LabCacheHandler) loadSchoolsFromDB(ctx context.Context) ([]labSchool, error) {
	select {
	case <-time.After(h.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	var rows []models.School
	if err := h.db.WithContext(ctx).
		Select("id", "name", "monthly_fee").
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query schools: %w", err)
	}
	items := make([]labSchool, 0, len(rows))
	for _, r := range rows {
		items = append(items, labSchool{
			ID:         r.ID,
			Name:       r.Name,
			MonthlyFee: r.MonthlyFee,
		})
	}
	return items, nil
}
