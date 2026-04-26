# Lab #11 pprof — 使い方チートシート

## 前提

```bash
make up-lab
# backend  → http://localhost:8090
# pprof    → http://localhost:6060
# フロント → http://localhost:5173/lab/pprof
```

## pprof とは（1 行で）

> Go runtime が裏で持っている計測データ（heap alloc 場所 / goroutine stack / CPU sample 等）を HTTP/CLI 経由で取り出して、**関数別の負荷分布** を見るツール。

### Profile 種別

| profile | 取り方 | 何が分かる |
|---|---|---|
| **CPU** (`/profile?seconds=N`) | **N 秒間サンプリング**（オンデマンド） | どの関数が CPU を食ってるか |
| **Heap inuse** (`/heap`) | **その瞬間のスナップショット** | 今 inuse な heap がどの関数由来か |
| **Heap alloc** (`/heap` + `alloc_space`) | スナップショット | 起動以来の累計 alloc がどの関数由来か |
| **Goroutine** (`/goroutine`) | スナップショット | goroutine がどの関数で止まっているか |
| **Block** (`/block`) | スナップショット | チャネル/ロックでブロックしてる時間が長い関数 |
| **Mutex** (`/mutex`) | スナップショット | mutex 競合してる関数 |

→ **CPU だけ「期間サンプリング」、他は「瞬間スナップショット」** が重要。

---

## 王道の調査手順

```
[ メトリクス側 ]
   Datadog / Grafana / コンテナのメモリ使用量グラフを見る
        ↓
   「絶対量が異常」「右肩上がり」を察知
        ↓
[ pprof 側 ]
   ① 負荷を起こす（または既に起きている）
        ↓
   ② プロファイル取得
        ↓
   ③ top -cum で関数ランキング → flat 大きい関数を特定
        ↓
   ④ list 関数名 で行レベルに降りる
```

**役割分担が重要**：

| 道具 | 役割 |
|---|---|
| **メトリクス**（Datadog 等） | 異常の **判定**（絶対量・長期トレンド） |
| **pprof** | 異常の **原因特定**（関数別の比率・行レベル） |

→ pprof は判定の道具ではない。**「異常があると分かったあとに、どこの関数が原因か絞る」道具**。

---

## シナリオ A: CPU を食ってる関数を特定

### コマンド → 結果

| # | コマンド | 何が起きるか |
|---|---|---|
| 1 | UI で「100 回」ボタンを押す | 約 3 秒の CPU 負荷が走る |
| 2 | 直後に別ターミナルで実行 | 5 秒間サンプリングして top 表示 |

```bash
go tool pprof -top -cum "http://localhost:6060/debug/pprof/profile?seconds=5"
```

### 出力例

```
      flat  flat%   sum%        cum   cum%
         0     0%     0%      9.15s 99.24%  handlers.(*LabPprofHandler).Heavy
     9.04s 98.05% 98.48%      9.07s 98.37%  sha256.blockGeneric    ← 真のボトルネック
```

### 行単位

```bash
go tool pprof -list 'Heavy' "http://localhost:6060/debug/pprof/profile?seconds=5"
```

### Flame Graph

```bash
go tool pprof -http=:9090 "http://localhost:6060/debug/pprof/profile?seconds=5"
```

→ ブラウザが `http://localhost:9090` を開き、Flame Graph で視覚的に確認できる。

### CPU profile はタイミング命

- `?seconds=N` は **その N 秒間だけ** 動いた関数を集計
- アイドル時に取ると **些細な処理が % 高く出る**
- → 「**機能を動かしている最中**」に取るのが鉄則

---

## シナリオ B: メモリを食ってる関数を特定

### コマンド → 結果

| # | 操作 | 何が起きるか |
|---|---|---|
| 1 | UI 「+100MB」を 2 回押す | handler の global slice に 200MB 滞留 |
| 2 | UI 上部「Refresh」 | `HeapAlloc` が +200MB |
| 3 | ターミナルで heap profile 取得 | 関数別 inuse_space ランキング |

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

### inuse_space vs alloc_space の使い分け

