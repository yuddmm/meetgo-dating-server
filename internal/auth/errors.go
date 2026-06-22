package auth

import (
	"net/http"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// APIError is the shared transport error type (see internal/platform/httpx).
type APIError = httpx.APIError

// Contract errors (docs/api.md → Авторизация).
var (
	errRateLimited    = httpx.NewError(http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
	errOTPInvalid     = httpx.NewError(http.StatusUnauthorized, "OTP_INVALID", "invalid code")
	errOTPExpired     = httpx.NewError(http.StatusUnauthorized, "OTP_EXPIRED", "code expired")
	errSessionRevoked = httpx.NewError(http.StatusUnauthorized, "SESSION_REVOKED", "session revoked")
	errInvalidRefresh = httpx.NewError(http.StatusUnauthorized, "INVALID_REFRESH", "invalid refresh token")
	errUnauthorized   = httpx.NewError(http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid access token")

	// For RU region a Russian email domain is required (.ru/.su/.рф).
	errRussianEmailRequired = &httpx.APIError{
		Status:  http.StatusUnprocessableEntity,
		Code:    "RU_EMAIL_REQUIRED",
		Message: "russian email is required in this region",
		Details: map[string]string{"email": "use a .ru/.su/.рф email"},
	}
)

// validationError builds a 422 with per-field details.
func validationError(details map[string]string) *APIError {
	return httpx.ValidationError(details)
}
