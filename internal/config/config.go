package config

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config holds every environment-derived setting the app needs at boot.
type Config struct {
	Environment string // "dev" | "staging" | "prod"
	Port        string

	DatabaseURL string // Supabase Postgres connection string

	SupabaseURL string

	R2AccessKey string
	R2SecretKey string
	R2Bucket    string
	R2Endpoint  string
}

// Load reads .env (if present) and all required env vars into a Config.
// It fatals immediately if anything required is missing, so a bad
// deploy fails at boot instead of failing mysteriously later.
func Load() *Config {
	// In prod, real env vars are already set by the platform, so a
	// missing .env file here is fine — don't fatal on that.
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, relying on real environment variables")
	}

	cfg := &Config{
		Environment: getEnvDefault("ENVIRONMENT", "dev"),
		Port:        getEnvDefault("PORT", "8080"),

		DatabaseURL: os.Getenv("DATABASE_URL"),

		SupabaseURL: os.Getenv("SUPABASE_URL"),

		R2AccessKey: os.Getenv("R2_ACCESS_KEY"),
		R2SecretKey: os.Getenv("R2_SECRET_KEY"),
		R2Bucket:    os.Getenv("R2_BUCKET"),
		R2Endpoint:  os.Getenv("R2_ENDPOINT"),
	}

	cfg.mustValidate()

	return cfg
}

// mustValidate fatals with a specific message if any required field
// is empty. Port/Environment are excluded since they have defaults.
func (c *Config) mustValidate() {
	required := map[string]string{
		"DATABASE_URL":  c.DatabaseURL,
		"SUPABASE_URL":  c.SupabaseURL,
		"R2_ACCESS_KEY": c.R2AccessKey,
		"R2_SECRET_KEY": c.R2SecretKey,
		"R2_BUCKET":     c.R2Bucket,
		"R2_ENDPOINT":   c.R2Endpoint,
	}

	var missing []string
	for name, val := range required {
		if val == "" {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		log.Fatal(formatMissingErr(missing))
	}
}

func formatMissingErr(missing []string) string {
	return fmt.Sprintf("config: missing required environment variable(s): %v", missing)
}

func getEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
