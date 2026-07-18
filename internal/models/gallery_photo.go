package models

import "time"

// GalleryPhoto represents a photo uploaded to a class's shared gallery.
// Same staged-upload lifecycle as Worksheet: presigned PUT to R2 first,
// then a confirm call persists this row.
type GalleryPhoto struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ClassID       uint      `gorm:"column:class_id;not null;index" json:"class_id"`
	Caption       string    `gorm:"column:caption;size:255" json:"caption"`
	StorageKey    string    `gorm:"column:storage_key;size:1024;not null;uniqueIndex" json:"storage_key"`
	URL           string    `gorm:"column:url;size:1024;not null;uniqueIndex" json:"url"`
	ContentType   string    `gorm:"column:content_type;size:100;not null" json:"content_type"`
	FileSizeBytes int64     `gorm:"column:file_size_bytes;not null" json:"file_size_bytes"`
	UploadedBy    uint      `gorm:"column:uploaded_by;not null" json:"uploaded_by"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"created_at"`
}

// TableName pins the table name explicitly (matches the "gallery_photos.url"
// column referenced in the Stage 5 open items).
func (GalleryPhoto) TableName() string {
	return "gallery_photos"
}