package main

import (
	"fmt"
	"os"
	"time"

	"fanc-api/src/handlers"
	"fanc-api/src/models"
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

	mysqlUser := os.Getenv("MYSQL_USER")
	mysqlPassword := os.Getenv("MYSQL_PASSWORD")
	mysqlDataBase := os.Getenv("MYSQL_DATABASE")
	connectionString := fmt.Sprintf("%s:%s@tcp(mysql:3306)/%s?charset=utf8&parseTime=True&loc=Local", mysqlUser, mysqlPassword, mysqlDataBase)
	for i := 0; i < 10; i++ {
		db, err = gorm.Open("mysql", connectionString)
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

	// Migrate the schema
	db.AutoMigrate(&models.Staff{}, &models.Tag{}) // StaffとTagのテーブルを作成または更新

	staffHandler := handlers.NewStaffHandler(db)
	tagHandler := handlers.NewTagHandler(db)

	routes.SetupRoutes(e, staffHandler, tagHandler)

	e.Start(":8080")
}