| sample 種別 | 意味 | 用途 | 一時バッファは映る？ |
|---|---|---|---|
| **inuse_space** (デフォルト) | **今この瞬間に保持中** の heap | リーク調査・常時保持の確認 | ❌ 処理後に free 済みなら映らない |
| **alloc_space** | 起動以来の **累計 alloc** | GC pressure 調査・頻繁な alloc/free 検知 | ✅ free 済みも累計に計上 |

```bash
# 累計 alloc を見る
go tool pprof -sample_index=alloc_space http://localhost:6060/debug/pprof/heap
```

#### 重要な注意：「重い処理の一時バッファ」は inuse_space に映らない

```
時刻       heap 状態
t0:        100MB（アイドル）
t1: 開始    100MB
t2: 処理中  500MB ← 一時バッファで膨張
t3: 終了    100MB ← GC で free
```

| いつスナップショットを取るか | 何が見えるか |
|---|---|
| t0 / t3 | 100MB しか見えない、一時バッファは映らない |
| t2 | 500MB 見える |

→ **一時的な大量 alloc を捕まえたいなら処理中に取る or `alloc_space` を見る**。

---

## シナリオ C: goroutine リーク調査

### コマンド → 結果

| # | 操作 | 何が起きるか |
|---|---|---|
| 1 | UI 「+1000 goroutine」 | 1000 個の goroutine が永久ブロック |
| 2 | UI 上部「Refresh」 | `NumGoroutine` が +1000 |
| 3 | ブラウザで下記 URL | 関数別 goroutine 集計 |

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
- `#` 行 = スタックトレース（リークしている関数名）
- **数百〜数千で同じ関数が並んだらリーク確定**

---

## top の読み方を深掘り

### 各カラムの意味

| カラム | 意味 |
|---|---|
| **flat** | その関数 **自身** が直接消費したリソース（自分の行で `make` した量等）。子関数分は含まない |
| **flat%** | `flat ÷ profile 合計 × 100` |
| **sum%** | 上から順に flat% を累積した値（「上位 N 個で何 % 占めるか」が見える） |
| **cum** | その関数 **+ そこから呼ばれた配下関数全部** の合計 |
| **cum%** | `cum ÷ profile 合計 × 100` |

### flat と cum の使い分け

```go
func A() {        // flat=0, cum=100MB ← 配下のせいで cum だけ大きい
    B()
}
func B() {        // flat=100MB, cum=100MB ← 自分で alloc してる
    make([]byte, 100MB)
}
```

- **flat 大** = **真犯人**（このコードを直せば直接効く）
- **cum 大 / flat 小** = **真犯人を呼んでる上流**（呼び出し元の追跡用）

→ 「**flat が突出した行を上から探す**」が犯人特定の鉄則。

### % の分母は「heap 全体」ではない

heap profile を取った時、出力ヘッダに必ずこれが出る：

```
Showing nodes accounting for 17417.18kB, 100% of 17417.18kB total
                                          ↑ これが分母
```

- 分母 = **profile に含まれる全関数の合計**（= profile の総量）
- **heap 全体の絶対量ではない**
- pprof の heap profile は **サンプリング**（512KB に 1 回）なので、実 heap が 200MB でも profile では数 MB しか見えない

→ **絶対値は信用しない、相対値（%）で見る**。実 heap 量を知りたいなら `runtime.ReadMemStats` の `HeapAlloc` を別途確認する。

### % 単独では判断できない

% は「**この profile の中で偏ってる場所**」であって「絶対的に重い」ではない：

| 状況 | 全体 | HeapLeakStart の flat | flat% | 問題か |
|---|---|---|---|---|
| ① アイドル時 | 2MB | 1MB | **50%** | 問題なし（そもそも 2MB） |
| ② 高負荷時 | 200MB | 10MB | **58%** | 問題（200MB の主犯） |

→ **flat の絶対値で犯人候補を見つけ、% で修正の優先度を決める**。両方使う。

### 犯人と言える目安

#### Heap profile

| 関数 flat | 全体に占める割合 | 判定 |
|---|---|---|
| MB 単位 | 10% 以上 | 怪しい、調査対象 |
| 数十 MB | 30% 以上 | **確実な犯人** |
| 数百 MB〜GB | - | OOM 直前、即対応 |

