package meeting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the persistence layer for the meeting module (hand-written pgx).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

type adRow struct {
	ID              uuid.UUID
	AuthorProfileID uuid.UUID
	Description     string
	Status          string
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

const adCols = `id, author_profile_id, description, status, created_at, expires_at`

func scanAd(row pgx.Row) (*adRow, error) {
	var a adRow
	err := row.Scan(&a.ID, &a.AuthorProfileID, &a.Description, &a.Status, &a.CreatedAt, &a.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// --- Tags ---

func (r *Repository) listTags(ctx context.Context) ([]Tag, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, value FROM meeting_tags ORDER BY value`)
	if err != nil {
		return nil, fmt.Errorf("meeting: list tags: %w", err)
	}
	defer rows.Close()
	out := make([]Tag, 0, 64)
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Value); err != nil {
			return nil, fmt.Errorf("meeting: scan tag: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) countTags(ctx context.Context, ids []uuid.UUID) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM meeting_tags WHERE id = ANY($1::uuid[])`, uuidStrings(ids),
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("meeting: count tags: %w", err)
	}
	return n, nil
}

func (r *Repository) tagsByAdID(ctx context.Context, adID uuid.UUID) ([]Tag, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT t.id, t.value FROM meeting_ad_tags mt
		JOIN meeting_tags t ON t.id = mt.tag_id
		WHERE mt.meeting_ad_id = $1 ORDER BY t.value`, adID)
	if err != nil {
		return nil, fmt.Errorf("meeting: ad tags: %w", err)
	}
	defer rows.Close()
	out := make([]Tag, 0, maxTags)
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Value); err != nil {
			return nil, fmt.Errorf("meeting: scan ad tag: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// --- Author profile ---

// profileForUser returns the author's profile id and whether onboarding is done.
func (r *Repository) profileForUser(ctx context.Context, userID uuid.UUID) (profileID uuid.UUID, created, found bool, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT p.id, u.is_created_profile
		FROM users u JOIN profiles p ON p.user_id = u.id
		WHERE u.id = $1`, userID).Scan(&profileID, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, false, nil
	}
	if err != nil {
		return uuid.Nil, false, false, fmt.Errorf("meeting: profile for user: %w", err)
	}
	return profileID, created, true, nil
}

// --- Ads ---

// expireStale flips the profile's overdue ACTIVE ad to EXPIRED (lazy expiry).
func (r *Repository) expireStale(ctx context.Context, profileID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE meeting_ads SET status = 'EXPIRED'
		 WHERE author_profile_id = $1 AND status = 'ACTIVE' AND expires_at <= now()`, profileID)
	if err != nil {
		return fmt.Errorf("meeting: expire stale: %w", err)
	}
	return nil
}

func (r *Repository) activeAdByProfile(ctx context.Context, profileID uuid.UUID) (*adRow, error) {
	a, err := scanAd(r.pool.QueryRow(ctx,
		`SELECT `+adCols+` FROM meeting_ads WHERE author_profile_id = $1 AND status = 'ACTIVE'`, profileID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("meeting: active ad: %w", err)
	}
	return a, nil
}

func (r *Repository) adByID(ctx context.Context, adID uuid.UUID) (*adRow, error) {
	a, err := scanAd(r.pool.QueryRow(ctx, `SELECT `+adCols+` FROM meeting_ads WHERE id = $1`, adID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("meeting: ad by id: %w", err)
	}
	return a, nil
}

func (r *Repository) createAd(ctx context.Context, profileID uuid.UUID, desc string, tagIDs []uuid.UUID) (*adRow, error) {
	var a *adRow
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		row, err := scanAd(tx.QueryRow(ctx, `
			INSERT INTO meeting_ads (author_profile_id, description, expires_at)
			VALUES ($1, $2, now() + make_interval(secs => $3))
			RETURNING `+adCols, profileID, desc, adTTL.Seconds()))
		if err != nil {
			return err
		}
		a = row
		return insertAdTags(ctx, tx, row.ID, tagIDs)
	})
	if err != nil {
		return nil, fmt.Errorf("meeting: create ad: %w", err)
	}
	return a, nil
}

// updateActiveAd updates description/tags of the profile's active ad; returns
// nil if there is no active ad.
func (r *Repository) updateActiveAd(ctx context.Context, profileID uuid.UUID, desc string, tagIDs []uuid.UUID) (*adRow, error) {
	var a *adRow
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		row, err := scanAd(tx.QueryRow(ctx, `
			UPDATE meeting_ads SET description = $2
			WHERE author_profile_id = $1 AND status = 'ACTIVE'
			RETURNING `+adCols, profileID, desc))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // a stays nil
		}
		if err != nil {
			return err
		}
		a = row
		if _, err := tx.Exec(ctx, `DELETE FROM meeting_ad_tags WHERE meeting_ad_id = $1`, row.ID); err != nil {
			return err
		}
		return insertAdTags(ctx, tx, row.ID, tagIDs)
	})
	if err != nil {
		return nil, fmt.Errorf("meeting: update ad: %w", err)
	}
	return a, nil
}

// closeActiveAd sets the profile's active ad to CLOSED; returns whether one existed.
func (r *Repository) closeActiveAd(ctx context.Context, profileID uuid.UUID) (bool, error) {
	ct, err := r.pool.Exec(ctx,
		`UPDATE meeting_ads SET status = 'CLOSED' WHERE author_profile_id = $1 AND status = 'ACTIVE'`, profileID)
	if err != nil {
		return false, fmt.Errorf("meeting: close ad: %w", err)
	}
	return ct.RowsAffected() > 0, nil
}

func insertAdTags(ctx context.Context, tx pgx.Tx, adID uuid.UUID, tagIDs []uuid.UUID) error {
	for _, id := range tagIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO meeting_ad_tags (meeting_ad_id, tag_id) VALUES ($1, $2)`, adID, id); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return out
}
