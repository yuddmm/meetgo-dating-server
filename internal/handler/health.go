// Package handler contains HTTP handlers.
package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Health groups liveness and readiness handlers.
type Health struct {
	pool *pgxpool.Pool
}

// NewHealth constructs a Health handler.
func NewHealth(pool *pgxpool.Pool) *Health {
	return &Health{pool: pool}
}

// Healthz is a liveness probe: it always returns 200 if the process is up.
func (h *Health) Healthz(w http.ResponseWriter, _ *http.Request) {
	writeText(w, http.StatusOK, "ok")
}

// Readyz is a readiness probe: it returns 200 only when the database is
// reachable, otherwise 503.
func (h *Health) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.pool.Ping(ctx); err != nil {
		writeText(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeText(w, http.StatusOK, "ready")
}

func writeText(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg))
}