→ ただしリークは絶対値より **時系列で増え続けてるか** が本質。

#### CPU profile

| flat% | 判定 |
|---|---|
| 〜5% | 通常範囲、無視 |
| 5〜10% | 最適化候補 |
| 10% 以上 | 確実なホットスポット |
| 30% 以上 | 即対応レベル |

#### Goroutine profile

| 同一関数の数 | 判定 |
|---|---|
| 〜数十個 | 通常（worker pool 等） |
| 数百〜数千個 | **リーク確定** |
| 1 万超 | OOM 直前 |

---

## メモリリーク調査の実務フロー

### リークの定義

> **処理のたびに新しい alloc が追加されて、free されずに累積していく状態**

```go
// 典型例: SSE subscriber の delete 忘れ
var subscribers = map[string]chan Event{}

func handleSSE(c echo.Context) {
    id := uuid.New()
    ch := make(chan Event)
    subscribers[id] = ch       // ← 接続のたびに追加
    // 切断時に delete(subscribers, id) を忘れる
}
```

| 時刻 | リクエスト数 | subscribers | heap |
|---|---|---|---|
| t0 | 0 | 0 個 | 50MB |
| t1 | 1000 | 1000 個 | 100MB |
| t2 (1h後) | 5000 | 5000 個 | 250MB |
| t3 (2h後) | 10000 | 10000 個 | 500MB |

→ **「1 個保持して終わり」じゃなく「処理のたびに新インスタンスが積まれる」のがリーク**。だから時間と共に増える。

### リーク有無は メトリクスで判定する

pprof では **判定不可**。長期グラフで以下を見る：

#### ① 谷を見る、ピークじゃない

```
[ 正常 ] 1 日のグラフ
     ┌─┐ ┌─┐ ┌─┐
─────┘ └─┘ └─┘ └─    ← 谷が一定 = 健康

[ リーク ] 1 週間のグラフ
              ┌─
         ┌─┐ ┌┘
    ┌─┐ ┌┘ └─┘            ← 谷が右肩上がり = リーク
─┐ ┌┘ └─┘
 └─┘
```

#### ② 再起動パターンで判別

```
月: 100MB ──────┐
火:    └ 増加  └→ 200MB
水:               └ 増加 → 400MB
日:                          → 1GB（OOM 寸前）
月: デプロイ → 100MB（リセット） ← 同じカーブで増え始める
```

→ **デプロイのたびにリセットされて、また同じカーブで増える** ならリーク確定。

#### ③ Per-request memory で正規化

```
HeapAlloc ÷ 累計リクエスト数 = 1 リクエストあたりのメモリ
```

→ トラフィックの絶対量と無関係になる。**この値が時間とともに増えてればリーク**。

### リークがあると分かったら pprof で原因特定

#### diff (`-base`) で「増えた分」を浮き上がらせる

```bash
# 1 時間あけて 2 枚撮る
curl -o before.pb.gz http://localhost:6060/debug/pprof/heap
sleep 3600
curl -o after.pb.gz http://localhost:6060/debug/pprof/heap

# 差分を取る
go tool pprof -base before.pb.gz after.pb.gz
```

#### diff の効果

```
# 通常の top（before）
flat=200MB  cache.Get          ← 意図した cache（定常）
flat=100MB  HeapLeakStart      ← リーク
flat= 50MB  user.Find          ← 通常処理（定常）

# diff した top（after - before）
flat=+100MB  HeapLeakStart  ← 増えた分だけ出る、これが犯人
                            ← cache や user.Find は変動なしなので消える
```

→ **定常分が消えて、増えた分だけ浮き上がる**ので、リーク源が一目瞭然。

### 重要：「絶対量大 ≠ リーク」「定常 vs 累積」が判定基準

| 状態 | 問題か |
|---|---|
| `inuse_space` flat 大 + **定常** | 問題なし（意図した cache・マスタデータ等） |
| `inuse_space` flat 大 + **増え続ける** | **リーク確定** |
| `alloc_space` flat 大 | GC pressure の原因（最適化候補） |

