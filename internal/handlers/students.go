package handlers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Its-Ameekh/school_software_backend/internal/apierrors"
	"github.com/Its-Ameekh/school_software_backend/internal/middleware"
	"github.com/Its-Ameekh/school_software_backend/internal/models"
	"github.com/Its-Ameekh/school_software_backend/internal/services"
)

// StudentHandlers holds everything the student/guardian/admission-intake
// pipeline needs. Constructed once in main.go and passed into router.go —
// matches the Stage 1 "app container" pattern, nothing global.
type StudentHandlers struct {
	db             *gorm.DB
	auditLogger    *services.AuditLogger
	feeService     services.FeeLedgerService // sealed contract, see CreateStudent
	supabaseURL    string
	serviceRoleKey string
}

func NewStudentHandlers(db *gorm.DB, audit *services.AuditLogger, fee services.FeeLedgerService, supabaseURL, serviceRoleKey string) *StudentHandlers {
	return &StudentHandlers{
		db:             db,
		auditLogger:    audit,
		feeService:     fee,
		supabaseURL:    supabaseURL,
		serviceRoleKey: serviceRoleKey,
	}
}

// dobPattern gives a fast, client-facing 422 before we ever call
// services.CreateAuthUser, which validates the same shape internally.
var dobPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ---- Request/response DTOs ----

type CreateStudentRequest struct {
	// Student — matches the confirmed models.Student columns.
	// RollNumber is unique/not-null in the schema but no generation
	// strategy has been defined anywhere in this project yet (no
	// sequence, no per-grade format). Taken as required input for now;
	// flag if you want this auto-generated instead (e.g. a
	// grade+year+sequence scheme, similar to the PFM-YYYY-NNNN pattern
	// from the earlier Preschool Fee Manager project) — that's a real
	// decision, not something to guess silently.
	RollNumber      string  `json:"roll_number" binding:"required"`
	FullName        string  `json:"full_name" binding:"required"`
	DOB             string  `json:"dob" binding:"required"` // "YYYY-MM-DD"; also doubles as the guardian's temp password per Stage 3 design
	Gender          string  `json:"gender" binding:"required"`
	GradeTier       string  `json:"grade_tier" binding:"required"`
	BloodGroup      *string `json:"blood_group"`
	Allergies       *string `json:"allergies"`
	SpecialTalents  *string `json:"special_talents"`
	LanguagesSpoken *string `json:"languages_spoken"`
	FoodType        *string `json:"food_type"`

	// Primary guardian — becomes the auth account (role=PARENT) and one
	// Guardian row (StudentID FK'd after the student is created).
	GuardianFullName     string  `json:"guardian_full_name" binding:"required"`
	GuardianRelationship string  `json:"guardian_relationship" binding:"required"` // e.g. "Father", "Mother", "Guardian"
	GuardianPhone        string  `json:"guardian_phone" binding:"required"`        // E.164 expected; CreateAuthUser does not normalize this
	GuardianEmail        *string `json:"guardian_email"`
	GuardianOccupation   *string `json:"guardian_occupation"`

	// Admission intake — payment/enrollment details, per the confirmed
	// models.AdmissionIntake columns (this table is NOT admin notes,
	// despite the original blueprint's wording — it's fee/transport
	// intake data).
	PayMode       string  `json:"pay_mode" binding:"required"` // "Cash" | "DD" | "Cheque"
	AmountPaid    float64 `json:"amount_paid" binding:"required"`
	ReceiptNumber string  `json:"receipt_number" binding:"required"`
	TransportPref string  `json:"transport_pref" binding:"required"` // "School Van" | "Own"
}

type CreateStudentResponse struct {
	StudentID  uint   `json:"student_id"`
	GuardianID uint   `json:"guardian_id"` // Guardian's own PK, distinct from the linked user_id
	IntakeID   uint   `json:"intake_id"`
	UserID     uint   `json:"user_id"`
	AuthID     string `json:"auth_id"`
	Message    string `json:"message"`
}

type AssignClassRequest struct {
	ClassID uint `json:"class_id" binding:"required"`
}

// ---- Handlers ----

