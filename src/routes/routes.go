package routes

import (
	"fanc-api/src/handlers"

	"github.com/labstack/echo/v4"
)

func SetupRoutes(e *echo.Echo, staffHandler *handlers.StaffHandler) {
	e.POST("/api/staff", staffHandler.CreateStaff)
	e.GET("/api/staff", staffHandler.GetStaffs)
}