→ 1 枚のスナップショット（flat 100MB）だけでは健康/病気は判定不可。

---

## ローカル vs 本番の使い分け

### ローカル pprof で十分な問題

- アルゴリズム / 計算量（O(n²) になってないか）
- 不要な alloc（ループ内で string 結合等）
- goroutine リーク（コードレベルの終了経路ミス）
- 明らかな N+1

→ **ロジック起因なら本番でもローカルでも同じ関数が top に出る**。

### 本番じゃないと分からない問題

- 実データ量依存（テーブル 1000 万行、JSON 巨大）
- 本番トラフィック特有の競合（mutex contention、同時接続数）
- インスタンス spec 依存（CPU コア数で並列度変わる）
- 時間経過で表面化（数日でやっと見える heap leak）

### 本番では Continuous Profiling を仕込む

「障害が起きた瞬間に手で `?seconds=N` を叩く」では遅すぎる。

- **Datadog Continuous Profiler** / **Grafana Pyroscope** を本番に仕込む
- 数分間隔で自動的に CPU/heap/goroutine profile を集める
- 1 ヶ月分くらい遡って **「先週の障害時の profile」** を見られる
- → 本番では「**事後に過去を遡れる状態**」を作る

| 環境 | 何のために pprof |
|---|---|
| **ローカル** | 仮説検証 / 修正前後の比較 / 再現できる問題 |
| **ステージング** | リリース前確認（本番に近い構成で） |
| **本番** | **Continuous Profiling で常時自動収集** → 障害時にダウンロードして調査 |

---

## OOM の挙動

### コンテナレベルの OOM (OOMKilled)

```
コンテナの memory limit (例: 1GB) を超える
        ↓
Linux カーネルの OOM Killer が発動
        ↓
プロセスを SIGKILL で強制終了 (grace period なし)
```

| 環境 | 起きること |
|---|---|
| Kubernetes Pod | `OOMKilled` で Pod 再起動 |
| ECS / Fargate | タスクが落ちて再スケジュール |
| EC2 | プロセスが kill |

### Go の場合：「画面エラーだけで生き残る」パターン

Echo / gin など Web フレームワークは `middleware.Recover()` をデフォルトで仕込む：

```go
e.Use(middleware.Recover())

// 内部で panic を catch して 500 を返す
defer func() {
    if r := recover(); r != nil {
        c.Error(echo.NewHTTPError(500, "internal error"))
    }
}()
```

| ケース | 起きること |
|---|---|
| ① 巨大 alloc 失敗 | `runtime: out of memory` で panic → recover が catch → **500 を返してプロセス継続** |
| ② OOMKilled | cgroup limit 超 → SIGKILL → 該当リクエストだけ失敗、他は継続 |
| ③ アプリ層チェック | 「対象が大きすぎる」を事前に弾いてエラー返す |

→ 「画面エラーだけで生き残る」なら **①の recover パターン** が一番ありそう。

### 一時ピーク vs リーク

| 種別 | 特徴 | 対処 |
|---|---|---|
| **一時ピーク** | 重い処理 1 回で大量 alloc → 終われば free | ストリーム処理（`io.Copy` 等）に変える |
| **リーク** | 処理のたびに累積 → 戻らない | 累積場所を `pprof -base` で特定 |

→ 「大量フォルダコピー時だけ落ちる、平常時は大丈夫」なら **一時ピーク**。コードを見て **逐次処理** に変える。

---

## 早見表（コマンド → 何が出るか）

| 目的 | コマンド | 出力形式 |
|---|---|---|
| CPU 上位関数を一覧 | `go tool pprof -top -cum "http://localhost:6060/debug/pprof/profile?seconds=5"` | テキスト table |
| Heap inuse 上位関数 | `go tool pprof -top -cum http://localhost:6060/debug/pprof/heap` | テキスト table |
| Heap alloc 累計上位 | `go tool pprof -sample_index=alloc_space -top -cum http://localhost:6060/debug/pprof/heap` | テキスト table |
| 関数の行別コスト | `go tool pprof -list '関数名' <URL>` | 行番号付きソース |
| Flame Graph で視覚化 | `go tool pprof -http=:9090 <URL>` | ブラウザで Web UI |
| Heap diff（リーク調査） | `go tool pprof -base before.pb.gz after.pb.gz` | 差分 top（増えた分だけ） |
| goroutine 一覧（簡易） | ブラウザで `http://localhost:6060/debug/pprof/goroutine?debug=1` | テキスト |
| 概要メニュー | ブラウザで `http://localhost:6060/debug/pprof/` | リンク集 |

