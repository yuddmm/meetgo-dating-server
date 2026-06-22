package auth

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Onboarding step (BASICS→ABOUT→PHOTOS→DONE) is owned by the profile module and
// stored on users.onboarding_step; auth simply echoes it back to the client.

// --- Requests ---

type sendCodeRequest struct {
	Email string `json:"email"`
}

type checkCodeRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// --- Responses ---

type sendCodeResponse struct {
	ExpiresIn   int `json:"expiresIn"`   // seconds
	ResendAfter int `json:"resendAfter"` // seconds
}

type tokenResponse struct {
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken"`
	IsCreatedProfile bool   `json:"isCreatedProfile"`
	OnboardingStep   string `json:"onboardingStep"`
}

type meResponse struct {
	ID               uuid.UUID `json:"id"`
	Email            string    `json:"email"`
	IsCreatedProfile bool      `json:"isCreatedProfile"`
	OnboardingStep   string    `json:"onboardingStep"`
}

// --- Validation ---

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// normalizeEmail trims and lowercases an email for storage and lookup.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validateEmail(email string) (string, *APIError) {
	norm := normalizeEmail(email)
	if norm == "" || len(norm) > 254 || !emailRe.MatchString(norm) {
		return "", validationError(map[string]string{"email": "invalid email"})
	}
	return norm, nil
}

// russianEmailTLDs lists the domain suffixes considered a "Russian email".
// Easily extendable with explicit provider domains if needed.
var russianEmailTLDs = []string{".ru", ".su", ".рф", ".xn--p1ai"}

// isRussianEmail reports whether the (normalized) email is on a Russian domain.
func isRussianEmail(email string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := email[at+1:]
	for _, tld := range russianEmailTLDs {
		if strings.HasSuffix(domain, tld) {
			return true
		}
	}
	return false
}

var codeRe = regexp.MustCompile(`^\d{6}$`)

func validateCode(code string) *APIError {
	if !codeRe.MatchString(code) {
		return validationError(map[string]string{"code": "must be 6 digits"})
	}
	return nil
}
