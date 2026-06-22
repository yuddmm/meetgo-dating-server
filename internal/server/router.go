package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/yuddmm/meetgo-dating-server/internal/auth"
	"github.com/yuddmm/meetgo-dating-server/internal/handler"
	"github.com/yuddmm/meetgo-dating-server/internal/interest"
	"github.com/yuddmm/meetgo-dating-server/internal/platform/storage"
	"github.com/yuddmm/meetgo-dating-server/internal/profile"
)

// Deps holds the dependencies required to build the router.
type Deps struct {
	Logger   *slog.Logger
	Pool     *pgxpool.Pool
	Auth     *auth.Handler
	Interest *interest.Handler
	Profile  *profile.Handler
	Storage  storage.Storage
}

// NewRouter builds the application HTTP handler with middleware and routes.
func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(deps.Logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	health := handler.NewHealth(deps.Pool)
	r.Get("/healthz", health.Healthz)
	r.Get("/readyz", health.Readyz)

	// The local storage provider serves uploaded files statically; S3 does not.
	if reg, ok := deps.Storage.(interface{ Register(chi.Router) }); ok {
		reg.Register(r)
	}

	// Swagger UI, served from the generated internal/docs spec.
	r.Get("/swagger/*", httpSwagger.Handler(httpSwagger.URL("/swagger/doc.json")))

	// Versioned API namespace — domain routes are mounted here.
	r.Route("/api/v1", func(r chi.Router) {
		// Public + auth-managed routes (/auth/*, /me).
		deps.Auth.Routes(r)

		// Authenticated routes for the remaining domain modules.
		r.Group(func(r chi.Router) {
			r.Use(deps.Auth.Middleware)
			deps.Interest.Routes(r)
			deps.Profile.Routes(r)
		})
		// TODO: mount further domain routes (meetings, ...) here.
	})

	return r
}

// requestLogger logs each request using slog with method, path, status and duration.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()

			defer func() {
				logger.Info("http_request",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", ww.Status()),
					slog.Int("bytes", ww.BytesWritten()),
					slog.Duration("duration", time.Since(start)),
					slog.String("request_id", middleware.GetReqID(r.Context())),
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}
