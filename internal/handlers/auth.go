// Package handlers contains Gin HTTP handlers for the School Software
// backend. This file implements Stage 3 item 17: the one and only auth
// route on this backend, GET /api/auth/me.
//
// ASSUMPTIONS I had to make, since I don't have your actual models/app
// container in this session — check these against your real code before
// compiling:
//   - models.User has Name and Phone string fields alongside the
//     ID/AuthID/Role/DeletedAt fields already assumed in auth.go and
//     auth_provisioning.go. Adjust field names if yours differ.
//   - AuthHandlers holds a *gorm.DB, following the same "build once,
//     reuse" pattern as AuthMiddleware and RateLimiter.
package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Its-Ameekh/school_software_backend/internal/middleware" // adjust if your middleware package differs
	"github.com/Its-Ameekh/school_software_backend/internal/models"     // adjust if your model lives elsewhere
)

// AuthHandlers groups the handful of handlers this package owns. Build
// once at startup (same pattern as AuthMiddleware/RateLimiter) and wire
// into the router.
type AuthHandlers struct {
	db *gorm.DB
}

// NewAuthHandlers constructs an AuthHandlers. Call once during app
// setup, alongside NewAuthMiddleware.
func NewAuthHandlers(db *gorm.DB) *AuthHandlers {
	return &AuthHandlers{db: db}
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
