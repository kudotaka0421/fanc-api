# Dockerfile
FROM --platform=linux/amd64 golang:1.17

# ワーキングディレクトリを設定
WORKDIR /app

# MySQLクライアントのインストール
RUN apt-get update && apt-get install -y default-mysql-client && rm -rf /var/lib/apt/lists/*

# 依存関係のファイルをコピー
COPY go.mod .
COPY go.sum .

# 依存関係のインストール
RUN go mod download

# gooseをインストール
RUN go get -u github.com/pressly/goose/cmd/goose

# ソースコードをコピー
COPY . .

# アプリケーションをビルド
RUN GOOS=linux GOARCH=amd64 go build -o main .

# ポートをエクスポート
EXPOSE 8080

# アプリケーションが環境変数を受け取れるようにする
CMD ["./main", "-e", "${APP_ENV}"]
