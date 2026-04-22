# fanc-api

PitaScho（オンラインカウンセリング相談予約サービス）のカウンセリング管理サービス「fanc」のバックエンド API。

## 技術スタック

- **言語**: Go 1.24
- **Web フレームワーク**: Echo v4.10
- **ORM**: GORM v1.25
- **DB**: MySQL 8.0（ドライバ: go-sql-driver/mysql）
- **認証**: JWT（golang-jwt/jwt v3, labstack/echo-jwt/v4）
- **バリデーション**: go-playground/validator v10
- **メール送信**: SendGrid
- **マイグレーション**: goose（SQL ファイル形式）
- **暗号化**: golang.org/x/crypto
- **AWS SDK**: aws-sdk-go-v2（lab 用。S3 presigned multipart など）

## ディレクトリ構造

レイヤード構造。

```
src/
├── handlers/     HTTP ハンドラー層（auth, user, tag, school, counseling, health_check, lab_s3, lab_sqs）
├── models/       DB モデル層（user, tag, schools, school_tag, counseling）
└── routes/       ルーティング設定（routes.go）

cmd/
└── worker/       lab 用 SQS worker バイナリ（別プロセス起動）

db/
└── migrations/   goose 用 SQL マイグレーション
```

テーブル: `schools`, `tags`, `school_tags`, `users`, `counselings`

## 主要コマンド

| 操作 | コマンド |
|------|---------|
| ビルド | `go build -o main .` |
| 起動 | `./main`（ポート 8080） |
| ローカル起動（最小: MySQL + backend） | `make up` |
| ローカル起動（lab 用 LocalStack / Postgres / Redis 込み） | `make up-lab` |
| 停止 | `make down` |
| ログ追従 | `make logs` |

※ CI にテストステップの定義はあるが、実装は空。

## lab トピック用の追加サービス

`docker-compose.lab.yml` が以下を追加する：

| サービス | 用途 | ホスト側ポート |
|---------|-----|----------|
| LocalStack | S3 / SQS / SNS / Lambda | 4566 |
| Postgres | RLS / パーティショニング / bulk | 5433 |
| Redis | Cache-Aside | 6379 |

初期化は `scripts/init-localstack.sh`（SQS キュー / SNS トピック / S3 バケット作成）と `scripts/init-postgres.sql` が自動実行する。

## API 概要

- 認証: `POST /api/login`, `POST /api/user`, `GET /api/confirm-account/:token`
- JWT 保護: `GET /api/me`
- リソース CRUD: `/api/tag`, `/api/school`, `/api/user`, `/api/counseling`
- ヘルスチェック: `GET /healthcheck`
- Lab (学習用、認証なし、LocalStack 前提):
  - `POST /api/lab/s3/presign-put` — PutObject 用 presigned URL を 15 分有効で発行
  - `GET  /api/lab/s3/objects` — アップロード済みファイル一覧
  - `POST /api/lab/sqs/publish` — lab-primary へメッセージ送信
  - `GET  /api/lab/sqs/stats` — primary / dlq の概算メッセージ数
  - `POST /api/lab/sns/publish` — lab-events topic に publish → lab-primary / lab-audit へ fanout
  - `GET  /api/lab/sns/stats` — primary / audit の概算メッセージ数
  - `GET  /api/lab/cache/schools` — Redis Cache-Aside（HIT/MISS/elapsedMs を返却、TTL 60s、DB 側に 300ms 擬似遅延）
  - `DELETE /api/lab/cache/schools` — キャッシュ key 削除

lab 用の SQS worker は `cmd/worker` 配下に別バイナリとしてあり、docker-compose.lab.yml の `worker` サービスで起動する。1 プロセス内で **2 goroutine が primary / audit を独立に long polling** する構造で、SNS → SQS fanout の両キュー同時消費を観察できる。"fail" を含むメッセージは削除せず redrive policy (maxReceiveCount=3) で DLQ へ送る挙動も観察可能。

  ナレッジ集:
  - ファイルアップロード方式 (A/B/C): `~/.claude/docs/interview/system-design/file-upload-patterns.md`
  - SQS 運用ノウハウ（VisibilityTimeout / DLQ / スケール）: `~/.claude/docs/interview/system-design/sqs-essentials.md`
  - SNS → SQS fanout: `~/.claude/docs/interview/system-design/sns-fanout-essentials.md`
  - S3 起点 Lambda: `~/.claude/docs/interview/system-design/s3-lambda-essentials.md`
  - Redis Cache-Aside: `~/.claude/docs/interview/system-design/fanc-lab/5-cache-aside.md`

main.go では CORS ミドルウェアを適用し、MySQL 接続を最大 10 回・5 秒間隔でリトライ。

## 環境変数

- `MYSQL_ROOT_PASSWORD`, `MYSQL_DATABASE`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_HOST`
- `JWT_SECRET_KEY`
- `SENDGRID_API_KEY`
- `CORS_ALLOW_ORIGIN`
- `SLACK_WEBHOOK_COUNSELING_COMPLETION`
- Lab 用（`make up-lab` で docker-compose.lab.yml が注入）:
  - `AWS_ENDPOINT_URL` — backend → LocalStack 内部通信用 (`http://localstack:4566`)
  - `AWS_PUBLIC_ENDPOINT_URL` — ブラウザ向け presigned URL 生成用 (`http://localhost:4566`)
  - `AWS_REGION`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`
  - `LAB_S3_BUCKET`（既定 `lab-uploads`）
  - `LAB_SQS_PRIMARY_URL`, `LAB_SQS_AUDIT_URL`, `LAB_SQS_DLQ_URL`
  - `LAB_SNS_TOPIC_ARN`（既定 `arn:aws:sns:ap-northeast-1:000000000000:lab-events`）
  - `POSTGRES_DSN`, `REDIS_ADDR`

## CI/CD

`.github/workflows/api-ci.yml` — Go build → ECR push → ECS タスク定義更新・サービス更新（main: prod、develop: stg）→ Slack 通知。現在 AWS 停止によりコメントアウト中。

## 関連リポジトリ

- [fanc-front](https://github.com/kudotaka0421/fanc-front) — フロントエンド
- [fanc-terraform](https://github.com/kudotaka0421/fanc-terraform) — AWS インフラ
