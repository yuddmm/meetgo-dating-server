// Package server wires the HTTP server and its lifecycle.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/yuddmm/meetgo-dating-server/internal/config"
)

// Server wraps an http.Server with graceful shutdown.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
	shutdownTO time.Duration
}

// New constructs a Server from config and a handler.
func New(cfg *config.Config, logger *slog.Logger, h http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         ":" + cfg.HTTPPort,
			Handler:      h,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
		logger:     logger,
		shutdownTO: cfg.ShutdownTimeout,
	}
}

// Run starts the server and blocks until ctx is cancelled, then performs a
// graceful shutdown bounded by the configured shutdown timeout.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		s.logger.Info("http server starting", slog.String("addr", s.httpServer.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("shutdown signal received, draining connections")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTO)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		s.logger.Info("http server stopped cleanly")
		return nil
	}
}
