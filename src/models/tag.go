package models

import (
	"time"

	"github.com/go-playground/validator/v10"
)

type Tag struct {
	ID        uint       `gorm:"primary_key" json:"id"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `sql:"index" json:"deletedAt,omitempty"`
	Text      string     `gorm:"size:50;not null" validate:"required" json:"text"`
}

func (s *Tag) Validate() error {
	validate := validator.New()
	return validate.Struct(s)
}
