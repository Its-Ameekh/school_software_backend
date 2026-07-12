package models

import "time"

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
	DeletedAt           *time.Time
}

func (Guardian) TableName() string { return "guardians" }
