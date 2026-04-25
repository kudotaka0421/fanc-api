package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/sony/gobreaker/v2"
)

// LabBreakerHandler は sony/gobreaker で Circuit Breaker の 3 状態
// (Closed / Open / Half-Open) を体感するための lab ハンドラ。
// 同プロセス内にある mock エンドポイントを HTTP 越しに叩き、失敗モードを
// トグルで切り替えて閾値超え → Open → タイムアウト経過 → Half-Open → Closed
// の遷移を UI から観察する。
type LabBreakerHandler struct {
	cb          *gobreaker.CircuitBreaker[string]
	httpClient  *http.Client
	mockURL     string
	failMode    atomic.Bool
	mockLatency time.Duration

	mu          sync.Mutex
	transitions []stateTransition
}

type stateTransition struct {
	At   time.Time             `json:"at"`
	From gobreaker.State       `json:"from"`
	To   gobreaker.State       `json:"to"`
}

// NewLabBreakerHandler は gobreaker を 3 failures / 5s Open / Half-Open 1 req の設定で初期化する。
// 本番相当の厳しい値ではなく、UI 連打で遷移を全部観察できる「軽い値」に寄せている。
func NewLabBreakerHandler() (*LabBreakerHandler, error) {
	mockURL := getenv("LAB_BREAKER_MOCK_URL", "http://localhost:8080/api/lab/breaker/mock")
	h := &LabBreakerHandler{
		httpClient:  &http.Client{Timeout: 3 * time.Second},
		mockURL:     mockURL,
		mockLatency: parseDurationEnv("LAB_BREAKER_MOCK_LATENCY", 50*time.Millisecond),
	}

	settings := gobreaker.Settings{
		Name:        "lab-upstream",
		MaxRequests: 1,
		Interval:    0, // 常時カウント（Closed 中に定期リセットしない）
		Timeout:     5 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.transitions = append(h.transitions, stateTransition{
				At: time.Now(), From: from, To: to,
			})
			if len(h.transitions) > 50 {
				h.transitions = h.transitions[len(h.transitions)-50:]
			}
		},
	}
	h.cb = gobreaker.NewCircuitBreaker[string](settings)
	return h, nil
}

type breakerCallResp struct {
	Ok         bool                `json:"ok"`
	Source     string              `json:"source"`
	HTTPStatus int                 `json:"httpStatus,omitempty"`
	Body       string              `json:"body,omitempty"`
	Error      string              `json:"error,omitempty"`
	State      string              `json:"state"`
	Counts     gobreaker.Counts    `json:"counts"`
	ElapsedMs  int64               `json:"elapsedMs"`
}

type breakerStateResp struct {
	State       string            `json:"state"`
	Counts      gobreaker.Counts  `json:"counts"`
	FailMode    bool              `json:"failMode"`
	Transitions []transitionView  `json:"transitions"`
}

type transitionView struct {
	At   string `json:"at"`
	From string `json:"from"`
	To   string `json:"to"`
}

type breakerToggleResp struct {
	FailMode bool `json:"failMode"`
}

// Call は gobreaker.Execute で mock エンドポイントを HTTP 越しに叩く。
// Open 中は実際のリクエストが発生せず即 `ErrOpenState` が返る。
// レスポンスに現在の state / counts を常に載せて UI の状態表示を更新する。
func (h *LabBreakerHandler) Call(c echo.Context) error {
	ctx := c.Request().Context()
	start := time.Now()
	body, err := h.cb.Execute(func() (string, error) {
		return h.callMock(ctx)
	})
	elapsed := time.Since(start).Milliseconds()
	state := h.cb.State().String()
	counts := h.cb.Counts()

	if err != nil {
		source := "upstream"
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			// ErrOpenState: Open 中の短絡。ErrTooManyRequests: Half-Open 中に probe が既に走っている。
			// どちらも「breaker が手前で止めた」ので、upstream に到達すらしていない。
			source = "breaker"
		}
		return c.JSON(http.StatusOK, breakerCallResp{
			Ok:        false,
			Source:    source,
			Error:     err.Error(),
			State:     state,
			Counts:    counts,
			ElapsedMs: elapsed,
		})
	}
	return c.JSON(http.StatusOK, breakerCallResp{
		Ok:         true,
		Source:     "upstream",
		HTTPStatus: http.StatusOK,
		Body:       body,
		State:      state,
		Counts:     counts,
		ElapsedMs:  elapsed,
	})
}

// State は UI のポーリング / 初期表示用に、現在の state と直近の遷移履歴を返す。
func (h *LabBreakerHandler) State(c echo.Context) error {
	h.mu.Lock()
	trs := make([]transitionView, 0, len(h.transitions))
	for _, t := range h.transitions {
		trs = append(trs, transitionView{
			At:   t.At.Format(time.RFC3339Nano),
			From: t.From.String(),
			To:   t.To.String(),
		})
	}
	h.mu.Unlock()
	return c.JSON(http.StatusOK, breakerStateResp{
		State:       h.cb.State().String(),
		Counts:      h.cb.Counts(),
		FailMode:    h.failMode.Load(),
		Transitions: trs,
	})
}

// ToggleFail は mock 側の失敗モードを反転する。ON の間は mock が 500 を返すので
// breaker 経由の呼び出しが失敗としてカウントされる。
func (h *LabBreakerHandler) ToggleFail(c echo.Context) error {
	next := !h.failMode.Load()
	h.failMode.Store(next)
	return c.JSON(http.StatusOK, breakerToggleResp{FailMode: next})
}

// Mock は breaker の保護対象となる upstream の代役。
// failMode=true の間は 500 を返す。実際の外部 API を叩きに行くと lab の再現性が落ちるため、
// 同プロセス内に置いて HTTP 越しに叩く形にした。
func (h *LabBreakerHandler) Mock(c echo.Context) error {
	time.Sleep(h.mockLatency)
	if h.failMode.Load() {
		return echo.NewHTTPError(http.StatusInternalServerError, "mock upstream in fail mode")
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "at": time.Now().Format(time.RFC3339)})
}

func (h *LabBreakerHandler) callMock(ctx context.Context) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, h.mockURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 500 {
		return "", fmt.Errorf("upstream %d: %s", resp.StatusCode, string(body))
	}
	return string(body), nil
}

func parseDurationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	if ms, err := strconv.Atoi(v); err == nil {
		return time.Duration(ms) * time.Millisecond
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	return fallback
}
