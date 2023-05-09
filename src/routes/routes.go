package routes

import (
	"fanc-api/src/handlers"

	"github.com/labstack/echo/v4"
)

func SetupRoutes(e *echo.Echo, staffHandler *handlers.StaffHandler) {
	e.POST("/api/staff", staffHandler.CreateStaff)
	e.GET("/api/staff", staffHandler.GetStaffs)
	e.GET("/api/staff/:staff_id", staffHandler.GetStaffByID)
	e.PUT("/api/staff/:staff_id", staffHandler.UpdateStaff)
}
