package services

import (
	"context"
	"fmt"
	"time"

	"github.com/Its-Ameekh/school_software_backend/internal/models"
	"gorm.io/gorm"
)

// FeeLedgerService defines the strict compilation contract for handling financial seeding operations.
type FeeLedgerService interface {
	GenerateTermLedger(ctx context.Context, tx *gorm.DB, studentID uint, gradeTier string) error
}

// RealFeeLedgerService implements the FeeLedgerService contract using a live GORM engine.
type RealFeeLedgerService struct{}

// NewFeeLedgerService creates a new instance of the database-backed ledger service.
func NewFeeLedgerService() *RealFeeLedgerService {
	return &RealFeeLedgerService{}
}

// GenerateTermLedger dynamically pulls current matching term billing parameters
// and seeds pending accounts for the student inside the active database transaction context.
func (s *RealFeeLedgerService) GenerateTermLedger(ctx context.Context, tx *gorm.DB, studentID uint, gradeTier string) error {
	var terms []models.FeeTerm

	// 1. Locate all active fee terms mapped to the student's GradeTier
	err := tx.WithContext(ctx).
		Joins("JOIN fee_structures ON fee_structures.id = fee_terms.fee_structure_id").
		Where("fee_structures.grade_tier = ?", gradeTier).
		Find(&terms).Error

	if err != nil {
		return fmt.Errorf("failed to fetch active fee terms for grade tier %s: %w", gradeTier, err)
	}

	// Safety Check: if the school has not set up a pricing sheet for this grade tier yet,
	// fail the transaction so the database doesn't create a student with broken tracking.
	if len(terms) == 0 {
		return fmt.Errorf("no billing terms found in database for grade tier: %s", gradeTier)
	}

	// 2. Map terms directly into pending rows inside the Student Ledger
	for _, term := range terms {
		ledgerEntry := models.StudentFeeLedger{
			StudentID: studentID,
			FeeTermID: term.ID,
			AmountDue: term.Amount,
			Status:    "PENDING",
			CreatedAt: time.Now(),
		}

		if err := tx.WithContext(ctx).Create(&ledgerEntry).Error; err != nil {
			return fmt.Errorf("failed to write ledger entry for term ID %d: %w", term.ID, err)
		}
	}

	return nil
}
