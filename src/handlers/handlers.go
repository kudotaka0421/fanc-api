package handlers

import (
	"net/http"

	"fanc-api/src/models"

	"github.com/jinzhu/gorm"
	"github.com/labstack/echo/v4"
)

type StaffHandler struct {
	db *gorm.DB
}

func NewStaffHandler(db *gorm.DB) *StaffHandler {
	return &StaffHandler{db}
}

func (h *StaffHandler) CreateStaff(c echo.Context) error {
	staff := new(models.Staff)

	// リクエストのボディのデータをstaffにバインドする
	if err := c.Bind(staff); err != nil {
		// エラーステータス500のJSONレスポンスを返す
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Invalid data",
		})
	}

	// バリデーションの実行
	if err := staff.Validate(); err != nil {
		return c.JSON(http.StatusBadRequest, err)
	}

	if err := h.db.Create(staff).Error; err != nil {
		// エラーステータス500のJSONレスポンスを返す
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to create staff",
		})
	}
	return c.JSON(http.StatusCreated, map[string]string{
		"message": "Staff created successfully",
	})
}
