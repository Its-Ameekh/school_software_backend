// Package app wires together the Gin router, middleware stack, and
// Swagger security metadata for the School Software backend.
//
// @title School Software API
// @version 1.0
// @description Backend API for the School Software platform (Principal / Teacher / Parent).
//
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and the Supabase-issued JWT, e.g. "Bearer eyJhbGciOi...".
package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/Its-Ameekh/school_software_backend/internal/database"
	"github.com/Its-Ameekh/school_software_backend/internal/handlers"
	"github.com/Its-Ameekh/school_software_backend/internal/middleware"
)

// NewRouter builds the Gin engine and wires all middleware and handlers.
func NewRouter(
	container *Container,
	authMW *middleware.AuthMiddleware,
	limiter *middleware.RateLimiter,
	authHandlers *handlers.AuthHandlers,
	financeHandlers *handlers.FinanceHandlers,
	progressHandlers *handlers.ProgressHandlers,
	studentHandlers *handlers.StudentHandlers,
	classHandlers *handlers.ClassHandlers,
	leaveHandlers *handlers.LeaveHandlers,
) *gin.Engine {

	if container.Config.Environment == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// Global middleware
	r.Use(gin.Recovery())
	r.Use(requestLogger(container))

	// Health endpoint
	registerHealthRoute(r, container)

	// Swagger UI
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Protected routes
	v1 := r.Group("/api")
	v1.Use(authMW.RequireAuth(), limiter.Limit())
	{
		// Authentication
		v1.GET("/auth/me", authHandlers.Me)

		// ==========================================
		// ENG A TRACK: Identity, Classes, Leaves
		// ==========================================

		// Student endpoints
		students := v1.Group("/students")
		{
			students.POST("", studentHandlers.CreateStudent)
			students.GET("/unassigned", studentHandlers.GetUnassignedStudents)
			students.PATCH("/:id/assign-class", studentHandlers.AssignClass)

			// Student leave hooks nested under student resource context
			students.POST("/:id/leave-requests", leaveHandlers.CreateStudentLeaveRequest)
			students.GET("/:id/leave-requests", leaveHandlers.GetStudentLeaveHistory)
		}

		// Class & Timetable endpoints
		classes := v1.Group("/classes")
		{
			classes.POST("", classHandlers.CreateClass)
			classes.GET("", classHandlers.ListClasses)
			classes.PATCH("/:id/teacher", classHandlers.AssignTeacher)
			classes.PATCH("/:id/substitute", classHandlers.ToggleSubstitute)
			classes.PUT("/:id/timetable/:day/:period", classHandlers.UpsertTimetableSlot)
		}

		// Top-level Leave administration endpoints
		v1.PATCH("/leave-requests/:id", leaveHandlers.UpdateStudentLeaveStatus)
		v1.PATCH("/teacher-leave-requests/:id", leaveHandlers.UpdateTeacherLeaveStatus)

		// Teacher personal workspace routes
		teachers := v1.Group("/teachers")
		{
			teachers.POST("/me/leave-requests", leaveHandlers.CreateTeacherLeaveRequest)
			teachers.GET("/me/leave-requests", leaveHandlers.GetMyTeacherLeaveRequests)
		}

		// ==========================================
		// ENG B TRACK: Attendance, Finance, Progress
		// ==========================================

		// Finance endpoints
		finance := v1.Group("/finance")
		{
			finance.GET("/summary", financeHandlers.Summary)
			finance.POST("/payment", financeHandlers.Payment)
			finance.POST("/waive", financeHandlers.Waive)
			finance.POST("/reminders", financeHandlers.Reminders)
		}

		// Progress endpoints
		progress := v1.Group("/progress")
		{
			progress.POST("/evaluation", progressHandlers.EnterEvaluation)
			progress.GET("/view", progressHandlers.ViewProgress)
		}
	}

	return r
}

// requestLogger logs every request.
func requestLogger(container *Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		container.Logger.Info(
			"request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
		)
	}
}

// registerHealthRoute registers GET /health.
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

		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
		})
	})
}
