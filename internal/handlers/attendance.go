// Package handlers contains Gin HTTP handlers for the School Software
// backend. This file implements Eng B's Stage 4 attendance domain:
// register grid marking, the submit/lock action, the 4:00 PM cutoff,
// and queuing SMS notifications on initial absence / present-reversal.
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

// Allowed attendance status values. Nothing else is accepted for v1.
const (
	StatusPresent = "PRESENT"
	StatusAbsent  = "ABSENT"
)

// attendanceCutoffHour is the hour (24h, local server time) after which
// non-Principal edits to TODAY's attendance are rejected.
const attendanceCutoffHour = 16 // 4:00 PM

// AttendanceHandlers groups this package's attendance-domain handlers.
// Built once at startup and wired into the router, same pattern as
// AuthHandlers.
type AttendanceHandlers struct {
	db          *gorm.DB
	auditLogger *services.AuditLogger
}

// NewAttendanceHandlers constructs an AttendanceHandlers.
func NewAttendanceHandlers(db *gorm.DB, auditLogger *services.AuditLogger) *AttendanceHandlers {
	return &AttendanceHandlers{db: db, auditLogger: auditLogger}
}

// MarkEntry is one student's attendance mark within a bulk-mark request.
type MarkEntry struct {
	StudentID uint   `json:"student_id" binding:"required"`
	Status    string `json:"status" binding:"required"`
}

// MarkRequest is the body for POST /api/attendance/mark -- one whole
// class's attendance grid for a single date, submitted in one call.
type MarkRequest struct {
	ClassID uint        `json:"class_id" binding:"required"`
	Date    string      `json:"date" binding:"required"` // "YYYY-MM-DD"
	Entries []MarkEntry `json:"entries" binding:"required,min=1"`
}

// MarkResponse reports how many rows were written and lists any
// per-entry failures without aborting the rest of the batch.
type MarkResponse struct {
	Saved  int      `json:"saved"`
	Failed []string `json:"failed,omitempty"`
}

// Mark godoc
//
//	@Summary Mark attendance for a class on a given date
//	@Description Bulk-upserts attendance for every student entry supplied. Rejects edits to today's attendance after 4:00 PM unless the caller is PRINCIPAL. Queues an SMS notification row on a student's first absence mark for the day, and again if an existing absence is edited back to present.
//	@Tags attendance
//	@Security ApiKeyAuth
//	@Accept json
//	@Produce json
//	@Param request body MarkRequest true "Class, date, and per-student marks"
//	@Success 200 {object} MarkResponse
//	@Failure 400 {object} apierrors.ErrorResponse
//	@Failure 403 {object} apierrors.ErrorResponse
//	@Router /api/attendance/mark [post]
func (h *AttendanceHandlers) Mark(c *gin.Context) {
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Forbidden(c)
		return
	}
	role, _ := middleware.GetUserRole(c)

	var req MarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}

	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		apierrors.BadRequest(c, "date must be in YYYY-MM-DD format")
		return
	}

	// Cutoff check: only applies to TODAY's date. Past dates are covered
	// by the per-row LockedAt check inside markOne. Future dates aren't
	// meaningful for attendance and are rejected outright.
	today := time.Now().Truncate(24 * time.Hour)
	requestedDay := date.Truncate(24 * time.Hour)

	if requestedDay.After(today) {
		apierrors.BadRequest(c, "cannot mark attendance for a future date")
		return
	}

	if requestedDay.Equal(today) && role != "PRINCIPAL" && time.Now().Hour() >= attendanceCutoffHour {
		apierrors.Forbidden(c)
		return
	}

	response := MarkResponse{}

	for _, entry := range req.Entries {
		if entry.Status != StatusPresent && entry.Status != StatusAbsent {
			response.Failed = append(response.Failed, "student "+strconv.FormatUint(uint64(entry.StudentID), 10)+": invalid status")
			continue
		}

		if err := h.markOne(c, actorID, role, entry.StudentID, req.ClassID, date, entry.Status); err != nil {
			response.Failed = append(response.Failed, "student "+strconv.FormatUint(uint64(entry.StudentID), 10)+": "+err.Error())
			continue
		}
		response.Saved++
	}

	c.JSON(http.StatusOK, response)
}

