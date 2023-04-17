# Dockerfile
FROM golang:1.17

# ワーキングディレクトリを設定
WORKDIR /app

# 依存関係のファイルをコピー
COPY go.mod .
COPY go.sum .

# 依存関係のインストール
RUN go mod download

# ソースコードをコピー
COPY . .

# アプリケーションをビルド
RUN go build -o main .

# ポートをエクスポート
EXPOSE 8080

# アプリケーションを実行
CMD ["./main"]
