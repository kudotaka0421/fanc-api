package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	e := echo.New()

	// CORSの設定
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{

		AllowOrigins: []string{"http://localhost:5173"},
		AllowMethods: []string{echo.GET, echo.PUT, echo.POST, echo.DELETE},
	}))

	e.GET("/api", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"message": "Hello from API!",
		})
	})

	e.Start(":8080")
}
