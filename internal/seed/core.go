package seed

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Its-Ameekh/school_software_backend/internal/models"
)

func SeedUsers(db *gorm.DB) ([]models.User, error) {
	var users []models.User
	roles := []string{"ADMIN", "TEACHER", "TEACHER", "TEACHER", "GUARDIAN", "GUARDIAN", "GUARDIAN"}
	for i, role := range roles {
		email := fmt.Sprintf("%s%d@starlight.test", role, i+1)
		users = append(users, models.User{
			AuthID: uuid.NewString(),
			Email:  &email,
			Role:   role,
			Name:   fmt.Sprintf("%s User %d", role, i+1),
			Phone:  fmt.Sprintf("98765430%02d", i),
		})
	}
	if err := db.Create(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func SeedTeacherProfiles(db *gorm.DB, users []models.User) error {
	var profiles []models.TeacherProfile
	spec := "Mathematics"
	for _, u := range users {
		if u.Role != "TEACHER" {
			continue
		}
		profiles = append(profiles, models.TeacherProfile{
			UserID:         u.ID,
			Specialization: &spec,
			IsAvailableSub: true,
		})
	}
	if len(profiles) == 0 {
		return nil
	}
	return db.Create(&profiles).Error
}

func SeedClasses(db *gorm.DB, teacherUsers []models.User) ([]models.Class, error) {
	if len(teacherUsers) == 0 {
		return nil, fmt.Errorf("no teacher users to assign to classes")
	}
	names := []string{"Grade 8 - A", "Grade 9 - A", "Grade 10 - A"}
	var classes []models.Class
	for i, name := range names {
		teacherID := teacherUsers[i%len(teacherUsers)].ID
		classes = append(classes, models.Class{
			Name:      name,
			TeacherID: &teacherID,
		})
	}
	if err := db.Create(&classes).Error; err != nil {
		return nil, err
	}
	return classes, nil
}

func SeedStudents(db *gorm.DB, classes []models.Class) ([]models.Student, error) {
	genders := []string{"MALE", "FEMALE"}
	var students []models.Student
	for i := 0; i < 30; i++ {
		classID := classes[i%len(classes)].ID
		students = append(students, models.Student{
			RollNumber: fmt.Sprintf("R2026-%04d", i+1),
			FullName:   fmt.Sprintf("Student %d", i+1),
			DOB:        time.Date(2013, time.Month(i%12+1), 10, 0, 0, 0, 0, time.UTC),
			Gender:     genders[i%2],
			ClassID:    &classID,
			GradeTier:  "STANDARD",
		})
	}
	if err := db.Create(&students).Error; err != nil {
		return nil, err
	}
	return students, nil
}

func SeedGuardians(db *gorm.DB, students []models.Student, guardianUsers []models.User) ([]models.Guardian, error) {
	var guardians []models.Guardian
	for i, s := range students {
		var userID *uint
		if len(guardianUsers) > 0 {
			id := guardianUsers[i%len(guardianUsers)].ID
			userID = &id
		}
		guardians = append(guardians, models.Guardian{
			StudentID:           s.ID,
			UserID:              userID,
			FullName:            fmt.Sprintf("Guardian of %s", s.FullName),
			Relationship:        []string{"FATHER", "MOTHER"}[i%2],
			IsPrimaryContact:    true,
			AuthorizedForPickup: true,
		})
	}
	if err := db.Create(&guardians).Error; err != nil {
		return nil, err
	}
	return guardians, nil
}

func SeedAdmissionIntake(db *gorm.DB, students []models.Student) error {
	payModes := []string{"CASH", "ONLINE", "BANK"}
	var intakes []models.AdmissionIntake
	for i, s := range students {
		intakes = append(intakes, models.AdmissionIntake{
			StudentID:     s.ID,
			PayMode:       payModes[i%3],
			AmountPaid:    5000,
			ReceiptNumber: fmt.Sprintf("RCPT-%05d", i+1),
			TransportPref: []string{"BUS", "SELF"}[i%2],
			AdmittedAt:    time.Now(),
		})
	}
	return db.Create(&intakes).Error
}

func SeedFeeStructures(db *gorm.DB) ([]models.FeeStructure, error) {
	structures := []models.FeeStructure{
		{AcademicYear: "2026-2027", GradeTier: "STANDARD", InitialPayment: 5000, RegularFeeTotal: 40000},
	}
	if err := db.Create(&structures).Error; err != nil {
		return nil, err
	}
	return structures, nil
}

func SeedFeeTerms(db *gorm.DB, structures []models.FeeStructure) ([]models.FeeTerm, error) {
	var terms []models.FeeTerm
	for _, s := range structures {
		perTerm := s.RegularFeeTotal / 4
		for t := 1; t <= 4; t++ {
			terms = append(terms, models.FeeTerm{
				FeeStructureID: s.ID,
				TermNumber:     int8(t),
				Amount:         perTerm,
				DueDate:        time.Now().AddDate(0, t*2, 0),
			})
		}
	}
	if err := db.Create(&terms).Error; err != nil {
		return nil, err
	}
	return terms, nil
}

func SeedStudentFeeLedger(db *gorm.DB, students []models.Student, terms []models.FeeTerm) error {
	statuses := []string{"PENDING", "PAID", "OVERDUE"}
	var ledger []models.StudentFeeLedger
	for i, s := range students {
		for _, term := range terms {
			status := statuses[(i+int(term.TermNumber))%3]
			var paidAt *time.Time
			if status == "PAID" {
				now := time.Now()
				paidAt = &now
			}
			ledger = append(ledger, models.StudentFeeLedger{
				StudentID: s.ID,
				FeeTermID: term.ID,
				AmountDue: term.Amount,
				Status:    status,
				PaidAt:    paidAt,
			})
		}
	}
	return db.Create(&ledger).Error
}

func SeedAttendance(db *gorm.DB, students []models.Student, markedByUserID uint) error {
	statuses := []string{"PRESENT", "PRESENT", "PRESENT", "ABSENT", "LATE"}
	var records []models.Attendance
	for _, s := range students {
		for d := 0; d < 5; d++ {
			date := time.Now().AddDate(0, 0, -d)
			records = append(records, models.Attendance{
				StudentID: s.ID,
				ClassID:   s.ClassID,
				Date:      date,
				Status:    statuses[(int(s.ID)+d)%len(statuses)],
				MarkedBy:  &markedByUserID,
			})
		}
	}
	return db.Create(&records).Error
}
