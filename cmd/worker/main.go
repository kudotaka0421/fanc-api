// Package main は lab 用の SQS worker バイナリ。
// docker-compose.lab.yml の worker サービスで起動し、lab-primary を long polling する。
// メッセージ本文に "fail" を含む場合は DeleteMessage を呼ばず、visibility timeout 切れ
// による再配信を発生させる（redrive policy により maxReceiveCount 到達で DLQ に移る）。
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	region := getenv("AWS_REGION", "ap-northeast-1")
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}

	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	client := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	queueURL := getenv("LAB_SQS_PRIMARY_URL", "http://localstack:4566/000000000000/lab-primary")
	log.Printf("worker: starting, polling %s", queueURL)

	for ctx.Err() == nil {
		out, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 10,
			// long polling: メッセージが無ければ最大 20 秒サーバー側で待つ
			// 空ポーリングによる無駄なリクエストと料金を抑える基本設定
			WaitTimeSeconds: 20,
			// 処理中は他 worker から見えないようにする。この時間内に DeleteMessage
			// を呼ばないと再配信される（失敗扱い）
			VisibilityTimeout: 10,
		})
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Printf("receive error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		for _, msg := range out.Messages {
			body := aws.ToString(msg.Body)
			log.Printf("worker: received: %s", body)
			// ダミー処理（1 秒のウェイト）
			time.Sleep(1 * time.Second)

			if strings.Contains(body, "fail") {
				log.Printf("worker: simulated failure for %q — skipping delete → redelivery", body)
				continue
			}

			_, err := client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(queueURL),
				ReceiptHandle: msg.ReceiptHandle,
			})
			if err != nil {
				log.Printf("delete error: %v", err)
				continue
			}
			log.Printf("worker: processed: %s", body)
		}
	}
	log.Println("worker: shutdown")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
