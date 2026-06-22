package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Rate-limit / anti-bruteforce knobs (see docs/api.md, meetgo-auth-decisions).
const (
	otpMaxAttempts         = 5               // wrong tries per code before it is invalidated
	sendMaxPerEmailPerHour = 5               // OTP sends per email per hour
	sendMaxPerIPPer10Min   = 10              // OTP sends per client IP per 10 minutes
	emailRateWindow        = time.Hour       //
	ipRateWindow           = 10 * time.Minute //
)

// Service implements the auth use-cases.
type Service struct {
	repo           *Repository
	tokens         *TokenService
	mailer         Mailer
	otpTTL         time.Duration
	otpResendAfter time.Duration
	refreshTTL     time.Duration
	devMode        bool
}

// ServiceParams groups the Service dependencies and tunables.
type ServiceParams struct {
	Repo           *Repository
	Tokens         *TokenService
	Mailer         Mailer
	OTPTTL         time.Duration
	OTPResendAfter time.Duration
	RefreshTTL     time.Duration
	DevMode        bool
}

// NewService constructs a Service.
func NewService(p ServiceParams) *Service {
	return &Service{
		repo:           p.Repo,
		tokens:         p.Tokens,
		mailer:         p.Mailer,
		otpTTL:         p.OTPTTL,
		otpResendAfter: p.OTPResendAfter,
		refreshTTL:     p.RefreshTTL,
		devMode:        p.DevMode,
	}
}

// SendCode issues an OTP for the (normalized) email, enforcing cooldown and
// per-email / per-IP rate limits. The response is identical whether or not an
// account exists (no enumeration). For the RU region a Russian email domain is
// required (country comes from GeoIP, "" when unknown — then no restriction).
func (s *Service) SendCode(ctx context.Context, email, ip, country string) (sendCodeResponse, error) {
	if country == "RU" && !isRussianEmail(email) {
		return sendCodeResponse{}, errRussianEmailRequired
	}

	now := time.Now()

	if last, ok, err := s.repo.lastSendAt(ctx, email); err != nil {
		return sendCodeResponse{}, err
	} else if ok && now.Sub(last) < s.otpResendAfter {
		return sendCodeResponse{}, errRateLimited
	}

	if n, err := s.repo.countSendsByEmailSince(ctx, email, now.Add(-emailRateWindow)); err != nil {
		return sendCodeResponse{}, err
	} else if n >= sendMaxPerEmailPerHour {
		return sendCodeResponse{}, errRateLimited
	}

	if n, err := s.repo.countSendsByIPSince(ctx, ip, now.Add(-ipRateWindow)); err != nil {
		return sendCodeResponse{}, err
	} else if n >= sendMaxPerIPPer10Min {
		return sendCodeResponse{}, errRateLimited
	}

	code := s.generateCode()
	if err := s.repo.upsertOTP(ctx, email, hashOTP(email, code), now.Add(s.otpTTL)); err != nil {
		return sendCodeResponse{}, err
	}
	if err := s.mailer.SendOTP(ctx, email, code); err != nil {
		return sendCodeResponse{}, fmt.Errorf("auth: send otp: %w", err)
	}
	if err := s.repo.insertSendEvent(ctx, email, ip); err != nil {
		return sendCodeResponse{}, err
	}

	return sendCodeResponse{
		ExpiresIn:   int(s.otpTTL.Seconds()),
		ResendAfter: int(s.otpResendAfter.Seconds()),
	}, nil
}