// CreateStudent handles the full admission intake pipeline:
// provision guardian login -> transaction (student, guardian, intake,
// fee ledger) -> audit log -> response.
//
// Creation order inside the transaction is Student, then Guardian, then
// AdmissionIntake — Guardian.StudentID is the FK direction (a guardian
// belongs to a student, not the reverse), so Student must exist first.
//
// @Summary Admit a new student
// @Description Provisions the primary guardian's Supabase auth account, then creates student, guardian, and admission_intake rows in a single transaction, and triggers termly fee ledger generation.
// @Tags students
// @Accept json
// @Produce json
// @Param request body CreateStudentRequest true "Admission intake payload"
// @Success 201 {object} CreateStudentResponse
// @Failure 400 {object} apierrors.ErrorResponse
// @Failure 403 {object} apierrors.ErrorResponse
// @Failure 500 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/students [post]
func (h *StudentHandlers) CreateStudent(c *gin.Context) {
	// --- 1. Validate input ---
	var req CreateStudentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}
	if !dobPattern.MatchString(req.DOB) {
		apierrors.ValidationFailed(c, "dob must be formatted as YYYY-MM-DD")
		return
	}

	// --- 2. Check role context ---
	// RequireRoles(RolePrincipal) should already sit in front of this
	// route in router.go; this handler trusts that gate and only reads
	// the actor for the audit entry — no second query against users
	// here, per the Stage 3 carryforward note.
	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Unauthorized(c)
		return
	}

	// --- 3. Provision the guardian's login BEFORE opening our own
	// transaction. services.CreateAuthUser does the Supabase Admin API
	// call AND the local users-row insert itself, with its own
	// compensating-delete if that insert fails. We don't wrap this call
	// in our own transaction: it would hold a DB transaction open across
	// an external network hop, and a rollback here couldn't undo the
	// Supabase side anyway.
	//
	// Note: the child's DOB is deliberately what's passed as the temp
	// password here, not the guardian's own — models.Guardian has no DOB
	// field at all, so this is the only DOB available for this purpose,
	// consistent with how the admission blueprint specified it.
	authID, err := services.CreateAuthUser(
		c.Request.Context(),
		h.db,
		h.supabaseURL,
		h.serviceRoleKey,
		req.GuardianPhone,
		req.DOB,
		"PARENT",
		req.GuardianFullName,
	)
	if err != nil {
		apierrors.Internal(c, fmt.Errorf("guardian provisioning failed: %w", err))
		return
	}

	// CreateAuthUser only returns the Supabase auth_id, not the new
	// users.id — one lookup to get it for the Guardian.UserID FK below.
	// This is fetching a row we just created via a function that doesn't
	// hand back its ID, not a freshness re-check, so it doesn't fall
	// under the "don't re-query" rule.
	var guardianUser models.User
	if err := h.db.WithContext(c.Request.Context()).Where("auth_id = ?", authID).First(&guardianUser).Error; err != nil {
		h.logOrphanedAccount(c.Request.Context(), authID, 0, fmt.Errorf("could not re-fetch just-created user: %w", err))
		apierrors.Internal(c, err)
		return
	}

	// --- 4. DB work, in one transaction ---
	// If this fails, step 3's user+auth account is left behind: a valid
	// login attached to nothing. Logged below for manual cleanup rather
	// than silently lost.
	var student models.Student
	var guardian models.Guardian
	var intake models.AdmissionIntake

	txErr := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		dob, err := time.Parse("2006-01-02", req.DOB)
		if err != nil {
			// Unreachable given the regex check above, but fail loudly
			// rather than trust an already-validated value blindly.
			return fmt.Errorf("dob parse: %w", err)
		}

		student = models.Student{
			RollNumber:      req.RollNumber,
			FullName:        req.FullName,
			DOB:             dob,
			Gender:          req.Gender,
			GradeTier:       req.GradeTier,
			BloodGroup:      req.BloodGroup,
			Allergies:       req.Allergies,
			SpecialTalents:  req.SpecialTalents,
			LanguagesSpoken: req.LanguagesSpoken,
			FoodType:        req.FoodType,
			ClassID:         nil, // unassigned at intake
		}
		if err := tx.Create(&student).Error; err != nil {
			return fmt.Errorf("student insert: %w", err)
		}

		userID := guardianUser.ID
		guardian = models.Guardian{
			StudentID:           student.ID,
			UserID:              &userID,
			FullName:            req.GuardianFullName,
			Relationship:        req.GuardianRelationship,
			Occupation:          req.GuardianOccupation,
			Email:               req.GuardianEmail,
			Mobile:              &req.GuardianPhone,
			IsPrimaryContact:    true,
			AuthorizedForPickup: true,
		}
		if err := tx.Create(&guardian).Error; err != nil {
			return fmt.Errorf("guardian insert: %w", err)
		}

		intake = models.AdmissionIntake{
			StudentID:     student.ID,
			PayMode:       req.PayMode,
			AmountPaid:    req.AmountPaid,
			ReceiptNumber: req.ReceiptNumber,
			TransportPref: req.TransportPref,
			// AdmittedAt left zero-value on purpose: GORM omits
			// zero-value fields on insert when the column has a
			// `default` tag, letting Postgres's default now() apply.
		}
		if err := tx.Create(&intake).Error; err != nil {
			return fmt.Errorf("admission_intake insert: %w", err)
		}

		// Sealed contract with Eng B's Finance track:
		//   GenerateTermLedger(ctx, tx, studentID, gradeTier) error
		// Takes the in-flight *gorm.DB so it commits atomically with
		// student/guardian/intake above.
		if err := h.feeService.GenerateTermLedger(c.Request.Context(), tx, student.ID, student.GradeTier); err != nil {
			return fmt.Errorf("fee ledger generation: %w", err)
		}

		return nil
	})

	if txErr != nil {
		h.logOrphanedAccount(c.Request.Context(), authID, guardianUser.ID, txErr)
		apierrors.Internal(c, txErr)
		return
	}

	// --- 5. Audit log (fail-open, per team convention) ---
	if err := h.auditLogger.Log(c.Request.Context(), actorID, services.AuditCreate, "student", student.ID, nil, student); err != nil {
		_ = err
	}

	// --- 6. Response ---
	c.JSON(http.StatusCreated, CreateStudentResponse{
		StudentID:  student.ID,
		GuardianID: guardian.ID,
		IntakeID:   intake.ID,
		UserID:     guardianUser.ID,
		AuthID:     authID,
		Message:    "student admitted successfully",
	})
}

