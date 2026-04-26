# Lab #11 pprof — 使い方チートシート

## 前提

```bash
make up-lab
# backend  → http://localhost:8090
# pprof    → http://localhost:6060
# フロント → http://localhost:5173/lab/pprof
```

## 王道の調査手順（3 ステップ）

```
① 負荷を起こす  → ② プロファイル取得  → ③ top → list で関数特定
```

ボタン操作は **フロントの UI から**、計測は **ターミナルで `go tool pprof`** が定番。

---

## シナリオ A: CPU を食ってる関数を特定したい

### コマンド → 結果

| # | コマンド | 表示される内容 |
|---|---|---|
| 1 | UI で「100 回」ボタンを押す | 約 3 秒の CPU 負荷が走る |
| 2 | 1 を押した直後に別ターミナルで実行 | 5 秒間サンプリングして top 表示 |

```bash
# 5 秒間の CPU profile を取って top 表示
go tool pprof -top -cum "http://localhost:6060/debug/pprof/profile?seconds=5"
```

### 出力例（抜粋）

```
      flat  flat%   sum%        cum   cum%
         0     0%     0%      9.15s 99.24%  handlers.(*LabPprofHandler).Heavy
     9.04s 98.05% 98.48%      9.07s 98.37%  sha256.blockGeneric    ← 真のボトルネック
```

### 読み方

- `flat` … その関数 **自身** が消費した CPU 時間。**ここが大きい関数が犯人**
- `cum` … その関数 + 配下の合計。**呼び出し元の追跡**に使う
- 上の例：`Heavy` は flat=0 だが配下の `sha256.blockGeneric` が flat 98% → SHA-256 計算がボトルネック

### 行単位で見る

```bash
go tool pprof -list 'Heavy' "http://localhost:6060/debug/pprof/profile?seconds=5"
```

→ `Heavy` 関数のソースコードに各行の `flat / cum` 時間がマッピングされて表示される。

### Flame Graph で見るなら

```bash
go tool pprof -http=:9090 "http://localhost:6060/debug/pprof/profile?seconds=5"
```

→ 自動でブラウザが `http://localhost:9090` を開く。`VIEW → Flame Graph` で炎グラフ表示。

---

## シナリオ B: メモリを食ってる関数を特定したい

### コマンド → 結果

| # | 操作 | 何が起きるか |
|---|---|---|
| 1 | UI 「+100MB」を 2 回押す | handler の global slice に 200MB 滞留 |
| 2 | UI 上部「Refresh」 | `HeapAlloc` が +200MB されているのが見える |
| 3 | ターミナルで heap profile 取得 | 関数別の inuse_space ランキング |

```bash
go tool pprof -top -cum http://localhost:6060/debug/pprof/heap
```

### 出力例

```
      flat  flat%   sum%        cum   cum%
   10240kB 58.79% 58.79%    10240kB 58.79%  handlers.(*LabPprofHandler).HeapLeakStart  ← 犯人
```

### 行単位

```bash
go tool pprof -list 'HeapLeakStart' http://localhost:6060/debug/pprof/heap
```

### 補足: `inuse_space` vs `alloc_space`

- `inuse_space` (デフォルト) … **今この瞬間に保持している** メモリ。**リーク調査向け**
- `alloc_space` … 起動後の累計 alloc 量。**頻繁に alloc/free する関数を探す**用途

```bash
# 累計 alloc を見たい場合
go tool pprof -sample_index=alloc_space http://localhost:6060/debug/pprof/heap
```

---

## シナリオ C: goroutine リーク調査

### コマンド → 結果

| # | 操作 | 何が起きるか |
|---|---|---|
| 1 | UI 「+1000 goroutine」 | 1000 個の goroutine が永久ブロック状態で残る |
| 2 | UI 上部「Refresh」 | `NumGoroutine` が +1000 |
| 3 | ブラウザで下記 URL | 関数別の goroutine 数集計（テキスト） |

```
http://localhost:6060/debug/pprof/goroutine?debug=1
```

### 出力例

```
goroutine profile: total 1010
1000 @ 0x481d6e 0x416bf3 0x416752 0x1153465 0x489c01
#  fanc-api/src/handlers.leakedWorker  ← 1000 個の goroutine がここで止まっている

3 @ ...
#  pgxpool.(*Pool).backgroundHealthCheck   ← 通常の常駐 goroutine
```

### 読み方

- 先頭の数字 = **同じスタックトレースを持つ goroutine の数**
- 行頭 `#` = スタックトレース（リークしている関数名がここで分かる）
- **数十〜数百で同じ関数が並んだら 100% リーク**

### Flame Graph で見るなら

```bash
go tool pprof -http=:9090 http://localhost:6060/debug/pprof/goroutine
```

---

## 早見表（コマンド → 何が出るか）

| 目的 | コマンド | 出力形式 |
|---|---|---|
| CPU 上位関数を一覧 | `go tool pprof -top -cum "http://localhost:6060/debug/pprof/profile?seconds=5"` | テキスト table |
| Heap 上位関数を一覧 | `go tool pprof -top -cum http://localhost:6060/debug/pprof/heap` | テキスト table |
| 関数の行別コスト | `go tool pprof -list '関数名' <profile URL>` | 行番号付きソース |
| Flame Graph で視覚化 | `go tool pprof -http=:9090 <profile URL>` | ブラウザで Web UI |
| goroutine 一覧（簡易） | ブラウザで `http://localhost:6060/debug/pprof/goroutine?debug=1` | テキスト |
| 概要メニュー | ブラウザで `http://localhost:6060/debug/pprof/` | リンク集 |

## 30 秒回答テンプレ（面接用）

> 「`pprof?seconds=N` で profile を取って `go tool pprof` の `top -cum` で上位関数を見る。`flat` が大きいのが真のボトルネック、`cum` は呼び出し階層の上流。怪しい関数があれば `list 関数名` で行レベルまで降りる。視覚化したいときは `-http` で Flame Graph に切り替える。goroutine リークは `goroutine?debug=1` をブラウザで開いて、同じ関数で大量に止まっている stack を探す」
