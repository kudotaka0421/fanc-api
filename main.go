package main

import (
	"time"

	"fanc-api/src/handlers"
	"fanc-api/src/routes"

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
		db, err = gorm.Open("mysql", "root:root_password@tcp(mysql:3306)/fanc?charset=utf8&parseTime=True&loc=Local")
		if err == nil {
			break
		}
		e.Logger.Warnf("Failed to connect to MySQL (attempt %d): %s", i+1, err.Error())
		time.Sleep(5 * time.Second)
	}

	if err != nil {
		e.Logger.Fatal(err)
	}
	defer db.Close()

	staffHandler := handlers.NewStaffHandler(db)
	routes.SetupRoutes(e, staffHandler)

	e.Start(":8080")
}
