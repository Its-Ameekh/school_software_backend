package app

import (
	"log/slog"

	"gorm.io/gorm"

	"github.com/Its-Ameekh/school_software_backend/internal/config"
)

// Container holds every shared dependency the app needs — config,
// logger, DB connection — and gets passed down to routes/handlers
// instead of relying on package-level globals.
type Container struct {
	Config *config.Config
	Logger *slog.Logger
	DB     *gorm.DB
}

// New wires up a Container from already-initialized dependencies.
// It doesn't do any initialization itself — that happens in main.go,
// in order, so failures are attributable to the exact step that failed.
func New(cfg *config.Config, logger *slog.Logger, db *gorm.DB) *Container {
	return &Container{
		Config: cfg,
		Logger: logger,
		DB:     db,
	}
}
