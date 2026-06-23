package profile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuddmm/meetgo-dating-server/internal/interest"
)

// Repository is the persistence layer for the profile module (hand-written pgx).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

type profileRow struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Name        string
	Gender      string
	BirthDate   time.Time
	Description string
	DatingGoal  *string
	HeightCm    *float64
	WeightKg    *float64
	ShowZodiac  bool
}

const profileCols = `id, user_id, name, gender, birth_date, description,
	dating_goal, height_cm, weight_kg, show_zodiac`

func scanProfile(row pgx.Row) (*profileRow, error) {
	var p profileRow
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Gender, &p.BirthDate,
		&p.Description, &p.DatingGoal, &p.HeightCm, &p.WeightKg, &p.ShowZodiac)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// effectiveCity returns the user's current effective city (from geo), or nil.
func (r *Repository) effectiveCity(ctx context.Context, userID uuid.UUID) (*cityRef, error) {
	var c cityRef
	err := r.pool.QueryRow(ctx, `
		SELECT c.id, COALESCE(c.name_ru, c.name)
		FROM user_geo ug JOIN cities c ON c.id = ug.resolved_city_id
		WHERE ug.user_id = $1`, userID).Scan(&c.ID, &c.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("profile: effective city: %w", err)
	}
	return &c, nil
}

// profileByUserID returns the profile for a user, or nil if none exists.
func (r *Repository) profileByUserID(ctx context.Context, userID uuid.UUID) (*profileRow, error) {
	p, err := scanProfile(r.pool.QueryRow(ctx,
		`SELECT `+profileCols+` FROM profiles WHERE user_id = $1`, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("profile: by user id: %w", err)
	}
	return p, nil
}

// profileByID returns the profile by its id, or nil if none exists.
func (r *Repository) profileByID(ctx context.Context, profileID uuid.UUID) (*profileRow, error) {
	p, err := scanProfile(r.pool.QueryRow(ctx,
		`SELECT `+profileCols+` FROM profiles WHERE id = $1`, profileID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("profile: by id: %w", err)
	}
	return p, nil
}

// interestsByProfileID returns the profile's interests as reference items.
func (r *Repository) interestsByProfileID(ctx context.Context, profileID uuid.UUID) ([]interest.Interest, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT i.id, i.value
		FROM profile_interests pi
		JOIN interests i ON i.id = pi.interest_id
		WHERE pi.profile_id = $1
		ORDER BY i.value`, profileID)
	if err != nil {
		return nil, fmt.Errorf("profile: interests: %w", err)
	}
	defer rows.Close()

	items := make([]interest.Interest, 0, maxInterests)
	for rows.Next() {
		var it interest.Interest
		if err := rows.Scan(&it.ID, &it.Value); err != nil {
			return nil, fmt.Errorf("profile: scan interest: %w", err)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// onboardingStep returns the user's current onboarding step.
func (r *Repository) onboardingStep(ctx context.Context, userID uuid.UUID) (string, error) {
	var step string
	if err := r.pool.QueryRow(ctx,
		`SELECT onboarding_step FROM users WHERE id = $1`, userID).Scan(&step); err != nil {
		return "", fmt.Errorf("profile: onboarding step: %w", err)
	}
	return step, nil
}

// countInterests returns how many of the given interest ids exist in the reference.
func (r *Repository) countInterests(ctx context.Context, ids []uuid.UUID) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM interests WHERE id = ANY($1::uuid[])`, uuidStrings(ids),
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("profile: count interests: %w", err)
	}
	return n, nil
}

// saveBasics upserts the basics step and advances the onboarding step to ABOUT.
func (r *Repository) saveBasics(ctx context.Context, userID uuid.UUID, name, gender string, birth time.Time) (*profileRow, error) {
	var p *profileRow
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		row, err := scanProfile(tx.QueryRow(ctx, `
			INSERT INTO profiles (user_id, name, gender, birth_date)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_id) DO UPDATE
			SET name = EXCLUDED.name, gender = EXCLUDED.gender,
			    birth_date = EXCLUDED.birth_date, updated_at = now()
			RETURNING `+profileCols, userID, name, gender, birth))
		if err != nil {
			return err
		}
		p = row
		return advanceStep(ctx, tx, userID, "ABOUT")
	})
	if err != nil {
		return nil, fmt.Errorf("profile: save basics: %w", err)
	}
	return p, nil
}

// saveAbout updates the about fields, replaces the interest set and advances the
// onboarding step to PHOTOS, atomically.
func (r *Repository) saveAbout(ctx context.Context, userID, profileID uuid.UUID, req aboutRequest, interestIDs []uuid.UUID) error {
	desc := normalizeDescription(req.Description)
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE profiles
			SET description = $2, dating_goal = $3, height_cm = $4, weight_kg = $5,
			    show_zodiac = $6, updated_at = now()
			WHERE id = $1`,
			profileID, desc, req.DatingGoal, req.Height, req.Weight, req.ShowZodiac); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM profile_interests WHERE profile_id = $1`, profileID); err != nil {
			return err
		}
		for _, id := range interestIDs {
			if _, err := tx.Exec(ctx,
				`INSERT INTO profile_interests (profile_id, interest_id) VALUES ($1, $2)`,
				profileID, id); err != nil {
				return err
			}
		}
		return advanceStep(ctx, tx, userID, "PHOTOS")
	})
	if err != nil {
		return fmt.Errorf("profile: save about: %w", err)
	}
	return nil
}

// advanceStep moves the user's onboarding step forward to target, never backward.
func advanceStep(ctx context.Context, tx pgx.Tx, userID uuid.UUID, target string) error {
	_, err := tx.Exec(ctx, `
		UPDATE users SET onboarding_step = $2
		WHERE id = $1
		  AND array_position(ARRAY['BASICS','ABOUT','PHOTOS','DONE'], onboarding_step)
		    < array_position(ARRAY['BASICS','ABOUT','PHOTOS','DONE'], $2)`,
		userID, target)
	return err
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