---

## 30 秒回答テンプレ（面接用）

### 全体

> 「pprof は **runtime が裏で持ってる計測データを取り出して関数別の負荷分布を見るツール**。`top -cum` で関数ランキング、`flat` 大きいのが真犯人、`cum` 大きいのは呼び出し階層の上流。怪しい関数は `list 関数名` で行レベルまで降りる。視覚化は `-http` で Flame Graph」

### % と絶対値

> 「pprof の % は **profile 内の相対比率** で、絶対量じゃない。アイドル時に取れば些細な alloc でも % 高くなる。だから **絶対値（flat の MB 等）と組み合わせて判断**、修正の優先度判定に % を使う」

### リーク調査

> 「リーク有無は **メトリクス側（Datadog 等）で長期トレンドを見る**。特に **谷の値**（深夜のアイドル時の最小値）が右肩上がりかを見る。**再起動のたびにリセット → 同じカーブで増える** が見えれば確定。原因特定は **pprof の `-base` で時間差 diff** を取って『増えた分』に出てくる関数を特定」

### 本番運用

> 「手で `?seconds=N` 叩くのはローカル/ステージング向け。本番では **Datadog Continuous Profiler や Pyroscope で常時自動収集** しておいて、障害発生時に遡って調査するのが定番」

---

## 面接 Q&A（LayerX シニアプロダクトエンジニア向け）

システム設計・技術課題で **概念レベル** で聞かれそうな観点に絞った版。
コード詳細・カラム定義・細かい profile 種別の使い分けは出ない想定。
**「経験 + 設計判断 + 本番運用の理想形 + トレードオフ」** が問われる。

### Q1. pprof 使った経験ありますか？

> 「現職では **重い機能を実装する時の事前確認** で使いました。ファイルコピー機能でメモリ消費が懸念されたので、**ローカルで顧客と同じデータ量を再現** して heap profile を取り、**処理を見直すべきレベルか、一時ピークで許容できるか** を定量判断しました。
>
> 障害駆動ではなく **予防的に計測してから判断する** 姿勢を意識しています」

**評価ポイント**: 計測駆動の判断 / 判断軸の明示 / 予防的使用

### Q2. 本番には pprof 入れてましたか？

> 「セキュリティ観点で **素の `net/http/pprof` エンドポイントは本番に出さない** 判断でした。情報漏洩・DoS の踏み台リスクがあり、業界標準だと認識しています。
>
> 本番運用の理想形は **Datadog Continuous Profiler や Grafana Pyroscope のような Continuous Profiling SaaS** を入れて、認証・サンプリング・暗号化込みで安全に常時収集することです。現職では未導入で、ローカル再現で対応していました」

**評価ポイント**: 素の pprof と Continuous Profiling の区別 / セキュリティ意識 / 本番運用の理想形

### Q3. メモリリークが起きたらどう調査しますか？

> 「**メトリクスと pprof の役割分担** で動きます。
>
> **判定はメトリクス側**（Datadog / Grafana のコンテナメモリ長期グラフ）で、**ピークじゃなく谷の値**（深夜アイドル時の最小値）が右肩上がりかを見ます。**再起動のたびにリセット → 同じカーブで増える** パターンが見えればリーク確定。
>
> **原因特定は pprof 側**。時間差で 2 枚スナップショットを取って diff を取ると、**定常分が消えて増えた分だけ浮き上がる** ので、リーク源の関数が一発で出ます」

**評価ポイント**: 役割分担の言語化 / 谷を見る発想 / 時系列 diff の概念

