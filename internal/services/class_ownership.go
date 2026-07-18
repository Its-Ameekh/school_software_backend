package services

import (
	"gorm.io/gorm"
)

// ClassOwnershipInfo holds the subset of the classes table needed to
// decide whether an actor is authorized to touch a given class's assets.
// Column names (teacher_id, substitute_teacher_id, substitute_active)
// match the real Goose migration for the classes table.
type ClassOwnershipInfo struct {
	ID                  uint
	TeacherID           *uint
	SubstituteTeacherID *uint
	SubstituteActive    bool
}

// GetClassOwnershipInfo loads ownership-relevant columns for a class.
// Returns gorm.ErrRecordNotFound (unwrapped, check with errors.Is) if the
// class doesn't exist — callers are expected to translate that into a 404,
// not a generic 500 or 403.
func GetClassOwnershipInfo(db *gorm.DB, classID uint) (*ClassOwnershipInfo, error) {
	var info ClassOwnershipInfo
	err := db.Table("classes").
		Select("id, teacher_id, substitute_teacher_id, substitute_active").
		Where("id = ?", classID).
		Take(&info).Error
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// IsAuthorizedForClass applies the Stage 5 class ownership rule:
//   - PRINCIPAL: always authorized.
//   - TEACHER: authorized only if they are the assigned teacher_id, OR
//     they are substitute_teacher_id AND substitute_active is true.
//   - anything else (e.g. PARENT): never authorized.
func IsAuthorizedForClass(info *ClassOwnershipInfo, actorID uint, role string) bool {
	switch role {
	case "PRINCIPAL":
		return true
	case "TEACHER":
		if info.TeacherID != nil && *info.TeacherID == actorID {
			return true
		}
		if info.SubstituteActive && info.SubstituteTeacherID != nil && *info.SubstituteTeacherID == actorID {
			return true
		}
		return false
	default:
		return false
	}
}

// CheckClassOwnership is a convenience wrapper combining the two calls
// above for callers that don't need the raw ClassOwnershipInfo.
func CheckClassOwnership(db *gorm.DB, classID uint, actorID uint, role string) (bool, error) {
	info, err := GetClassOwnershipInfo(db, classID)
	if err != nil {
		return false, err
	}
	return IsAuthorizedForClass(info, actorID, role), nil
}