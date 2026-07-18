package services

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"

	"github.com/Its-Ameekh/school_software_backend/internal/config"
)

// Asset type identifiers, used to select the right validation rules and
// object-key prefix.
const (
	AssetTypeWorksheet = "worksheet"
	AssetTypeGallery   = "gallery"

	// presignTTL is locked in at 15 minutes per the Stage 5 architecture
	// decision — this is also what Step F's "wait past 15 minutes" test
	// exercises.
	presignTTL = 15 * time.Minute
)

var (
	worksheetAllowedTypes = map[string]bool{
		"application/pdf": true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
		"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
		"image/jpeg": true,
		"image/png":  true,
	}
	galleryAllowedTypes = map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
	}

	worksheetMaxBytes int64 = 20 * 1024 * 1024
	galleryMaxBytes   int64 = 8 * 1024 * 1024

	// unsafeFilenameChars strips anything that isn't a safe filename
	// character before a filename is embedded in an object key.
	unsafeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
)

// PresignedUploadService owns everything about the two-step staged upload
// flow: validating a requested asset, building its server-generated
// object key, issuing the presigned PUT, and deleting/checking objects in
// R2 later.
type PresignedUploadService struct {
	client    *s3.Client
	cfg       *config.Config
	presigner *s3.PresignClient
}

// NewPresignedUploadService wires a PresignedUploadService from an
// already-constructed R2 client (see NewR2Client) and app config.
func NewPresignedUploadService(client *s3.Client, cfg *config.Config) *PresignedUploadService {
	return &PresignedUploadService{
		client:    client,
		cfg:       cfg,
		presigner: s3.NewPresignClient(client),
	}
}

// ValidateAssetUpload enforces the per-asset-type size cap and allowed
// content-type list. This runs BEFORE a presigned URL is ever issued
// (Step D of the test plan) — an invalid request should never reach R2.
func (s *PresignedUploadService) ValidateAssetUpload(assetType, contentType string, sizeBytes int64) error {
	if sizeBytes <= 0 {
		return fmt.Errorf("file_size_bytes must be greater than zero")
	}

	var allowed map[string]bool
	var max int64
	switch assetType {
	case AssetTypeWorksheet:
		allowed, max = worksheetAllowedTypes, worksheetMaxBytes
	case AssetTypeGallery:
		allowed, max = galleryAllowedTypes, galleryMaxBytes
	default:
		return fmt.Errorf("unknown asset type %q", assetType)
	}

	if !allowed[contentType] {
		return fmt.Errorf("content_type %q is not permitted for %s uploads", contentType, assetType)
	}
	if sizeBytes > max {
		return fmt.Errorf("file_size_bytes %d exceeds the %d byte cap for %s uploads", sizeBytes, max, assetType)
	}
	return nil
}

// sanitizeFilename strips any path component and replaces anything that
// isn't alphanumeric, '.', '_', or '-' with '_'. This runs before the
// filename is ever embedded in a server-generated object key, so a
// filename like "../../etc/passwd" can't escape its intended prefix.
func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "file"
	}
	if idx := strings.LastIndexAny(name, `/\`); idx >= 0 {
		name = name[idx+1:]
	}
	name = unsafeFilenameChars.ReplaceAllString(name, "_")
	if len(name) > 140 {
		name = name[len(name)-140:]
	}
	return name
}

// BuildObjectKey produces the server-generated object key for an asset:
// "worksheets/{class_id}/{uuid}-{sanitized_filename}" or
// "gallery/{class_id}/{uuid}-{sanitized_filename}". The client never
// supplies or influences the key beyond the original filename.
func (s *PresignedUploadService) BuildObjectKey(assetType string, classID uint, filename string) string {
	prefix := "worksheets"
	if assetType == AssetTypeGallery {
		prefix = "gallery"
	}
	return fmt.Sprintf("%s/%d/%s-%s", prefix, classID, uuid.NewString(), sanitizeFilename(filename))
}

// ExtractClassIDFromKey parses the {class_id} segment back out of a
// server-generated object key. Used to cross-check a confirm request's
// class_id against the class scope actually embedded in the storage_key
// (closes Stage 5 open item #3).
func ExtractClassIDFromKey(key string) (uint, error) {
	parts := strings.Split(key, "/")
	if len(parts) < 2 {
		return 0, fmt.Errorf("storage_key %q is not in the expected {prefix}/{class_id}/... format", key)
	}
	id, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("storage_key %q does not have a numeric class id segment: %w", key, err)
	}
	return uint(id), nil
}

// GeneratePresignedPutURL issues a presigned PUT URL valid for 15 minutes,
// with Content-Type and Content-Length pinned as signed conditions — R2
// will reject a PUT whose actual headers don't match these, which is what
// Step F exercises.
func (s *PresignedUploadService) GeneratePresignedPutURL(ctx context.Context, key, contentType string, sizeBytes int64) (string, time.Time, error) {
	out, err := s.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.cfg.R2Bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(sizeBytes),
	}, s3.WithPresignExpires(presignTTL))
	if err != nil {
		return "", time.Time{}, err
	}
	return out.URL, time.Now().Add(presignTTL), nil
}

// ObjectExists does a HeadObject check against R2. Used by the confirm
// handlers to reject a confirm call whose storage_key was never actually
// uploaded (closes Stage 5 open item #2).
//
// NOTE: this treats ANY HeadObject error as "does not exist" rather than
// distinguishing a 404 from a transient R2/network error. That's
// deliberate for now — a transient error just means the confirm fails and
// the client can retry — but if you want NotFound-vs-real-error precision,
// inspect the returned error for a smithy *http.ResponseError with
// StatusCode 404 rather than treating them the same.
func (s *PresignedUploadService) ObjectExists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.cfg.R2Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return false, nil
	}
	return true, nil
}

// DeleteObject removes an object from R2. Handlers call this BEFORE
// deleting the corresponding DB row (see class_ownership.go / handlers)
// so a failed R2 delete never leaves an orphaned DB pointer.
func (s *PresignedUploadService) DeleteObject(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.cfg.R2Bucket),
		Key:    aws.String(key),
	})
	return err
}

// PublicFileURL builds the public-facing URL for an object key using the
// configured public base domain (fronting the bucket, e.g. via a custom
// R2 domain or CDN).
func (s *PresignedUploadService) PublicFileURL(key string) string {
	return strings.TrimRight(s.cfg.R2PublicBaseURL, "/") + "/" + key
}

// ExtractObjectKey reverses PublicFileURL, given a stored file_url/url
// value.
func (s *PresignedUploadService) ExtractObjectKey(fileURL string) (string, error) {
	base := strings.TrimRight(s.cfg.R2PublicBaseURL, "/") + "/"
	if !strings.HasPrefix(fileURL, base) {
		return "", fmt.Errorf("file_url %q does not match the configured public base URL", fileURL)
	}
	return strings.TrimPrefix(fileURL, base), nil
}