// markOne handles a single student's attendance upsert: the per-row
// lock check, the actual write, the audit log entry, and any resulting
// attendance_notifications row. Wrapped in its own transaction so one
// bad entry in a batch can't corrupt another's write.
func (h *AttendanceHandlers) markOne(c *gin.Context, actorID uint, role string, studentID, classID uint, date time.Time, newStatus string) error {
	ctx := c.Request.Context()

	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.Attendance
		result := tx.Where("student_id = ? AND date = ?", studentID, date).First(&existing)

		found := true
		if result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				found = false
			} else {
				return result.Error
			}
		}

		// Per-row lock: if this specific row is already locked and the
		// caller isn't Principal, reject -- this covers a day that's
		// already past its own cutoff, independent of the batch-level
		// cutoff check already done in Mark().
		if found && existing.LockedAt != nil && role != "PRINCIPAL" {
			return errors.New("attendance already locked")
		}

		var before *models.Attendance
		wasAbsent := false
		if found {
			b := existing
			before = &b
			wasAbsent = existing.Status == StatusAbsent
		}

		record := existing
		record.StudentID = studentID
		record.ClassID = &classID
		record.Date = date
		record.Status = newStatus
		record.MarkedBy = &actorID

		// If a Principal is editing a row that was already locked, this is
		// an override -- refresh the lock timestamp and flag it as a
		// principal-driven lock, rather than leaving the original
		// submission-time lock in place.
		if role == "PRINCIPAL" && found && existing.LockedAt != nil {
			now := time.Now()
			record.LockedAt = &now
			record.LockedByPrincipal = true
		}

		if err := tx.Save(&record).Error; err != nil {
			return err
		}

		// Audit log -- failure here must not roll back the actual
		// attendance write, per Eng A's rule. Since we're inside a DB
		// transaction, log the audit failure and continue; don't
		// return an error from it.
		action := services.AuditCreate
		if found {
			action = services.AuditUpdate
		}
		if err := h.auditLogger.Log(ctx, actorID, action, "attendance", record.ID, before, record); err != nil {
			// Intentionally not returned -- audit failures must not
			// fail the parent request/transaction.
			_ = err // TODO: wire to container.Logger once available in this handler
		}

		// Notification logic: initial absence, or an edit that reverses
		// an existing absence back to present.
		var triggerReason string
		switch {
		case newStatus == StatusAbsent && (!found || !wasAbsent):
			triggerReason = models.TriggerInitialAbsent
		case newStatus == StatusPresent && found && wasAbsent:
			triggerReason = models.TriggerEditToPresent
		}

		if triggerReason != "" {
			if err := h.queueNotification(tx, record.ID, studentID, triggerReason); err != nil {
				return err
			}
		}

		return nil
	})
}

// queueNotification finds the student's primary-contact guardian and
// writes a PENDING attendance_notifications row for them. If no primary
// contact with a linked login exists, it silently skips -- there's no
// one to notify, and that's not a failure of the attendance write
// itself.
func (h *AttendanceHandlers) queueNotification(tx *gorm.DB, attendanceID, studentID uint, triggerReason string) error {
	var guardian models.Guardian
	result := tx.Where("student_id = ? AND is_primary_contact = true AND user_id IS NOT NULL", studentID).
		First(&guardian)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil // no notifiable primary contact -- not an error
		}
		return result.Error
	}

	notification := models.AttendanceNotification{
		AttendanceID:   attendanceID,
		GuardianUserID: *guardian.UserID,
		TriggerReason:  triggerReason,
		Status:         models.NotificationPending,
	}

	return tx.Create(&notification).Error
}

// SubmitRequest is the body for POST /api/attendance/submit.
type SubmitRequest struct {
	ClassID uint   `json:"class_id" binding:"required"`
	Date    string `json:"date" binding:"required"` // "YYYY-MM-DD"
}

// SubmitResponse reports how many attendance rows got locked by this
// submission.
type SubmitResponse struct {
	Message     string `json:"message"`
	LockedCount int64  `json:"locked_count"`
}

// Submit godoc
//
//	@Summary Submit and lock a class's attendance for a date
//	@Description Records that attendance for the given class/date has been submitted, and locks every attendance row for that class/date (sets locked_at) so further edits require PRINCIPAL. Rejects if this class/date was already submitted.
//	@Tags attendance
//	@Security ApiKeyAuth
//	@Accept json
//	@Produce json
//	@Param request body SubmitRequest true "Class and date to submit"
//	@Success 200 {object} SubmitResponse
//	@Failure 400 {object} apierrors.ErrorResponse
//	@Failure 409 {object} apierrors.ErrorResponse
//	@Router /api/attendance/submit [post]
func (h *AttendanceHandlers) Submit(c *gin.Context) {
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Forbidden(c)
		return
	}

	var req SubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}

	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		apierrors.BadRequest(c, "date must be in YYYY-MM-DD format")
		return
	}

	ctx := c.Request.Context()

	// Reject if this class/date was already submitted -- submission is a
	// one-time action per class per day, not something to silently repeat.
	var existing models.AttendanceSubmission
	result := h.db.WithContext(ctx).
		Where("class_id = ? AND date = ?", req.ClassID, date).
		First(&existing)
	if result.Error == nil {
		apierrors.Conflict(c, "attendance already submitted for this class and date")
		return
	}
	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		apierrors.Internal(c, result.Error)
		return
	}

	var lockedCount int64

	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		submission := models.AttendanceSubmission{
			ClassID:     req.ClassID,
			Date:        date,
			SubmittedBy: actorID,
			SubmittedAt: time.Now(),
		}
		if err := tx.Create(&submission).Error; err != nil {
			return err
		}

		now := time.Now()
		lockResult := tx.Model(&models.Attendance{}).
			Where("class_id = ? AND date = ? AND locked_at IS NULL", req.ClassID, date).
			Updates(map[string]any{"locked_at": now})
		if lockResult.Error != nil {
			return lockResult.Error
		}
		lockedCount = lockResult.RowsAffected

		if err := h.auditLogger.Log(ctx, actorID, services.AuditCreate, "attendance_submission", req.ClassID, nil, submission); err != nil {
			// Audit failure must not fail the submission itself.
			_ = err // TODO: wire to container.Logger once available in this handler
		}

		return nil
	})

	if err != nil {
		apierrors.Internal(c, err)
		return
	}

	c.JSON(http.StatusOK, SubmitResponse{
		Message:     "attendance submitted and locked",
		LockedCount: lockedCount,
	})
}

