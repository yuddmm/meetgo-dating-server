package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenService issues and verifies short-lived access tokens (HS256 JWT) and
// generates opaque refresh tokens. The refresh token itself is returned to the
// client; only its SHA-256 hash is stored server-side.
type TokenService struct {
	secret    []byte
	accessTTL time.Duration
}

// NewTokenService constructs a TokenService.
func NewTokenService(secret string, accessTTL time.Duration) *TokenService {
	return &TokenService{secret: []byte(secret), accessTTL: accessTTL}
}

// IssueAccess creates a signed access JWT with claims { sub, iat, exp }.
func (s *TokenService) IssueAccess(userID uuid.UUID, now time.Time) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign access token: %w", err)
	}
	return signed, nil
}

// ParseAccess verifies an access token and returns its subject (user id).
func (s *TokenService) ParseAccess(raw string) (uuid.UUID, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, fmt.Errorf("auth: bad subject: %w", err)
	}
	return id, nil
}

// NewRefreshToken returns a new opaque refresh token (base64url) and its SHA-256
// hash for storage.
func NewRefreshToken() (token string, hash []byte, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("auth: generate refresh token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, hashToken(token), nil
}

// hashToken returns the SHA-256 hash of a refresh token for storage/lookup.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// hashOTP returns the SHA-256 hash of an OTP code, salted by email so the same
// code for different addresses hashes differently.
func hashOTP(email, code string) []byte {
	sum := sha256.Sum256([]byte(email + "\x00" + code))
	return sum[:]
}
