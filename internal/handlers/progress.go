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

// ProgressHandlers groups this package's progress-domain handlers.
type ProgressHandlers struct {
	db          *gorm.DB
	auditLogger *services.AuditLogger
}

// NewProgressHandlers constructs a ProgressHandlers.
func NewProgressHandlers(db *gorm.DB, auditLogger *services.AuditLogger) *ProgressHandlers {
	return &ProgressHandlers{db: db, auditLogger: auditLogger}
}

// EnterProgressRequest defines the input payload for student grading and feedback evaluation.
type EnterProgressRequest struct {
	StudentID   uint    `json:"student_id" binding:"required"`
	Term        string  `json:"term" binding:"required"`
	Subject     string  `json:"subject" binding:"required"`
	MaxScore    float64 `json:"max_score" binding:"required,gt=0"`
	ScoredValue float64 `json:"scored_value" binding:"required,gte=0"`
	GradeValue  string  `json:"grade_value" binding:"required"`
	Remark      string  `json:"remark" binding:"required"`
}

// ProgressViewResponse holds the academic scores and textual remarks returned to viewers.
type ProgressViewResponse struct {
	StudentID uint                    `json:"student_id"`
	Scores    []models.ProgressScore  `json:"scores"`
	Remarks   []models.ProgressRemark `json:"remarks"`
}

// EnterEvaluation handles adding or updating progress scores and remarks using GORM transactions.
//
// @Summary Enter academic evaluations and remarks
// @Description Upserts student progress scores and global remarks within an isolated database transaction.
// @Tags progress
// @Accept json
// @Produce json
// @Param request body EnterProgressRequest true "Academic evaluation details"
// @Success 200 {object} map[string]string "Saved successfully confirmation statement"
// @Failure 400 {object} apierrors.ErrorResponse
// @Failure 403 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/progress/evaluation [post]
func (h *ProgressHandlers) EnterEvaluation(c *gin.Context) {
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Forbidden(c)
		return
	}

	var req EnterProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}

	// Double-check validation rules to make sure scored value doesn't exceed maximum bounds
	if req.ScoredValue > req.MaxScore {
		apierrors.BadRequest(c, "scored_value cannot be greater than max_score")
		return
	}

	ctx := c.Request.Context()

	// Execute inside an isolated transaction blocks
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// --- 1. UPSERT PROGRESS SCORE ---
		var scoreRecord models.ProgressScore
		scoreErr := tx.Where("student_id = ? AND term = ? AND subject = ?", req.StudentID, req.Term, req.Subject).First(&scoreRecord).Error

		beforeScore := scoreRecord
		scoreRecord.StudentID = req.StudentID
		scoreRecord.Term = req.Term
		scoreRecord.Subject = req.Subject
		scoreRecord.MaxScore = req.MaxScore
		scoreRecord.ScoredValue = req.ScoredValue
		scoreRecord.GradeValue = req.GradeValue
		scoreRecord.GradedBy = actorID
		scoreRecord.UpdatedAt = time.Now()

		if errors.Is(scoreErr, gorm.ErrRecordNotFound) {
			if err := tx.Create(&scoreRecord).Error; err != nil {
				return err
			}
			if err := h.auditLogger.Log(ctx, actorID, services.AuditCreate, "progress_scores", scoreRecord.ID, nil, scoreRecord); err != nil {
				_ = err
			}
		} else if scoreErr == nil {
			if err := tx.Save(&scoreRecord).Error; err != nil {
				return err
			}
			if err := h.auditLogger.Log(ctx, actorID, services.AuditUpdate, "progress_scores", scoreRecord.ID, beforeScore, scoreRecord); err != nil {
				_ = err
			}
		} else {
			return scoreErr
		}

		// --- 2. UPSERT PROGRESS REMARK ---
		var remarkRecord models.ProgressRemark
		remarkErr := tx.Where("student_id = ? AND term = ?", req.StudentID, req.Term).First(&remarkRecord).Error

		beforeRemark := remarkRecord
		remarkRecord.StudentID = req.StudentID
		remarkRecord.Term = req.Term
		remarkRecord.Remarks = req.Remark
		remarkRecord.WrittenBy = actorID
		remarkRecord.UpdatedAt = time.Now()

		if errors.Is(remarkErr, gorm.ErrRecordNotFound) {
			if err := tx.Create(&remarkRecord).Error; err != nil {
				return err
			}
			// Using 0 as entityID because progress_remarks doesn't have an auto-incrementing single primary key field
			if err := h.auditLogger.Log(ctx, actorID, services.AuditCreate, "progress_remarks", 0, nil, remarkRecord); err != nil {
				_ = err
			}
		} else if remarkErr == nil {
			if err := tx.Save(&remarkRecord).Error; err != nil {
				return err
			}
			if err := h.auditLogger.Log(ctx, actorID, services.AuditUpdate, "progress_remarks", 0, beforeRemark, remarkRecord); err != nil {
				_ = err
			}
		} else {
			return remarkErr
		}

		return nil
	})

	if err != nil {
		apierrors.Internal(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "progress records saved successfully"})
}

// ViewProgress fetches evaluations and features deep security gating for parent associations.
//
// @Summary View a student's progress scores and remarks
// @Description Returns the entire academic scorecard tracking matrix for a student ID. Gated for linked child profiles if called by a PARENT role.
// @Tags progress
// @Produce json
// @Param student_id query integer true "Student ID identifier mapping"
// @Success 200 {object} ProgressViewResponse
// @Failure 400 {object} apierrors.ErrorResponse
// @Failure 403 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/progress/view [get]
func (h *ProgressHandlers) ViewProgress(c *gin.Context) {
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
		apierrors.BadRequest(c, "invalid student_id format")
		return
	}
	studentID := uint(id)

	ctx := c.Request.Context()

	// Defensive Gating: Parents can only access records matching their child's identifier
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

	var scores []models.ProgressScore
	if err := h.db.WithContext(ctx).Where("student_id = ?", studentID).Find(&scores).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	var remarks []models.ProgressRemark
	if err := h.db.WithContext(ctx).Where("student_id = ?", studentID).Find(&remarks).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	c.JSON(http.StatusOK, ProgressViewResponse{
		StudentID: studentID,
		Scores:    scores,
		Remarks:   remarks,
	})
}