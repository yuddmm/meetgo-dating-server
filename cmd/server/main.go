package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/yuddmm/meetgo-dating-server/internal/auth"
	"github.com/yuddmm/meetgo-dating-server/internal/config"
	"github.com/yuddmm/meetgo-dating-server/internal/database"
	"github.com/yuddmm/meetgo-dating-server/internal/interest"
	"github.com/yuddmm/meetgo-dating-server/internal/platform/geoip"
	"github.com/yuddmm/meetgo-dating-server/internal/platform/storage"
	"github.com/yuddmm/meetgo-dating-server/internal/profile"
	"github.com/yuddmm/meetgo-dating-server/internal/server"

	_ "github.com/yuddmm/meetgo-dating-server/internal/docs" // generated swagger docs
)

// @title						MeetGo API
// @version					0.1.0
// @description				Backend API for the MeetGo dating app.
// @BasePath					/api/v1
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	// Cancel the context on SIGINT/SIGTERM for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	logger.Info("connected to database")

	// GeoIP (optional): enables the RU-region Russian-email rule when a database
	// is configured; otherwise resolution is disabled and the rule never fires.
	geo, err := geoip.New(cfg.GeoIPDBPath)
	if err != nil {
		return err
	}
	defer geo.Close()
	logger.Info("geoip", slog.Bool("enabled", geo.Enabled()))

	// Auth module wiring. The dev mailer logs OTP codes; a real provider is
	// added later.
	tokens := auth.NewTokenService(cfg.JWTSecret, cfg.AccessTTL)
	authHandler := auth.NewHandler(
		auth.NewService(auth.ServiceParams{
			Repo:           auth.NewRepository(pool),
			Tokens:         tokens,
			Mailer:         auth.NewLogMailer(logger),
			OTPTTL:         cfg.OTPTTL,
			OTPResendAfter: cfg.OTPResendAfter,
			RefreshTTL:     cfg.RefreshTTL,
			DevMode:        cfg.IsDev(),
		}),
		tokens,
		geo,
		cfg.IsDev(),
	)

	// Photo storage provider chosen by config (local FS in dev, S3 in prod).
	store, err := storage.New(cfg.Storage)
	if err != nil {
		return err
	}
	logger.Info("storage initialized", slog.String("provider", cfg.Storage.Provider))

	interestHandler := interest.NewHandler(interest.NewRepository(pool))
	profileHandler := profile.NewHandler(profile.NewService(profile.NewRepository(pool), store))

	router := server.NewRouter(server.Deps{
		Logger:   logger,
		Pool:     pool,
		Auth:     authHandler,
		Interest: interestHandler,
		Profile:  profileHandler,
		Storage:  store,
	})
	srv := server.New(cfg, logger, router)

	return srv.Run(ctx)
}

func newLogger(cfg *config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	var h slog.Handler
	if cfg.Env == "prod" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h)
}
