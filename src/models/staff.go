package models

import (
	"time"

	"github.com/go-playground/validator/v10"
)

type Staff struct {
	ID            uint       `gorm:"primary_key" json:"id"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	DeletedAt     *time.Time `sql:"index" json:"deletedAt,omitempty"`
	FirstName     string     `gorm:"size:255;not null" validate:"required" json:"firstName"`
	LastName      string     `gorm:"size:255;not null" validate:"required" json:"lastName"`
	FirstNameKana string     `gorm:"size:255;not null" validate:"required" json:"firstNameKana"`
	LastNameKana  string     `gorm:"size:255;not null" validate:"required" json:"lastNameKana"`
	Email         string     `gorm:"size:255;not null;unique" validate:"required,email" json:"email"`
}

func (s *Staff) Validate() error {
	validate := validator.New()
	return validate.Struct(s)
}
