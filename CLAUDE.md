# fanc-api

PitaScho（オンラインカウンセリング相談予約サービス）のカウンセリング管理サービス「fanc」のバックエンド API。

## 技術スタック

- **言語**: Go 1.20
- **Web フレームワーク**: Echo v4.10
- **ORM**: GORM v1.25
- **DB**: MySQL 8.0（ドライバ: go-sql-driver/mysql）
- **認証**: JWT（golang-jwt/jwt v3, labstack/echo-jwt/v4）
- **バリデーション**: go-playground/validator v10
- **メール送信**: SendGrid
- **マイグレーション**: goose（SQL ファイル形式）
- **暗号化**: golang.org/x/crypto

## ディレクトリ構造

レイヤード構造。

```
src/
├── handlers/     HTTP ハンドラー層（auth, user, tag, school, counseling, health_check）
├── models/       DB モデル層（user, tag, schools, school_tag, counseling）
└── routes/       ルーティング設定（routes.go）

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

main.go では CORS ミドルウェアを適用し、MySQL 接続を最大 10 回・5 秒間隔でリトライ。

## 環境変数

- `MYSQL_ROOT_PASSWORD`, `MYSQL_DATABASE`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_HOST`
- `JWT_SECRET_KEY`
- `SENDGRID_API_KEY`
- `CORS_ALLOW_ORIGIN`
- `SLACK_WEBHOOK_COUNSELING_COMPLETION`

## CI/CD

`.github/workflows/api-ci.yml` — Go build → ECR push → ECS タスク定義更新・サービス更新（main: prod、develop: stg）→ Slack 通知。現在 AWS 停止によりコメントアウト中。

## 関連リポジトリ

- [fanc-front](https://github.com/kudotaka0421/fanc-front) — フロントエンド
- [fanc-terraform](https://github.com/kudotaka0421/fanc-terraform) — AWS インフラ
