package main

import (
	"fmt"
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
		db, err = gorm.Open("mysql", "username:password@tcp(mysql:3306)/dbname?charset=utf8&parseTime=True&loc=Local")
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

	// マイグレーションの実行
	err = RemoveMailColumnFromStaffTable(db)
	if err != nil {
		fmt.Printf("カラムの削除に失敗しました: %v\n", err)
	} else {
		fmt.Println("カラムの削除が正常に実行されました")
	}

	e.GET("/api", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"message": "Hello from API!",
		})
	})

	e.Start(":8080")
}

// staffsテーブルからmailカラムを削除
func RemoveMailColumnFromStaffTable(db *gorm.DB) error {
	err := db.Exec("ALTER TABLE staffs DROP COLUMN mail").Error
	if err != nil {
		return err
	}
	return nil
}
