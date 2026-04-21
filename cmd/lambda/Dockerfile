# lab 用 Lambda のビルド + LocalStack へのデプロイ用イメージ。
# 1. ビルドステージで Go Lambda バイナリ (provided.al2 runtime 用に bootstrap と命名) を作る
# 2. デプロイステージは awslocal を持ち、bootstrap を zip して LocalStack に上げ、
#    lab-lambda バケットに S3 イベント通知を設定する。
# docker-compose.lab.yml の lambda-deployer サービスが利用する。

FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/lambda cmd/lambda
# provided.al2 の entrypoint は /var/runtime/bootstrap。
# zip のルート直下に "bootstrap" を置く必要がある。
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /out/bootstrap ./cmd/lambda

FROM python:3.11-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends zip ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && pip install --no-cache-dir awscli awscli-local
COPY --from=build /out/bootstrap /workspace/bootstrap
COPY scripts/deploy-lambda.sh /workspace/deploy-lambda.sh
RUN chmod +x /workspace/deploy-lambda.sh
WORKDIR /workspace
CMD ["/workspace/deploy-lambda.sh"]