### Q4. メトリクス・APM・pprof の使い分けは？

> 「3 層で役割分担します。
>
> - **メトリクス**（コンテナ CPU/メモリ、レスポンス時間）→ **異常の察知**
> - **APM**（Datadog APM 等）→ **重いリクエスト経路の特定**（DB 500ms、外部 API 300ms 等の経路別時間分解）
> - **pprof** → **重い関数の特定**（CPU の 30% を食ってる関数等）
>
> 障害調査の流れは **メトリクスで異常察知 → APM で経路特定 → pprof で関数特定** の 3 段階です」

**評価ポイント**: 3 層の使い分け / 経路 vs 関数の違い / 障害調査フロー

### Q5. 重い機能を実装する時にどうパフォーマンスを担保しますか？

> 「**実装前の見積もり → 実装後の計測 → 本番監視** の 3 段階です。
>
> 実装前にデータ量とアルゴリズムの計算量を見積もり、許容範囲か判断。実装後は **ローカルで顧客相当のデータ量で pprof** を取り、関数別の負荷分布を確認して **想定通りか・想定外の重さがないか** を検証。本番投入後は **メトリクスで継続観測**、可能なら **Continuous Profiling で事後遡及できる状態** を作ります」

**評価ポイント**: 段階的アプローチ / 見積もり + 計測 + 観測 / 本番監視まで設計

### Q6. ローカルで取った pprof は本番運用の判断材料に使えますか？

> 「**1 リクエスト処理あたりの絶対値はデータ量同じならほぼ一致** するので、コードレベルの問題（重い関数、計算量、無駄な alloc）はローカル調査で十分判断できます。
>
> ただし **並行リクエスト下のメモリピーク・GC 頻度・mutex 競合** は spec とトラフィック依存なので、ローカル単発調査では再現できません。本番運用として完全に把握したいなら **Continuous Profiling での事後遡及** が必要、というトレードオフです」

**評価ポイント**: 一致する部分・違う部分の切り分け / トレードオフの言語化

### Q7. Continuous Profiling とは？なぜ本番に入れる価値がありますか？

> 「**pprof を内部で使うが、認証・サンプリング・SaaS 送信込みで安全に常時動かせる仕組み** です。Datadog Continuous Profiler や Grafana Pyroscope が代表的。
>
> 数分間隔で profile を自動収集して SaaS に送信、**1 ヶ月分くらい遡って『先週の障害時の profile』を見られる** のが価値。本番障害は **起きた瞬間に手で取りに行っても遅い** ので、**事後遡及できる状態を作っておく** のが本質です。コストとオーバーヘッド数% を払う価値があるかは事業フェーズ次第」

**評価ポイント**: 素の pprof との違い / 事後遡及の概念 / コストとのトレードオフ意識

---

## 想定される深掘り質問

| 質問 | 回答方針 |
|---|---|
| 「pprof で何が分かる？」 | 「関数別の CPU 時間 / heap alloc / goroutine 数。**どの関数が負荷の主犯か** を可視化する」 |
| 「flat と cum って何？」 | 「flat = その関数自身、cum = 自身 + 配下の合計。**flat が突出してる関数が真犯人**」 |
| 「リークの定義は？」 | 「処理のたびに新しい alloc が追加されて free されずに **累積していく** 状態。1 個保持じゃなく時間と共に増える」 |
| 「なぜ素の pprof は本番に出せない？」 | 「認証なし → 機密情報漏洩リスク + DoS 踏み台リスク」 |
| 「OOM とリークの関係は？」 | 「リーク → 累積 → コンテナ memory limit 超 → OOMKilled。一時ピークで OOM するのはリークじゃなく実装の問題（ストリーム処理に変える等で対処）」 |

---

## 一番大事な 1 点

LayerX シニアプロダクトエンジニア面接で pprof について問われた時、**「使った経験」よりも「設計判断と運用の理想形を語れること」** が評価されます。

> 「**素の pprof と Continuous Profiling の区別ができていて、本番セキュリティ観点・コスト観点でトレードオフが言える**」

→ これが言えれば、pprof 自体を頻繁に使ってなくても合格ライン。

