package models

type Class struct {
	ID                  uint   `gorm:"primaryKey"`
	Name                string `gorm:"not null"`
	TeacherID           *uint
	SubstituteTeacherID *uint
	SubstituteActive    bool `gorm:"default:false"`
}

func (Class) TableName() string { return "classes" }
