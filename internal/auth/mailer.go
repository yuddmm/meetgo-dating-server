package auth

import (
	"context"
	"log/slog"
)

// Mailer delivers OTP codes. Production implementations (SMTP/provider) are
// added later; for now only the dev logger exists.
type Mailer interface {
	SendOTP(ctx context.Context, email, code string) error
}

// LogMailer is the development mailer: it logs the code instead of sending mail.
type LogMailer struct {
	logger *slog.Logger
}

// NewLogMailer constructs a LogMailer.
func NewLogMailer(logger *slog.Logger) *LogMailer {
	return &LogMailer{logger: logger}
}

// SendOTP logs the OTP code (dev only — never use in production).
func (m *LogMailer) SendOTP(_ context.Context, email, code string) error {
	m.logger.Info("otp_code (dev)", slog.String("email", email), slog.String("code", code))
	return nil
}
