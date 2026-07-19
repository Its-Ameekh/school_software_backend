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
// @description      Backend API for the School Software platform (Preschool Management System)[cite: 1].
// @BasePath         /

func main() {
	// 1. Config first[cite: 1]
	cfg := config.Load()

	// 2. Logger next[cite: 1]
	log := logger.New(cfg.Environment)

	// 3. Database[cite: 1]
	db, err := database.Connect(cfg.DatabaseURL, log)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	// [Stage 5 Infrastructure Initialization]
	// Build the Cloudflare R2 storage client and presigned asset lifecycle engine[cite:1]
	r2Client := services.NewR2Client(cfg)
	uploadService := services.NewPresignedUploadService(r2Client, cfg)

	// 4. Container — bundles config/logger/db/r2[cite: 1]
	container := app.New(cfg, log, db, r2Client)

	// [Stage 3 Core Singletons][cite: 1]
	// Initialize JWKS Auth Middleware using the base Supabase URL from config[cite: 1]
	authMW, err := middleware.NewAuthMiddleware(context.Background(), db, cfg.SupabaseURL)
	if err != nil {
		log.Error("failed to initialize auth middleware", "error", err)
		os.Exit(1)
	}

	// Initialize the structural rate limiter singleton[cite: 1]
	limiter := middleware.NewRateLimiter()

	// Supabase Admin API client — built once here with the service_role
	// key from config (never exposed to the frontend). Used by
	// ChangeTemporaryPassword to set passwords and clear
	// must_change_password atomically via the GoTrue Admin API.
	supabaseAdmin := services.NewSupabaseAdminClient(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey)

	// Initialize the authentication route handler engines[cite: 1]
	authHandlers := handlers.NewAuthHandlers(db, supabaseAdmin)

	// Shared audit logger instance[cite: 1]
	auditLogger := services.NewAuditLogger(db)

	// [Stage 4 — Eng B Track Components][cite: 1]
	financeHandlers := handlers.NewFinanceHandlers(db, auditLogger)
	progressHandlers := handlers.NewProgressHandlers(db, auditLogger)

	// [Stage 4 — Eng A Track Components][cite: 1]
	// Instantiate the live production fee ledger service to automatically seed student financial billing balances[cite: 1]
	feeService := services.NewFeeLedgerService()

	studentHandlers := handlers.NewStudentHandlers(db, auditLogger, feeService, cfg.SupabaseURL, cfg.SupabaseServiceRoleKey)
	classHandlers := handlers.NewClassHandlers(db, auditLogger)
	leaveHandlers := handlers.NewLeaveHandlers(db, auditLogger)

	worksheetHandlers := handlers.NewWorksheetHandlers(container.DB, uploadService)
	galleryHandlers := handlers.NewGalleryHandlers(container.DB, uploadService)

	// 5. Router — pass all required dependencies to fulfill the routing signature[cite: 1]
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
		worksheetHandlers, // Stage 5 Integration[cite: 1]
		galleryHandlers,   // Stage 5 Integration[cite: 1]
	)

	// 6. Server — blocks here until SIGINT/SIGTERM, then drains cleanly[cite: 1]
	if err := app.RunServer(container, router); err != nil {
		log.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}