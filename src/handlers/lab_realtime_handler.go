package handlers

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
)

// LabRealtimeHandler は SSE (Server-Sent Events) で 1 対多のサーバ push を体感するための lab ハンドラ。
// publisher → broker → 各 subscriber の channel に fan-out し、subscriber goroutine が
// HTTP response writer に書き込んで Flush する。
//
// SSE を選んだ理由: 一方向 push かつ HTTP/1.1 でそのまま動く（プロキシ・ALB 互換）。
// 双方向必要なら WebSocket、フル通信が必要なら gRPC stream を選ぶ層分け。
type LabRealtimeHandler struct {
	mu          sync.RWMutex
	subscribers map[int64]chan realtimeEvent

	nextID    atomic.Int64
	totalSent atomic.Int64
}

type realtimeEvent struct {
	ID        int64     `json:"id"`
	Message   string    `json:"message"`
	PublishedAt time.Time `json:"publishedAt"`
}

// NewLabRealtimeHandler は subscribers map のみ初期化する。env も外部接続も無いので失敗しない。
func NewLabRealtimeHandler() (*LabRealtimeHandler, error) {
	return &LabRealtimeHandler{
		subscribers: make(map[int64]chan realtimeEvent),
	}, nil
}

// Stream は SSE エンドポイント。クライアントは EventSource("/api/lab/realtime/stream") で接続する。
//
// 流れ:
//  1. このリクエスト専用の channel を作って subscribers に登録
//  2. 切断時 (ctx.Done) にループを抜けて subscribers から外す
//  3. ループ内で channel から event を受け取って `data: <json>\n\n` を書き込み Flush
//  4. 15s 毎にハートビート (`:hb\n\n`) を送ってアイドル切断を防ぐ
//
// バッファサイズ 16 にしているのは「遅い consumer が publisher をブロックしない」ため。
// 溢れたら drop する（lab なので panic より無難な挙動を選ぶ）。
func (h *LabRealtimeHandler) Stream(c echo.Context) error {
	w := c.Response()
	w.Header().Set(echo.HeaderContentType, "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// nginx 経由の場合 proxy buffering を切らないと flush が即時届かない
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	id := h.nextID.Add(1)
	ch := make(chan realtimeEvent, 16)
	h.mu.Lock()
	h.subscribers[id] = ch
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.subscribers, id)
		close(ch)
		h.mu.Unlock()
	}()

	// 接続直後に「あなたの subscriber id」を hello イベントで返す（UI のデバッグ用）
	if _, err := fmt.Fprintf(w, "event: hello\ndata: {\"subscriberId\":%d}\n\n", id); err != nil {
		return nil
	}
	w.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	ctx := c.Request().Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeat.C:
			// SSE のコメント行 (`:` 始まり) はクライアントが無視する。
			// プロキシ / LB 側の idle timeout を踏まないための keep-alive。
			if _, err := fmt.Fprint(w, ":hb\n\n"); err != nil {
				return nil
			}
			w.Flush()
		case ev := <-ch:
			if _, err := fmt.Fprintf(w, "id: %d\ndata: {\"id\":%d,\"message\":%q,\"publishedAt\":%q}\n\n",
				ev.ID, ev.ID, ev.Message, ev.PublishedAt.Format(time.RFC3339Nano)); err != nil {
				return nil
			}
			w.Flush()
		}
	}
}

type realtimePublishReq struct {
	Message string `json:"message"`
}

type realtimePublishResp struct {
	ID          int64  `json:"id"`
	Subscribers int    `json:"subscribers"`
	Dropped     int    `json:"dropped"`
}

// Publish は 1 メッセージを全 subscriber に fan-out する。
// 遅い consumer の channel が満杯なら「その subscriber 宛だけ」drop してカウントを返す。
// 全員に届いた数 = subscribers - dropped、で UI 側に表示する。
func (h *LabRealtimeHandler) Publish(c echo.Context) error {
	var req realtimePublishReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Message == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "message is required")
	}
	ev := realtimeEvent{
		ID:          h.totalSent.Add(1),
		Message:     req.Message,
		PublishedAt: time.Now(),
	}

	h.mu.RLock()
	chans := make([]chan realtimeEvent, 0, len(h.subscribers))
	for _, ch := range h.subscribers {
		chans = append(chans, ch)
	}
	h.mu.RUnlock()

	dropped := 0
	for _, ch := range chans {
		select {
		case ch <- ev:
		default:
			// 遅い consumer は drop。publisher を絶対にブロックしない方針。
			dropped++
		}
	}
	return c.JSON(http.StatusOK, realtimePublishResp{
		ID:          ev.ID,
		Subscribers: len(chans),
		Dropped:     dropped,
	})
}

type realtimeStatsResp struct {
	Subscribers int   `json:"subscribers"`
	TotalSent   int64 `json:"totalSent"`
}

// Stats は UI の「接続中 N 人 / 累計送信 M 件」表示用の軽量エンドポイント。
func (h *LabRealtimeHandler) Stats(c echo.Context) error {
	h.mu.RLock()
	n := len(h.subscribers)
	h.mu.RUnlock()
	return c.JSON(http.StatusOK, realtimeStatsResp{
		Subscribers: n,
		TotalSent:   h.totalSent.Load(),
	})
}
