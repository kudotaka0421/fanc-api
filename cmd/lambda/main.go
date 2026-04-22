// Package main は lab 用の S3 → Lambda トリガー処理バイナリ。
// LocalStack 上で provided.al2 runtime として実行される。
// lab-lambda バケットの uploads/ 配下に PutObject されるとこの Lambda が呼ばれ、
// 対象オブジェクトを GetObject → SHA-256・サイズ・ContentType を計算し、
// 同バケット results/ 配下に JSON として書き戻す。
//
// 無限ループ防止のため、S3 側のイベント通知は prefix=uploads/ でフィルタしているので
// results/ への PutObject では再発火しない（deploy-lambda.sh 参照）。
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// result は Lambda が uploads/ のオブジェクトを処理して results/*.json に書き戻す内容。
type result struct {
	Key         string    `json:"key"`
	Bucket      string    `json:"bucket"`
	Size        int64     `json:"size"`
	ContentType string    `json:"contentType"`
	SHA256      string    `json:"sha256"`
	ProcessedAt time.Time `json:"processedAt"`
}

// handler は S3 イベントの各 record を順に処理する。
func handler(ctx context.Context, evt events.S3Event) error {
	client, err := newS3Client(ctx)
	if err != nil {
		return err
	}

	for _, r := range evt.Records {
		bucket := r.S3.Bucket.Name
		key := r.S3.Object.Key
		log.Printf("event: s3://%s/%s", bucket, key)

		// 念のためのガード（イベント通知側で prefix=uploads/ を効かせているが、
		// 誤って results/ が入ってきたときに自己再帰で料金事故にならないよう二重防御）。
		if strings.HasPrefix(key, "results/") {
			log.Printf("skip self-generated result object: %s", key)
			continue
		}

		if err := processOne(ctx, client, bucket, key); err != nil {
			return fmt.Errorf("process %s: %w", key, err)
		}
	}
	return nil
}

func processOne(ctx context.Context, client *s3.Client, bucket, key string) error {
	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	sum := sha256.Sum256(data)
	body, err := json.MarshalIndent(result{
		Key:         key,
		Bucket:      bucket,
		Size:        int64(len(data)),
		ContentType: aws.ToString(out.ContentType),
		SHA256:      hex.EncodeToString(sum[:]),
		ProcessedAt: time.Now().UTC(),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	resultKey := "results/" + strings.TrimPrefix(key, "uploads/") + ".json"
	if _, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(resultKey),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("application/json"),
	}); err != nil {
		return fmt.Errorf("put result: %w", err)
	}
	log.Printf("wrote: s3://%s/%s (size=%d)", bucket, resultKey, len(data))
	return nil
}

// newS3Client は LocalStack 内の Lambda ランタイム上で S3 を呼べるクライアントを返す。
// LocalStack では Lambda コンテナに LOCALSTACK_HOSTNAME (または AWS_ENDPOINT_URL) が
// 注入される。本番 AWS ではこれらを設定しないので endpoint override は効かず、
// 通常通り AWS 公開エンドポイントに向く。
func newS3Client(ctx context.Context) (*s3.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	if endpoint == "" {
		if h := os.Getenv("LOCALSTACK_HOSTNAME"); h != "" {
			endpoint = "http://" + h + ":4566"
		}
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		}
	}), nil
}

func main() {
	lambda.Start(handler)
}
