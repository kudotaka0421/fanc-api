package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type HealthCheckHandler struct {
}

func NewHealthCheckHandler() *HealthCheckHandler {
	return &HealthCheckHandler{}
}

func (handler *HealthCheckHandler) HealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "OK"})
}
