package meeting

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Like records the caller's like on someone else's active ad.
func (s *Service) Like(ctx context.Context, userID, adID uuid.UUID) error {
	viewerProfileID, created, found, err := s.repo.profileForUser(ctx, userID)
	if err != nil {
		return err
	}
	if !found || !created {
		return errProfileRequired
	}

	ad, err := s.repo.adByID(ctx, adID)
	if err != nil {
		return err
	}
	if ad == nil {
		return errAdNotFound
	}
	if ad.Status != "ACTIVE" || time.Now().After(ad.ExpiresAt) {
		return errAdNotActive
	}
	if ad.AuthorProfileID == viewerProfileID {
		return errCannotLikeOwn
	}

	createdLike, err := s.repo.createCandidate(ctx, adID, viewerProfileID)
	if err != nil {
		return err
	}
	if !createdLike {
		return errAlreadyResponded
	}
	return nil
}

// ListLikes returns incoming pending likes on the caller's active ad ([] if none).
func (s *Service) ListLikes(ctx context.Context, userID uuid.UUID) ([]likeResponse, error) {
	profileID, _, found, err := s.repo.profileForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !found {
		return []likeResponse{}, nil
	}
	if err := s.repo.expireStale(ctx, profileID); err != nil {
		return nil, err
	}
	active, err := s.repo.activeAdByProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if active == nil {
		return []likeResponse{}, nil
	}
	rows, err := s.repo.pendingLikesForAd(ctx, active.ID)
	if err != nil {
		return nil, err
	}
	out := make([]likeResponse, len(rows))
	for i, l := range rows {
		out[i] = toLikeResponse(l)
	}
	return out, nil
}

// Accept accepts a like on the caller's ad and opens a chat.
func (s *Service) Accept(ctx context.Context, userID, candidateID uuid.UUID) (acceptResponse, error) {
	cand, err := s.requireOwnCandidate(ctx, userID, candidateID)
	if err != nil {
		return acceptResponse{}, err
	}
	if cand.Status != "PENDING" {
		return acceptResponse{}, errLikeNotPending
	}
	chatID, err := s.repo.acceptCandidate(ctx, candidateID)
	if err != nil {
		return acceptResponse{}, err
	}
	return acceptResponse{ChatID: chatID}, nil
}

// Reject rejects a like on the caller's ad.
func (s *Service) Reject(ctx context.Context, userID, candidateID uuid.UUID) error {
	if _, err := s.requireOwnCandidate(ctx, userID, candidateID); err != nil {
		return err
	}
	return s.repo.rejectCandidate(ctx, candidateID)
}

// requireOwnCandidate loads a candidate that belongs to the caller's ad.
func (s *Service) requireOwnCandidate(ctx context.Context, userID, candidateID uuid.UUID) (*candidateRow, error) {
	profileID, _, found, err := s.repo.profileForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errLikeNotFound
	}
	cand, err := s.repo.candidateForAuthor(ctx, candidateID, profileID)
	if err != nil {
		return nil, err
	}
	if cand == nil {
		return nil, errLikeNotFound
	}
	return cand, nil
}
