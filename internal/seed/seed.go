package seed

import (
	"log"

	"gorm.io/gorm"

	"github.com/Its-Ameekh/school_software_backend/internal/models"
)

func Run(db *gorm.DB) error {
	var count int64
	db.Model(&models.User{}).Count(&count)
	if count > 0 {
		log.Println("Seed data already exists. Use --reset to wipe and reseed.")
		return nil
	}

	users, err := SeedUsers(db)
	if err != nil {
		return err
	}
	if err := SeedTeacherProfiles(db, users); err != nil {
		return err
	}

	var teacherUsers, guardianUsers []models.User
	for _, u := range users {
		if u.Role == "TEACHER" {
			teacherUsers = append(teacherUsers, u)
		}
		// Maps cleanly to PARENT context system constraints
		if u.Role == "PARENT" { 
			guardianUsers = append(guardianUsers, u)
		}
	}

	classes, err := SeedClasses(db, teacherUsers)
	if err != nil {
		return err
	}

	students, err := SeedStudents(db, classes)
	if err != nil {
		return err
	}

	if _, err := SeedGuardians(db, students, guardianUsers); err != nil {
		return err
	}

	if err := SeedAdmissionIntake(db, students); err != nil {
		return err
	}

	structures, err := SeedFeeStructures(db)
	if err != nil {
		return err
	}

	terms, err := SeedFeeTerms(db, structures)
	if err != nil {
		return err
	}

	if err := SeedStudentFeeLedger(db, students, terms); err != nil {
		return err
	}

	adminID := users[0].ID // First seeded user maps to the system administrator instance
	if err := SeedAttendance(db, students, adminID); err != nil {
		return err
	}

	log.Println("Core seed completed successfully with real Supabase Phone Authentication profiles.")
	return nil
}