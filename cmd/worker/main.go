// Package main は lab 用の SQS worker バイナリ。
// docker-compose.lab.yml の worker サービスで起動し、lab-primary / lab-audit を
// long polling する。1 プロセス内で 2 goroutine が別キューを独立に消費する構造で、
// SNS → SQS fanout の両キュー同時消費を観察できる。
// メッセージ本文に "fail" を含む場合は DeleteMessage を呼ばず、visibility timeout 切れ
// による再配信を発生させる（primary の redrive policy により maxReceiveCount 到達で DLQ に移る）。
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
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

	queues := []struct {
		label string
		url   string
	}{
		{"primary", getenv("LAB_SQS_PRIMARY_URL", "http://localstack:4566/000000000000/lab-primary")},
		{"audit", getenv("LAB_SQS_AUDIT_URL", "http://localstack:4566/000000000000/lab-audit")},
	}

	var wg sync.WaitGroup
	for _, q := range queues {
		wg.Add(1)
		go func(label, url string) {
			defer wg.Done()
			pollLoop(ctx, client, label, url)
		}(q.label, q.url)
	}
	wg.Wait()
	log.Println("worker: shutdown")
}

// pollLoop は 1 キュー分の long polling ループ。SIGTERM で ctx がキャンセルされるまで回る。
func pollLoop(ctx context.Context, client *sqs.Client, label, queueURL string) {
	log.Printf("[%s] worker starting, polling %s", label, queueURL)
	for ctx.Err() == nil {
		out, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(queueURL),
			MaxNumberOfMessages: 10,
			// long polling: メッセージが無ければ最大 20 秒サーバー側で待つ
			WaitTimeSeconds: 20,
			// 処理中は他 worker から見えないようにする。この時間内に DeleteMessage
			// を呼ばないと再配信される
			VisibilityTimeout: 10,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[%s] receive error: %v", label, err)
			time.Sleep(1 * time.Second)
			continue
		}
		for _, msg := range out.Messages {
			body := aws.ToString(msg.Body)
			log.Printf("[%s] received: %s", label, body)
			time.Sleep(1 * time.Second) // ダミー処理

			if strings.Contains(body, "fail") {
				log.Printf("[%s] simulated failure for %q — skipping delete → redelivery", label, body)
				continue
			}

			_, err := client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(queueURL),
				ReceiptHandle: msg.ReceiptHandle,
			})
			if err != nil {
				log.Printf("[%s] delete error: %v", label, err)
				continue
			}
			log.Printf("[%s] processed: %s", label, body)
		}
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
