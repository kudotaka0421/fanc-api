package routes

import (
	"fanc-api/src/handlers"

	"github.com/labstack/echo/v4"
)

func SetupRoutes(e *echo.Echo, tagHandler *handlers.TagHandler, schoolHandler *handlers.SchoolHandler, userHandler *handlers.UserHandler) {
	//User
	e.POST("/api/user", userHandler.CreateUser)
	e.GET("/api/user", userHandler.GetUsers)
	e.GET("/api/user/:user_id", userHandler.GetUserByID)
	e.PUT("/api/user/:user_id", userHandler.UpdateUser)
	e.DELETE("/api/user/:user_id", userHandler.DeleteUser)
	e.GET("/confirm-account/:token", userHandler.ConfirmAccount)

	// Tag
	e.GET("/api/tag", tagHandler.GetTags)
	e.POST("/api/tag", tagHandler.CreateTag)
	e.GET("/api/tag/:tag_id", tagHandler.GetTagByID)
	e.PUT("/api/tag/:tag_id", tagHandler.UpdateTag)
	e.DELETE("/api/tag/:tag_id", tagHandler.DeleteTag)

	// School
	e.POST("/api/school", schoolHandler.CreateSchool)
	e.GET("/api/school", schoolHandler.GetSchools)
	e.GET("/api/school/:school_id", schoolHandler.GetSchoolByID)
	e.PUT("/api/school/:school_id", schoolHandler.UpdateSchool)
	e.DELETE("/api/school/:school_id", schoolHandler.DeleteSchool)

}
