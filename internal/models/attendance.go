package models

import "time"

type Attendance struct {
	ID                uint `gorm:"primaryKey"`
	StudentID         uint
	ClassID           *uint
	Date              time.Time `gorm:"not null"`
	Status            string    `gorm:"not null"`
	MarkedBy          *uint
	LockedAt          *time.Time
	LockedByPrincipal bool `gorm:"default:false"`
}

func (Attendance) TableName() string { return "attendance" }
