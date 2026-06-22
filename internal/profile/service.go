// Package profile implements profile creation / onboarding (docs/api.md).
package profile

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
	"github.com/yuddmm/meetgo-dating-server/internal/platform/storage"
)

// stepRank orders the onboarding steps for comparison.
var stepRank = map[string]int{"BASICS": 0, "ABOUT": 1, "PHOTOS": 2, "DONE": 3}

// Service implements the profile use-cases.
type Service struct {
	repo    *Repository
	storage storage.Storage
}

// NewService constructs a Service.
func NewService(repo *Repository, store storage.Storage) *Service {
	return &Service{repo: repo, storage: store}
}

// Get returns the current user's profile, or PROFILE_NOT_FOUND.
func (s *Service) Get(ctx context.Context, userID uuid.UUID) (profileResponse, error) {
	p, err := s.buildProfile(ctx, userID)
	if err != nil {
		return profileResponse{}, err
	}
	if p == nil {
		return profileResponse{}, errProfileNotFound
	}
	return *p, nil
}

// SaveBasics upserts step 1 and advances the onboarding step to ABOUT.
func (s *Service) SaveBasics(ctx context.Context, userID uuid.UUID, req basicsRequest) (profileEnvelope, error) {
	birth, verr := validateBasics(req)
	if verr != nil {
		return profileEnvelope{}, verr
	}
	if _, err := s.repo.saveBasics(ctx, userID, strings.TrimSpace(req.Name), req.Gender, birth, strings.TrimSpace(req.City)); err != nil {
		return profileEnvelope{}, err
	}
	return s.buildEnvelope(ctx, userID)
}

// SaveAbout updates step 2 and advances the onboarding step to PHOTOS. It
// requires basics to be completed first (STEP_ORDER).
func (s *Service) SaveAbout(ctx context.Context, userID uuid.UUID, req aboutRequest) (profileEnvelope, error) {
	ids, verr := validateAbout(req)
	if verr != nil {
		return profileEnvelope{}, verr
	}

	step, err := s.repo.onboardingStep(ctx, userID)
	if err != nil {
		return profileEnvelope{}, err
	}
	if stepRank[step] < stepRank["ABOUT"] {
		return profileEnvelope{}, errStepOrder
	}

	prof, err := s.repo.profileByUserID(ctx, userID)
	if err != nil {
		return profileEnvelope{}, err
	}
	if prof == nil {
		return profileEnvelope{}, errStepOrder
	}

	// All referenced interests must exist in the reference.
	if n, err := s.repo.countInterests(ctx, ids); err != nil {
		return profileEnvelope{}, err
	} else if n != len(ids) {
		return profileEnvelope{}, httpx.ValidationError(map[string]string{"interestIds": "unknown interest id"})
	}

	if err := s.repo.saveAbout(ctx, userID, prof.ID, req, ids); err != nil {
		return profileEnvelope{}, err
	}
	return s.buildEnvelope(ctx, userID)
}

// PublicByID returns a profile by its id (public view), or PROFILE_NOT_FOUND.
func (s *Service) PublicByID(ctx context.Context, profileID uuid.UUID) (profileResponse, error) {
	row, err := s.repo.profileByID(ctx, profileID)
	if err != nil {
		return profileResponse{}, err
	}
	if row == nil {
		return profileResponse{}, errProfileNotFound
	}
	resp, err := s.buildFromRow(ctx, row)
	if err != nil {
		return profileResponse{}, err
	}
	return *resp, nil
}

// buildProfile assembles the profile response (or nil when there is no profile).
func (s *Service) buildProfile(ctx context.Context, userID uuid.UUID) (*profileResponse, error) {
	row, err := s.repo.profileByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	return s.buildFromRow(ctx, row)
}

// buildFromRow assembles the full profile response (interests + photos) from a row.
func (s *Service) buildFromRow(ctx context.Context, row *profileRow) (*profileResponse, error) {
	interests, err := s.repo.interestsByProfileID(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	photoRows, err := s.repo.photosByProfileID(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	photos := make([]photoResponse, len(photoRows))
	for i, p := range photoRows {
		photos[i] = toPhotoResponse(p)
	}
	resp := toProfileResponse(row, interests, photos)
	return &resp, nil
}

// buildEnvelope wraps the (existing) profile with the current onboarding step.
func (s *Service) buildEnvelope(ctx context.Context, userID uuid.UUID) (profileEnvelope, error) {
	p, err := s.buildProfile(ctx, userID)
	if err != nil {
		return profileEnvelope{}, err
	}
	if p == nil {
		return profileEnvelope{}, errProfileNotFound
	}
	step, err := s.repo.onboardingStep(ctx, userID)
	if err != nil {
		return profileEnvelope{}, err
	}
	return profileEnvelope{Profile: *p, OnboardingStep: step}, nil
}
