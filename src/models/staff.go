package models

import "github.com/jinzhu/gorm"

type Staff struct {
	gorm.Model
	FirstName     string `gorm:"size:255;not null"`
	LastName      string `gorm:"size:255;not null"`
	FirstNameKana string `gorm:"size:255;not null"`
	LastNameKana  string `gorm:"size:255;not null"`
	Mail          string `gorm:"size:255;not null;unique"`
}
