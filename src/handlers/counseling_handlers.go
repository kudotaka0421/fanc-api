package handlers

import (
	"bytes"
	"encoding/json"
	"fanc-api/src/models"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
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
	ID            uint      `json:"id"`
	CounseleeName string    `json:"counseleeName"`
	Email         string    `json:"email"`
	Status        int       `json:"status"`
	Date          time.Time `json:"date"`
	Remarks       *string   `json:"remarks"`
	Message       *string   `json:"message"`
	UserID        uint      `json:"userId"` // Changed from *uint to uint
	SchoolIds     *[]uint   `json:"schoolIds"`
}

const (
	readyStatus     = 1
	completedStatus = 2
	canceledStatus  = 3
)

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

	if counseling.Status == completedStatus {
		if err := h.sendToSlack(counseling); err != nil {
			fmt.Println("Failed to send message to Slack:", err)
		}
	}

	return c.JSON(http.StatusCreated, map[string]string{
		"message": "Counseling created successfully",
	})
}

func (h *CounselingHandler) UpdateCounseling(c echo.Context) error {
	counselingId, err := strconv.Atoi(c.Param("counseling_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Invalid counseling ID",
		})
	}

	params := new(CounselingParams)
	if err := c.Bind(params); err != nil {
		fmt.Println("Bind Error:", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Invalid data",
		})
	}

	// Fetch the current counseling record from the database before updating.
	existingCounseling := models.Counseling{}
	if err := h.db.First(&existingCounseling, counselingId).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"message": fmt.Sprintf("No counseling found with ID: %d", counselingId),
		})
	}

	counseling := &models.Counseling{
		Model:         gorm.Model{ID: params.ID},
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

	// Clear existing associations and add new ones
	if err := h.db.Model(counseling).Association("Schools").Clear(); err != nil {
		return c.JSON(http.StatusInternalServerError, err)
	}
	h.db.Model(counseling).Association("Schools").Replace(schools)

	result := h.db.Model(&models.Counseling{}).Where("id = ?", counselingId).Updates(counseling)
	if result.Error != nil {
		return c.JSON(http.StatusInternalServerError, result.Error)
	}

	if (existingCounseling.Status == readyStatus || existingCounseling.Status == canceledStatus) && counseling.Status == completedStatus {
		if err := h.sendToSlack(counseling); err != nil {
			fmt.Println("Failed to send message to Slack:", err)
		}
	}

	return c.JSON(http.StatusOK, counseling)
}
func (h *CounselingHandler) DeleteCounseling(c echo.Context) error {
	counselingId, err := strconv.Atoi(c.Param("counseling_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Invalid counseling ID",
		})
	}

	counseling := new(models.Counseling)
	if err := h.db.First(counseling, counselingId).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Counseling not found"})
	}

	// Delete associated CounselingSchools first
	if err := h.db.Model(counseling).Association("Schools").Clear(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete associated CounselingSchools"})
	}

	// Then delete the Counseling
	result := h.db.Delete(counseling)
	if result.Error != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete counseling"})
	}

	// 削除が成功したらステータスコード204を返す
	return c.NoContent(http.StatusNoContent)

}

func (h *CounselingHandler) sendToSlack(counseling *models.Counseling) error {

	if err := h.db.Preload("User").First(counseling, counseling.ID).Error; err != nil {
		return err
	}

	// Get the names of schools associated with the counseling
	schoolNames := make([]string, len(counseling.Schools))
	for i, school := range counseling.Schools {
		schoolNames[i] = school.Name
	}

	var jst = time.FixedZone("Asia/Tokyo", 9*60*60)
	jstDate := counseling.Date.In(jst) // Convert to JST
	formattedJSTDate := jstDate.Format("2006-01-02 15:04:05")

	message := fmt.Sprintf(
		"カウンセリングが完了しました\n【相談者名】: %s\n【Email】: %s\n【担当者】: %s\n【日時】: %s\n【提案したスクール】: %s\n【メッセージ】: \n%s\n【備考】: \n%s",
		counseling.CounseleeName,
		counseling.Email,
		counseling.User.Name,
		formattedJSTDate,
		strings.Join(schoolNames, ", "),
		*counseling.Message,
		*counseling.Remarks,
	)

	// [TODO] SlackのIncoming Webhooks URLを環境変数から取得するようにする
	slackURL := "https://hooks.slack.com/services/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"
	payload := map[string]interface{}{
		"text": message,
	}
	jsonPayload, _ := json.Marshal(payload)

	resp, err := http.Post(slackURL, "application/json", bytes.NewReader(jsonPayload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("failed to send to Slack: %v", string(body))
	}

	return nil
}
