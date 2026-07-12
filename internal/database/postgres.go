package database

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Connect opens a GORM connection to the given Postgres DSN and
// configures sane pool limits for a small backend on Lightsail talking
// to Supabase's free-tier connection cap.
func Connect(databaseURL string, appLogger *slog.Logger) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("database: failed to connect: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("database: failed to get underlying sql.DB: %w", err)
	}

	// Pool limits — kept conservative since Supabase's free tier caps
	// concurrent connections. Tighten/loosen once you know real numbers.
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	// Fail fast at boot if the DB is unreachable, rather than finding
	// out on the first real request.
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("database: ping failed: %w", err)
	}

	appLogger.Info("database connection established")

	return db, nil
}

// Ping checks the connection is alive right now. Used by the /health
// endpoint (Stage 1, item #8) to report DB status on demand, rather
// than relying on the one-time check at boot.
func Ping(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("database: failed to get underlying sql.DB: %w", err)
	}
	return sqlDB.Ping()
}
