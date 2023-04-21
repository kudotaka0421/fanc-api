package main

import (
	"fanc-api/src/models"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
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

	// Initialize GORM
	var db *gorm.DB
	var err error

	for i := 0; i < 10; i++ {
		db, err = gorm.Open("mysql", "fanc_user:fanc_password@tcp(mysql:3306)/fanc?charset=utf8&parseTime=True&loc=Local")
		if err == nil {
			break
		}
		e.Logger.Warnf("Failed to connect to MySQL (attempt %d): %s", i+1, err.Error())
		time.Sleep(5 * time.Second)
	}

	// マイグレーション
	result := db.AutoMigrate(&models.Staff{})
	if result.Error != nil {
		log.Fatalf("Failed to auto migrate: %v", err)
	} else {
		fmt.Println("Migration succeeded")
	}

	e.GET("/api", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"message": "Hello from API!",
		})
	})

	e.Start(":8080")
}
