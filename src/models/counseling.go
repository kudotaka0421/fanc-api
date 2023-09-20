package models

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

type Counseling struct {
	gorm.Model
	CounseleeName string    `gorm:"not null" json:"counseleeName"`
	Email         string    `gorm:"not null" json:"email"`
	Status        int       `gorm:"not null" json:"status"`
	Date          time.Time `gorm:"not null" json:"date"`
	Remarks       *string   `json:"remarks"`
	Message       *string   `json:"message"`
	UserID        uint      `json:"userId"`
	User          User      `gorm:"foreignKey:UserID" json:"-"`
	Schools       []School  `gorm:"many2many:counseling_schools;" json:"schools"`
}

func (c *Counseling) Validate() error {
	if c.CounseleeName == "" {
		return errors.New("counselee name is required")
	}
	if c.Email == "" {
		return errors.New("email is required")
	}
	if c.Status <= 0 {
		return errors.New("status must be greater than 0")
	}
	if c.UserID == 0 {
		return errors.New("user id is required")
	}
	return nil
}
