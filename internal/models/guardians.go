package models

import (
	"gorm.io/gorm" // Added
)
type Guardian struct {
	ID                  uint `gorm:"primaryKey"`
	StudentID           uint
	UserID              *uint
	FullName            string `gorm:"not null"`
	Relationship        string `gorm:"not null"`
	Occupation          *string
	Email               *string
	Mobile              *string
	IsPrimaryContact    bool `gorm:"default:false"`
	AuthorizedForPickup bool `gorm:"default:true"`
	DeletedAt           gorm.DeletedAt `gorm:"index"` // Fixed
}

func (Guardian) TableName() string { return "guardians" }
