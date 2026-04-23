package main

import (
	"fmt"
	"os"
	"time"

	"fanc-api/src/handlers"
	"fanc-api/src/routes"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	e := echo.New()

	// CORSの設定
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{os.Getenv("CORS_ALLOW_ORIGIN")},
		AllowMethods: []string{echo.GET, echo.PUT, echo.POST, echo.DELETE},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, "X-Tenant-Id"},
	}))

	// Initialize GORM
	var db *gorm.DB
	var err error

	mysqlUser := os.Getenv("MYSQL_USER")
	mysqlPassword := os.Getenv("MYSQL_PASSWORD")
	mysqlDataBase := os.Getenv("MYSQL_DATABASE")
	mysqlHost := os.Getenv("MYSQL_HOST")
	connectionString := fmt.Sprintf("%s:%s@tcp(%s:3306)/%s?charset=utf8&parseTime=True&loc=Local", mysqlUser, mysqlPassword, mysqlHost, mysqlDataBase)

	for i := 0; i < 10; i++ {
		db, err = gorm.Open(mysql.Open(connectionString), &gorm.Config{})
		if err == nil {
			break
		}
		e.Logger.Warnf("Failed to connect to MySQL (attempt %d): %s", i+1, err.Error())
		time.Sleep(5 * time.Second)
	}

	if err != nil {
		e.Logger.Fatal(err)
	}

	tagHandler := handlers.NewTagHandler(db)
	schoolHandler := handlers.NewSchoolHandler(db)
	counselingHandler := handlers.NewCounselingHandler(db)
	userHandler := handlers.NewUserHandler(db)
	authHandler := handlers.NewAuthHandler(db)
	healthCheckHandler := handlers.NewHealthCheckHandler()

	// Lab 用ハンドラは LocalStack (AWS_ENDPOINT_URL) が無くても初期化自体は成功する
	// 実際の S3/SQS 呼び出し時に LocalStack へ到達できなければエラーになる
	labS3Handler, err := handlers.NewLabS3Handler()
	if err != nil {
		e.Logger.Warnf("lab s3 handler init failed, /api/lab/s3/* disabled: %s", err.Error())
	}
	labSQSHandler, err := handlers.NewLabSQSHandler()
	if err != nil {
		e.Logger.Warnf("lab sqs handler init failed, /api/lab/sqs/* disabled: %s", err.Error())
	}
	labSNSHandler, err := handlers.NewLabSNSHandler()
	if err != nil {
		e.Logger.Warnf("lab sns handler init failed, /api/lab/sns/* disabled: %s", err.Error())
	}
	labLambdaHandler, err := handlers.NewLabLambdaHandler()
	if err != nil {
		e.Logger.Warnf("lab lambda handler init failed, /api/lab/lambda/* disabled: %s", err.Error())
	}
	labCacheHandler, err := handlers.NewLabCacheHandler(db)
	if err != nil {
		e.Logger.Warnf("lab cache handler init failed, /api/lab/cache/* disabled: %s", err.Error())
	}
	labRLSHandler, err := handlers.NewLabRLSHandler()
	if err != nil {
		e.Logger.Warnf("lab rls handler init failed, /api/lab/rls/* disabled: %s", err.Error())
	}

	routes.SetupRoutes(e, tagHandler, schoolHandler, userHandler, authHandler, healthCheckHandler, counselingHandler, labS3Handler, labSQSHandler, labSNSHandler, labLambdaHandler, labCacheHandler, labRLSHandler)

	e.Start(":8080")
}
