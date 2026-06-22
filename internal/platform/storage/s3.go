package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/yuddmm/meetgo-dating-server/internal/config"
)

// S3 stores objects in any S3-compatible service (MinIO, AWS S3, Cloudflare R2).
// Intended for production.
type S3 struct {
	client    *minio.Client
	bucket    string
	publicURL string
}

func newS3(cfg config.StorageConfig) (*S3, error) {
	if cfg.S3Endpoint == "" || cfg.S3Bucket == "" {
		return nil, fmt.Errorf("storage: s3 requires STORAGE_S3_ENDPOINT and STORAGE_S3_BUCKET")
	}
	client, err := minio.New(cfg.S3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure: cfg.S3UseSSL,
		Region: cfg.S3Region,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: s3 client: %w", err)
	}
	return &S3{
		client:    client,
		bucket:    cfg.S3Bucket,
		publicURL: strings.TrimRight(cfg.PublicURL, "/"),
	}, nil
}

// Put uploads the object and returns its public URL.
func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", fmt.Errorf("storage: s3 put: %w", err)
	}
	return s.publicURL + "/" + key, nil
}

// Delete removes the object.
func (s *S3) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("storage: s3 delete: %w", err)
	}
	return nil
}
