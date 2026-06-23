package meeting

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

const (
	feedDefaultLimit  = 20
	feedMaxLimit      = 50
	feedDefaultRadius = 50.0  // km
	feedMaxRadius     = 500.0 // km
	ageFloor          = 18
	ageCeil           = 99

	sortDistance = "distance"
	sortDate     = "date"
)

// feedFilters holds the parsed, validated discovery query. Modes are exclusive:
// CityID set → city mode; otherwise radius mode.
type feedFilters struct {
	Sort   string
	Radius float64
	CityID *uuid.UUID // city mode
	Gender *string
	AgeMin int
	AgeMax int
	Goal      *string
	TagIDs    []uuid.UUID
	Limit     int
	CursorRaw string // decoded in the service (self-describes its sort)
}

// cityRef is a compact city reference (author's effective city).
type cityRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// feedAuthor is the compact author card for a feed item.
type feedAuthor struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Age       int       `json:"age"`
	Gender    string    `json:"gender"`
	AvatarURL *string   `json:"avatarUrl"`
}

type feedItem struct {
	ID          uuid.UUID  `json:"id"` // ad id
	Description string     `json:"description"`
	Tags        []Tag      `json:"tags"`
	City        *cityRef   `json:"city"`
	DistanceKm  *int       `json:"distanceKm"` // null in city mode without viewer location
	Author      feedAuthor `json:"author"`
}

type feedResponse struct {
	Items      []feedItem `json:"items"`
	NextCursor *string    `json:"nextCursor"`
}

// feedCursor is the opaque keyset cursor, scoped to the sort it was issued for.
// It also pins the viewer's origin (their OWN coords) so distances stay computed
// from a fixed point for the whole scroll session — no third-party coords here.
type feedCursor struct {
	Sort     string   `json:"s"`           // distance | date (self-describing)
	Dist     *float64 `json:"d,omitempty"` // for sort=distance
	Ts       *string  `json:"t,omitempty"` // RFC3339Nano, for sort=date
	ID       string   `json:"i"`           // last ad id
	Lat      float64  `json:"la"`          // pinned viewer origin
	Lng      float64  `json:"lo"`          // pinned viewer origin
	HasPoint bool     `json:"hp"`          // whether the viewer had a location
}

func encodeCursor(c feedCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(raw string) (*feedCursor, *httpx.APIError) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, badCursor()
	}
	var c feedCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, badCursor()
	}
	if c.Sort != sortDistance && c.Sort != sortDate {
		return nil, badCursor()
	}
	if _, err := uuid.Parse(c.ID); err != nil {
		return nil, badCursor()
	}
	return &c, nil
}

func badCursor() *httpx.APIError {
	return httpx.ValidationError(map[string]string{"cursor": "invalid cursor"})
}

// distanceKm renders kilometres: 0 for <1 km, otherwise math-rounded.
func distanceKm(km float64) int {
	if km < 1 {
		return 0
	}
	return int(km + 0.5)
}

func ageFromBirth(birth, now time.Time) int {
	y := now.Year() - birth.Year()
	if now.Month() < birth.Month() || (now.Month() == birth.Month() && now.Day() < birth.Day()) {
		y--
	}
	return y
}
