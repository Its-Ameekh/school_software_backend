// Package handlers contains Gin HTTP handlers for the School Software
// backend. This file implements Eng B's Stage 4 finance domain: fee
// summary, recording payments, waiving fees, and payment reminders.
package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Its-Ameekh/school_software_backend/internal/apierrors"
	"github.com/Its-Ameekh/school_software_backend/internal/middleware"
	"github.com/Its-Ameekh/school_software_backend/internal/models"
	"github.com/Its-Ameekh/school_software_backend/internal/services"
)

// Allowed payment methods for v1. The DB column itself has no CHECK
// constraint (it already anticipates a v2 'online' method per the
// migration comment), so this restriction is enforced here in code.
const (
	PaymentMethodBank = "bank"
	PaymentMethodDesk = "desk"
)

// Ledger status values.
const (
	LedgerPending = "PENDING"
	LedgerPaid    = "PAID"
	LedgerWaived  = "WAIVED"
)

// FinanceHandlers groups this package's finance-domain handlers.
type FinanceHandlers struct {
	db          *gorm.DB
	auditLogger *services.AuditLogger
}

// NewFinanceHandlers constructs a FinanceHandlers.
func NewFinanceHandlers(db *gorm.DB, auditLogger *services.AuditLogger) *FinanceHandlers {
	return &FinanceHandlers{db: db, auditLogger: auditLogger}
}

// FeeSummaryRecord is one ledger entry as returned by the summary
// endpoint.
type FeeSummaryRecord struct {
	ID            uint       `json:"id"`
	StudentID     uint       `json:"student_id"`
	FeeTermID     uint       `json:"fee_term_id"`
	AmountDue     float64    `json:"amount_due"`
	Status        string     `json:"status"`
	PaymentMethod *string    `json:"payment_method,omitempty"`
	WaiveReason   *string    `json:"waive_reason,omitempty"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// Summary godoc
//
//	@Summary View a student's fee ledger summary
//	@Description Returns every student_fee_ledger row for the given student. PARENT callers are restricted to their own linked child.
//	@Tags finance
//	@Security ApiKeyAuth
//	@Produce json
//	@Param student_id query int true "Student to summarize"
//	@Success 200 {array} FeeSummaryRecord
//	@Failure 400 {object} apierrors.ErrorResponse
//	@Failure 403 {object} apierrors.ErrorResponse
//	@Router /api/finance/summary [get]
func (h *FinanceHandlers) Summary(c *gin.Context) {
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Forbidden(c)
		return
	}
	role, _ := middleware.GetUserRole(c)

	studentIDStr := c.Query("student_id")
	if studentIDStr == "" {
		apierrors.BadRequest(c, "student_id is required")
		return
	}
	id, err := strconv.ParseUint(studentIDStr, 10, 64)
	if err != nil {
		apierrors.BadRequest(c, "invalid student_id")
		return
	}
	studentID := uint(id)

	ctx := c.Request.Context()

	if role == "PARENT" {
		var guardian models.Guardian
		result := h.db.WithContext(ctx).
			Where("student_id = ? AND user_id = ?", studentID, actorID).
			First(&guardian)
		if result.Error != nil {
			apierrors.Forbidden(c)
			return
		}
	}

	var ledgerRows []models.StudentFeeLedger
	if err := h.db.WithContext(ctx).
		Where("student_id = ?", studentID).
		Order("created_at DESC").
		Find(&ledgerRows).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	response := make([]FeeSummaryRecord, 0, len(ledgerRows))
	for _, r := range ledgerRows {
		response = append(response, FeeSummaryRecord{
			ID:            r.ID,
			StudentID:     r.StudentID,
			FeeTermID:     r.FeeTermID,
			AmountDue:     r.AmountDue,
			Status:        r.Status,
			PaymentMethod: r.PaymentMethod,
			WaiveReason:   r.WaiveReason,
			PaidAt:        r.PaidAt,
			CreatedAt:     r.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, response)
}

// PaymentRequest is the body for POST /api/finance/payment.
type PaymentRequest struct {
	LedgerID      uint   `json:"ledger_id" binding:"required"`
	PaymentMethod string `json:"payment_method" binding:"required"`
}

// Payment godoc
//
//	@Summary Record a manual payment against a fee ledger entry
//	@Description Marks a student_fee_ledger row PAID. payment_method is restricted to 'bank' or 'desk' for v1. Rejects if the ledger entry is already PAID.
//	@Tags finance
//	@Security ApiKeyAuth
//	@Accept json
//	@Produce json
//	@Param request body PaymentRequest true "Ledger entry and payment method"
//	@Success 200 {object} FeeSummaryRecord
//	@Failure 400 {object} apierrors.ErrorResponse
//	@Failure 409 {object} apierrors.ErrorResponse
//	@Router /api/finance/payment [post]
func (h *FinanceHandlers) Payment(c *gin.Context) {
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Forbidden(c)
		return
	}

	var req PaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}

	if req.PaymentMethod != PaymentMethodBank && req.PaymentMethod != PaymentMethodDesk {
		apierrors.BadRequest(c, "payment_method must be 'bank' or 'desk'")
		return
	}

	ctx := c.Request.Context()
	var record models.StudentFeeLedger

	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ?", req.LedgerID).First(&record)
		if result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return errNotFoundLedger
			}
			return result.Error
		}

		// Reject if this ledger entry is already settled -- a duplicate
		// payment attempt against a PAID row is rejected outright for v1,
		// rather than silently accepted or converted into credit.
		if record.Status == LedgerPaid {
			return errAlreadyPaid
		}

		before := record

		now := time.Now()
		record.Status = LedgerPaid
		record.PaymentMethod = &req.PaymentMethod
		record.PaidAt = &now

		if err := tx.Save(&record).Error; err != nil {
			return err
		}

		if err := h.auditLogger.Log(ctx, actorID, services.AuditUpdate, "student_fee_ledger", record.ID, before, record); err != nil {
			// Audit failure must not fail the payment itself.
			_ = err // TODO: wire to container.Logger once available in this handler
		}

		return nil
	})

	if err != nil {
		switch {
		case errors.Is(err, errNotFoundLedger):
			apierrors.NotFound(c, "fee ledger entry")
		case errors.Is(err, errAlreadyPaid):
			apierrors.Conflict(c, "this fee ledger entry is already paid")
		default:
			apierrors.Internal(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, FeeSummaryRecord{
		ID:            record.ID,
		StudentID:     record.StudentID,
		FeeTermID:     record.FeeTermID,
		AmountDue:     record.AmountDue,
		Status:        record.Status,
		PaymentMethod: record.PaymentMethod,
		WaiveReason:   record.WaiveReason,
		PaidAt:        record.PaidAt,
		CreatedAt:     record.CreatedAt,
	})
}

var (
	errNotFoundLedger = errors.New("fee ledger entry not found")
	errAlreadyPaid    = errors.New("fee ledger entry already paid")
)
