package profile

import (
	"context"
	"encoding/json"
	"io"

	"github.com/google/uuid"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// UploadPhoto validates and stores one photo, appending it to the profile.
// Requires the about step to be completed (STEP_ORDER otherwise).
func (s *Service) UploadPhoto(ctx context.Context, userID uuid.UUID, r io.Reader, size int64, contentType string) (photoResponse, error) {
	ext, ok := allowedImageTypes[contentType]
	if !ok {
		return photoResponse{}, httpx.ValidationError(map[string]string{"photo": "must be jpeg, png or webp"})
	}

	prof, err := s.profileForPhotos(ctx, userID)
	if err != nil {
		return photoResponse{}, err
	}

	n, err := s.repo.countPhotos(ctx, prof.ID)
	if err != nil {
		return photoResponse{}, err
	}
	if n >= maxPhotos {
		return photoResponse{}, tooManyPhotosError()
	}

	key := "profiles/" + prof.ID.String() + "/" + uuid.NewString() + ext
	url, err := s.storage.Put(ctx, key, r, size, contentType)
	if err != nil {
		return photoResponse{}, err
	}

	row, err := s.repo.insertPhoto(ctx, prof.ID, key, url)
	if err != nil {
		// Best-effort cleanup of the just-stored object.
		_ = s.storage.Delete(ctx, key)
		return photoResponse{}, err
	}
	return toPhotoResponse(row), nil
}

// SetCrop sets/updates the crop of a photo (intended for the main photo).
func (s *Service) SetCrop(ctx context.Context, userID, photoID uuid.UUID, c crop) (photoResponse, error) {
	if !c.valid() {
		return photoResponse{}, httpx.ValidationError(map[string]string{"crop": "x,y,size must be within [0,1]"})
	}
	prof, err := s.requireProfile(ctx, userID)
	if err != nil {
		return photoResponse{}, err
	}
	raw, _ := json.Marshal(c)
	updated, err := s.repo.updatePhotoCrop(ctx, prof.ID, photoID, raw)
	if err != nil {
		return photoResponse{}, err
	}
	if updated == nil {
		return photoResponse{}, errPhotoNotFound
	}
	return toPhotoResponse(*updated), nil
}

// Reorder sets the photo order (first = main) and clears non-main crops. The
// order must list exactly the profile's photos.
func (s *Service) Reorder(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) ([]photoResponse, error) {
	prof, err := s.requireProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	existing, err := s.repo.photosByProfileID(ctx, prof.ID)
	if err != nil {
		return nil, err
	}
	if !sameIDSet(existing, ids) {
		return nil, httpx.ValidationError(map[string]string{"order": "must list all photos exactly once"})
	}

	if err := s.repo.reorderPhotos(ctx, prof.ID, ids); err != nil {
		return nil, err
	}

	rows, err := s.repo.photosByProfileID(ctx, prof.ID)
	if err != nil {
		return nil, err
	}
	return toPhotoResponses(rows), nil
}

// DeletePhoto removes a photo and its stored object.
func (s *Service) DeletePhoto(ctx context.Context, userID, photoID uuid.UUID) error {
	prof, err := s.requireProfile(ctx, userID)
	if err != nil {
		return err
	}
	key, found, err := s.repo.deletePhoto(ctx, prof.ID, photoID)
	if err != nil {
		return err
	}
	if !found {
		return errPhotoNotFound
	}
	_ = s.storage.Delete(ctx, key) // best-effort; DB is the source of truth
	return nil
}

// Complete is the final gate: all steps filled and at least 2 photos, then it
// sets isCreatedProfile and the DONE step atomically.
func (s *Service) Complete(ctx context.Context, userID uuid.UUID) (completeResponse, error) {
	step, err := s.repo.onboardingStep(ctx, userID)
	if err != nil {
		return completeResponse{}, err
	}
	prof, err := s.repo.profileByUserID(ctx, userID)
	if err != nil {
		return completeResponse{}, err
	}
	if prof == nil || stepRank[step] < stepRank["PHOTOS"] {
		return completeResponse{}, incompleteError(map[string]string{"step": "complete previous steps first"})
	}

	n, err := s.repo.countPhotos(ctx, prof.ID)
	if err != nil {
		return completeResponse{}, err
	}
	if n < minPhotosToComplete {
		return completeResponse{}, incompleteError(map[string]string{"photos": "min 2"})
	}

	if err := s.repo.markComplete(ctx, userID); err != nil {
		return completeResponse{}, err
	}
	return completeResponse{IsCreatedProfile: true, OnboardingStep: "DONE"}, nil
}

// profileForPhotos returns the profile only when the photo step is reachable.
func (s *Service) profileForPhotos(ctx context.Context, userID uuid.UUID) (*profileRow, error) {
	prof, err := s.repo.profileByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if prof == nil {
		return nil, errStepOrder
	}
	step, err := s.repo.onboardingStep(ctx, userID)
	if err != nil {
		return nil, err
	}
	if stepRank[step] < stepRank["PHOTOS"] {
		return nil, errStepOrder
	}
	return prof, nil
}

func (s *Service) requireProfile(ctx context.Context, userID uuid.UUID) (*profileRow, error) {
	prof, err := s.repo.profileByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if prof == nil {
		return nil, errProfileNotFound
	}
	return prof, nil
}

func toPhotoResponses(rows []photoRow) []photoResponse {
	out := make([]photoResponse, len(rows))
	for i, p := range rows {
		out[i] = toPhotoResponse(p)
	}
	return out
}

func sameIDSet(rows []photoRow, ids []uuid.UUID) bool {
	if len(rows) != len(ids) {
		return false
	}
	set := make(map[uuid.UUID]bool, len(rows))
	for _, p := range rows {
		set[p.ID] = true
	}
	for _, id := range ids {
		if !set[id] {
			return false
		}
	}
	return true
}
