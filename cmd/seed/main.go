package main

import (
	"flag"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/Its-Ameekh/school_software_backend/internal/seed"
)

func main() {
	reset := flag.Bool("reset", false, "wipe existing data before seeding")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL") // use whatever env var name you agreed on with Eng A
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
