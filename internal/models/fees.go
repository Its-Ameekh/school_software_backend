package models

import "time"

type FeeStructure struct {
	ID              uint    `gorm:"primaryKey"`
	AcademicYear    string  `gorm:"not null"`
	GradeTier       string  `gorm:"not null"`
	InitialPayment  float64 `gorm:"not null"`
	RegularFeeTotal float64 `gorm:"not null"`
	CreatedAt       time.Time
}

func (FeeStructure) TableName() string { return "fee_structures" }

type FeeTerm struct {
	ID             uint `gorm:"primaryKey"`
	FeeStructureID uint
	TermNumber     int8      `gorm:"not null"`
	Amount         float64   `gorm:"not null"`
	DueDate        time.Time `gorm:"not null"`
}

func (FeeTerm) TableName() string { return "fee_terms" }

type StudentFeeLedger struct {
	ID            uint `gorm:"primaryKey"`
	StudentID     uint
	FeeTermID     uint
	AmountDue     float64 `gorm:"not null"`
	Status        string  `gorm:"not null;default:PENDING"`
	PaymentMethod *string
	WaiveReason   *string
	PaidAt        *time.Time
	CreatedAt     time.Time
}

func (StudentFeeLedger) TableName() string { return "student_fee_ledger" }