// CheckCode verifies the OTP; on success it creates the user if needed, resets
// the single session and returns a fresh token pair.
func (s *Service) CheckCode(ctx context.Context, email, code string) (tokenResponse, error) {
	now := time.Now()

	otp, err := s.repo.otpByEmail(ctx, email)
	if err != nil {
		return tokenResponse{}, err
	}
	if otp == nil {
		return tokenResponse{}, errOTPInvalid
	}
	if now.After(otp.ExpiresAt) {
		_ = s.repo.deleteOTP(ctx, email)
		return tokenResponse{}, errOTPExpired
	}

	if subtle.ConstantTimeCompare(otp.CodeHash, hashOTP(email, code)) != 1 {
		attempts, err := s.repo.incrementOTPAttempts(ctx, email)
		if err != nil {
			return tokenResponse{}, err
		}
		if attempts >= otpMaxAttempts {
			_ = s.repo.deleteOTP(ctx, email)
		}
		return tokenResponse{}, errOTPInvalid
	}

	// Correct code — consume it.
	if err := s.repo.deleteOTP(ctx, email); err != nil {
		return tokenResponse{}, err
	}

	user, err := s.repo.userByEmail(ctx, email)
	if err != nil {
		return tokenResponse{}, err
	}
	if user == nil {
		if user, err = s.repo.createUser(ctx, email); err != nil {
			return tokenResponse{}, err
		}
	}

	access, refresh, hash, err := s.issuePair(user.ID, now)
	if err != nil {
		return tokenResponse{}, err
	}
	if err := s.repo.resetSession(ctx, user.ID, hash, now.Add(s.refreshTTL)); err != nil {
		return tokenResponse{}, err
	}

	return tokenResponse{
		AccessToken:      access,
		RefreshToken:     refresh,
		IsCreatedProfile: user.IsCreatedProfile,
		OnboardingStep:   user.OnboardingStep,
	}, nil
}

// Refresh rotates the token pair for a valid, non-revoked refresh token.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (tokenResponse, error) {
	now := time.Now()

	sess, err := s.repo.sessionByHash(ctx, hashToken(refreshToken))
	if err != nil {
		return tokenResponse{}, err
	}
	if sess == nil {
		return tokenResponse{}, errInvalidRefresh
	}
	if sess.RevokedAt != nil {
		return tokenResponse{}, errSessionRevoked
	}
	if now.After(sess.ExpiresAt) {
		return tokenResponse{}, errInvalidRefresh
	}

	user, err := s.repo.userByID(ctx, sess.UserID)
	if err != nil {
		return tokenResponse{}, err
	}
	if user == nil {
		return tokenResponse{}, errInvalidRefresh
	}

	access, refresh, hash, err := s.issuePair(user.ID, now)
	if err != nil {
		return tokenResponse{}, err
	}
	if err := s.repo.rotateSession(ctx, sess.ID, user.ID, hash, now.Add(s.refreshTTL)); err != nil {
		return tokenResponse{}, err
	}

	return tokenResponse{
		AccessToken:      access,
		RefreshToken:     refresh,
		IsCreatedProfile: user.IsCreatedProfile,
		OnboardingStep:   user.OnboardingStep,
	}, nil
}

// Logout revokes the user's (single) active session.
func (s *Service) Logout(ctx context.Context, userID uuid.UUID) error {
	return s.repo.revokeUserSessions(ctx, userID)
}

// Me returns the account identity for the authenticated user.
func (s *Service) Me(ctx context.Context, userID uuid.UUID) (meResponse, error) {
	user, err := s.repo.userByID(ctx, userID)
	if err != nil {
		return meResponse{}, err
	}
	if user == nil {
		return meResponse{}, errUnauthorized
	}
	return meResponse{
		ID:               user.ID,
		Email:            user.Email,
		IsCreatedProfile: user.IsCreatedProfile,
		OnboardingStep:   user.OnboardingStep,
	}, nil
}

// issuePair builds an access JWT plus an opaque refresh token (and its hash).
func (s *Service) issuePair(userID uuid.UUID, now time.Time) (access, refresh string, hash []byte, err error) {
	access, err = s.tokens.IssueAccess(userID, now)
	if err != nil {
		return "", "", nil, err
	}
	refresh, hash, err = NewRefreshToken()
	if err != nil {
		return "", "", nil, err
	}
	return access, refresh, hash, nil
}

// generateCode returns the OTP code: a fixed 000000 in dev, random 6 digits otherwise.
func (s *Service) generateCode() string {
	if s.devMode {
		return "000000"
	}
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is fatal in practice; fall back to a non-zero code.
		return "000001"
	}
	n := binary.BigEndian.Uint32(b[:]) % 1_000_000
	return fmt.Sprintf("%06d", n)
}
