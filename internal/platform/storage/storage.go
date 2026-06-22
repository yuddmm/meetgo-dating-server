// Package storage provides a provider-agnostic object store for photos. The
// concrete provider (local filesystem or S3-compatible) is selected by config
// and injected via the Storage interface — callers never branch on it.
package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/yuddmm/meetgo-dating-server/internal/config"
)

// Storage stores and removes objects by key, returning a publicly reachable URL.
type Storage interface {
	// Put stores the object under key and returns its public URL.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error)
	// Delete removes the object at key (no error if it is already gone).
	Delete(ctx context.Context, key string) error
}

// New builds the Storage provider selected by cfg.Provider.
func New(cfg config.StorageConfig) (Storage, error) {
	switch cfg.Provider {
	case "s3":
		return newS3(cfg)
	case "local", "":
		return newLocal(cfg)
	default:
		return nil, fmt.Errorf("storage: unknown provider %q", cfg.Provider)
	}
}
