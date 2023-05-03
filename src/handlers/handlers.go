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
		if gorm.IsRecordNotFoundError(err) {
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
