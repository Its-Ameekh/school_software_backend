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

	R2AccessKey     string
	R2SecretKey     string
	R2Bucket        string
	R2Endpoint      string
	R2PublicBaseURL string 

	SupabaseServiceRoleKey string // SECRET — server-side only, NEVER prefixed NEXT_PUBLIC_, never sent to frontend
}

// Load reads .env (if present) and all required env vars into a Config.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, relying on real environment variables")
	}

	cfg := &Config{
		Environment: getEnvDefault("APP_ENV", "dev"),
		Port:        getEnvDefault("PORT", "80"),

		DatabaseURL: os.Getenv("DATABASE_URL"),
		SupabaseURL: os.Getenv("SUPABASE_URL"),

		R2AccessKey:     os.Getenv("R2_ACCESS_KEY"),
		R2SecretKey:     os.Getenv("R2_SECRET_KEY"),
		R2Bucket:        os.Getenv("R2_BUCKET"),
		R2Endpoint:      os.Getenv("R2_ENDPOINT"),
		R2PublicBaseURL: os.Getenv("R2_PUBLIC_BASE_URL"),

		SupabaseServiceRoleKey: os.Getenv("SUPABASE_SERVICE_ROLE_KEY"),
	}

	cfg.mustValidate()

	return cfg
}

// mustValidate fatals with a specific message if any required field is empty.
func (c *Config) mustValidate() {
	required := map[string]string{
		"DATABASE_URL":              c.DatabaseURL,
		"SUPABASE_URL":              c.SupabaseURL,
		"R2_ACCESS_KEY":             c.R2AccessKey,
		"R2_SECRET_KEY":             c.R2SecretKey,
		"R2_BUCKET":                 c.R2Bucket,
		"R2_ENDPOINT":               c.R2Endpoint,
		"R2_PUBLIC_BASE_URL":        c.R2PublicBaseURL,
		"SUPABASE_SERVICE_ROLE_KEY": c.SupabaseServiceRoleKey,
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