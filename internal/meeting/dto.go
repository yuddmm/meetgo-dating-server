// Package meeting implements meeting ads (MeetingAd): creation, lifecycle, and
// the tags reference. Product logic — docs/matching.md; contract — docs/meeting-list.md.
package meeting

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

const (
	descMinLen = 30
	descMaxLen = 500
	minTags    = 1
	maxTags    = 3
	adTTL      = 24 * time.Hour
)

// Tag is a meeting-tag reference item: id for refs, value as the stable i18n key.
type Tag struct {
	ID    uuid.UUID `json:"id"`
	Value string    `json:"value"`
}

// --- Requests ---

type adRequest struct {
	Description string   `json:"description"`
	TagIDs      []string `json:"tagIds"`
}

// --- Responses ---

type adResponse struct {
	ID              uuid.UUID `json:"id"`
	AuthorProfileID uuid.UUID `json:"authorProfileId"`
	Description     string    `json:"description"`
	Tags            []Tag     `json:"tags"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"createdAt"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

func toAdResponse(a adRow, tags []Tag) adResponse {
	return adResponse{
		ID:              a.ID,
		AuthorProfileID: a.AuthorProfileID,
		Description:     a.Description,
		Tags:            tags,
		Status:          a.Status,
		CreatedAt:       a.CreatedAt.UTC(),
		ExpiresAt:       a.ExpiresAt.UTC(),
	}
}

// --- Validation ---

func validateAd(req adRequest) (string, []uuid.UUID, *httpx.APIError) {
	details := map[string]string{}

	desc := strings.TrimSpace(req.Description)
	if n := len([]rune(desc)); n < descMinLen || n > descMaxLen {
		details["description"] = "must be 30–500 chars"
	}

	ids, msg := parseTagIDs(req.TagIDs)
	if msg != "" {
		details["tagIds"] = msg
	}

	if len(details) > 0 {
		return "", nil, httpx.ValidationError(details)
	}
	return desc, ids, nil
}

func parseTagIDs(raw []string) ([]uuid.UUID, string) {
	if len(raw) < minTags || len(raw) > maxTags {
		return nil, "must contain 1–3 tags"
	}
	seen := make(map[uuid.UUID]bool, len(raw))
	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, "contains an invalid id"
		}
		if seen[id] {
			return nil, "contains duplicates"
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, ""
}
