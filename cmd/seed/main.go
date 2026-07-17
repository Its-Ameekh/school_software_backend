package main

import (
	"flag"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/Its-Ameekh/school_software_backend/internal/seed"
	"github.com/joho/godotenv"
)

func main() {
	// Automatically parses your .env file into memory so dsn isn't empty
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found, using system environment variables")
	}

	reset := flag.Bool("reset", false, "wipe existing data before seeding")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is blank or missing!")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}

	if *reset {
		db.Exec(`TRUNCATE TABLE 
            student_fee_ledger, fee_terms, fee_structures, 
            admission_intake, guardians, attendance, 
            students, classes, teacher_profiles, users 
            RESTART IDENTITY CASCADE`)
		log.Println("Reset complete")
	}

	if err := seed.Run(db); err != nil {
		log.Fatalf("seed failed: %v", err)
	}
}
