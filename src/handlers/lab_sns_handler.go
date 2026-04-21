package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/labstack/echo/v4"
)

// LabSNSHandler は SNS → SQS fanout の publish と 2 つのキューの stats を提供する。
// 1 回の SNS Publish で lab-primary / lab-audit の両方にメッセージがコピーされる様子を
// UI から観察できるようにするのが目的。
type LabSNSHandler struct {
	snsClient   *sns.Client
	sqsClient   *sqs.Client
	topicARN    string
	primaryURL  string
	auditURL    string
}

// NewLabSNSHandler は LocalStack 前提で SNS / SQS クライアントを初期化する。
func NewLabSNSHandler() (*LabSNSHandler, error) {
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(getenv("AWS_REGION", "ap-northeast-1")),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	snsClient := sns.NewFromConfig(cfg, func(o *sns.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})
	sqsClient := sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	})

	return &LabSNSHandler{
		snsClient:  snsClient,
		sqsClient:  sqsClient,
		topicARN:   getenv("LAB_SNS_TOPIC_ARN", "arn:aws:sns:ap-northeast-1:000000000000:lab-events"),
		primaryURL: getenv("LAB_SQS_PRIMARY_URL", "http://localstack:4566/000000000000/lab-primary"),
		auditURL:   getenv("LAB_SQS_AUDIT_URL", "http://localstack:4566/000000000000/lab-audit"),
	}, nil
}

type snsPublishReq struct {
	Body string `json:"body"`
}

type snsPublishRes struct {
	MessageID string `json:"messageId"`
}

// PublishToTopic は SNS topic にメッセージを publish する。
// subscribe されている全 SQS キュー（lab-primary / lab-audit）に同じメッセージが配信される。
func (h *LabSNSHandler) PublishToTopic(c echo.Context) error {
	var req snsPublishReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Body == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "body required")
	}
	out, err := h.snsClient.Publish(c.Request().Context(), &sns.PublishInput{
		TopicArn: aws.String(h.topicARN),
		Message:  aws.String(req.Body),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, snsPublishRes{MessageID: aws.ToString(out.MessageId)})
}

type fanoutQueueStat struct {
	Name     string `json:"name"`
	Visible  int    `json:"visible"`
	InFlight int    `json:"inFlight"`
}

type fanoutStatsRes struct {
	Queues []fanoutQueueStat `json:"queues"`
}

// FanoutStats は primary / audit の概算件数を返す。
// publish 直後に両方の visible が +1 されることで fanout の挙動を確認する。
func (h *LabSNSHandler) FanoutStats(c echo.Context) error {
	ctx := c.Request().Context()
	queues := []struct {
		name string
		url  string
	}{
		{"lab-primary", h.primaryURL},
		{"lab-audit", h.auditURL},
	}
	stats := make([]fanoutQueueStat, 0, len(queues))
	for _, q := range queues {
		out, err := h.sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
			QueueUrl: aws.String(q.url),
			AttributeNames: []types.QueueAttributeName{
				types.QueueAttributeNameApproximateNumberOfMessages,
				types.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
			},
		})
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, q.name+": "+err.Error())
		}
		stats = append(stats, fanoutQueueStat{
			Name:     q.name,
			Visible:  atoi(out.Attributes["ApproximateNumberOfMessages"]),
			InFlight: atoi(out.Attributes["ApproximateNumberOfMessagesNotVisible"]),
		})
	}
	return c.JSON(http.StatusOK, fanoutStatsRes{Queues: stats})
}
