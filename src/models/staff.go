package models

import (
	"github.com/go-playground/validator/v10"
	"github.com/jinzhu/gorm"
)

type Staff struct {
	gorm.Model
	FirstName     string `gorm:"size:255;not null" validate:"required"`
	LastName      string `gorm:"size:255;not null" validate:"required"`
	FirstNameKana string `gorm:"size:255;not null" validate:"required"`
	LastNameKana  string `gorm:"size:255;not null" validate:"required"`
	Email         string `gorm:"size:255;not null;unique" validate:"required,email"`
}

func (s *Staff) Validate() error {
	validate := validator.New()
	return validate.Struct(s)
}
