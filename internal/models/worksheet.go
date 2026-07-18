package models

import "time"

// Worksheet represents a teacher-uploaded worksheet file attached to a
// class. Rows are only ever created via the confirm-upload step, after the
// frontend has already PUT the bytes to R2 using a presigned URL.
type Worksheet struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ClassID       uint      `gorm:"column:class_id;not null;index" json:"class_id"`
	Title         string    `gorm:"column:title;size:255;not null" json:"title"`
	StorageKey    string    `gorm:"column:storage_key;size:1024;not null;uniqueIndex" json:"storage_key"`
	FileURL       string    `gorm:"column:file_url;size:1024;not null;uniqueIndex" json:"file_url"`
	ContentType   string    `gorm:"column:content_type;size:100;not null" json:"content_type"`
	FileSizeBytes int64     `gorm:"column:file_size_bytes;not null" json:"file_size_bytes"`
	UploadedBy    uint      `gorm:"column:uploaded_by;not null" json:"uploaded_by"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
}

// TableName pins the table name explicitly rather than relying on GORM's
// pluralization, so it can't silently drift if the struct name changes.
func (Worksheet) TableName() string {
	return "worksheets"
}