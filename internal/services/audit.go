package services

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"
)

// AuditAction is a closed set so callers can't typo the action string.
type AuditAction string

const (
	AuditCreate AuditAction = "CREATE"
	AuditUpdate AuditAction = "UPDATE"
	AuditDelete AuditAction = "DELETE" // soft-delete, per your schema
)

// AuditLogEntry mirrors the audit_log table. before/after are
// json.RawMessage, not map[string]interface{} — GORM writes RawMessage
// straight through to a JSONB column with no marshal/unmarshal
// round-trip on the hot path, and it lets a handler pass either a
// pre-serialized diff or nil without type gymnastics.
type AuditLogEntry struct {
	ActorID  uint            `gorm:"column:actor_id"`
	Action   AuditAction     `gorm:"column:action"`
	Entity   string          `gorm:"column:entity"` // e.g. "student", "fee_payment"
	EntityID uint            `gorm:"column:entity_id"`
	Before   json.RawMessage `gorm:"column:before_state;type:jsonb"`
	After    json.RawMessage `gorm:"column:after_state;type:jsonb"`
}

type AuditLogger struct {
	db *gorm.DB
}

func NewAuditLogger(db *gorm.DB) *AuditLogger {
	return &AuditLogger{db: db}
}

// Log writes one audit entry. before/after should be structs, not
// pre-marshaled JSON — this handles serialization so handlers don't
// each reimplement it slightly differently.
//
//	auditLogger.Log(ctx, actorID, services.AuditUpdate, "student", student.ID, oldStudent, newStudent)
//
// For CREATE, pass before=nil. For DELETE, pass after=nil.
func (a *AuditLogger) Log(ctx context.Context, actorID uint, action AuditAction, entity string, entityID uint, before, after any) error {
	entry := AuditLogEntry{
		ActorID:  actorID,
		Action:   action,
		Entity:   entity,
		EntityID: entityID,
	}

	if before != nil {
		b, err := json.Marshal(before)
		if err != nil {
			return err
		}
		entry.Before = b
	}
	if after != nil {
		a, err := json.Marshal(after)
		if err != nil {
			return err
		}
		entry.After = a
	}

	return a.db.WithContext(ctx).Table("audit_log").Create(&entry).Error
}

// AlertFailure provides a uniform internal-alert logging pipeline across the system
// when an append-only mutation fails to record cleanly to the database ledger.
func (a *AuditLogger) AlertFailure(entity string, entityID uint, action AuditAction, err error) {
	println("[AUDIT_ALERT_FAILURE] Failed to write mutation trail for entity=" + entity + " during action=" + string(action) + ": " + err.Error())
}