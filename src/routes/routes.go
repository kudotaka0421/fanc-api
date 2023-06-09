package routes

import (
	"fanc-api/src/handlers"

	"github.com/labstack/echo/v4"
)

func SetupRoutes(e *echo.Echo, tagHandler *handlers.TagHandler, schoolHandler *handlers.SchoolHandler) {
	// Staff
	// e.POST("/api/staff", staffHandler.CreateStaff)
	// e.GET("/api/staff", staffHandler.GetStaffs)
	// e.GET("/api/staff/:staff_id", staffHandler.GetStaffByID)
	// e.PUT("/api/staff/:staff_id", staffHandler.UpdateStaff)
	// e.DELETE("/api/staff/:staff_id", staffHandler.DeleteStaff)

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
