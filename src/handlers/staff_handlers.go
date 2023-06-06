package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"fanc-api/src/models"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm" // Replace the old gorm import
)

type StaffHandler struct {
	db *gorm.DB
}

func NewStaffHandler(db *gorm.DB) *StaffHandler {
	return &StaffHandler{db}
}

func (h *StaffHandler) GetStaffs(c echo.Context) error {
	staffs := []models.Staff{}

	if err := h.db.Find(&staffs).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to retrieve staffs",
		})
	}

	return c.JSON(http.StatusOK, staffs)
}

func (h *StaffHandler) GetStaffByID(c echo.Context) error {
	staffID := c.Param("staff_id")
	staff := new(models.Staff)

	if err := h.db.Select("id, first_name, last_name, first_name_kana, last_name_kana, email").Where("id = ?", staffID).First(&staff).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{
				"message": "Staff not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to retrieve staff",
		})
	}
	response := map[string]interface{}{
		"id":            staff.ID,
		"firstName":     staff.FirstName,
		"lastName":      staff.LastName,
		"firstNameKana": staff.FirstNameKana,
		"lastNameKana":  staff.LastNameKana,
		"email":         staff.Email,
	}

	return c.JSON(http.StatusOK, response)
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

func (h *StaffHandler) UpdateStaff(c echo.Context) error {
	// URLからIDを取得
	id, err := strconv.Atoi(c.Param("staff_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, err)
	}

	staff := new(models.Staff)
	//リクエストボディからデータをバインド
	if err := c.Bind(staff); err != nil {
		return c.JSON(http.StatusBadRequest, err)
	}

	if err := staff.Validate(); err != nil {
		return c.JSON(http.StatusBadRequest, err)
	}

	result := h.db.Model(&models.Staff{}).Where("id = ?", id).Updates(staff)

	if result.Error != nil {
		return c.JSON(http.StatusInternalServerError, result.Error)
	}

	if result.RowsAffected == 0 {
		return c.JSON(http.StatusNotFound, fmt.Errorf("No staff found with ID: %d", id))
	}

	return c.JSON(http.StatusOK, staff)
}

func (h *StaffHandler) DeleteStaff(c echo.Context) error {
	// URLからIDを取得
	id, err := strconv.Atoi(c.Param("staff_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
	}

	staff := new(models.Staff)
	result := h.db.Model(&models.Staff{}).Where("id = ?", id).Delete(staff)
	if result.Error != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete staff"})
	}

	if result.RowsAffected == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Staff not found"})
	}
	// 削除が成功したらステータスコード204を返す
	return c.NoContent(http.StatusNoContent)

}
