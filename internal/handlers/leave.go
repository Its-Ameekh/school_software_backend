package handlers

import (
	"fmt"
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

type LeaveHandlers struct {
	db          *gorm.DB
	auditLogger *services.AuditLogger
}

func NewLeaveHandlers(db *gorm.DB, audit *services.AuditLogger) *LeaveHandlers {
	return &LeaveHandlers{db: db, auditLogger: audit}
}

const (
	leaveStatusPending  = "PENDING"
	leaveStatusApproved = "APPROVED"
	leaveStatusRejected = "REJECTED"
)

var validReviewStatuses = map[string]bool{
	leaveStatusApproved: true,
	leaveStatusRejected: true,
}

// ---- DTOs ----

type CreateStudentLeaveRequestBody struct {
	Date   string `json:"date" binding:"required"` // "YYYY-MM-DD"
	Reason string `json:"reason" binding:"required"`
}

type CreateTeacherLeaveRequestBody struct {
	FromDate  string `json:"from_date" binding:"required"`  // "YYYY-MM-DD"
	ToDate    string `json:"to_date" binding:"required"`    // "YYYY-MM-DD"
	LeaveType string `json:"leave_type" binding:"required"` // Casual | Sick Medical | etc.
	Reason    string `json:"reason" binding:"required"`
}

type UpdateLeaveStatusRequest struct {
	Status string `json:"status" binding:"required"` // APPROVED | REJECTED
}

// ==================== STUDENT LEAVE ====================

// CreateStudentLeaveRequest creates a leave request for a student.
//
// @Summary Create a student leave request
// @Tags leave
// @Accept json
// @Produce json
// @Param id path integer true "Student ID"
// @Param request body CreateStudentLeaveRequestBody true "Leave request payload"
// @Success 21 Created {object} models.LeaveRequest
// @Failure 400 {object} apierrors.ErrorResponse
// @Failure 401 {object} apierrors.ErrorResponse
// @Failure 403 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/students/{id}/leave [post]
func (h *LeaveHandlers) CreateStudentLeaveRequest(c *gin.Context) {
	studentID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apierrors.BadRequest(c, "invalid student id format")
		return
	}

	var req CreateStudentLeaveRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}

	leaveDate, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		apierrors.ValidationFailed(c, "date must be formatted as YYYY-MM-DD")
		return
	}

	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Unauthorized(c)
		return
	}
	actorRole, _ := middleware.GetUserRole(c)

	if actorRole != "PRINCIPAL" {
		var guardian models.Guardian
		err := h.db.WithContext(c.Request.Context()).
			Where("student_id = ? AND user_id = ?", studentID, actorID).
			First(&guardian).Error
		if err != nil {
			apierrors.Forbidden(c)
			return
		}
	}

	leave := models.LeaveRequest{
		StudentID:   uint(studentID),
		RequestedBy: actorID,
		Date:        leaveDate,
		Reason:      req.Reason,
		Status:      leaveStatusPending,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&leave).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	if err := h.auditLogger.Log(c.Request.Context(), actorID, services.AuditCreate, "leave_request", leave.ID, nil, leave); err != nil {
		h.auditLogger.AlertFailure("leave_request", leave.ID, services.AuditCreate, err)
	}

	c.JSON(http.StatusCreated, leave)
}

// GetStudentLeaveHistory fetches a student's leave requests.
//
// @Summary Get student leave history
// @Tags leave
// @Produce json
// @Param id path integer true "Student ID"
// @Success 200 {array} models.LeaveRequest
// @Failure 400 {object} apierrors.ErrorResponse
// @Failure 401 {object} apierrors.ErrorResponse
// @Failure 403 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/students/{id}/leave [get]
func (h *LeaveHandlers) GetStudentLeaveHistory(c *gin.Context) {
	studentID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apierrors.BadRequest(c, "invalid student id format")
		return
	}

	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Unauthorized(c)
		return
	}
	actorRole, _ := middleware.GetUserRole(c)

	if actorRole != "PRINCIPAL" {
		var guardian models.Guardian
		err := h.db.WithContext(c.Request.Context()).
			Where("student_id = ? AND user_id = ?", studentID, actorID).
			First(&guardian).Error
		if err != nil {
			apierrors.Forbidden(c)
			return
		}
	}

	var leaves []models.LeaveRequest
	if err := h.db.WithContext(c.Request.Context()).
		Where("student_id = ?", studentID).
		Order("created_at DESC").
		Find(&leaves).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	c.JSON(http.StatusOK, leaves)
}

// UpdateStudentLeaveStatus updates a student leave request status.
//
// @Summary Update student leave request status
// @Tags leave
// @Accept json
// @Produce json
// @Param id path integer true "Leave Request ID"
// @Param request body UpdateLeaveStatusRequest true "Status update payload"
// @Success 200 {object} models.LeaveRequest
// @Failure 400 {object} apierrors.ErrorResponse
// @Failure 401 {object} apierrors.ErrorResponse
// @Failure 404 {object} apierrors.ErrorResponse
// @Failure 409 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/leaves/student/{id}/status [patch]
func (h *LeaveHandlers) UpdateStudentLeaveStatus(c *gin.Context) {
	requestID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apierrors.BadRequest(c, "invalid leave request id format")
		return
	}

	var req UpdateLeaveStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}
	if !validReviewStatuses[req.Status] {
		apierrors.ValidationFailed(c, "status must be APPROVED or REJECTED")
		return
	}

	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Unauthorized(c)
		return
	}

	var leave models.LeaveRequest
	if err := h.db.WithContext(c.Request.Context()).First(&leave, requestID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			apierrors.NotFound(c, "leave request")
			return
		}
		apierrors.Internal(c, err)
		return
	}
	if leave.Status != leaveStatusPending {
		apierrors.Conflict(c, fmt.Sprintf("leave request already reviewed (status=%s)", leave.Status))
		return
	}

	before := leave
	now := time.Now()
	leave.Status = req.Status
	leave.ReviewedBy = &actorID
	leave.ReviewedAt = &now

	if err := h.db.WithContext(c.Request.Context()).Save(&leave).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	if err := h.auditLogger.Log(c.Request.Context(), actorID, services.AuditUpdate, "leave_request", leave.ID, before, leave); err != nil {
		h.auditLogger.AlertFailure("leave_request", leave.ID, services.AuditUpdate, err)
	}

	c.JSON(http.StatusOK, leave)
}

