package main

import (
	"context"
	"os"

	_ "github.com/Its-Ameekh/school_software_backend/docs"
	"github.com/Its-Ameekh/school_software_backend/internal/app"
	"github.com/Its-Ameekh/school_software_backend/internal/config"
	"github.com/Its-Ameekh/school_software_backend/internal/database"
	"github.com/Its-Ameekh/school_software_backend/internal/handlers"
	"github.com/Its-Ameekh/school_software_backend/internal/logger"
	"github.com/Its-Ameekh/school_software_backend/internal/middleware"
	"github.com/Its-Ameekh/school_software_backend/internal/services"
)

// @title            School Software API
// @version          1.0
// @description      Backend API for the School Software platform (Preschool Management System).
// @BasePath         /

func main() {
	// 1. Config first
	cfg := config.Load()

	// 2. Logger next
	log := logger.New(cfg.Environment)

	// 3. Database
	db, err := database.Connect(cfg.DatabaseURL, log)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	// [Stage 5 Infrastructure Initialization]
	// Build the Cloudflare R2 storage client and presigned asset lifecycle engine
	r2Client := services.NewR2Client(cfg)
	uploadService := services.NewPresignedUploadService(r2Client, cfg)

	// 4. Container — bundles config/logger/db/r2
	container := app.New(cfg, log, db, r2Client)

	// [Stage 3 Core Singletons]
	// Initialize JWKS Auth Middleware using the base Supabase URL from config
	authMW, err := middleware.NewAuthMiddleware(context.Background(), db, cfg.SupabaseURL)
	if err != nil {
		log.Error("failed to initialize auth middleware", "error", err)
		os.Exit(1)
	}

	// Initialize the structural rate limiter singleton
	limiter := middleware.NewRateLimiter()

	// Supabase Admin API client — built once here with the service_role
	// key from config (never exposed to the frontend). Used by
	// ChangeTemporaryPassword to set passwords and clear
	// must_change_password atomically via the GoTrue Admin API.
	supabaseAdmin := services.NewSupabaseAdminClient(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey)

	// Initialize the authentication route handler engines
	authHandlers := handlers.NewAuthHandlers(db, supabaseAdmin)

	// Shared audit logger instance
	auditLogger := services.NewAuditLogger(db)

	// [Stage 4 — Eng B Track Components]
	financeHandlers := handlers.NewFinanceHandlers(db, auditLogger)
	progressHandlers := handlers.NewProgressHandlers(db, auditLogger)
	attendanceHandlers := handlers.NewAttendanceHandlers(db, auditLogger) // Mapped for Attendance Tracking

	// [Stage 4 — Eng A Track Components]
	// Instantiate the live production fee ledger service to automatically seed student financial billing balances
	feeService := services.NewFeeLedgerService()

	studentHandlers := handlers.NewStudentHandlers(db, auditLogger, feeService, cfg.SupabaseURL, cfg.SupabaseServiceRoleKey)
	classHandlers := handlers.NewClassHandlers(db, auditLogger)
	leaveHandlers := handlers.NewLeaveHandlers(db, auditLogger)

	worksheetHandlers := handlers.NewWorksheetHandlers(container.DB, uploadService)
	galleryHandlers := handlers.NewGalleryHandlers(container.DB, uploadService)

	// 5. Router — pass all required dependencies to fulfill the routing signature
	router := app.NewRouter(
		container,
		authMW,
		limiter,
		authHandlers,
		financeHandlers,
		progressHandlers,
		studentHandlers,
		classHandlers,
		leaveHandlers,
		worksheetHandlers,  // Stage 5 Integration
		galleryHandlers,    // Stage 5 Integration
		attendanceHandlers, // Connected Track Mapping
	)

	// 6. Server — blocks here until SIGINT/SIGTERM, then drains cleanly
	if err := app.RunServer(container, router); err != nil {
		log.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}