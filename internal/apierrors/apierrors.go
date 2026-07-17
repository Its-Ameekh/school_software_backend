package apierrors

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorResponse is the single JSON error shape every handler in this
// backend returns. Keeping this in one package means Eng A and Eng B
// can never accidentally diverge on the envelope shape.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`    // machine-readable, e.g. "FORBIDDEN"
	Details string `json:"details,omitempty"` // optional extra context, dev-facing
}

func respond(c *gin.Context, status int, code, msg string, details ...string) {
	resp := ErrorResponse{Error: msg, Code: code}
	if len(details) > 0 {
		resp.Details = details[0]
	}
	c.AbortWithStatusJSON(status, resp)
}

// Forbidden replaces the duplicated denyForbidden in rbac.go and
// handlers/auth.go. Both callers should switch to this.
func Forbidden(c *gin.Context) {
	respond(c, http.StatusForbidden, "FORBIDDEN", "you do not have permission to perform this action")
}

func Unauthorized(c *gin.Context) {
	respond(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
}

func NotFound(c *gin.Context, resource string) {
	respond(c, http.StatusNotFound, "NOT_FOUND", resource+" not found")
}

func BadRequest(c *gin.Context, msg string) {
	respond(c, http.StatusBadRequest, "BAD_REQUEST", msg)
}

func ValidationFailed(c *gin.Context, details string) {
	respond(c, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "input validation failed", details)
}

func Internal(c *gin.Context, err error) {
	// Log err via your structured logger here before responding —
	// never leak internal error text into the response body.
	respond(c, http.StatusInternalServerError, "INTERNAL", "something went wrong")
}

func Conflict(c *gin.Context, msg string) {
	respond(c, http.StatusConflict, "CONFLICT", msg)
}