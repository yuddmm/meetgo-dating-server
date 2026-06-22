package meeting

import (
	"net/http"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// Contract errors (docs/meeting-list.md).
var (
	errProfileRequired = httpx.NewError(http.StatusForbidden, "PROFILE_REQUIRED", "complete your profile first")
	errActiveAdExists  = httpx.NewError(http.StatusConflict, "ACTIVE_AD_EXISTS", "you already have an active ad")
	errNoActiveAd      = httpx.NewError(http.StatusNotFound, "NO_ACTIVE_AD", "no active ad")
	errAdNotFound      = httpx.NewError(http.StatusNotFound, "AD_NOT_FOUND", "ad not found")
	errAdNotActive     = httpx.NewError(http.StatusConflict, "AD_NOT_ACTIVE", "ad is not active")
	errCannotLikeOwn   = httpx.NewError(http.StatusUnprocessableEntity, "CANNOT_LIKE_OWN_AD", "cannot like your own ad")
	errAlreadyResponded = httpx.NewError(http.StatusConflict, "ALREADY_RESPONDED", "already responded to this ad")
	errLikeNotFound    = httpx.NewError(http.StatusNotFound, "LIKE_NOT_FOUND", "like not found")
	errLikeNotPending  = httpx.NewError(http.StatusConflict, "LIKE_NOT_PENDING", "like already handled")
	errUnauthorized    = httpx.NewError(http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid access token")
)

func unknownTagError() *httpx.APIError {
	return httpx.ValidationError(map[string]string{"tagIds": "unknown tag id"})
}