// ==================== TEACHER LEAVE ====================

// CreateTeacherLeaveRequest creates a leave request for a teacher.
//
// @Summary Create a teacher leave request
// @Tags leave
// @Accept json
// @Produce json
// @Param request body CreateTeacherLeaveRequestBody true "Leave request payload"
// @Success 201 {object} models.TeacherLeaveRequest
// @Failure 400 {object} apierrors.ErrorResponse
// @Failure 401 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/teachers/leave [post]
func (h *LeaveHandlers) CreateTeacherLeaveRequest(c *gin.Context) {
	var req CreateTeacherLeaveRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}

	fromDate, err := time.Parse("2006-01-02", req.FromDate)
	if err != nil {
		apierrors.ValidationFailed(c, "from_date must be formatted as YYYY-MM-DD")
		return
	}
	toDate, err := time.Parse("2006-01-02", req.ToDate)
	if err != nil {
		apierrors.ValidationFailed(c, "to_date must be formatted as YYYY-MM-DD")
		return
	}
	if toDate.Before(fromDate) {
		apierrors.ValidationFailed(c, "to_date cannot be before from_date")
		return
	}

	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Unauthorized(c)
		return
	}

	leave := models.TeacherLeaveRequest{
		TeacherID: actorID,
		FromDate:  fromDate,
		ToDate:    toDate,
		LeaveType: req.LeaveType,
		Reason:    req.Reason,
		Status:    leaveStatusPending,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&leave).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	if err := h.auditLogger.Log(c.Request.Context(), actorID, services.AuditCreate, "teacher_leave_request", leave.ID, nil, leave); err != nil {
		h.auditLogger.AlertFailure("teacher_leave_request", leave.ID, services.AuditCreate, err)
	}

	c.JSON(http.StatusCreated, leave)
}

// GetMyTeacherLeaveRequests lists leave requests for the logged-in teacher.
//
// @Summary Get logged-in teacher's leave history
// @Tags leave
// @Produce json
// @Success 200 {array} models.TeacherLeaveRequest
// @Failure 401 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/teachers/leave [get]
func (h *LeaveHandlers) GetMyTeacherLeaveRequests(c *gin.Context) {
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Unauthorized(c)
		return
	}

	var leaves []models.TeacherLeaveRequest
	if err := h.db.WithContext(c.Request.Context()).
		Where("teacher_id = ?", actorID).
		Order("created_at DESC").
		Find(&leaves).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	c.JSON(http.StatusOK, leaves)
}

// UpdateTeacherLeaveStatus updates a teacher leave request status.
//
// @Summary Update teacher leave request status
// @Tags leave
// @Accept json
// @Produce json
// @Param id path integer true "Leave Request ID"
// @Param request body UpdateLeaveStatusRequest true "Status update payload"
// @Success 200 {object} models.TeacherLeaveRequest
// @Failure 400 {object} apierrors.ErrorResponse
// @Failure 401 {object} apierrors.ErrorResponse
// @Failure 404 {object} apierrors.ErrorResponse
// @Failure 409 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/leaves/teacher/{id}/status [patch]
func (h *LeaveHandlers) UpdateTeacherLeaveStatus(c *gin.Context) {
	requestID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apierrors.BadRequest(c, "invalid leave request id format")
		return
	}

	var req UpdateLeaveStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}
	if !validReviewStatuses[req.Status] {
		apierrors.ValidationFailed(c, "status must be APPROVED or REJECTED")
		return
	}

	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Unauthorized(c)
		return
	}

	var leave models.TeacherLeaveRequest
	if err := h.db.WithContext(c.Request.Context()).First(&leave, requestID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			apierrors.NotFound(c, "teacher leave request")
			return
		}
		apierrors.Internal(c, err)
		return
	}
	if leave.Status != leaveStatusPending {
		apierrors.Conflict(c, fmt.Sprintf("leave request already reviewed (status=%s)", leave.Status))
		return
	}

	before := leave
	now := time.Now()
	leave.Status = req.Status
	leave.ReviewedBy = &actorID
	leave.ReviewedAt = &now

	// Using a transaction since approving a teacher leave side-effects the Class assignment state
	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&leave).Error; err != nil {
			return err
		}

		// Cross-file coordination logic: If approved, auto-activate substitute coverage for classes this teacher teaches
		if leave.Status == leaveStatusApproved {
			if err := tx.Model(&models.Class{}).
				Where("teacher_id = ?", leave.TeacherID).
				Update("substitute_active", true).Error; err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		apierrors.Internal(c, err)
		return
	}

	if err := h.auditLogger.Log(c.Request.Context(), actorID, services.AuditUpdate, "teacher_leave_request", leave.ID, before, leave); err != nil {
		h.auditLogger.AlertFailure("teacher_leave_request", leave.ID, services.AuditUpdate, err)
	}

	c.JSON(http.StatusOK, leave)
}