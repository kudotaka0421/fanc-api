#!/bin/sh
# lab 用 Go Lambda を LocalStack にデプロイして、lab-lambda バケットに
# uploads/ prefix の PutObject → Lambda 呼び出しの S3 イベント通知を設定する。
# docker-compose.lab.yml の lambda-deployer 一回限りサービスから実行される。
set -e

FUNCTION_NAME=lab-s3-processor
BUCKET=lab-lambda
# LocalStack では IAM をモックするため、role ARN の実在性は問われない。
# 本番では Lambda 実行ロール（logs:CreateLogGroup / s3:GetObject / s3:PutObject 等）を作成する。
ROLE_ARN=arn:aws:iam::000000000000:role/lambda-role

echo "[deploy-lambda] waiting for localstack..."
# init-localstack.sh で lab-lambda バケットが作られるまで待つ。
until awslocal s3 ls "s3://${BUCKET}" > /dev/null 2>&1; do
    sleep 2
done

cd /workspace
rm -f lambda.zip
zip -q lambda.zip bootstrap

if awslocal lambda get-function --function-name "$FUNCTION_NAME" > /dev/null 2>&1; then
    echo "[deploy-lambda] updating existing function"
    awslocal lambda update-function-code \
        --function-name "$FUNCTION_NAME" \
        --zip-file fileb://lambda.zip > /dev/null
else
    echo "[deploy-lambda] creating function"
    awslocal lambda create-function \
        --function-name "$FUNCTION_NAME" \
        --runtime provided.al2 \
        --handler bootstrap \
        --role "$ROLE_ARN" \
        --zip-file fileb://lambda.zip \
        --timeout 30 \
        --memory-size 256 \
        --environment "Variables={LAB_LAMBDA_BUCKET=${BUCKET}}" > /dev/null
fi

echo "[deploy-lambda] waiting for function to be Active"
# LocalStack の Lambda は非同期に Active になるため provisioning 完了を待つ。
for _ in $(seq 1 30); do
    STATE=$(awslocal lambda get-function --function-name "$FUNCTION_NAME" --query 'Configuration.State' --output text 2>/dev/null || true)
    if [ "$STATE" = "Active" ]; then
        break
    fi
    sleep 1
done

FUNCTION_ARN=$(awslocal lambda get-function --function-name "$FUNCTION_NAME" --query 'Configuration.FunctionArn' --output text)
echo "[deploy-lambda] function ARN: $FUNCTION_ARN"

# S3 → Lambda 呼び出し権限。既にあれば無視。
awslocal lambda add-permission \
    --function-name "$FUNCTION_NAME" \
    --statement-id s3-invoke \
    --action lambda:InvokeFunction \
    --principal s3.amazonaws.com \
    --source-arn "arn:aws:s3:::${BUCKET}" > /dev/null 2>&1 || true

echo "[deploy-lambda] configuring S3 event notification on ${BUCKET} (prefix=uploads/)"
# prefix=uploads/ を絞ることで results/ への書き戻しが再発火しないようにしている。
# (Lambda 側でも results/ プレフィックスの early return を入れているが二重防御)
cat > /tmp/s3-notification.json <<EOF
{
  "LambdaFunctionConfigurations": [
    {
      "Id": "lab-lambda-on-uploads",
      "LambdaFunctionArn": "${FUNCTION_ARN}",
      "Events": ["s3:ObjectCreated:Put"],
      "Filter": {
        "Key": {
          "FilterRules": [
            { "Name": "prefix", "Value": "uploads/" }
          ]
        }
      }
    }
  ]
}
EOF

awslocal s3api put-bucket-notification-configuration \
    --bucket "$BUCKET" \
    --notification-configuration file:///tmp/s3-notification.json

# lab-lambda バケットにも CORS を付けておく (presigned PUT をブラウザから叩くため)。
# init-localstack.sh では lab-uploads にしか入れてないので、ここで補完。
cat > /tmp/lab-lambda-cors.json <<'EOF'
{
  "CORSRules": [
    {
      "AllowedOrigins": ["*"],
      "AllowedMethods": ["PUT", "GET", "HEAD"],
      "AllowedHeaders": ["*"],
      "ExposeHeaders": ["ETag"],
      "MaxAgeSeconds": 3000
    }
  ]
}
EOF
awslocal s3api put-bucket-cors \
    --bucket "$BUCKET" \
    --cors-configuration file:///tmp/lab-lambda-cors.json

echo "[deploy-lambda] done"
