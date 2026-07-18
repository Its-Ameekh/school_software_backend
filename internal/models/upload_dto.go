package models

import "time"

// UploadURLRequest is the shared shape for POST /worksheets/upload-url and
// POST /gallery/upload-url. Content-Type and Content-Length declared here
// are the exact values pinned as signed conditions on the presigned PUT —
// they are not just used for app-layer validation.
type UploadURLRequest struct {
	ClassID       uint   `json:"class_id" binding:"required"`
	Filename      string `json:"filename" binding:"required"`
	ContentType   string `json:"content_type" binding:"required"`
	FileSizeBytes int64  `json:"file_size_bytes" binding:"required"`
}

// UploadURLResponse is returned from both upload-url endpoints.
type UploadURLResponse struct {
	UploadURL  string    `json:"upload_url"`
	StorageKey string    `json:"storage_key"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// ConfirmWorksheetRequest is the body for POST /worksheets. ClassID and
// ContentType/FileSizeBytes are re-sent (rather than trusted from the
// upload-url step) so this endpoint can independently re-validate
// everything before writing a DB row.
type ConfirmWorksheetRequest struct {
	ClassID       uint   `json:"class_id" binding:"required"`
	StorageKey    string `json:"storage_key" binding:"required"`
	Title         string `json:"title" binding:"required"`
	ContentType   string `json:"content_type" binding:"required"`
	FileSizeBytes int64  `json:"file_size_bytes" binding:"required"`
}

// ConfirmGalleryRequest is the body for POST /gallery.
type ConfirmGalleryRequest struct {
	ClassID       uint   `json:"class_id" binding:"required"`
	StorageKey    string `json:"storage_key" binding:"required"`
	Caption       string `json:"caption"`
	ContentType   string `json:"content_type" binding:"required"`
	FileSizeBytes int64  `json:"file_size_bytes" binding:"required"`
}