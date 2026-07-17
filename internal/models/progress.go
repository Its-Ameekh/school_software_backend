package models

import "time"

// ProgressScore is one subject's numeric mark for one student in one
// term. The unique constraint on (student_id, term, subject) means an
// "entry" endpoint should UPSERT (update if the row already exists),
// not blindly INSERT and risk a constraint violation on re-entry.
type ProgressScore struct {
ID          uint      `gorm:"column:id;primaryKey"`
StudentID   uint      `gorm:"column:student_id"`
Term        string    `gorm:"column:term"`
Subject     string    `gorm:"column:subject"`
MaxScore    float64   `gorm:"column:max_score"`
ScoredValue float64   `gorm:"column:scored_value"`
GradeValue  string    `gorm:"column:grade_value"`
GradedBy    uint      `gorm:"column:graded_by"`
UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (ProgressScore) TableName() string { return "progress_scores" }

// ProgressRemark is the single free-text comment for one student for
// one term -- note the primary key is (student_id, term), NOT an
// auto-incrementing id, so there is only ever one remarks row per
// student per term. Writing a new remark for the same term must
// UPDATE the existing row, not insert a second one.
type ProgressRemark struct {
StudentID uint      `gorm:"column:student_id;primaryKey"`
Term      string    `gorm:"column:term;primaryKey"`
Remarks   string    `gorm:"column:remarks"`
WrittenBy uint      `gorm:"column:written_by"`
UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (ProgressRemark) TableName() string { return "progress_remarks" }