// HistoryRecord is one attendance row as returned by the history
// endpoint.
type HistoryRecord struct {
	ID                uint       `json:"id"`
	StudentID         uint       `json:"student_id"`
	ClassID           *uint      `json:"class_id,omitempty"`
	Date              time.Time  `json:"date"`
	Status            string     `json:"status"`
	MarkedBy          *uint      `json:"marked_by,omitempty"`
	LockedAt          *time.Time `json:"locked_at,omitempty"`
	LockedByPrincipal bool       `json:"locked_by_principal"`
}

// History godoc
//
//	@Summary View attendance history
//	@Description Filters attendance records by student_id and/or class_id, optionally bounded by a from/to date range. PARENT callers must supply student_id and are restricted to their own linked child; class_id-only queries are rejected for PARENT.
//	@Tags attendance
//	@Security ApiKeyAuth
//	@Produce json
//	@Param student_id query int false "Filter by student"
//	@Param class_id query int false "Filter by class"
//	@Param from query string false "Start date, YYYY-MM-DD"
//	@Param to query string false "End date, YYYY-MM-DD"
//	@Success 200 {array} HistoryRecord
//	@Failure 400 {object} apierrors.ErrorResponse
//	@Failure 403 {object} apierrors.ErrorResponse
//	@Router /api/attendance/history [get]
func (h *AttendanceHandlers) History(c *gin.Context) {
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Forbidden(c)
		return
	}
	role, _ := middleware.GetUserRole(c)

	studentIDStr := c.Query("student_id")
	classIDStr := c.Query("class_id")

	if studentIDStr == "" && classIDStr == "" {
		apierrors.BadRequest(c, "must supply student_id or class_id")
		return
	}

	ctx := c.Request.Context()
	query := h.db.WithContext(ctx).Model(&models.Attendance{})

	var studentID uint
	if studentIDStr != "" {
		id, err := strconv.ParseUint(studentIDStr, 10, 64)
		if err != nil {
			apierrors.BadRequest(c, "invalid student_id")
			return
		}
		studentID = uint(id)
	}

	// PARENT restriction: must supply student_id, and it must be their
	// own linked child. class_id-only queries would let a parent browse
	// an entire class's attendance, so that's rejected outright.
	if role == "PARENT" {
		if studentID == 0 {
			apierrors.Forbidden(c)
			return
		}

		var guardian models.Guardian
		result := h.db.WithContext(ctx).
			Where("student_id = ? AND user_id = ?", studentID, actorID).
			First(&guardian)
		if result.Error != nil {
			apierrors.Forbidden(c)
			return
		}
	}

	if studentID != 0 {
		query = query.Where("student_id = ?", studentID)
	}
	if classIDStr != "" {
		classID, err := strconv.ParseUint(classIDStr, 10, 64)
		if err != nil {
			apierrors.BadRequest(c, "invalid class_id")
			return
		}
		query = query.Where("class_id = ?", classID)
	}

	if fromStr := c.Query("from"); fromStr != "" {
		from, err := time.Parse("2006-01-02", fromStr)
		if err != nil {
			apierrors.BadRequest(c, "from must be in YYYY-MM-DD format")
			return
		}
		query = query.Where("date >= ?", from)
	}
	if toStr := c.Query("to"); toStr != "" {
		to, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			apierrors.BadRequest(c, "to must be in YYYY-MM-DD format")
			return
		}
		query = query.Where("date <= ?", to)
	}

	var records []models.Attendance
	if err := query.Order("date DESC").Find(&records).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	response := make([]HistoryRecord, 0, len(records))
	for _, r := range records {
		response = append(response, HistoryRecord{
			ID:                r.ID,
			StudentID:         r.StudentID,
			ClassID:           r.ClassID,
			Date:              r.Date,
			Status:            r.Status,
			MarkedBy:          r.MarkedBy,
			LockedAt:          r.LockedAt,
			LockedByPrincipal: r.LockedByPrincipal,
		})
	}

	c.JSON(http.StatusOK, response)
}
