package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// LabPprofHandler は net/http/pprof で CPU プロファイルを観察するための lab ハンドラ。
// Heavy エンドポイントが SHA-256 を回す CPU bound な仕事を起動し、
// `go tool pprof http://localhost:6060/debug/pprof/profile?seconds=10` でホットスポット
// (sha256.block) が flame graph に出ることを確認するための題材。
//
// pprof 自体のハンドラ登録は main.go 側で `_ "net/http/pprof"` を import + :6060 で
// ListenAndServe する構造になっており、このハンドラは「重い処理の発火源」だけを担当する。
type LabPprofHandler struct{}

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
//
// 内部 buf を 1 outer iteration に 1 回だけ alloc して、inner loop は copy + Sum256 のみ。
// alloc を抑えて GC pressure を下げ、CPU profile が SHA-256 のホットスポットを純粋に映すようにしている。
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
