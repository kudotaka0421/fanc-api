package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// LabPprofHandler は net/http/pprof で 3 種類の profile を観察するための lab ハンドラ。
//
//   - Heavy            … CPU profile の題材（SHA-256 ループ）
//   - HeapLeakStart/Reset … heap profile の題材（global slice に巨大バッファを溜める）
//   - GoroutineLeak    … goroutine profile の題材（永久ブロックする goroutine を起動）
//   - Runtime          … NumGoroutine / heap stats を JSON で返す（before/after の比較用）
//
// pprof 自体のハンドラ登録は main.go 側で `_ "net/http/pprof"` を import + :6060 で
// ListenAndServe する構造で、このハンドラは「観察対象を作る発火源」だけを担当する。
type LabPprofHandler struct {
	mu        sync.Mutex
	heapLeak  [][]byte // GC されないように intentionally 保持する
	leakStops []chan struct{}
}

// NewLabPprofHandler は state を持たないので失敗しない。
func NewLabPprofHandler() (*LabPprofHandler, error) {
	return &LabPprofHandler{}, nil
}

type pprofHeavyResp struct {
	Iterations int    `json:"iterations"`
	ElapsedMs  int64  `json:"elapsedMs"`
	LastDigest string `json:"lastDigest"`
}

// Heavy は ?n=N で N 回 (デフォルト 1, 上限 200) CPU bound な処理を実行する。
// 1 iteration = 15,000 回の SHA-256 ハッシュ ≒ 数十 ms 想定。
// n=100 で約 3 秒の CPU 仕事になり、`pprof?seconds=10` 窓で十分なサンプル数が取れる。
func (h *LabPprofHandler) Heavy(c echo.Context) error {
	n, _ := strconv.Atoi(c.QueryParam("n"))
	if n <= 0 {
		n = 1
	}
	if n > 200 {
		n = 200
	}

	const innerHashes = 15_000
	start := time.Now()
	var d [32]byte
	for i := 0; i < n; i++ {
		buf := make([]byte, 1024+32)
		for j := 0; j < 1024; j++ {
			buf[j] = byte(i + j)
		}
		for k := 0; k < innerHashes; k++ {
			copy(buf[1024:], d[:])
			d = sha256.Sum256(buf)
		}
	}

	return c.JSON(http.StatusOK, pprofHeavyResp{
		Iterations: n,
		ElapsedMs:  time.Since(start).Milliseconds(),
		LastDigest: hex.EncodeToString(d[:]),
	})
}

type heapLeakResp struct {
	AddedMB     int `json:"addedMB"`
	TotalChunks int `json:"totalChunks"`
	TotalMB     int `json:"totalMB"`
}

// HeapLeakStart は ?mb=N (デフォルト 10, 上限 200) で N MB のバッファを heap に保持する。
// 結果は handler の slice に append され、handler 自体が長命なので GC されない。
// 連打すると線形に増え、pprof heap profile の inuse_space に出る。
func (h *LabPprofHandler) HeapLeakStart(c echo.Context) error {
	mb, _ := strconv.Atoi(c.QueryParam("mb"))
	if mb <= 0 {
		mb = 10
	}
	if mb > 200 {
		mb = 200
	}

	buf := make([]byte, mb*1024*1024)
	// touch しておかないと OS の lazy commit で RSS に乗らない可能性がある
	for i := 0; i < len(buf); i += 4096 {
		buf[i] = 1
	}

	h.mu.Lock()
	h.heapLeak = append(h.heapLeak, buf)
	total := 0
	for _, b := range h.heapLeak {
		total += len(b)
	}
	chunks := len(h.heapLeak)
	h.mu.Unlock()

	return c.JSON(http.StatusOK, heapLeakResp{
		AddedMB:     mb,
		TotalChunks: chunks,
		TotalMB:     total / (1024 * 1024),
	})
}

// HeapLeakReset は溜め込んだ slice を解放する。明示的に GC を回し、heap profile が即座に下がるのを観察する。
func (h *LabPprofHandler) HeapLeakReset(c echo.Context) error {
	h.mu.Lock()
	h.heapLeak = nil
	h.mu.Unlock()

	runtime.GC()

	return c.JSON(http.StatusOK, map[string]string{"status": "reset"})
}

type goroutineLeakResp struct {
	Started   int `json:"started"`
	NumActive int `json:"numActive"`
}

// GoroutineLeak は ?n=N (デフォルト 100, 上限 5000) で N 個の goroutine を起動する。
// 各 goroutine は stop チャネルからの受信で永久にブロックし、StopGoroutines が呼ばれるまで終了しない。
// /debug/pprof/goroutine?debug=1 で関数ごとの goroutine 数が増えるのを観察するための題材。
func (h *LabPprofHandler) GoroutineLeak(c echo.Context) error {
	n, _ := strconv.Atoi(c.QueryParam("n"))
	if n <= 0 {
		n = 100
	}
	if n > 5000 {
		n = 5000
	}

	stops := make([]chan struct{}, n)
	for i := 0; i < n; i++ {
		stop := make(chan struct{})
		stops[i] = stop
		go leakedWorker(stop)
	}

	h.mu.Lock()
	h.leakStops = append(h.leakStops, stops...)
	h.mu.Unlock()

	return c.JSON(http.StatusOK, goroutineLeakResp{
		Started:   n,
		NumActive: runtime.NumGoroutine(),
	})
}

// StopGoroutines は LeakWorker を全て stop させる。close(stop) で <-stop が即 unblock し、関数を抜ける。
// 即時には NumGoroutine に反映されないことがある（runtime のスケジューリング都合）ので数秒待ってから再観察する。
func (h *LabPprofHandler) StopGoroutines(c echo.Context) error {
	h.mu.Lock()
	stopped := len(h.leakStops)
	for _, s := range h.leakStops {
		close(s)
	}
	h.leakStops = nil
	h.mu.Unlock()

	return c.JSON(http.StatusOK, map[string]int{"stopped": stopped})
}

// leakedWorker は handler パッケージ直下の named function。
// pprof goroutine profile に「fanc-api/src/handlers.leakedWorker」として独立した山で出るので
// 「どの関数の goroutine が増えたか」が一目で分かる（クロージャだと無名関数の山に埋もれる）。
func leakedWorker(stop <-chan struct{}) {
	<-stop
}

type runtimeResp struct {
	NumGoroutine int    `json:"numGoroutine"`
	HeapAllocMB  uint64 `json:"heapAllocMB"`
	HeapSysMB    uint64 `json:"heapSysMB"`
	NumGC        uint32 `json:"numGC"`
}

// Runtime は NumGoroutine と heap stats を返す。リーク前/後の比較用。
func (h *LabPprofHandler) Runtime(c echo.Context) error {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return c.JSON(http.StatusOK, runtimeResp{
		NumGoroutine: runtime.NumGoroutine(),
		HeapAllocMB:  ms.HeapAlloc / (1024 * 1024),
		HeapSysMB:    ms.HeapSys / (1024 * 1024),
		NumGC:        ms.NumGC,
	})
}
