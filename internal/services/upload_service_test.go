package services

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"

	"github.com/Its-Ameekh/school_software_backend/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		R2Bucket:        "test-bucket",
		R2PublicBaseURL: "https://files.example.com",
	}
}

// testS3Client builds a real (but inert) s3.Client. NewPresignedUploadService
// constructs an s3.PresignClient internally, which panics on a nil client —
// so tests that never actually make a network call still need a non-nil
// client to avoid that panic. No requests are made against this in the
// tests below, so bogus credentials/endpoint are fine.
func testS3Client() *s3.Client {
	return s3.New(s3.Options{
		Region:       "auto",
		BaseEndpoint: aws.String("https://example.invalid"),
	})
}

func TestValidateAssetUpload(t *testing.T) {
	// ValidateAssetUpload never touches R2, but the constructor still needs
	// a non-nil client — see testS3Client above.
	svc := NewPresignedUploadService(testS3Client(), testConfig())

	cases := []struct {
		name        string
		assetType   string
		contentType string
		size        int64
		wantErr     bool
	}{
		{"worksheet pdf ok", AssetTypeWorksheet, "application/pdf", 10 * 1024 * 1024, false},
		{"worksheet at cap ok", AssetTypeWorksheet, "application/pdf", 20 * 1024 * 1024, false},
		{"worksheet over cap", AssetTypeWorksheet, "application/pdf", 21 * 1024 * 1024, true},
		{"worksheet disallowed type", AssetTypeWorksheet, "video/mp4", 1024, true},
		{"gallery webp ok", AssetTypeGallery, "image/webp", 1024, false},
		{"gallery at cap ok", AssetTypeGallery, "image/png", 8 * 1024 * 1024, false},
		{"gallery over cap", AssetTypeGallery, "image/png", 9 * 1024 * 1024, true},
		{"gallery disallowed type", AssetTypeGallery, "application/pdf", 1024, true},
		{"zero size", AssetTypeWorksheet, "application/pdf", 0, true},
		{"negative size", AssetTypeWorksheet, "application/pdf", -5, true},
		{"unknown asset type", "video", "video/mp4", 1024, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.ValidateAssetUpload(tc.assetType, tc.contentType, tc.size)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	assert.Equal(t, "my_worksheet.pdf", sanitizeFilename("my worksheet.pdf"))
	assert.Equal(t, "evil.sh", sanitizeFilename("../../etc/evil.sh"))
	assert.Equal(t, "file", sanitizeFilename(""))
	assert.Equal(t, "photo.jpg", sanitizeFilename(`C:\Users\me\photo.jpg`))
}

func TestBuildObjectKey(t *testing.T) {
	svc := NewPresignedUploadService(testS3Client(), testConfig())

	key := svc.BuildObjectKey(AssetTypeWorksheet, 42, "homework.pdf")
	assert.Contains(t, key, "worksheets/42/")
	assert.Contains(t, key, "homework.pdf")

	key2 := svc.BuildObjectKey(AssetTypeGallery, 7, "photo.png")
	assert.Contains(t, key2, "gallery/7/")
	assert.Contains(t, key2, "photo.png")

	// Two calls for the same input should never collide (uuid-based).
	key3 := svc.BuildObjectKey(AssetTypeWorksheet, 42, "homework.pdf")
	assert.NotEqual(t, key, key3)
}

func TestPublicFileURLAndExtractObjectKey(t *testing.T) {
	svc := NewPresignedUploadService(testS3Client(), testConfig())

	key := "worksheets/42/abc-homework.pdf"
	url := svc.PublicFileURL(key)
	assert.Equal(t, "https://files.example.com/worksheets/42/abc-homework.pdf", url)

	extracted, err := svc.ExtractObjectKey(url)
	assert.NoError(t, err)
	assert.Equal(t, key, extracted)

	_, err = svc.ExtractObjectKey("https://not-my-domain.com/foo")
	assert.Error(t, err)
}

func TestExtractClassIDFromKey(t *testing.T) {
	id, err := ExtractClassIDFromKey("worksheets/42/abc-homework.pdf")
	assert.NoError(t, err)
	assert.Equal(t, uint(42), id)

	_, err = ExtractClassIDFromKey("worksheets")
	assert.Error(t, err)

	_, err = ExtractClassIDFromKey("worksheets/not-a-number/abc.pdf")
	assert.Error(t, err)
}