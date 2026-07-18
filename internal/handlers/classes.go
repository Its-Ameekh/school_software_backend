package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Its-Ameekh/school_software_backend/internal/apierrors"
	"github.com/Its-Ameekh/school_software_backend/internal/middleware"
	"github.com/Its-Ameekh/school_software_backend/internal/models"
	"github.com/Its-Ameekh/school_software_backend/internal/services"
)

// ClassHandlers holds everything the class/timetable pipeline needs.
// Constructed once in main.go and passed into router.go.
type ClassHandlers struct {
	db          *gorm.DB
	auditLogger *services.AuditLogger
}

func NewClassHandlers(db *gorm.DB, audit *services.AuditLogger) *ClassHandlers {
	return &ClassHandlers{db: db, auditLogger: audit}
}

var validDaysOfWeek = map[string]bool{
	"Monday":    true,
	"Tuesday":   true,
	"Wednesday": true,
	"Thursday":  true,
	"Friday":    true,
	"Saturday":  true,
}

const (
	minPeriodNumber = 1
	maxPeriodNumber = 10
)

// ---- Request/response DTOs ----

type CreateClassRequest struct {
	Name string `json:"name" binding:"required"`
}

type AssignTeacherRequest struct {
	TeacherID uint `json:"teacher_id" binding:"required"`
}

type UpsertTimetableSlotRequest struct {
	Subject   string `json:"subject" binding:"required"`
	Room      string `json:"room"`
	StartTime string `json:"start_time" binding:"required"`
	EndTime   string `json:"end_time" binding:"required"`
}

// ---- Handlers ----

// CreateClass creates a new class entity.
//
// @Summary Create a class
// @Tags classes
// @Accept json
// @Produce json
// @Param request body CreateClassRequest true "Class name"
// @Success 201 {object} models.Class
// @Failure 400 {object} apierrors.ErrorResponse
// @Failure 403 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/classes [post]
func (h *ClassHandlers) CreateClass(c *gin.Context) {
	// --- Validate input ---
	var req CreateClassRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}

	// --- Role context ---
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Unauthorized(c)
		return
	}

	// --- DB operation ---
	class := models.Class{Name: req.Name}
	if err := h.db.WithContext(c.Request.Context()).Create(&class).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	// --- Audit (fail-open) ---
	if err := h.auditLogger.Log(c.Request.Context(), actorID, services.AuditCreate, "class", class.ID, nil, class); err != nil {
		h.auditLogger.AlertFailure("class", class.ID, services.AuditCreate, err)
	}

	// --- Response ---
	c.JSON(http.StatusCreated, class)
}

// ListClasses returns every class.
//
// @Summary List all classes
// @Tags classes
// @Produce json
// @Success 200 {array} models.Class
// @Failure 403 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/classes [get]
func (h *ClassHandlers) ListClasses(c *gin.Context) {
	var classes []models.Class
	if err := h.db.WithContext(c.Request.Context()).Find(&classes).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}
	c.JSON(http.StatusOK, classes)
}

// AssignTeacher sets a class's lead teacher.
//
// @Summary Assign lead teacher to a class
// @Tags classes
// @Accept json
// @Produce json
// @Param id path integer true "Class ID"
// @Param request body AssignTeacherRequest true "Teacher to assign"
// @Success 200 {object} models.Class
// @Failure 400 {object} apierrors.ErrorResponse
// @Failure 403 {object} apierrors.ErrorResponse
// @Failure 404 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/classes/{id}/teacher [patch]
func (h *ClassHandlers) AssignTeacher(c *gin.Context) {
	// --- Validate input ---
	var req AssignTeacherRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}

	// --- Role context ---
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Unauthorized(c)
		return
	}

	// --- DB operation ---
	var class models.Class
	if err := h.db.WithContext(c.Request.Context()).First(&class, c.Param("id")).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			apierrors.NotFound(c, "class")
			return
		}
		apierrors.Internal(c, err)
		return
	}

	before := class
	class.TeacherID = &req.TeacherID

	if err := h.db.WithContext(c.Request.Context()).Save(&class).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	// --- Audit (fail-open) ---
	if err := h.auditLogger.Log(c.Request.Context(), actorID, services.AuditUpdate, "class", class.ID, before, class); err != nil {
		h.auditLogger.AlertFailure("class", class.ID, services.AuditUpdate, err)
	}

	// --- Response ---
	c.JSON(http.StatusOK, class)
}

