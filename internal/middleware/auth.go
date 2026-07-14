// Package middleware contains Gin middleware for the School Software
// backend. This file implements Stage 3 item 15: JWKS-based verification
// of Supabase-issued JWTs and a read-only lookup of the local user record.
//
// Design recap (full rationale in system design doc, Part 4, Decision 15):
//   - The Supabase project uses asymmetric ECC (P-256) signing keys, so
//     tokens are verified against Supabase's live JWKS endpoint, never a
//     static shared secret.
//   - This middleware is strictly READ-ONLY. It never creates or updates
//     a `users` row. Accounts are pre-provisioned via
//     internal/services/auth_provisioning.go (Stage 3 item 16). A
//     signature-valid token whose `sub` has no matching local user is
//     rejected with 403 — there is no lazy-provisioning branch here, on
//     purpose.
package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"github.com/Its-Ameekh/school_software_backend/internal/models" // adjust if your model lives elsewhere
)

// Context keys used to pass the authenticated local user's identity down
// to every handler after this middleware runs. Exported so rbac.go and
// any route handler can read them without repeating magic strings.
const (
	ContextKeyUserID   = "auth_user_id"
	ContextKeyUserRole = "auth_user_role"
)

// supabaseClaims models the subset of a Supabase-issued access token's
// claims this backend actually reads. `sub` (via RegisteredClaims) is the
// Supabase auth_id UUID — the only claim used to look up the local user.
//
// NOTE: the `Role` field here is Supabase's OWN internal claim (normally
// the literal string "authenticated"). It has nothing to do with our
// app's users.role (PRINCIPAL/TEACHER/PARENT) — that value only ever
// comes from our own database lookup below, never trusted from the token.
type supabaseClaims struct {
	jwt.RegisteredClaims
	Phone string `json:"phone,omitempty"`
	Role  string `json:"role,omitempty"`
}

// AuthMiddleware holds everything needed to verify a Supabase JWT and
// resolve it to a local user. Build it once at startup and reuse it for
// the lifetime of the process — do not construct one per request.
type AuthMiddleware struct {
	db     *gorm.DB
	jwks   keyfunc.Keyfunc
	issuer string
}

// NewAuthMiddleware builds the JWKS keyfunc (which launches its own
// background refresh goroutine tied to ctx) and returns a ready-to-use
// AuthMiddleware.
//
// Call this exactly ONCE, during router/app setup in main.go.
// supabaseProjectURL is the bare project URL, e.g.
// "https://xxxxxxxx.supabase.co" — no trailing slash required, and don't
// pass the full JWKS path yourself, this builds it.
//
// ctx should be a long-lived context (your app's root context) since
// keyfunc uses it to keep the background refresh goroutine alive for as
// long as the server runs; cancelling ctx stops key refreshing.
func NewAuthMiddleware(ctx context.Context, db *gorm.DB, supabaseProjectURL string) (*AuthMiddleware, error) {
	base := strings.TrimRight(supabaseProjectURL, "/")
	jwksURL := base + "/auth/v1/.well-known/jwks.json"
	issuer := base + "/auth/v1"

	k, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("auth middleware: failed to initialize JWKS client for %s: %w", jwksURL, err)
	}

	return &AuthMiddleware{
		db:     db,
		jwks:   k,
		issuer: issuer,
	}, nil
}

// RequireAuth returns the Gin middleware itself. Mount it on any route
// group that needs a logged-in user, e.g.:
//
//	protected := router.Group("/api")
//	protected.Use(authMW.RequireAuth())
func (a *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := extractBearerToken(c.GetHeader("Authorization"))
		if err != nil {
			deny(c, "missing or malformed Authorization header")
			return
		}

		claims := &supabaseClaims{}
		token, err := jwt.ParseWithClaims(
			tokenString,
			claims,
			a.jwks.Keyfunc,
			jwt.WithValidMethods([]string{"ES256"}), // pin to the ECC P-256 alg Supabase actually issues — blocks algorithm-confusion attacks
			jwt.WithIssuer(a.issuer),
			jwt.WithAudience("authenticated"), // Supabase's standard aud claim for logged-in users
		)
		if err != nil || !token.Valid {
			deny(c, "invalid or expired token")
			return
		}

		authID := claims.Subject // the `sub` claim — Supabase's auth_id UUID, as a string
		if authID == "" {
			deny(c, "token missing sub claim")
			return
		}

		var user models.User
		result := a.db.WithContext(c.Request.Context()).
			Where("auth_id = ? AND deleted_at IS NULL", authID).
			First(&user)

		if result.Error != nil {
			if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
				// A real DB error (connection blip, etc.) is different
				// from "no such user," but the spec calls for a flat
				// 403 either way. Worth wiring your structured logger
				// in here once auth.go has access to it, so this
				// distinction shows up in logs even though the HTTP
				// response doesn't change.
				deny(c, "unable to verify account")
				return
			}
			// No matching local user. This backend never
			// lazy-provisions — a signature-valid token with no local
			// match is still a flat rejection. See Decision 15.
			deny(c, "account not found")
			return
		}

		c.Set(ContextKeyUserID, user.ID)
		c.Set(ContextKeyUserRole, user.Role)
		c.Next()
	}
}

// extractBearerToken pulls the raw JWT out of an
// `Authorization: Bearer <token>` header value.
func extractBearerToken(header string) (string, error) {
	if header == "" {
		return "", errors.New("empty Authorization header")
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", errors.New("Authorization header must start with 'Bearer '")
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", errors.New("empty bearer token")
	}
	return token, nil
}

// deny writes a consistent 403 JSON body and aborts the request chain.
// Deliberately 403, not 401 — per the spec, a well-formed, validly-signed
// token still gets 403 if it has no matching local account.
func deny(c *gin.Context, reason string) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error":  "forbidden",
		"detail": reason,
	})
}

// GetUserID reads the authenticated local user's numeric ID out of the
// Gin context. Only meaningful on routes behind RequireAuth(); returns
// false if called on an unauthenticated route.
func GetUserID(c *gin.Context) (uint, bool) {
	v, exists := c.Get(ContextKeyUserID)
	if !exists {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok
}

// GetUserRole reads the authenticated local user's app role
// (PRINCIPAL/TEACHER/PARENT) out of the Gin context. Only meaningful on
// routes behind RequireAuth().
func GetUserRole(c *gin.Context) (string, bool) {
	v, exists := c.Get(ContextKeyUserRole)
	if !exists {
		return "", false
	}
	role, ok := v.(string)
	return role, ok
}
