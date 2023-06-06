package models

type SchoolTag struct {
	SchoolID uint `gorm:"primaryKey"`
	TagID    uint `gorm:"primaryKey"`
}