// ToggleSubstitute flips a class's substitute-coverage flag. Pure
// toggle — no request body, flips whatever the current value is.
//
// @Summary Toggle substitute coverage for a class
// @Tags classes
// @Produce json
// @Param id path integer true "Class ID"
// @Success 200 {object} models.Class
// @Failure 403 {object} apierrors.ErrorResponse
// @Failure 404 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/classes/{id}/substitute [patch]
func (h *ClassHandlers) ToggleSubstitute(c *gin.Context) {
	// --- Role context ---
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Unauthorized(c)
		return
	}

	// --- DB operation ---
	var class models.Class
	if err := h.db.WithContext(c.Request.Context()).First(&class, c.Param("id")).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			apierrors.NotFound(c, "class")
			return
		}
		apierrors.Internal(c, err)
		return
	}

	before := class
	class.SubstituteActive = !class.SubstituteActive

	if err := h.db.WithContext(c.Request.Context()).Save(&class).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	// --- Audit (fail-open) ---
	if err := h.auditLogger.Log(c.Request.Context(), actorID, services.AuditUpdate, "class", class.ID, before, class); err != nil {
		h.auditLogger.AlertFailure("class", class.ID, services.AuditUpdate, err)
	}

	// --- Response ---
	c.JSON(http.StatusOK, class)
}

// UpsertTimetableSlot creates or updates a single timetable slot.
//
// @Summary Upsert a timetable slot
// @Tags classes
// @Accept json
// @Produce json
// @Param id path integer true "Class ID"
// @Param day path string true "Day of week, e.g. Monday"
// @Param period path integer true "Period number (1-10)"
// @Param request body UpsertTimetableSlotRequest true "Slot details"
// @Success 200 {object} models.TimetableSlot
// @Failure 400 {object} apierrors.ErrorResponse
// @Failure 403 {object} apierrors.ErrorResponse
// @Failure 404 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/classes/{id}/timetable/{day}/{period} [put]
func (h *ClassHandlers) UpsertTimetableSlot(c *gin.Context) {
	// --- Validate input ---
	classID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		apierrors.BadRequest(c, "invalid class id")
		return
	}

	day := c.Param("day")
	if !validDaysOfWeek[day] {
		apierrors.ValidationFailed(c, fmt.Sprintf("day must be one of Monday..Saturday, got %q", day))
		return
	}

	period, err := strconv.Atoi(c.Param("period"))
	if err != nil || period < minPeriodNumber || period > maxPeriodNumber {
		apierrors.ValidationFailed(c, fmt.Sprintf("period must be an integer between %d and %d", minPeriodNumber, maxPeriodNumber))
		return
	}

	var req UpsertTimetableSlotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}

	// --- Role context ---
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Unauthorized(c)
		return
	}

	// --- DB operations ---
	var class models.Class
	if err := h.db.WithContext(c.Request.Context()).First(&class, classID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			apierrors.NotFound(c, "class")
			return
		}
		apierrors.Internal(c, err)
		return
	}

	slot := models.TimetableSlot{
		ClassID:      uint(classID),
		DayOfWeek:    day,
		PeriodNumber: int8(period),
		Subject:      req.Subject,
		Room:         req.Room,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
	}

	var existing models.TimetableSlot
	found := h.db.WithContext(c.Request.Context()).
		Where("class_id = ? AND day_of_week = ? AND period_number = ?", classID, day, period).
		First(&existing).Error == nil

	var auditAction services.AuditAction
	var before any

	if found {
		slot.ID = existing.ID
		if err := h.db.WithContext(c.Request.Context()).Save(&slot).Error; err != nil {
			apierrors.Internal(c, err)
			return
		}
		auditAction = services.AuditUpdate
		before = existing
	} else {
		if err := h.db.WithContext(c.Request.Context()).Create(&slot).Error; err != nil {
			apierrors.Internal(c, err)
			return
		}
		auditAction = services.AuditCreate
		before = nil
	}

	// --- Audit (fail-open) ---
	if err := h.auditLogger.Log(c.Request.Context(), actorID, auditAction, "timetable_slot", slot.ID, before, slot); err != nil {
		h.auditLogger.AlertFailure("timetable_slot", slot.ID, auditAction, err)
	}

	// --- Response ---
	c.JSON(http.StatusOK, slot)
}