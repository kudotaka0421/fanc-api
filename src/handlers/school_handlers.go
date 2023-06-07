package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"fanc-api/src/models"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm" // Replace the old gorm import
)

type SchoolHandler struct {
	db *gorm.DB
}

// フロントから送られてくるparamsのstruct
type SchoolParams struct {
	ID              uint     `json:"id"`
	IsShow          bool     `json:"isShow"`
	Name            string   `json:"name"`
	MonthlyFee      int      `json:"monthlyFee"`
	TermNum         int      `json:"termNum"`
	TermUnit        int      `json:"termUnit"`
	Remarks         *string  `json:"remarks"`
	Overview        string   `json:"overview"`
	ImageLinks      []string `json:"imageLinks"`
	Link            string   `json:"link"`
	Recommendations []string `json:"recommendations"`
	Features        []string `json:"features"`
	SelectedTagIds  []uint   `json:"selectedTagIds"`
}

type TagResponse struct {
	ID   uint   `json:"id"`
	Text string `json:"text"`
}

func NewSchoolHandler(db *gorm.DB) *SchoolHandler {
	return &SchoolHandler{db}
}

func (h *SchoolHandler) CreateSchool(c echo.Context) error {
	schoolParams := new(SchoolParams)

	// Binding all parameters in one call
	if err := c.Bind(schoolParams); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Invalid data",
		})
	}

	// string[]はそのままDBに保存できないので、json.RawMessageに変換する
	var imageLinksJson, recommendationsJson, featuresJson json.RawMessage // Change string to json.RawMessage

	if tmpJson, err := json.Marshal(schoolParams.ImageLinks); err != nil {
		fmt.Println("Failed to marshal ImageLinks:", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to marshal ImageLinks",
		})
	} else {
		imageLinksJson = tmpJson
	}

	if tmpJson, err := json.Marshal(schoolParams.Recommendations); err != nil {
		fmt.Println("Failed to marshal Recommendations:", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to marshal Recommendations",
		})
	} else {
		recommendationsJson = tmpJson
	}

	if tmpJson, err := json.Marshal(schoolParams.Features); err != nil {
		fmt.Println("Failed to marshal Features:", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to marshal Features",
		})
	} else {
		featuresJson = tmpJson
	}

	school := &models.School{
		Model:           gorm.Model{ID: schoolParams.ID},
		IsShow:          schoolParams.IsShow,
		Name:            schoolParams.Name,
		MonthlyFee:      schoolParams.MonthlyFee,
		TermNum:         schoolParams.TermNum,
		TermUnit:        schoolParams.TermUnit,
		Remarks:         schoolParams.Remarks,
		Overview:        schoolParams.Overview,
		ImageLinks:      imageLinksJson,
		Link:            schoolParams.Link,
		Recommendations: recommendationsJson,
		Features:        featuresJson,
	}

	// バリデーションの実行
	if err := school.Validate(); err != nil {
		fmt.Println("err2", err)
		return c.JSON(http.StatusBadRequest, err)
	}

	tags := []models.Tag{}
	for _, id := range schoolParams.SelectedTagIds {
		tag := models.Tag{}
		if err := h.db.First(&tag, id).Error; err != nil {
			fmt.Println("DB Error:", err)
			fmt.Println("Tag ID:", id)
			fmt.Println("Tag:", tag)
			return c.JSON(http.StatusBadRequest, err)
		}
		tags = append(tags, tag)
	}

	school.Tags = tags

	if err := h.db.Create(school).Error; err != nil {
		// エラーステータス500のJSONレスポンスを返す
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to create school",
		})
	}
	return c.JSON(http.StatusCreated, map[string]string{
		"message": "School created successfully",
	})
}

func (h *SchoolHandler) GetSchools(c echo.Context) error {
	schools := []models.School{}

	if err := h.db.Preload("Tags").Find(&schools).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to retrieve schools",
		})
	}
	schoolsResponse := make([]map[string]interface{}, len(schools))
	for i, school := range schools {
		// Unmarshal each JSON string field into []string
		var imageLinks, recommendations, features []string

		if err := json.Unmarshal(school.ImageLinks, &imageLinks); err != nil {
			fmt.Println("Failed to unmarshal ImageLinks:", err)
		}

		if err := json.Unmarshal(school.Recommendations, &recommendations); err != nil {
			fmt.Println("Failed to unmarshal Recommendations:", err)
		}

		if err := json.Unmarshal(school.Features, &features); err != nil {
			fmt.Println("Failed to unmarshal Features:", err)
		}

		// Convert school.Tags into []TagResponse
		tagResponses := make([]TagResponse, len(school.Tags))
		for i, tag := range school.Tags {
			tagResponses[i] = TagResponse{
				ID:   tag.ID,
				Text: tag.Text,
			}
		}

		schoolsResponse[i] = map[string]interface{}{
			"id":              school.ID,
			"isShow":          school.IsShow,
			"name":            school.Name,
			"monthlyFee":      school.MonthlyFee,
			"termNum":         school.TermNum,
			"termUnit":        school.TermUnit,
			"remarks":         school.Remarks,
			"overview":        school.Overview,
			"imageLinks":      imageLinks,
			"link":            school.Link,
			"recommendations": recommendations,
			"features":        features,
			"tags":            tagResponses, // use TagResponse instead of original Tag
		}
	}

	return c.JSON(http.StatusOK, schoolsResponse)
}

func (h *SchoolHandler) GetSchoolByID(c echo.Context) error {
	fmt.Println("GetSchoolByID")
	schoolID := c.Param("school_id")
	school := new(models.School)

	if err := h.db.Preload("Tags").Where("id = ?", schoolID).First(&school).Error; err != nil {
		fmt.Println("DB Error:", err)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fmt.Println("School not found")
			return c.JSON(http.StatusNotFound, map[string]string{
				"message": "School not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to retrieve school",
		})
	}
	// Unmarshal each JSON string field into []string
	var imageLinks, recommendations, features []string

	if err := json.Unmarshal(school.ImageLinks, &imageLinks); err != nil {
		fmt.Println("Failed to unmarshal ImageLinks:", err)
	}

	if err := json.Unmarshal(school.Recommendations, &recommendations); err != nil {
		fmt.Println("Failed to unmarshal Recommendations:", err)
	}

	if err := json.Unmarshal(school.Features, &features); err != nil {
		fmt.Println("Failed to unmarshal Features:", err)
	}

	response := map[string]interface{}{
		"id":              school.ID,
		"isShow":          school.IsShow,
		"name":            school.Name,
		"monthlyFee":      school.MonthlyFee,
		"termNum":         school.TermNum,
		"termUnit":        school.TermUnit,
		"remarks":         school.Remarks,
		"overview":        school.Overview,
		"imageLinks":      imageLinks,
		"link":            school.Link,
		"recommendations": recommendations,
		"features":        features,
		"tags":            school.Tags,
	}

	return c.JSON(http.StatusOK, response)
}
