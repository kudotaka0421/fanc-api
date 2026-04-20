#!/bin/bash
set -e

# LocalStack 起動時に実行される初期化スクリプト。
# SQS キュー、SNS トピック、S3 バケットを用意する。

ENDPOINT=http://localhost:4566
REGION=ap-northeast-1

echo "[init] creating S3 buckets"
awslocal s3 mb s3://lab-uploads --region "$REGION"
awslocal s3 mb s3://lab-lambda --region "$REGION"

echo "[init] configuring S3 CORS on lab-uploads (required for browser PUT)"
# ブラウザが presigned URL に直接 PUT するため CORS 設定が必要。
# ExposeHeaders に ETag を含めないとフロント側で Complete 時の ETag が拾えない。
cat > /tmp/lab-uploads-cors.json <<'EOF'
{
  "CORSRules": [
    {
      "AllowedOrigins": ["*"],
      "AllowedMethods": ["PUT", "GET", "HEAD", "POST"],
      "AllowedHeaders": ["*"],
      "ExposeHeaders": ["ETag"],
      "MaxAgeSeconds": 3000
    }
  ]
}
EOF
awslocal s3api put-bucket-cors \
  --bucket lab-uploads \
  --cors-configuration file:///tmp/lab-uploads-cors.json \
  --region "$REGION"

echo "[init] creating SQS queues"
awslocal sqs create-queue --queue-name lab-primary --region "$REGION"
awslocal sqs create-queue --queue-name lab-audit   --region "$REGION"
awslocal sqs create-queue --queue-name lab-dlq     --region "$REGION"

echo "[init] wiring redrive policy: lab-primary → lab-dlq after 3 failed receives"
# DLQ の ARN を取得してから、primary に「maxReceiveCount=3 を超えたら DLQ に送る」
# という RedrivePolicy を設定する。worker が "fail" を含むメッセージを消し損ねた
# 時の挙動観察に使う（visibility timeout 切れ × 3 回 → DLQ 行き）
REDRIVE_DLQ_ARN=$(awslocal sqs get-queue-attributes --queue-url "$ENDPOINT/000000000000/lab-dlq" --attribute-names QueueArn --region "$REGION" --query 'Attributes.QueueArn' --output text)
cat > /tmp/sqs-redrive.json <<EOF
{
  "RedrivePolicy": "{\"deadLetterTargetArn\":\"$REDRIVE_DLQ_ARN\",\"maxReceiveCount\":\"3\"}"
}
EOF
awslocal sqs set-queue-attributes \
  --queue-url "$ENDPOINT/000000000000/lab-primary" \
  --attributes file:///tmp/sqs-redrive.json \
  --region "$REGION"

echo "[init] creating SNS topic"
TOPIC_ARN=$(awslocal sns create-topic --name lab-events --region "$REGION" --query 'TopicArn' --output text)
PRIMARY_ARN=$(awslocal sqs get-queue-attributes --queue-url "$ENDPOINT/000000000000/lab-primary" --attribute-names QueueArn --region "$REGION" --query 'Attributes.QueueArn' --output text)
AUDIT_ARN=$(awslocal sqs get-queue-attributes   --queue-url "$ENDPOINT/000000000000/lab-audit"   --attribute-names QueueArn --region "$REGION" --query 'Attributes.QueueArn' --output text)

echo "[init] subscribing SQS queues to SNS topic"
awslocal sns subscribe --topic-arn "$TOPIC_ARN" --protocol sqs --notification-endpoint "$PRIMARY_ARN" --attributes RawMessageDelivery=true --region "$REGION"
awslocal sns subscribe --topic-arn "$TOPIC_ARN" --protocol sqs --notification-endpoint "$AUDIT_ARN"   --attributes RawMessageDelivery=true --region "$REGION"

echo "[init] done"
