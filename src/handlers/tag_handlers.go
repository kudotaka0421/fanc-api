package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"fanc-api/src/models"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type TagHandler struct {
	db *gorm.DB
}

func NewTagHandler(db *gorm.DB) *TagHandler {
	return &TagHandler{db}
}

func (h *TagHandler) GetTags(c echo.Context) error {
	tags := []models.Tag{}

	if err := h.db.Find(&tags).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to retrieve tags",
		})
	}

	return c.JSON(http.StatusOK, tags)
}

func (h *TagHandler) GetTagByID(c echo.Context) error {
	tagID := c.Param("tag_id")
	tag := new(models.Tag)

	if err := h.db.Select("id, text").Where("id = ?", tagID).First(&tag).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{
				"message": "Tag not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to retrieve tag",
		})
	}
	response := map[string]interface{}{
		"id":   tag.ID,
		"text": tag.Text,
	}

	return c.JSON(http.StatusOK, response)
}

func (h *TagHandler) CreateTag(c echo.Context) error {
	tag := new(models.Tag)
	log.Println("CreateTag handler called.")

	// リクエストのボディのデータをtagにバインドする
	if err := c.Bind(tag); err != nil {
		// エラーステータス500のJSONレスポンスを返す
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Invalid data",
		})
	}

	// バリデーションの実行
	if err := tag.Validate(); err != nil {
		return c.JSON(http.StatusBadRequest, err)
	}

	if err := h.db.Create(tag).Error; err != nil {
		// エラーステータス500のJSONレスポンスを返す
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to create tag",
		})
	}
	return c.JSON(http.StatusCreated, map[string]string{
		"message": "Tag created successfully",
	})
}

func (h *TagHandler) UpdateTag(c echo.Context) error {
	// URLからIDを取得
	id, err := strconv.Atoi(c.Param("tag_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, err)
	}

	tag := new(models.Tag)
	//リクエストボディからデータをバインド
	if err := c.Bind(tag); err != nil {
		return c.JSON(http.StatusBadRequest, err)
	}

	if err := tag.Validate(); err != nil {
		return c.JSON(http.StatusBadRequest, err)
	}

	result := h.db.Model(&models.Tag{}).Where("id = ?", id).Updates(tag)

	if result.Error != nil {
		return c.JSON(http.StatusInternalServerError, result.Error)
	}

	if result.RowsAffected == 0 {
		return c.JSON(http.StatusNotFound, fmt.Errorf("No tag found with ID: %d", id))
	}

	return c.JSON(http.StatusOK, tag)
}

func (h *TagHandler) DeleteTag(c echo.Context) error {
	// URLからIDを取得
	id, err := strconv.Atoi(c.Param("tag_id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
	}

	tag := new(models.Tag)
	result := h.db.Model(&models.Tag{}).Where("id = ?", id).Delete(tag)
	if result.Error != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete tag"})
	}

	if result.RowsAffected == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Tag not found"})
	}
	// 削除が成功したらステータスコード204を返す
	return c.NoContent(http.StatusNoContent)

}
