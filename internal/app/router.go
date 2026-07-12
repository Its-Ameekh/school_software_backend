package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/Its-Ameekh/school_software_backend/internal/database"
)

// NewRouter builds the Gin engine, wires in whatever global middleware
// Stage 1 needs (recovery from panics, request logging), and registers
// the /health route. Route groups for real endpoints get added in
// Stage 4. Named NewRouter (not New) to avoid colliding with the
// Container constructor already defined in container.go — same
// package, so both would otherwise be func New(...).
func NewRouter(container *Container) *gin.Engine {
	if container.Config.Environment == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// gin.Recovery() turns a panic in any handler into a 500 instead of
	// crashing the whole process — non-negotiable for anything with
	// real users hitting it.
	r.Use(gin.Recovery())
	r.Use(requestLogger(container))

	registerHealthRoute(r, container)

	// Swagger UI at /swagger/index.html — regenerate docs with
	// `swag init` after adding/changing any endpoint annotations.
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}

// requestLogger is a minimal middleware that logs each request through
// the structured logger instead of Gin's default plain-text logger.
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
// connection and reports healthy/unhealthy. This is what Cloudflare/
// UptimeRobot monitoring will poll (Stage 7, item #37), and it's the
// first real proof the bootstrap actually works end to end.
//
// @Summary      Health check
// @Description  Checks database connectivity and reports service status
// @Tags         system
// @Produce      json
// @Success      200 {object} map[string]string
// @Failure      503 {object} map[string]string
// @Router       /health [get]
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
