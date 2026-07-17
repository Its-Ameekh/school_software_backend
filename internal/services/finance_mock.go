package services

import (
	"context"
	"gorm.io/gorm"
)

// FeeLedgerService satisfies the parallel compilation contract for Engineer A's track
// until Engineer B lands the fully operational Finance track implementation.
type FeeLedgerService interface {
	GenerateTermLedger(ctx context.Context, tx *gorm.DB, studentID uint, gradeTier string) error
}