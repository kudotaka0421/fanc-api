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
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/labstack/echo/v4"
)

// LabS3Handler は S3 の multipart upload を presigned URL 方式で扱う lab 用ハンドラ。
// 内部用クライアント（backend → localstack）と、presign で使う公開エンドポイント用
// クライアントを分けて持つ。前者は CreateMultipartUpload / Complete / List など
// サーバーから直接 S3 を叩く用途、後者はブラウザが直接 PUT するための URL 生成用。
type LabS3Handler struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	bucket        string
}

// NewLabS3Handler は環境変数から S3 クライアントを初期化する。
// LocalStack 利用時は AWS_ENDPOINT_URL / AWS_PUBLIC_ENDPOINT_URL を分けて設定する。
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
	presignClient := s3.NewPresignClient(presignSource)

	return &LabS3Handler{
		client:        internalClient,
		presignClient: presignClient,
		bucket:        getenv("LAB_S3_BUCKET", "lab-uploads"),
	}, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type createMultipartReq struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
}

type createMultipartRes struct {
	UploadID string `json:"uploadId"`
	Key      string `json:"key"`
}

// CreateMultipart は multipart upload を開始し uploadId とオブジェクト key を返す。
func (h *LabS3Handler) CreateMultipart(c echo.Context) error {
	var req createMultipartReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Filename == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "filename required")
	}
	key := fmt.Sprintf("uploads/%s-%s", time.Now().UTC().Format("20060102-150405"), req.Filename)

	out, err := h.client.CreateMultipartUpload(c.Request().Context(), &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(h.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(req.ContentType),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, createMultipartRes{
		UploadID: aws.ToString(out.UploadId),
		Key:      key,
	})
}

type signPartReq struct {
	Key        string `json:"key"`
	UploadID   string `json:"uploadId"`
	PartNumber int32  `json:"partNumber"`
}

type signPartRes struct {
	URL string `json:"url"`
}

// SignPart は指定パート番号の UploadPart 用 presigned URL（PUT）を 15 分有効で返す。
func (h *LabS3Handler) SignPart(c echo.Context) error {
	var req signPartReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	presigned, err := h.presignClient.PresignUploadPart(c.Request().Context(), &s3.UploadPartInput{
		Bucket:     aws.String(h.bucket),
		Key:        aws.String(req.Key),
		UploadId:   aws.String(req.UploadID),
		PartNumber: aws.Int32(req.PartNumber),
	}, s3.WithPresignExpires(15*time.Minute))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, signPartRes{URL: presigned.URL})
}

type completePart struct {
	PartNumber int32  `json:"partNumber"`
	ETag       string `json:"eTag"`
}

type completeReq struct {
	Key      string         `json:"key"`
	UploadID string         `json:"uploadId"`
	Parts    []completePart `json:"parts"`
}

type completeRes struct {
	Location string `json:"location"`
}

// CompleteMultipart は各パートの ETag を結合して multipart upload を確定する。
func (h *LabS3Handler) CompleteMultipart(c echo.Context) error {
	var req completeReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	parts := make([]types.CompletedPart, 0, len(req.Parts))
	for _, p := range req.Parts {
		parts = append(parts, types.CompletedPart{
			PartNumber: aws.Int32(p.PartNumber),
			ETag:       aws.String(p.ETag),
		})
	}
	out, err := h.client.CompleteMultipartUpload(c.Request().Context(), &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(h.bucket),
		Key:             aws.String(req.Key),
		UploadId:        aws.String(req.UploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, completeRes{Location: aws.ToString(out.Location)})
}

type abortReq struct {
	Key      string `json:"key"`
	UploadID string `json:"uploadId"`
}

// AbortMultipart は未完了の multipart upload を中止する。
// 放置するとパートが S3 にゴミとして残り課金されるため、失敗時は必ず呼ぶ。
func (h *LabS3Handler) AbortMultipart(c echo.Context) error {
	var req abortReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	_, err := h.client.AbortMultipartUpload(c.Request().Context(), &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(h.bucket),
		Key:      aws.String(req.Key),
		UploadId: aws.String(req.UploadID),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]bool{"ok": true})
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
