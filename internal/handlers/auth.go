// Package handlers contains Gin HTTP handlers for the School Software
// backend. This file implements:
//   - Stage 3 item 17: GET /api/auth/me
//   - Stage 6: POST /api/auth/change-temporary-password — verifies a
//     pending must_change_password flag on the caller's Supabase account,
//     validates the new password server-side, then uses the Supabase
//     Admin API to atomically set the new password and clear the flag.
//     The frontend never touches Supabase admin operations directly.
package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"
	

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Its-Ameekh/school_software_backend/internal/middleware" // adjust if your middleware package differs
	"github.com/Its-Ameekh/school_software_backend/internal/models"     // adjust if your model lives elsewhere
	"github.com/Its-Ameekh/school_software_backend/internal/services"   // adjust if your services package differs
)

// AuthHandlers groups the handful of handlers this package owns. Build
// once at startup (same pattern as AuthMiddleware/RateLimiter) and wire
// into the router.
type AuthHandlers struct {
	db            *gorm.DB
	supabaseAdmin *services.SupabaseAdminClient
}

// NewAuthHandlers constructs an AuthHandlers. Call once during app
// setup, alongside NewAuthMiddleware. supabaseAdmin must be built with
// the service_role key — see internal/services/supabase_admin.go.
func NewAuthHandlers(db *gorm.DB, supabaseAdmin *services.SupabaseAdminClient) *AuthHandlers {
	return &AuthHandlers{db: db, supabaseAdmin: supabaseAdmin}
}

// MeResponse is the JSON body returned by GET /api/auth/me.
type MeResponse struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Role  string `json:"role"`
}

// ErrorResponse is the standard error JSON shape used across this
// backend's 403/429 responses (matches auth.go's deny() and
// ratelimit.go's 429 body).
type ErrorResponse struct {
	Error  string `json:"error"`
	Detail string `json:"detail"`
}

// Me godoc
//
//	@Summary		Get authenticated caller's profile
//	@Description	Returns the local profile and role of whichever user the bearer token resolves to. Open to any authenticated role (PRINCIPAL, TEACHER, PARENT) — this is the proof-of-life route for the auth chain, not role-restricted.
//	@Tags			auth
//	@Security		ApiKeyAuth
//	@Produce		json
//	@Success		200	{object}	MeResponse
//	@Failure		403	{object}	ErrorResponse	"invalid/expired token, or no matching local user"
//	@Router			/api/auth/me [get]
func (h *AuthHandlers) Me(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		// Should be unreachable in practice — RequireAuth() always sets
		// this before a handler runs. Fail closed anyway rather than
		// assume the context is trustworthy.
		denyForbidden(c, "user identity not found in request context")
		return
	}

	// Deliberate second DB read, distinct from the lookup RequireAuth()
	// already did to resolve the token. RequireAuth() only proves the
	// user existed at token-verification time a moment ago; re-checking
	// here confirms the row is still active (not soft-deleted) at the
	// exact instant this handler runs, e.g. if an account was disabled
	// in the seconds between token verification and this request being
	// handled. A cheap, indexed primary-key lookup, so the redundancy is
	// worth the freshness guarantee on a security-relevant read.
	var user models.User
	result := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted_at IS NULL", userID).
		First(&user)

	if result.Error != nil {
		if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			denyForbidden(c, "unable to verify account")
			return
		}
		denyForbidden(c, "account not found")
		return
	}

	c.JSON(http.StatusOK, MeResponse{
		ID:    user.ID,
		Name:  user.Name,
		Phone: user.Phone,
		Role:  user.Role,
	})
}

// ChangeTemporaryPasswordRequest is the JSON body expected by
// POST /api/auth/change-temporary-password.
type ChangeTemporaryPasswordRequest struct {
	NewPassword string `json:"newPassword" binding:"required"`
}

// ChangeTemporaryPassword godoc
//
//	@Summary		Change a temporary (DOB) password and clear must_change_password
//	@Description	Verifies the caller's Supabase account currently has must_change_password=true, validates the new password server-side, then uses the Admin API to atomically set the new password and clear the flag. Frontend never touches Supabase admin operations directly.
//	@Tags			auth
//	@Security		ApiKeyAuth
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]bool
//	@Failure		400	{object}	ErrorResponse	"weak password"
//	@Failure		403	{object}	ErrorResponse	"no pending password change, or unauthenticated"
//	@Router			/api/auth/change-temporary-password [post]
func (h *AuthHandlers) ChangeTemporaryPassword(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		denyForbidden(c, "user identity not found in request context")
		return
	}

	var req ChangeTemporaryPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:  "bad_request",
			Detail: "newPassword is required",
		})
		return
	}

	if !validatePasswordPolicy(req.NewPassword) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:  "weak_password",
			Detail: "Password must be at least 8 characters and include an uppercase letter, a lowercase letter, and a digit.",
		})
		return
	}

	// Local lookup to get this user's Supabase auth_id — this backend
	// never stores passwords, only the link to the Supabase account.
	var user models.User
	result := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND deleted_at IS NULL", userID).
		First(&user)
	if result.Error != nil {
		denyForbidden(c, "account not found")
		return
	}

	ctx := c.Request.Context()

	adminUser, err := h.supabaseAdmin.GetUserByID(ctx, user.AuthID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:  "internal_error",
			Detail: "could not verify account state",
		})
		return
	}

	mustChange, _ := adminUser.UserMetadata["must_change_password"].(bool)
	if !mustChange {
		// Deliberately reject if this flag isn't set — this endpoint is
		// only for the forced first-login flow, not a general
		// change-password route. Prevents any authenticated user from
		// hitting this to bypass normal password-change UX/flows.
		denyForbidden(c, "no pending temporary password change for this account")
		return
	}

	// GoTrue replaces user_metadata wholesale on update, so merge locally
	// first — don't just send {"must_change_password": false} and wipe
	// out any other metadata keys that might exist.
	metadata := adminUser.UserMetadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["must_change_password"] = false

	if err := h.supabaseAdmin.UpdateUser(ctx, user.AuthID, req.NewPassword, metadata); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:  "internal_error",
			Detail: "could not update password",
		})
		return
	}

	// Construct structural before/after states for tracking the metadata mutation
	beforeJSON, _ := json.Marshal(map[string]interface{}{"must_change_password": true})
	afterJSON, _ := json.Marshal(map[string]interface{}{"must_change_password": false})

	// Match the database schema columns precisely
	auditRow := map[string]interface{}{
		"actor_user_id": user.ID,
		"action":        "UPDATE", // Bound by CREATE | UPDATE | DELETE constraint
		"table_name":    "users",
		"record_id":     user.ID,
		"before_state":  beforeJSON,
		"after_state":   afterJSON,
		"created_at":    time.Now().UTC(),
	}

	// Write securely to "audit_log" (singular) matching your migration configuration schema
	if err := h.db.Table("audit_log").WithContext(c.Request.Context()).Create(&auditRow).Error; err != nil {
		// Log the failure internally but don't break the client response flow
		// since the core password mutation in Supabase was fully successful.
		log.Printf("CRITICAL: failed to write security audit trail for user %d: %v", user.ID, err)
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}


// denyForbidden writes the same 403 JSON shape auth.go's deny() and
// rbac.go's denyForbidden() use, so every layer of this backend returns
// a consistent error body regardless of which check rejected the
// request.
func denyForbidden(c *gin.Context, reason string) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error":  "forbidden",
		"detail": reason,
	})
}