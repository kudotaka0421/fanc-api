package models

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

type School struct {
	gorm.Model
	IsShow          bool            `json:"isShow"`
	Name            string          `json:"name"`
	MonthlyFee      int             `json:"monthlyFee"`
	TermNum         int             `json:"termNum"`
	TermUnit        int             `json:"termUnit"`
	Remarks         *string         `json:"remarks"`
	Overview        string          `json:"overview"`
	ImageLinks      json.RawMessage `gorm:"type:json" json:"imageLinks"`
	Link            string          `json:"link"`
	Recommendations json.RawMessage `gorm:"type:json" json:"recommendations"`
	Features        json.RawMessage `gorm:"type:json" json:"features"`
	Tags            []Tag           `gorm:"many2many:school_tags;" json:"tags"`
}

func (s *School) Validate() error {
	if s.Name == "" {
		return errors.New("school name is required")
	}
	if s.MonthlyFee <= 0 {
		return errors.New("monthly fee must be greater than 0")
	}
	// [TODO]add more validation if necessary
	return nil
}
