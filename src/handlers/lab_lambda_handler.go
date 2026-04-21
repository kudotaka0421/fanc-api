package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/labstack/echo/v4"
)

// LabLambdaHandler は S3 → Lambda トリガー lab 用のハンドラ。
// ブラウザは lab-lambda バケットに presigned PUT でアップロードし、
// LocalStack の S3 イベント通知で発火した Go Lambda が
// results/{key}.json を書き戻す。フロントは results/ を polling して結果を表示する。
type LabLambdaHandler struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	bucket        string
}

// NewLabLambdaHandler は backend 内部 S3 クライアントとブラウザ向け presigned URL
// 生成用クライアントを分離して初期化する (LabS3Handler と同じ理由)。
func NewLabLambdaHandler() (*LabLambdaHandler, error) {
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(getenv("AWS_REGION", "ap-northeast-1")),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	internalEndpoint := os.Getenv("AWS_ENDPOINT_URL")
	internalClient := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if internalEndpoint != "" {
			o.BaseEndpoint = aws.String(internalEndpoint)
			o.UsePathStyle = true
		}
	})

	publicEndpoint := getenv("AWS_PUBLIC_ENDPOINT_URL", "http://localhost:4566")
	presignSource := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(publicEndpoint)
		o.UsePathStyle = true
	})

	return &LabLambdaHandler{
		client:        internalClient,
		presignClient: s3.NewPresignClient(presignSource),
		bucket:        getenv("LAB_LAMBDA_BUCKET", "lab-lambda"),
	}, nil
}

type lambdaPresignPutReq struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
}

type lambdaPresignPutRes struct {
	URL string `json:"url"`
	Key string `json:"key"`
}

// PresignPut は lab-lambda バケットの uploads/ prefix に向けた PUT 用 presigned URL を発行する。
// ブラウザがこの URL に PUT すると S3 イベント通知 → Lambda が発火する。
func (h *LabLambdaHandler) PresignPut(c echo.Context) error {
	var req lambdaPresignPutReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Filename == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "filename required")
	}
	key := fmt.Sprintf("uploads/%s-%s", time.Now().UTC().Format("20060102-150405"), req.Filename)

	presigned, err := h.presignClient.PresignPutObject(c.Request().Context(), &s3.PutObjectInput{
		Bucket:      aws.String(h.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(req.ContentType),
	}, s3.WithPresignExpires(15*time.Minute))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, lambdaPresignPutRes{URL: presigned.URL, Key: key})
}

type lambdaObject struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified"`
}

type lambdaResult struct {
	Key          string          `json:"key"`
	Size         int64           `json:"size"`
	LastModified string          `json:"lastModified"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

// ListUploads は uploads/ 配下の元ファイル一覧を返す。
func (h *LabLambdaHandler) ListUploads(c echo.Context) error {
	items, err := h.list(c.Request().Context(), "uploads/")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"uploads": items})
}

// ListResults は results/ 配下の JSON を一覧する。
// 小さい JSON (< 64 KiB) は payload に中身をそのまま埋めて、フロントが 1 回の API で
// 処理結果を表示できるようにする。
func (h *LabLambdaHandler) ListResults(c echo.Context) error {
	ctx := c.Request().Context()
	items, err := h.list(ctx, "results/")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	results := make([]lambdaResult, 0, len(items))
	for _, it := range items {
		r := lambdaResult{Key: it.Key, Size: it.Size, LastModified: it.LastModified}
		if it.Size > 0 && it.Size < 1<<16 {
			out, err := h.client.GetObject(ctx, &s3.GetObjectInput{
				Bucket: aws.String(h.bucket),
				Key:    aws.String(it.Key),
			})
			if err == nil {
				data, _ := io.ReadAll(out.Body)
				out.Body.Close()
				r.Payload = data
			}
		}
		results = append(results, r)
	}
	return c.JSON(http.StatusOK, map[string]any{"results": results})
}

func (h *LabLambdaHandler) list(ctx context.Context, prefix string) ([]lambdaObject, error) {
	out, err := h.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(h.bucket),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return nil, err
	}
	items := make([]lambdaObject, 0, len(out.Contents))
	for _, o := range out.Contents {
		lm := ""
		if o.LastModified != nil {
			lm = o.LastModified.UTC().Format(time.RFC3339)
		}
		size := int64(0)
		if o.Size != nil {
			size = *o.Size
		}
		items = append(items, lambdaObject{
			Key:          aws.ToString(o.Key),
			Size:         size,
			LastModified: lm,
		})
	}
	return items, nil
}
