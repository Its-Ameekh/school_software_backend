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
)

// @title           School Software API
// @version         1.0
// @description     Backend API for the School Software preschool management system.
// @BasePath        /

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

	// 4. Container — bundles config/logger/db
	container := app.New(cfg, log, db)

	// [Stage 3 Core Singletons]
	// Initialize JWKS Auth Middleware using the base Supabase URL from config
	authMW, err := middleware.NewAuthMiddleware(context.Background(), db, cfg.SupabaseURL)
	if err != nil {
		log.Error("failed to initialize auth middleware", "error", err)
		os.Exit(1)
	}

	// Initialize the structural rate limiter singleton
	limiter := middleware.NewRateLimiter()

	// Initialize the authentication route handler engines
	authHandlers := handlers.NewAuthHandlers(db)

	// 5. Router — pass all 4 required dependencies to fulfill the routing signature
	router := app.NewRouter(container, authMW, limiter, authHandlers)

	// 6. Server — blocks here until SIGINT/SIGTERM, then drains cleanly
	if err := app.RunServer(container, router); err != nil {
		log.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}
