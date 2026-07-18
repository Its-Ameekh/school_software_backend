package app

import (
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gorm.io/gorm"

	"github.com/Its-Ameekh/school_software_backend/internal/config"
)

// Container holds every shared dependency the app needs — config,
// logger, DB connection, and storage clients — and gets passed down to
// routes/handlers instead of relying on package-level globals[cite: 1].
type Container struct {
	Config *config.Config
	Logger *slog.Logger
	DB     *gorm.DB
	R2     *s3.Client // Stage 5: Cloudflare R2 AWS SDK Client[cite: 1]
}

// New wires up a Container from already-initialized dependencies.
// It doesn't do any initialization itself — that happens in main.go,
// in order, so failures are attributable to the exact step that failed[cite: 1].
func New(cfg *config.Config, logger *slog.Logger, db *gorm.DB, r2Client *s3.Client) *Container {
	return &Container{
		Config: cfg,
		Logger: logger,
		DB:     db,
		R2:     r2Client, // Stage 5 Integration[cite: 1]
	}
}