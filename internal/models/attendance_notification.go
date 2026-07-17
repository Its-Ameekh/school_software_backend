package models

import "time"

// Trigger reasons written to attendance_notifications.trigger_reason.
// Kept as typed constants so handlers can't typo the string.
const (
TriggerInitialAbsent = "INITIAL_ABSENT"
TriggerEditToPresent = "EDIT_TO_PRESENT"
)

// Notification status values.
const (
NotificationPending = "PENDING"
NotificationSent    = "SENT"
NotificationFailed  = "FAILED"
)

// AttendanceNotification is an audit-trail row queued whenever an
// attendance mark requires an SMS to a guardian: either a student's
// first absence mark for the day, or an edit that reverses an existing
// absence back to present. This handler layer only ever INSERTs rows
// here -- actually sending the SMS is a separate process's job.
type AttendanceNotification struct {
ID             uint       `gorm:"column:id;primaryKey"`
AttendanceID   uint       `gorm:"column:attendance_id"`
GuardianUserID uint       `gorm:"column:guardian_user_id"`
TriggerReason  string     `gorm:"column:trigger_reason"`
SentAt         *time.Time `gorm:"column:sent_at"`
Status         string     `gorm:"column:status;default:PENDING"`
}

func (AttendanceNotification) TableName() string { return "attendance_notifications" }

// AttendanceSubmission marks that a teacher has submitted (and thus
// locked, pending the 4:00 PM cutoff / Principal override) attendance
// for a given class on a given date. Composite primary key on
// (class_id, date) -- one submission record per class per day.
type AttendanceSubmission struct {
ClassID     uint      `gorm:"column:class_id;primaryKey"`
Date        time.Time `gorm:"column:date;primaryKey"`
SubmittedBy uint      `gorm:"column:submitted_by"`
SubmittedAt time.Time `gorm:"column:submitted_at"`
}

func (AttendanceSubmission) TableName() string { return "attendance_submissions" }
