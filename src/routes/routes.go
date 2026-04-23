package routes

import (
	"fanc-api/src/handlers"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func SetupRoutes(e *echo.Echo, tagHandler *handlers.TagHandler, schoolHandler *handlers.SchoolHandler, userHandler *handlers.UserHandler, authHandler *handlers.AuthHandler, healthCheckHandler *handlers.HealthCheckHandler, counselingHandler *handlers.CounselingHandler, labS3Handler *handlers.LabS3Handler, labSQSHandler *handlers.LabSQSHandler, labSNSHandler *handlers.LabSNSHandler, labLambdaHandler *handlers.LabLambdaHandler, labCacheHandler *handlers.LabCacheHandler, labRLSHandler *handlers.LabRLSHandler, labPartitionHandler *handlers.LabPartitionHandler) {
	e.GET("/healthcheck", healthCheckHandler.HealthCheck)

	// Lab (学習用、認証なし)。LocalStack 前提でローカル環境のみ使用する。
	lab := e.Group("/api/lab")
	if labS3Handler != nil {
		lab.GET("/s3/objects", labS3Handler.ListObjects)
		lab.POST("/s3/presign-put", labS3Handler.PresignPut)
	}
	if labSQSHandler != nil {
		lab.POST("/sqs/publish", labSQSHandler.Publish)
		lab.GET("/sqs/stats", labSQSHandler.Stats)
	}
	if labSNSHandler != nil {
		lab.POST("/sns/publish", labSNSHandler.PublishToTopic)
		lab.GET("/sns/stats", labSNSHandler.FanoutStats)
	}
	if labLambdaHandler != nil {
		lab.POST("/lambda/presign-put", labLambdaHandler.PresignPut)
		lab.GET("/lambda/uploads", labLambdaHandler.ListUploads)
		lab.GET("/lambda/results", labLambdaHandler.ListResults)
	}
	if labCacheHandler != nil {
		lab.GET("/cache/schools", labCacheHandler.GetSchools)
		lab.DELETE("/cache/schools", labCacheHandler.DeleteCache)
	}
	if labRLSHandler != nil {
		lab.GET("/rls/tenants", labRLSHandler.ListTenants)
		lab.GET("/rls/samples", labRLSHandler.ListSamples)
		lab.POST("/rls/samples", labRLSHandler.CreateSample)
	}
	if labPartitionHandler != nil {
		lab.GET("/partition", labPartitionHandler.Query)
	}
	// Auth
	// [TODO]/api/userは「/api/signup」として切り分けたい
	e.POST("/api/login", authHandler.Login)
	e.POST("/api/user", userHandler.CreateUser)
	e.POST("/api/confirm-account/:token", userHandler.ConfirmAccount)

	var jwtKey = []byte(os.Getenv("JWT_SECRET_KEY"))
	// Configure JWT middleware
	jwtMiddleware := middleware.JWTWithConfig(middleware.JWTConfig{
		SigningKey: []byte(jwtKey), // replace with your own secret
		ErrorHandler: func(err error) error {
			return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
		},
	})

	// Group for routes that require authentication
	authenticated := e.Group("")
	authenticated.Use(jwtMiddleware)

	// Authenticated /me route
	authenticated.GET("/api/me", authHandler.GetMe)

	//User
	// authenticated.POST("/api/user", userHandler.CreateUser)
	authenticated.GET("/api/user", userHandler.GetUsers)
	authenticated.GET("/api/user/:user_id", userHandler.GetUserByID)
	authenticated.PUT("/api/user/:user_id", userHandler.UpdateUser)
	authenticated.DELETE("/api/user/:user_id", userHandler.DeleteUser)
	authenticated.POST("/api/confirm-account/:token", userHandler.ConfirmAccount)

	// Tag
	authenticated.GET("/api/tag", tagHandler.GetTags)
	authenticated.POST("/api/tag", tagHandler.CreateTag)
	authenticated.GET("/api/tag/:tag_id", tagHandler.GetTagByID)
	authenticated.PUT("/api/tag/:tag_id", tagHandler.UpdateTag)
	authenticated.DELETE("/api/tag/:tag_id", tagHandler.DeleteTag)

	// School
	authenticated.POST("/api/school", schoolHandler.CreateSchool)
	authenticated.GET("/api/school", schoolHandler.GetSchools)
	authenticated.GET("/api/school/:school_id", schoolHandler.GetSchoolByID)
	authenticated.PUT("/api/school/:school_id", schoolHandler.UpdateSchool)
	authenticated.DELETE("/api/school/:school_id", schoolHandler.DeleteSchool)

	// Counseling
	authenticated.GET("/api/counseling", counselingHandler.GetCounselings)
	authenticated.GET("/api/counseling/:counseling_id", counselingHandler.GetCounselingByID)
	authenticated.POST("/api/counseling", counselingHandler.CreateCounseling)
	authenticated.PUT("/api/counseling/:counseling_id", counselingHandler.UpdateCounseling)
	authenticated.DELETE("/api/counseling/:counseling_id", counselingHandler.DeleteCounseling)
}
