package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"fanc-api/src/models"

	"github.com/golang-jwt/jwt"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthHandler struct {
	db *gorm.DB
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

func NewAuthHandler(db *gorm.DB) *AuthHandler {
	return &AuthHandler{db}
}

type MeResponse struct {
	IsAuthenticated bool    `json:"isAuthenticated"`
	Name            *string `json:"name"`
	Email           *string `json:"email"`
	Role            *int    `json:"role"`
}

func (h *AuthHandler) Login(c echo.Context) error {
	request := new(LoginRequest)
	if err := c.Bind(request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"message": "Invalid data",
		})
	}

	user := new(models.User)
	if err := h.db.Where("email = ?", request.Email).First(user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{
				"message": "User not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to retrieve user",
		})
	}

	// Compare the passwords
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(request.Password)); err != nil {
		// Password does not match
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"message": "Invalid password",
		})
	}

	// User is authenticated, so generate a JWT
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &jwt.StandardClaims{
		Subject:   strconv.FormatUint(uint64(user.ID), 10), // convert user.ID to a string and set it as the subject
		ExpiresAt: expirationTime.Unix(),
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := jwtToken.SignedString(jwtKey)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Failed to create JWT",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Login successful.",
		"token":   tokenString,
	})
}

func (h *AuthHandler) GetMe(c echo.Context) error {
	// Parse the user ID from the subject
	token := c.Get("user").(*jwt.Token)
	claims := token.Claims.(jwt.MapClaims)
	userId, _ := strconv.Atoi(claims["sub"].(string))

	user := new(models.User)
	if err := h.db.First(user, userId).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"message": "User not found",
		})
	}

	return c.JSON(http.StatusOK, MeResponse{
		IsAuthenticated: true,
		Name:            &user.Name,
		Email:           &user.Email,
		Role:            &user.Role,
	})
}