// GetUnassignedStudents lists students with no class assignment yet.
//
// @Summary List unassigned students
// @Tags students
// @Produce json
// @Success 200 {array} models.Student
// @Failure 403 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/students/unassigned [get]
func (h *StudentHandlers) GetUnassignedStudents(c *gin.Context) {
	var students []models.Student
	// models.Student has a confirmed gorm.DeletedAt column, so this
	// already excludes soft-deleted rows with no manual filter needed.
	if err := h.db.WithContext(c.Request.Context()).Where("class_id IS NULL").Find(&students).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}
	c.JSON(http.StatusOK, students)
}

// AssignClass places a previously-unassigned student into a class.
//
// @Summary Assign a student to a class
// @Tags students
// @Accept json
// @Produce json
// @Param id path integer true "Student ID"
// @Param request body AssignClassRequest true "Target class"
// @Success 200 {object} models.Student
// @Failure 400 {object} apierrors.ErrorResponse
// @Failure 403 {object} apierrors.ErrorResponse
// @Failure 404 {object} apierrors.ErrorResponse
// @Security BearerAuth
// @Router /api/students/{id}/assign-class [patch]
func (h *StudentHandlers) AssignClass(c *gin.Context) {
	studentID := c.Param("id")

	var req AssignClassRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apierrors.ValidationFailed(c, err.Error())
		return
	}

	actorID, ok := middleware.GetUserID(c)
	if !ok {
		apierrors.Unauthorized(c)
		return
	}

	var student models.Student
	if err := h.db.WithContext(c.Request.Context()).First(&student, studentID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			apierrors.NotFound(c, "student")
			return
		}
		apierrors.Internal(c, err)
		return
	}

	before := student // shallow copy for the audit diff, pre-mutation
	student.ClassID = &req.ClassID

	if err := h.db.WithContext(c.Request.Context()).Save(&student).Error; err != nil {
		apierrors.Internal(c, err)
		return
	}

	if err := h.auditLogger.Log(c.Request.Context(), actorID, services.AuditUpdate, "student", student.ID, before, student); err != nil {
		_ = err // fail-open, per team convention
	}

	c.JSON(http.StatusOK, student)
}

// logOrphanedAccount is a deliberately loud, separate log path (not the
// audit log — this is an operational alert, not a business-event record)
// for the case where step 3's guardian login was created successfully
// but step 4's transaction then failed. localUserID is 0 if the failure
// happened before we even managed to look that up.
func (h *StudentHandlers) logOrphanedAccount(ctx context.Context, authID string, localUserID uint, cause error) {
	// TODO: wire to the Stage 1 structured logger with a distinct
	// severity/tag (e.g. "ORPHANED_GUARDIAN_ACCOUNT") so this is
	// alertable/searchable separately from ordinary error logs.
	fmt.Printf("[ALERT] orphaned guardian account auth_id=%s user_id=%d cause=%v\n", authID, localUserID, cause)
}