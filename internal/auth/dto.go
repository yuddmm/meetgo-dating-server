package auth

import (
	"net/mail"
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

// validateEmail sanitizes and validates an email: strict parse via net/mail,
// header-injection guard (no CR/LF/NUL), bare-address only, lowercased.
func validateEmail(email string) (string, *APIError) {
	invalid := validationError(map[string]string{"email": "invalid email"})

	s := strings.TrimSpace(email)
	// Header-injection guard: reject control chars before parsing/sending.
	if strings.ContainsAny(s, "\r\n\x00") {
		return "", invalid
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Name != "" { // require a bare address, not "Name <a@b>"
		return "", invalid
	}
	norm := strings.ToLower(addr.Address)
	if len(norm) > 254 || strings.ContainsAny(norm, "\r\n\x00") {
		return "", invalid
	}
	return norm, nil
}

var gmailDomains = map[string]bool{"gmail.com": true, "googlemail.com": true}

// canonicalEmail returns a de-duplication key: plus-addressing stripped (most
// providers treat user+tag as the same inbox) and Gmail dot-folding +
// googlemail→gmail. Input is already lowercased. Used ONLY for uniqueness —
// OTP is still sent to the address the user entered.
func canonicalEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return email
	}
	local, domain := email[:at], email[at+1:]
	if gmailDomains[domain] {
		domain = "gmail.com"
		local = strings.ReplaceAll(local, ".", "")
	}
	if i := strings.IndexByte(local, '+'); i > 0 { // keep non-empty local part
		local = local[:i]
	}
	return local + "@" + domain
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
