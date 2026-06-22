package profile

import (
	"github.com/google/uuid"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// Photo limits / allowed types (docs/api.md, meetgo-profile-photo-decisions).
const (
	maxPhotos           = 5
	minPhotosToComplete = 2
	maxPhotoBytes       = 10 << 20 // 10 MiB
)

// allowedImageTypes maps a detected MIME type to its file extension.
var allowedImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// crop is the normalized [0,1] square (under the round avatar) of the main photo.
type crop struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Size float64 `json:"size"`
}

func (c crop) valid() bool {
	in01 := func(v float64) bool { return v >= 0 && v <= 1 }
	return in01(c.X) && in01(c.Y) && c.Size > 0 && c.Size <= 1 &&
		c.X+c.Size <= 1.0001 && c.Y+c.Size <= 1.0001
}

type photoResponse struct {
	ID       uuid.UUID `json:"id"`
	URL      string    `json:"url"`
	Position int       `json:"position"`
	Crop     *crop     `json:"crop"`
}

// --- Requests ---

type setCropRequest struct {
	Crop crop `json:"crop"`
}

type reorderRequest struct {
	Order []string `json:"order"`
}

type completeResponse struct {
	IsCreatedProfile bool   `json:"isCreatedProfile"`
	OnboardingStep   string `json:"onboardingStep"`
}

func validateReorder(raw []string) ([]uuid.UUID, *httpx.APIError) {
	if len(raw) == 0 {
		return nil, httpx.ValidationError(map[string]string{"order": "must not be empty"})
	}
	seen := make(map[uuid.UUID]bool, len(raw))
	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, httpx.ValidationError(map[string]string{"order": "contains an invalid id"})
		}
		if seen[id] {
			return nil, httpx.ValidationError(map[string]string{"order": "contains duplicates"})
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}
