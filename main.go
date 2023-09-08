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
	userHandler := handlers.NewUserHandler(db)
	authHandler := handlers.NewAuthHandler(db)
	healthCheckHandler := handlers.NewHealthCheckHandler()
	routes.SetupRoutes(e, tagHandler, schoolHandler, userHandler, authHandler, healthCheckHandler)

	e.Start(":8080")
}
