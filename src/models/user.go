package models

import (
	"time"

	"github.com/go-playground/validator/v10"
)

type User struct {
	ID          uint         `gorm:"primary_key" json:"id"`
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
	DeletedAt   *time.Time   `sql:"index" json:"deletedAt,omitempty"`
	Name        string       `gorm:"size:255;not null" validate:"required" json:"Name"`
	Password    string       `gorm:"size:255;not null" validate:"required" json:"Password"`
	Role        int          `gorm:"not null" validate:"required" json:"Role"`
	Email       string       `gorm:"size:255;not null;unique" validate:"required,email" json:"email"`
	IsActive    bool         `gorm:"not null;default:false" json:"isActive"`
	Token       string       `gorm:"size:255;not null;unique" json:"token"`
	Counselings []Counseling `gorm:"foreignKey:UserID" json:"counselings"`
}

func (u *User) Validate() error {
	validate := validator.New()
	return validate.Struct(u)
}
