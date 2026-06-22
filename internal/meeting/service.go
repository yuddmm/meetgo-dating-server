package meeting

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Service implements the meeting-ad use-cases.
type Service struct {
	repo *Repository
}

// NewService constructs a Service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// ListTags returns the meeting-tags reference.
func (s *Service) ListTags(ctx context.Context) ([]Tag, error) {
	return s.repo.listTags(ctx)
}

// Create publishes a new active ad (requires a completed profile, one active per user).
func (s *Service) Create(ctx context.Context, userID uuid.UUID, req adRequest) (adResponse, error) {
	desc, tagIDs, verr := validateAd(req)
	if verr != nil {
		return adResponse{}, verr
	}

	profileID, created, found, err := s.repo.profileForUser(ctx, userID)
	if err != nil {
		return adResponse{}, err
	}
	if !found || !created {
		return adResponse{}, errProfileRequired
	}

	if err := s.repo.expireStale(ctx, profileID); err != nil {
		return adResponse{}, err
	}
	if active, err := s.repo.activeAdByProfile(ctx, profileID); err != nil {
		return adResponse{}, err
	} else if active != nil {
		return adResponse{}, errActiveAdExists
	}
	if err := s.requireTagsExist(ctx, tagIDs); err != nil {
		return adResponse{}, err
	}

	row, err := s.repo.createAd(ctx, profileID, desc, tagIDs)
	if err != nil {
		return adResponse{}, err
	}
	return s.withTags(ctx, row)
}

// GetMine returns the caller's active ad.
func (s *Service) GetMine(ctx context.Context, userID uuid.UUID) (adResponse, error) {
	profileID, _, found, err := s.repo.profileForUser(ctx, userID)
	if err != nil {
		return adResponse{}, err
	}
	if !found {
		return adResponse{}, errNoActiveAd
	}
	if err := s.repo.expireStale(ctx, profileID); err != nil {
		return adResponse{}, err
	}
	active, err := s.repo.activeAdByProfile(ctx, profileID)
	if err != nil {
		return adResponse{}, err
	}
	if active == nil {
		return adResponse{}, errNoActiveAd
	}
	return s.withTags(ctx, active)
}

// GetByID returns any ad by id (not private). Expiry is reflected in the status.
func (s *Service) GetByID(ctx context.Context, adID uuid.UUID) (adResponse, error) {
	ad, err := s.repo.adByID(ctx, adID)
	if err != nil {
		return adResponse{}, err
	}
	if ad == nil {
		return adResponse{}, errAdNotFound
	}
	if ad.Status == "ACTIVE" && time.Now().After(ad.ExpiresAt) {
		ad.Status = "EXPIRED"
	}
	return s.withTags(ctx, ad)
}

// UpdateMine edits the caller's active ad (description + tags only).
func (s *Service) UpdateMine(ctx context.Context, userID uuid.UUID, req adRequest) (adResponse, error) {
	desc, tagIDs, verr := validateAd(req)
	if verr != nil {
		return adResponse{}, verr
	}
	profileID, _, found, err := s.repo.profileForUser(ctx, userID)
	if err != nil {
		return adResponse{}, err
	}
	if !found {
		return adResponse{}, errNoActiveAd
	}
	if err := s.repo.expireStale(ctx, profileID); err != nil {
		return adResponse{}, err
	}
	if err := s.requireTagsExist(ctx, tagIDs); err != nil {
		return adResponse{}, err
	}
	row, err := s.repo.updateActiveAd(ctx, profileID, desc, tagIDs)
	if err != nil {
		return adResponse{}, err
	}
	if row == nil {
		return adResponse{}, errNoActiveAd
	}
	return s.withTags(ctx, row)
}

// DeleteMine closes the caller's active ad.
func (s *Service) DeleteMine(ctx context.Context, userID uuid.UUID) error {
	profileID, _, found, err := s.repo.profileForUser(ctx, userID)
	if err != nil {
		return err
	}
	if !found {
		return errNoActiveAd
	}
	if err := s.repo.expireStale(ctx, profileID); err != nil {
		return err
	}
	ok, err := s.repo.closeActiveAd(ctx, profileID)
	if err != nil {
		return err
	}
	if !ok {
		return errNoActiveAd
	}
	return nil
}

func (s *Service) requireTagsExist(ctx context.Context, ids []uuid.UUID) error {
	n, err := s.repo.countTags(ctx, ids)
	if err != nil {
		return err
	}
	if n != len(ids) {
		return unknownTagError()
	}
	return nil
}

func (s *Service) withTags(ctx context.Context, row *adRow) (adResponse, error) {
	tags, err := s.repo.tagsByAdID(ctx, row.ID)
	if err != nil {
		return adResponse{}, err
	}
	return toAdResponse(*row, tags), nil
}
