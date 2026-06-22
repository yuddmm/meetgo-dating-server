package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/yuddmm/meetgo-dating-server/internal/config"
)

// Local stores objects on the local filesystem and serves them as static files.
// Intended for development (zero extra processes).
type Local struct {
	baseDir   string
	publicURL string // e.g. http://localhost:8080/uploads
	mountPath string // path component of publicURL, e.g. /uploads
}

func newLocal(cfg config.StorageConfig) (*Local, error) {
	if err := os.MkdirAll(cfg.LocalDir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: create local dir: %w", err)
	}
	u, err := url.Parse(cfg.PublicURL)
	if err != nil {
		return nil, fmt.Errorf("storage: parse public url: %w", err)
	}
	mount := u.Path
	if mount == "" {
		mount = "/uploads"
	}
	return &Local{
		baseDir:   cfg.LocalDir,
		publicURL: strings.TrimRight(cfg.PublicURL, "/"),
		mountPath: "/" + strings.Trim(mount, "/"),
	}, nil
}

// Put writes the object to baseDir/key and returns its public URL.
func (l *Local) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) (string, error) {
	full := filepath.Join(l.baseDir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", fmt.Errorf("storage: mkdir: %w", err)
	}
	f, err := os.Create(full)
	if err != nil {
		return "", fmt.Errorf("storage: create file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", fmt.Errorf("storage: write file: %w", err)
	}
	return l.publicURL + "/" + key, nil
}

// Delete removes the file for key (ignoring "not found").
func (l *Local) Delete(_ context.Context, key string) error {
	err := os.Remove(filepath.Join(l.baseDir, filepath.FromSlash(key)))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: delete file: %w", err)
	}
	return nil
}

// Register mounts a static file server so stored files are reachable at publicURL.
// Implemented only by the local provider (the router type-asserts for it).
func (l *Local) Register(r chi.Router) {
	fs := http.FileServer(http.Dir(l.baseDir))
	r.Handle(path.Join(l.mountPath, "*"), http.StripPrefix(l.mountPath+"/", fs))
}
