package profile

import (
	"net/http"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// Contract errors (docs/api.md → Профиль).
var (
	errProfileNotFound = httpx.NewError(http.StatusNotFound, "PROFILE_NOT_FOUND", "profile not found")
	errStepOrder       = httpx.NewError(http.StatusConflict, "STEP_ORDER", "previous onboarding step not completed")
	errUnauthorized    = httpx.NewError(http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid access token")
	errPhotoNotFound   = httpx.NewError(http.StatusNotFound, "PHOTO_NOT_FOUND", "photo not found")
)

func incompleteError(details map[string]string) *httpx.APIError {
	return &httpx.APIError{
		Status:  http.StatusUnprocessableEntity,
		Code:    "PROFILE_INCOMPLETE",
		Message: "profile is not complete",
		Details: details,
	}
}

func tooManyPhotosError() *httpx.APIError {
	return httpx.ValidationError(map[string]string{"photo": "max 5 photos"})
}
