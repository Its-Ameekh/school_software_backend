package services

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/Its-Ameekh/school_software_backend/internal/config"
)

// NewR2Client builds an AWS SDK v2 S3 client pointed at Cloudflare R2.
// Region is pinned to "auto" (R2 ignores region but the SDK requires a
// non-empty value), path-style addressing is forced on since R2 doesn't
// support virtual-hosted-style buckets the way AWS S3 does, and
// BaseEndpoint points at the account-specific R2 endpoint from config.
func NewR2Client(cfg *config.Config) *s3.Client {
	return s3.New(s3.Options{
		Region:       "auto",
		BaseEndpoint: aws.String(cfg.R2Endpoint),
		UsePathStyle: true,
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.R2AccessKey,
			cfg.R2SecretKey,
			"",
		),
	})
}