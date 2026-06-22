package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type photoRow struct {
	ID        uuid.UUID
	ObjectKey string
	URL       string
	Position  int
	CropRaw   []byte // raw jsonb, nil when no crop
}

func toPhotoResponse(p photoRow) photoResponse {
	resp := photoResponse{ID: p.ID, URL: p.URL, Position: p.Position}
	if len(p.CropRaw) > 0 {
		var c crop
		if err := json.Unmarshal(p.CropRaw, &c); err == nil {
			resp.Crop = &c
		}
	}
	return resp
}

const photoCols = `id, object_key, url, position, crop`

func scanPhoto(row pgx.Row) (photoRow, error) {
	var p photoRow
	err := row.Scan(&p.ID, &p.ObjectKey, &p.URL, &p.Position, &p.CropRaw)
	return p, err
}

// photosByProfileID returns the profile's photos ordered by position.
func (r *Repository) photosByProfileID(ctx context.Context, profileID uuid.UUID) ([]photoRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+photoCols+` FROM photos WHERE profile_id = $1 ORDER BY position`, profileID)
	if err != nil {
		return nil, fmt.Errorf("profile: photos: %w", err)
	}
	defer rows.Close()

	out := make([]photoRow, 0, maxPhotos)
	for rows.Next() {
		var p photoRow
		if err := rows.Scan(&p.ID, &p.ObjectKey, &p.URL, &p.Position, &p.CropRaw); err != nil {
			return nil, fmt.Errorf("profile: scan photo: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) countPhotos(ctx context.Context, profileID uuid.UUID) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM photos WHERE profile_id = $1`, profileID).Scan(&n); err != nil {
		return 0, fmt.Errorf("profile: count photos: %w", err)
	}
	return n, nil
}

// insertPhoto appends a photo at the next free position (0 if first).
func (r *Repository) insertPhoto(ctx context.Context, profileID uuid.UUID, key, url string) (photoRow, error) {
	p, err := scanPhoto(r.pool.QueryRow(ctx, `
		INSERT INTO photos (profile_id, object_key, url, position)
		VALUES ($1, $2, $3, (SELECT COALESCE(MAX(position) + 1, 0) FROM photos WHERE profile_id = $1))
		RETURNING `+photoCols, profileID, key, url))
	if err != nil {
		return photoRow{}, fmt.Errorf("profile: insert photo: %w", err)
	}
	return p, nil
}

// photoByID returns a photo scoped to the profile, or nil if not found.
func (r *Repository) photoByID(ctx context.Context, profileID, photoID uuid.UUID) (*photoRow, error) {
	p, err := scanPhoto(r.pool.QueryRow(ctx,
		`SELECT `+photoCols+` FROM photos WHERE id = $1 AND profile_id = $2`, photoID, profileID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("profile: photo by id: %w", err)
	}
	return &p, nil
}

// updatePhotoCrop sets the crop (cropJSON nil clears it) and returns the photo.
func (r *Repository) updatePhotoCrop(ctx context.Context, profileID, photoID uuid.UUID, cropJSON []byte) (*photoRow, error) {
	var arg any
	if cropJSON != nil {
		arg = string(cropJSON)
	}
	p, err := scanPhoto(r.pool.QueryRow(ctx,
		`UPDATE photos SET crop = $3::jsonb WHERE id = $1 AND profile_id = $2 RETURNING `+photoCols,
		photoID, profileID, arg))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("profile: update crop: %w", err)
	}
	return &p, nil
}

// deletePhoto removes a photo and returns its object key for storage cleanup.
func (r *Repository) deletePhoto(ctx context.Context, profileID, photoID uuid.UUID) (string, bool, error) {
	var key string
	err := r.pool.QueryRow(ctx,
		`DELETE FROM photos WHERE id = $1 AND profile_id = $2 RETURNING object_key`, photoID, profileID,
	).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("profile: delete photo: %w", err)
	}
	return key, true, nil
}

// reorderPhotos sets positions from the given id order and clears the crop of
// every non-main photo. Two-phase to avoid the unique(position) collision.
func (r *Repository) reorderPhotos(ctx context.Context, profileID uuid.UUID, ids []uuid.UUID) error {
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE photos SET position = position + 1000 WHERE profile_id = $1`, profileID); err != nil {
			return err
		}
		for i, id := range ids {
			ct, err := tx.Exec(ctx, `
				UPDATE photos
				SET position = $3, crop = CASE WHEN $3 = 0 THEN crop ELSE NULL END
				WHERE id = $1 AND profile_id = $2`, id, profileID, i)
			if err != nil {
				return err
			}
			if ct.RowsAffected() != 1 {
				return errReorderMismatch
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("profile: reorder photos: %w", err)
	}
	return nil
}

// markComplete flags the profile as created and sets the onboarding step to DONE.
func (r *Repository) markComplete(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE users SET is_created_profile = true, onboarding_step = 'DONE' WHERE id = $1`, userID); err != nil {
		return fmt.Errorf("profile: mark complete: %w", err)
	}
	return nil
}

var errReorderMismatch = errors.New("reorder: id set does not match profile photos")
