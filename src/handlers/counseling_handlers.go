package handlers

import (
	"fanc-api/src/models"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type CounselingHandler struct {
	db *gorm.DB
}

func NewCounselingHandler(db *gorm.DB) *CounselingHandler {
	return &CounselingHandler{db}
}

type CounselingParams struct {
	ID            *uint     `json:"id"`
	CounseleeName string    `json:"counseleeName"`
	Email         string    `json:"email"`
	Status        int       `json:"status"`
	Date          time.Time `json:"date"`
	Remarks       *string   `json:"remarks"`
	Message       *string   `json:"message"`
	UserID        uint      `json:"userId"` // Changed from *uint to uint
	SchoolIds     *[]uint   `json:"schoolIds"`
}

func (h *CounselingHandler) GetCounselings(c echo.Context) error {
	counselings := []models.Counseling{}

	if err := h.db.Preload("Schools").Preload("User").Order("date DESC").Find(&counselings).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to retrieve counselings",
		})
	}

	counselingsResponse := make([]map[string]interface{}, len(counselings))
	for i, counseling := range counselings {
		// Convert counseling.Schools into a slice of school IDs
		schoolIds := make([]uint, len(counseling.Schools))
		for j, school := range counseling.Schools {
			schoolIds[j] = school.ID
		}

		var jst = time.FixedZone("Asia/Tokyo", 9*60*60)
		jstDate := counseling.Date.In(jst) // JSTに変換

		counselingsResponse[i] = map[string]interface{}{
			"id":                counseling.ID,
			"counseleeName":     counseling.CounseleeName,
			"email":             counseling.Email,
			"status":            counseling.Status,
			"date":              jstDate,
			"remarks":           counseling.Remarks,
			"message":           counseling.Message,
			"user":              counseling.User,
			"selectedSchoolIds": schoolIds,
		}
	}

	return c.JSON(http.StatusOK, counselingsResponse)
}

func (h *CounselingHandler) GetCounselingByID(c echo.Context) error {
	counseling := models.Counseling{}
	counselingID := c.Param("counseling_id")

	if err := h.db.Preload("Schools").Preload("User").Order("date DESC").Where("id = ?", counselingID).First(&counseling).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to retrieve counselings",
		})
	}

	schoolIds := make([]uint, len(counseling.Schools))
	for j, school := range counseling.Schools {
		schoolIds[j] = school.ID
	}

	var jst = time.FixedZone("Asia/Tokyo", 9*60*60)
	jstDate := counseling.Date.In(jst) // JSTに変換

	counselingsResponse := map[string]interface{}{
		"id":                counseling.ID,
		"counseleeName":     counseling.CounseleeName,
		"email":             counseling.Email,
		"status":            counseling.Status,
		"date":              jstDate,
		"remarks":           counseling.Remarks,
		"message":           counseling.Message,
		"user":              counseling.User,
		"selectedSchoolIds": schoolIds,
	}

	return c.JSON(http.StatusOK, counselingsResponse)

}
func (h *CounselingHandler) CreateCounseling(c echo.Context) error {
	params := new(CounselingParams)

	if err := c.Bind(params); err != nil {
		fmt.Println("Bind Error:", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Invalid data",
		})
	}

	counseling := &models.Counseling{
		CounseleeName: params.CounseleeName,
		Email:         params.Email,
		Status:        params.Status,
		Date:          params.Date,
		Remarks:       params.Remarks,
		Message:       params.Message,
		UserID:        params.UserID,
	}

	if err := counseling.Validate(); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": err.Error(),
		})
	}

	schools := []models.School{}
	for _, id := range *params.SchoolIds {
		school := models.School{}
		if err := h.db.First(&school, id).Error; err != nil {
			fmt.Println("DB Error:", err)
			fmt.Println("School ID:", id)
			fmt.Println("School:", school)
			return c.JSON(http.StatusBadRequest, err)
		}
		schools = append(schools, school)
	}

	counseling.Schools = schools

	if err := h.db.Create(counseling).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to create counseling",
		})
	}
	return c.JSON(http.StatusCreated, map[string]string{
		"message": "Counseling created successfully",
	})
}
