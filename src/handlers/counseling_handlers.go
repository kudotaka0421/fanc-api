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
