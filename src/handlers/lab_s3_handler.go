package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/labstack/echo/v4"
)

// LabS3Handler は S3 への presigned PUT（single）方式アップロードを扱う lab 用ハンドラ。
// B2B SaaS の請求書・経費・領収書程度のサイズ（< 100 MB）を想定し、multipart を避けて
// 実装を最小化する方針。方式比較の詳細は
// ~/.claude/docs/interview/system-design/file-upload-patterns.md を参照。
type LabS3Handler struct {
	presignClient *s3.PresignClient
	client        *s3.Client
	bucket        string
}

// NewLabS3Handler は内部用 S3 クライアントと、ブラウザ向け presigned URL 発行用
// クライアントをそれぞれ初期化する。LocalStack 利用時は backend から見える
// `localstack:4566` とブラウザから見える `localhost:4566` が異なるため分離が必要。
func NewLabS3Handler() (*LabS3Handler, error) {
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

	return &LabS3Handler{
		client:        internalClient,
		presignClient: s3.NewPresignClient(presignSource),
		bucket:        getenv("LAB_S3_BUCKET", "lab-uploads"),
	}, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type presignPutReq struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
}

type presignPutRes struct {
	URL string `json:"url"`
	Key string `json:"key"`
}

// PresignPut は PutObject 用の presigned URL を 15 分有効で発行する。
// ブラウザはこの URL に対してファイル全体を 1 回の PUT で送る。
func (h *LabS3Handler) PresignPut(c echo.Context) error {
	var req presignPutReq
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
	return c.JSON(http.StatusOK, presignPutRes{URL: presigned.URL, Key: key})
}

type objectItem struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified"`
}

// ListObjects は bucket 内の uploads/ 配下オブジェクトを一覧する。
func (h *LabS3Handler) ListObjects(c echo.Context) error {
	out, err := h.client.ListObjectsV2(c.Request().Context(), &s3.ListObjectsV2Input{
		Bucket: aws.String(h.bucket),
		Prefix: aws.String("uploads/"),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	items := make([]objectItem, 0, len(out.Contents))
	for _, o := range out.Contents {
		lm := ""
		if o.LastModified != nil {
			lm = o.LastModified.UTC().Format(time.RFC3339)
		}
		size := int64(0)
		if o.Size != nil {
			size = *o.Size
		}
		items = append(items, objectItem{
			Key:          aws.ToString(o.Key),
			Size:         size,
			LastModified: lm,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{"objects": items})
}
