package routes

import (
	"fanc-api/src/handlers"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func SetupRoutes(e *echo.Echo, tagHandler *handlers.TagHandler, schoolHandler *handlers.SchoolHandler, userHandler *handlers.UserHandler, authHandler *handlers.AuthHandler, healthCheckHandler *handlers.HealthCheckHandler, counselingHandler *handlers.CounselingHandler) {
	e.GET("/healthcheck", healthCheckHandler.HealthCheck)
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
	// authenticated.POST("/api/confirm-account/:token", userHandler.ConfirmAccount)

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
	authenticated.POST("/api/counseling", counselingHandler.CreateCounseling)
}
