package handlers

import (
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"fanc-api/src/models"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm" // Replace the old gorm import
)

type UserHandler struct {
	db *gorm.DB
}

type UserResponse struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Role  int    `json:"role"`
	Email string `json:"email"`
}

func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{db}
}

func (h *UserHandler) GetUsers(c echo.Context) error {
	var users []models.User
	if err := h.db.Find(&users).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to retrieve users",
		})
	}

	userResponses := make([]UserResponse, len(users))
	for i, user := range users {
		userResponses[i] = UserResponse{
			ID:    user.ID,
			Name:  user.Name,
			Role:  user.Role,
			Email: user.Email,
		}
	}

	return c.JSON(http.StatusOK, userResponses)
}

func (h *UserHandler) CreateUser(c echo.Context) error {
	user := new(models.User)

	// リクエストのボディのデータをuserにバインドする
	if err := c.Bind(user); err != nil {
		// エラーステータス500のJSONレスポンスを返す
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Invalid data",
		})
	}

	// パスワードをハッシュ化する
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to hash password",
		})
	}
	// ハッシュ化したパスワードをセットする
	user.Password = string(hashedPassword)

	// バリデーションの実行
	if err := user.Validate(); err != nil {
		return c.JSON(http.StatusBadRequest, err)
	}

	if err := h.db.Create(user).Error; err != nil {
		// エラーステータス500のJSONレスポンスを返す
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to create user",
		})
	}
	return c.JSON(http.StatusCreated, map[string]string{
		"message": "User created successfully",
	})
}
