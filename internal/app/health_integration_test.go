package app_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Its-Ameekh/school_software_backend/internal/app"
	"github.com/Its-Ameekh/school_software_backend/internal/config"
	"github.com/Its-Ameekh/school_software_backend/internal/database"
	"github.com/Its-Ameekh/school_software_backend/internal/handlers"
	"github.com/Its-Ameekh/school_software_backend/internal/middleware"
	"github.com/Its-Ameekh/school_software_backend/internal/services"
)

// TestDatabaseConnection confirms the DB connection layer works end to
// end: real connect, real ping, against whatever DATABASE_URL points to.
func TestDatabaseConnection(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	db, err := database.Connect(dbURL, logger)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	if err := database.Ping(db); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

// TestHealthEndpoint spins up the real router against a real DB
// connection and confirms GET /health returns 200 + {"status":"healthy"}.
func TestHealthEndpoint(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	db, err := database.Connect(dbURL, logger)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	cfg := &config.Config{
		Environment: "test",
		Port:        "80",
		DatabaseURL: dbURL,
		SupabaseURL: "https://mockproject.supabase.co",
	}

	// Satisfy Stage 5 signature by appending nil for the *s3.Client
	container := app.New(cfg, logger, db, nil)

	ctx := context.Background()
	authMW, err := middleware.NewAuthMiddleware(ctx, db, cfg.SupabaseURL)
	if err != nil {
		t.Fatalf("failed to initialize auth middleware for test: %v", err)
	}
	limiter := middleware.NewRateLimiter()

	// Initialize all required production domains
	authHandlers := handlers.NewAuthHandlers(db, nil)
	auditLogger := services.NewAuditLogger(db)
	financeHandlers := handlers.NewFinanceHandlers(db, auditLogger)
	progressHandlers := handlers.NewProgressHandlers(db, auditLogger)

	// Live ledger automation for production student flows
	feeService := services.NewFeeLedgerService()
	studentHandlers := handlers.NewStudentHandlers(db, auditLogger, feeService, cfg.SupabaseURL, "mock-key")
	classHandlers := handlers.NewClassHandlers(db, auditLogger)
	leaveHandlers := handlers.NewLeaveHandlers(db, auditLogger)

	// Fully satisfied Stage 5 11-parameter router signature by appending nil for worksheet and gallery handlers
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
		nil, // worksheetHandlers
		nil, // galleryHandlers
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}

	if body["status"] != "healthy" {
		t.Fatalf("expected status 'healthy', got %q", body["status"])
	}
}