// Package middleware contains Gin middleware for the School Software
// backend. This file implements Stage 3 item 19: role-based access
// control, run after auth.go on any route that needs to restrict which
// of PRINCIPAL / TEACHER / PARENT may call it.
//
// RequireRoles must run AFTER AuthMiddleware.RequireAuth() in the chain
// — it only reads the context that RequireAuth() injects, it never
// touches the token or the database itself. Mount order matters, e.g.:
//
//	protected := router.Group("/api")
//	protected.Use(authMW.RequireAuth())
//	protected.POST("/teachers", middleware.RequireRoles("PRINCIPAL"), teacherHandlers.Create)
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireRoles returns Gin middleware that only allows the request
// through if the context-injected auth_user_role (set by
// AuthMiddleware.RequireAuth(), see ContextKeyUserRole in auth.go)
// exactly matches one of allowedRoles. Any other case — missing
// context, wrong type, or a role not in the allow-list — is rejected
// with 403.
//
// Pass the exact role strings as stored in users.role, e.g.:
//
//	middleware.RequireRoles("PRINCIPAL")
//	middleware.RequireRoles("PRINCIPAL", "TEACHER")
func RequireRoles(allowedRoles ...string) gin.HandlerFunc {
	// Build a set once, at route-registration time, rather than
	// re-scanning the slice on every request.
	allowed := make(map[string]bool, len(allowedRoles))
	for _, r := range allowedRoles {
		allowed[r] = true
	}

	return func(c *gin.Context) {
		role, ok := GetUserRole(c)
		if !ok {
			// Context missing or wrong type — either RequireRoles was
			// mounted without RequireAuth() ahead of it, or something
			// upstream changed the context shape. Fail closed.
			denyForbidden(c, "role not found in request context")
			return
		}

		if !allowed[role] {
			denyForbidden(c, "caller's role is not permitted for this action")
			return
		}

		c.Next()
	}
}

// denyForbidden writes the same 403 JSON shape auth.go's deny() uses,
// so every middleware in this package returns a consistent error body
// regardless of which check rejected the request.
func denyForbidden(c *gin.Context, reason string) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error":  "forbidden",
		"detail": reason,
	})
}
