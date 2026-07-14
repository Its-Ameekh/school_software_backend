// Package app wires together the Gin router, middleware stack, and
// Swagger security metadata for the School Software backend.
//
// This file implements Stage 3 item 20: documenting, in Swagger, which
// endpoints require a bearer token, and mounts the RequireAuth + rate-limiter
// middleware stack onto our protected route groups.
//
// @title						School Software API
// @version					1.0
// @description				Backend API for the School Software platform (Principal / Teacher / Parent).
//
// @securityDefinitions.apikey	ApiKeyAuth
// @in							header
// @name						Authorization
// @description				Type "Bearer" followed by a space and the Supabase-issued JWT, e.g. "Bearer eyJhbGciOi...".
package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/Its-Ameekh/school_software_backend/internal/database"
	"github.com/Its-Ameekh/school_software_backend/internal/handlers"   // Added for Stage 3/4 Handlers
	"github.com/Its-Ameekh/school_software_backend/internal/middleware" // Added for Stage 3 Middlewares
)

// NewRouter builds the Gin engine, wires in whatever global middleware
// Stage 1 needs, and configures the Stage 3 authentication access layers[cite: 1].
//
// Mounts RequireAuth (item 15) and Limit (item 18) globally on the protected group[cite: 1].
func NewRouter(container *Container, authMW *middleware.AuthMiddleware, limiter *middleware.RateLimiter, authHandlers *handlers.AuthHandlers) *gin.Engine {
	if container.Config.Environment == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// Global baseline middlewares[cite: 1]
	r.Use(gin.Recovery())           //[cite: 1]
	r.Use(requestLogger(container)) //[cite: 1]

	// Public system route[cite: 1]
	registerHealthRoute(r, container) //[cite: 1]

	// Swagger UI at /swagger/index.html[cite: 1]
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler)) //[cite: 1]

	// =========================================================================
	// STAGE 3 — PROTECTED ROUTE GROUPS
	// =========================================================================
	// Every route under this group sits behind JWKS token verification (item 15)
	// and the per-IP resource rate limiter (item 18)[cite: 1].
	v1 := r.Group("/api")
	v1.Use(authMW.RequireAuth(), limiter.Limit()) //[cite: 1]
	{
		// Item 17: The proof-of-life authentication path[cite: 1]
		v1.GET("/auth/me", authHandlers.Me) //[cite: 1]

		// Note: Stage 4 core CRUD endpoints will be appended here, using
		// middleware.RequireRoles("PRINCIPAL" | "TEACHER" | "PARENT") on a
		// per-route basis to enforce granular business access[cite: 1].
	}

	return r
}

// requestLogger is a minimal middleware that logs each request through
// the structured logger instead of Gin's default plain-text logger[cite: 1].
func requestLogger(container *Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		container.Logger.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
		)
	}
}

// registerHealthRoute wires up GET /health, which checks the DB
// connection and reports healthy/unhealthy[cite: 1].
//
// @Summary Health check
// @Description Checks database connectivity and reports service status
// @Tags system
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /health [get]
func registerHealthRoute(r *gin.Engine, container *Container) {
	r.GET("/health", func(c *gin.Context) {
		if err := database.Ping(container.DB); err != nil {
			container.Logger.Error("health check failed", "error", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"error":  err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})
}
