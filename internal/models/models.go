package models

import (
	"time"
	"gorm.io/gorm" // Added
)
type User struct {
	ID        uint    `gorm:"primaryKey"`
	AuthID    string  `gorm:"type:uuid;unique;not null"`
	Email     *string `gorm:"unique"`
	Role      string  `gorm:"not null"` //e.g. "principal", "teacher", "parent"
	Phone     string  `gorm:"unique;not null"`
	AvatarURL *string
	CreatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"` // Fixed
}

func (User) TableName() string { return "users" }

type TeacherProfile struct {
	UserID         uint `gorm:"primaryKey"`
	Specialization *string
	IsAvailableSub bool `gorm:"default:true"`
}

func (TeacherProfile) TableName() string { return "teacher_profiles" }
