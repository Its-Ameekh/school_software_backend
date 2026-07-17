package models

import (
	"time"
	"gorm.io/gorm" // Added
)

type Student struct {
	ID              uint      `gorm:"primaryKey"`
	RollNumber      string    `gorm:"unique;not null"`
	FullName        string    `gorm:"not null"`
	DOB             time.Time `gorm:"not null"`
	Gender          string    `gorm:"not null"`
	BloodGroup      *string
	Allergies       *string
	SpecialTalents  *string
	LanguagesSpoken *string
	FoodType        *string
	ClassID         *uint
	GradeTier       string `gorm:"not null"`
	CreatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"` // Fixed
}

func (Student) TableName() string { return "students" }

type AdmissionIntake struct {
	ID            uint `gorm:"primaryKey"`
	StudentID     uint
	PayMode       string  `gorm:"not null"`
	AmountPaid    float64 `gorm:"not null"`
	ReceiptNumber string  `gorm:"not null"`
	TransportPref string  `gorm:"not null"`
	AdmittedAt    time.Time
}

func (AdmissionIntake) TableName() string { return "admission_intake" }
