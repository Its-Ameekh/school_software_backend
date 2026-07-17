package models

import (
	"time"
	"gorm.io/gorm" 
)

type User struct {
	ID        uint           `gorm:"primaryKey"`
	AuthID    string         `gorm:"type:uuid;unique;not null"`
	Email     *string        `gorm:"unique"`
	Role      string         `gorm:"not null"` // e.g. "principal", "teacher", "parent"
	Name      string         `gorm:"not null"` // <--- RESTORED THIS LINE
	Phone     string         `gorm:"unique;not null"`
	AvatarURL *string
	CreatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"` 
}

func (User) TableName() string { return "users" }

type TeacherProfile struct {
	UserID         uint `gorm:"primaryKey"`
	Specialization *string
	IsAvailableSub bool `gorm:"default:true"`
}

func (TeacherProfile) TableName() string { return "teacher_profiles" }

type TimetableSlot struct {
	ID           uint   `gorm:"primaryKey"` 
	ClassID      uint   `gorm:"not null;index:idx_class_timetable,unique"`
	DayOfWeek    string `gorm:"not null;index:idx_class_timetable,unique"`
	PeriodNumber int8   `gorm:"not null;index:idx_class_timetable,unique"`
	Subject      string `gorm:"not null"`
	Room         string
	StartTime    string 
	EndTime      string 
}

func (TimetableSlot) TableName() string { return "timetable_slots" }

type LeaveRequest struct {
	ID          uint      `gorm:"primaryKey"`
	StudentID   uint      `gorm:"not null"`
	RequestedBy uint      `gorm:"not null"` // parent user_id
	Date        time.Time `gorm:"not null;type:date"`
	Reason      string    `gorm:"not null"`
	Status      string    `gorm:"not null;default:PENDING"` // PENDING | APPROVED | REJECTED
	ReviewedBy  *uint
	ReviewedAt  *time.Time
	CreatedAt   time.Time
}

func (LeaveRequest) TableName() string { return "leave_requests" }

type TeacherLeaveRequest struct {
	ID         uint      `gorm:"primaryKey"`
	TeacherID  uint      `gorm:"not null"`
	FromDate   time.Time `gorm:"not null;type:date"`
	ToDate     time.Time `gorm:"not null;type:date"`
	LeaveType  string    `gorm:"not null"` // Casual | Sick Medical | Emergency Context | Personal Assignment
	Reason     string    `gorm:"not null"`
	Status     string    `gorm:"not null;default:PENDING"` // PENDING | APPROVED | REJECTED
	ReviewedBy *uint
	ReviewedAt *time.Time
	CreatedAt  time.Time
}

func (TeacherLeaveRequest) TableName() string { return "teacher_leave_requests" }