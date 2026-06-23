package profile

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yuddmm/meetgo-dating-server/internal/interest"
	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// Validation bounds (docs/auth-flow.md, units.md, meetgo-profile-photo-decisions).
const (
	nameMaxLen   = 100
	descMinLen   = 30
	descMaxLen   = 1000
	minInterests = 1
	maxInterests = 5
	minAge       = 18

	heightMin, heightMax = 100.0, 250.0 // cm
	weightMin, weightMax = 30.0, 300.0  // kg

	dateLayout = "2006-01-02"
)

var validGenders = map[string]bool{"MALE": true, "FEMALE": true}

var validDatingGoals = map[string]bool{
	"FRIENDSHIP":             true,
	"LONG_TERM_RELATIONSHIP": true,
	"DATES":                  true,
	"NOTHING_SERIOUS":        true,
}

// --- Requests ---

type basicsRequest struct {
	Name      string `json:"name"`
	Gender    string `json:"gender"`
	BirthDate string `json:"birthDate"` // YYYY-MM-DD
}

// cityRef is the derived effective city of the profile (from geo), or null.
type cityRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type aboutRequest struct {
	InterestIDs []string `json:"interestIds"`
	Description string   `json:"description"`
	DatingGoal  string   `json:"datingGoal"`
	Height      *float64 `json:"height"` // cm, optional
	Weight      *float64 `json:"weight"` // kg, optional
	ShowZodiac  bool     `json:"showZodiac"`
}

// --- Responses ---

type profileResponse struct {
	ID          uuid.UUID           `json:"id"`
	UserID      uuid.UUID           `json:"userId"`
	Name        string              `json:"name"`
	Gender      string              `json:"gender"`
	BirthDate   string              `json:"birthDate"`
	City        *cityRef            `json:"city"`
	Interests   []interest.Interest `json:"interests"`
	Description string              `json:"description"`
	DatingGoal  *string             `json:"datingGoal"`
	Height      *float64            `json:"height"`
	Weight      *float64            `json:"weight"`
	ShowZodiac  bool                `json:"showZodiac"`
	Photos      []photoResponse     `json:"photos"`
}

type profileEnvelope struct {
	Profile        profileResponse `json:"profile"`
	OnboardingStep string          `json:"onboardingStep"`
}

func toProfileResponse(p *profileRow, interests []interest.Interest, photos []photoResponse, city *cityRef) profileResponse {
	return profileResponse{
		ID:          p.ID,
		UserID:      p.UserID,
		Name:        p.Name,
		Gender:      p.Gender,
		BirthDate:   p.BirthDate.Format(dateLayout),
		City:        city,
		Interests:   interests,
		Description: p.Description,
		DatingGoal:  p.DatingGoal,
		Height:      p.HeightCm,
		Weight:      p.WeightKg,
		ShowZodiac:  p.ShowZodiac,
		Photos:      photos,
	}
}

// --- Validation ---

func validateBasics(req basicsRequest) (time.Time, *httpx.APIError) {
	details := map[string]string{}

	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > nameMaxLen {
		details["name"] = "required, up to 100 chars"
	}
	if !validGenders[req.Gender] {
		details["gender"] = "must be MALE or FEMALE"
	}

	birth, err := time.Parse(dateLayout, req.BirthDate)
	switch {
	case err != nil:
		details["birthDate"] = "must be YYYY-MM-DD"
	case ageYears(birth, time.Now()) < minAge:
		details["birthDate"] = "must be 18+"
	}

	if len(details) > 0 {
		return time.Time{}, httpx.ValidationError(details)
	}
	return birth, nil
}

func validateAbout(req aboutRequest) ([]uuid.UUID, *httpx.APIError) {
	details := map[string]string{}

	ids, idErr := parseInterestIDs(req.InterestIDs)
	if idErr != "" {
		details["interestIds"] = idErr
	}

	desc := strings.TrimSpace(req.Description)
	if n := len([]rune(desc)); n < descMinLen || n > descMaxLen {
		details["description"] = "must be 30–1000 chars"
	}
	if !validDatingGoals[req.DatingGoal] {
		details["datingGoal"] = "invalid value"
	}
	if req.Height != nil && (*req.Height < heightMin || *req.Height > heightMax) {
		details["height"] = "must be 100–250 cm"
	}
	if req.Weight != nil && (*req.Weight < weightMin || *req.Weight > weightMax) {
		details["weight"] = "must be 30–300 kg"
	}

	if len(details) > 0 {
		return nil, httpx.ValidationError(details)
	}
	return ids, nil
}

// parseInterestIDs validates count/uniqueness and parses the ids. It returns a
// non-empty message on failure.
func parseInterestIDs(raw []string) ([]uuid.UUID, string) {
	if len(raw) < minInterests || len(raw) > maxInterests {
		return nil, "must contain 1–5 interests"
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

// normalizeDescription trims surrounding whitespace before storage.
func normalizeDescription(s string) string {
	return strings.TrimSpace(s)
}

func ageYears(birth, now time.Time) int {
	years := now.Year() - birth.Year()
	if now.Month() < birth.Month() || (now.Month() == birth.Month() && now.Day() < birth.Day()) {
		years--
	}
	return years
}
