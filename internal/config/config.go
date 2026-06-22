// Package config loads application configuration from the environment.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration for the server.
type Config struct {
	Env             string        // "dev" | "prod"
	HTTPPort        string        // e.g. "8080"
	DatabaseURL     string        // postgres connection string (DSN)
	LogLevel        slog.Level    // slog log level
	ReadTimeout     time.Duration // http server read timeout
	WriteTimeout    time.Duration // http server write timeout
	IdleTimeout     time.Duration // http server idle timeout
	ShutdownTimeout time.Duration // graceful shutdown deadline

	// Auth
	JWTSecret      string        // HS256 signing secret for access tokens
	AccessTTL      time.Duration // access token lifetime
	RefreshTTL     time.Duration // refresh token lifetime
	OTPTTL         time.Duration // OTP code lifetime (also reported as expiresIn)
	OTPResendAfter time.Duration // per-email resend cooldown (reported as resendAfter)

	// Storage (photo object storage; provider chosen via STORAGE_PROVIDER)
	Storage StorageConfig

	// GeoIP database path (.mmdb). Empty disables geo resolution.
	GeoIPDBPath string
}

// StorageConfig configures the photo storage provider. Provider "local" uses the
// filesystem (dev); "s3" uses any S3-compatible service (MinIO/AWS S3/R2, prod).
type StorageConfig struct {
	Provider  string // "local" | "s3"
	PublicURL string // base URL prepended to object keys in returned photo URLs

	// local
	LocalDir string // directory where files are written

	// s3
	S3Endpoint  string
	S3Region    string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	S3UseSSL    bool
}

// IsDev reports whether the server runs in development mode.
func (c *Config) IsDev() bool { return c.Env == "dev" }

// Load reads configuration from environment variables, applying sensible
// defaults. A local .env file is loaded first if present (ignored otherwise).
// The only required value is DATABASE_URL.
func Load() (*Config, error) {
	// Best-effort: load .env for local development. Missing file is fine.
	_ = godotenv.Load()

	cfg := &Config{
		Env:             getEnv("ENV", "dev"),
		HTTPPort:        getEnv("HTTP_PORT", "8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		LogLevel:        parseLogLevel(getEnv("LOG_LEVEL", "info")),
		ReadTimeout:     getEnvDuration("HTTP_READ_TIMEOUT", 10*time.Second),
		WriteTimeout:    getEnvDuration("HTTP_WRITE_TIMEOUT", 10*time.Second),
		IdleTimeout:     getEnvDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 10*time.Second),

		JWTSecret:      os.Getenv("JWT_SECRET"),
		AccessTTL:      getEnvDuration("ACCESS_TTL", 15*time.Minute),
		RefreshTTL:     getEnvDuration("REFRESH_TTL", 60*24*time.Hour),
		OTPTTL:         getEnvDuration("OTP_TTL", 5*time.Minute),
		OTPResendAfter: getEnvDuration("OTP_RESEND_AFTER", 60*time.Second),

		Storage: StorageConfig{
			Provider:    getEnv("STORAGE_PROVIDER", "local"),
			PublicURL:   getEnv("STORAGE_PUBLIC_URL", "http://localhost:8080/uploads"),
			LocalDir:    getEnv("STORAGE_LOCAL_DIR", "./uploads"),
			S3Endpoint:  os.Getenv("STORAGE_S3_ENDPOINT"),
			S3Region:    getEnv("STORAGE_S3_REGION", "us-east-1"),
			S3Bucket:    os.Getenv("STORAGE_S3_BUCKET"),
			S3AccessKey: os.Getenv("STORAGE_S3_ACCESS_KEY"),
			S3SecretKey: os.Getenv("STORAGE_S3_SECRET_KEY"),
			S3UseSSL:    getEnvBool("STORAGE_S3_USE_SSL", true),
		},

		GeoIPDBPath: os.Getenv("GEOIP_DB_PATH"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL is required")
	}

	// In dev we allow a default signing secret for convenience; prod must set one.
	if cfg.JWTSecret == "" {
		if cfg.IsDev() {
			cfg.JWTSecret = "dev-insecure-secret-change-me"
		} else {
			return nil, fmt.Errorf("config: JWT_SECRET is required")
		}
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		// allow numeric levels too
		if n, err := strconv.Atoi(s); err == nil {
			return slog.Level(n)
		}
		return slog.LevelInfo
	}
}
