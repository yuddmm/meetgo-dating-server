package meeting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type candidateRow struct {
	ID          uuid.UUID
	MeetingAdID uuid.UUID
	ProfileID   uuid.UUID
	Status      string
}

type likeRow struct {
	ID        uuid.UUID
	CreatedAt time.Time
	ProfileID uuid.UUID
	Name      string
	AvatarURL *string
}

// createCandidate inserts a PENDING like; created is false if one already exists.
func (r *Repository) createCandidate(ctx context.Context, adID, profileID uuid.UUID) (created bool, err error) {
	var id uuid.UUID
	err = r.pool.QueryRow(ctx, `
		INSERT INTO meeting_candidates (meeting_ad_id, profile_id) VALUES ($1, $2)
		ON CONFLICT (meeting_ad_id, profile_id) DO NOTHING
		RETURNING id`, adID, profileID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("meeting: create candidate: %w", err)
	}
	return true, nil
}

// pendingLikesForAd returns pending likes with a compact author card (name + avatar).
func (r *Repository) pendingLikesForAd(ctx context.Context, adID uuid.UUID) ([]likeRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.created_at, p.id, p.name, ph.url
		FROM meeting_candidates c
		JOIN profiles p ON p.id = c.profile_id
		LEFT JOIN photos ph ON ph.profile_id = p.id AND ph.position = 0
		WHERE c.meeting_ad_id = $1 AND c.status = 'PENDING'
		ORDER BY c.created_at DESC`, adID)
	if err != nil {
		return nil, fmt.Errorf("meeting: pending likes: %w", err)
	}
	defer rows.Close()
	out := make([]likeRow, 0, 16)
	for rows.Next() {
		var l likeRow
		if err := rows.Scan(&l.ID, &l.CreatedAt, &l.ProfileID, &l.Name, &l.AvatarURL); err != nil {
			return nil, fmt.Errorf("meeting: scan like: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// candidateForAuthor loads a candidate only if it belongs to an ad owned by
// authorProfileID; returns nil otherwise.
func (r *Repository) candidateForAuthor(ctx context.Context, candidateID, authorProfileID uuid.UUID) (*candidateRow, error) {
	var c candidateRow
	err := r.pool.QueryRow(ctx, `
		SELECT c.id, c.meeting_ad_id, c.profile_id, c.status
		FROM meeting_candidates c
		JOIN meeting_ads a ON a.id = c.meeting_ad_id
		WHERE c.id = $1 AND a.author_profile_id = $2`, candidateID, authorProfileID,
	).Scan(&c.ID, &c.MeetingAdID, &c.ProfileID, &c.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("meeting: candidate for author: %w", err)
	}
	return &c, nil
}

// acceptCandidate marks a PENDING candidate ACCEPTED and opens its chat, atomically.
func (r *Repository) acceptCandidate(ctx context.Context, candidateID uuid.UUID) (uuid.UUID, error) {
	var chatID uuid.UUID
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx,
			`UPDATE meeting_candidates SET status = 'ACCEPTED' WHERE id = $1 AND status = 'PENDING'`, candidateID)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return errLikeNotPending
		}
		return tx.QueryRow(ctx,
			`INSERT INTO chats (meeting_candidate_id) VALUES ($1) RETURNING id`, candidateID).Scan(&chatID)
	})
	if err != nil {
		return uuid.Nil, err
	}
	return chatID, nil
}

// rejectCandidate marks a PENDING candidate REJECTED.
func (r *Repository) rejectCandidate(ctx context.Context, candidateID uuid.UUID) error {
	ct, err := r.pool.Exec(ctx,
		`UPDATE meeting_candidates SET status = 'REJECTED' WHERE id = $1 AND status = 'PENDING'`, candidateID)
	if err != nil {
		return fmt.Errorf("meeting: reject candidate: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return errLikeNotPending
	}
	return nil
}
