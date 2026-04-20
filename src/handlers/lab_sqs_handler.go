package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/labstack/echo/v4"
)

// LabSQSHandler は SQS への publish と queue stats 取得を提供する lab 用ハンドラ。
// 実際にメッセージを消費する worker は別プロセス (cmd/worker) として動く。
type LabSQSHandler struct {
	client     *sqs.Client
	primaryURL string
	dlqURL     string
}

// NewLabSQSHandler は LocalStack 前提で SQS クライアントを初期化する。
// primary / dlq の URL はそれぞれ LAB_SQS_PRIMARY_URL / LAB_SQS_DLQ_URL で上書き可。
func NewLabSQSHandler() (*LabSQSHandler, error) {
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(getenv("AWS_REGION", "ap-northeast-1")),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	client := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	return &LabSQSHandler{
		client:     client,
		primaryURL: getenv("LAB_SQS_PRIMARY_URL", "http://localstack:4566/000000000000/lab-primary"),
		dlqURL:     getenv("LAB_SQS_DLQ_URL", "http://localstack:4566/000000000000/lab-dlq"),
	}, nil
}

type publishReq struct {
	Body string `json:"body"`
}

type publishRes struct {
	MessageID string `json:"messageId"`
}

// Publish はメッセージを lab-primary キューに送信する。
func (h *LabSQSHandler) Publish(c echo.Context) error {
	var req publishReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Body == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "body required")
	}
	out, err := h.client.SendMessage(c.Request().Context(), &sqs.SendMessageInput{
		QueueUrl:    aws.String(h.primaryURL),
		MessageBody: aws.String(req.Body),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, publishRes{MessageID: aws.ToString(out.MessageId)})
}

type queueAttr struct {
	Visible  int `json:"visible"`
	InFlight int `json:"inFlight"`
	Delayed  int `json:"delayed"`
}

type statsRes struct {
	Primary queueAttr `json:"primary"`
	DLQ     queueAttr `json:"dlq"`
}

// Stats は primary / dlq の概算メッセージ数を返す。
// SQS の Approximate 系属性は最終的整合性で、頻繁に呼んでも最新が返るとは限らない。
func (h *LabSQSHandler) Stats(c echo.Context) error {
	ctx := c.Request().Context()
	primary, err := h.queueAttrs(ctx, h.primaryURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "primary: "+err.Error())
	}
	dlq, err := h.queueAttrs(ctx, h.dlqURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "dlq: "+err.Error())
	}
	return c.JSON(http.StatusOK, statsRes{Primary: primary, DLQ: dlq})
}

func (h *LabSQSHandler) queueAttrs(ctx context.Context, url string) (queueAttr, error) {
	out, err := h.client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(url),
		AttributeNames: []types.QueueAttributeName{
			types.QueueAttributeNameApproximateNumberOfMessages,
			types.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
			types.QueueAttributeNameApproximateNumberOfMessagesDelayed,
		},
	})
	if err != nil {
		return queueAttr{}, err
	}
	return queueAttr{
		Visible:  atoi(out.Attributes["ApproximateNumberOfMessages"]),
		InFlight: atoi(out.Attributes["ApproximateNumberOfMessagesNotVisible"]),
		Delayed:  atoi(out.Attributes["ApproximateNumberOfMessagesDelayed"]),
	}, nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
