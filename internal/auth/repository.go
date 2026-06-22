package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the persistence layer for the auth module (hand-written pgx).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

type userRow struct {
	ID               uuid.UUID
	Email            string
	IsCreatedProfile bool
	OnboardingStep   string
}

type otpRow struct {
	CodeHash  []byte
	ExpiresAt time.Time
	Attempts  int
}

type sessionRow struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
	RevokedAt *time.Time
}

// --- Users ---

// userByCanonicalEmail returns the user matching the canonical email (used for
// de-duplication), or nil if none exists.
func (r *Repository) userByCanonicalEmail(ctx context.Context, canonical string) (*userRow, error) {
	var u userRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, is_created_profile, onboarding_step FROM users WHERE canonical_email = $1`, canonical,
	).Scan(&u.ID, &u.Email, &u.IsCreatedProfile, &u.OnboardingStep)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: user by canonical email: %w", err)
	}
	return &u, nil
}

// userByID returns the user for an id, or nil if none exists.
func (r *Repository) userByID(ctx context.Context, id uuid.UUID) (*userRow, error) {
	var u userRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, is_created_profile, onboarding_step FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.IsCreatedProfile, &u.OnboardingStep)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: user by id: %w", err)
	}
	return &u, nil
}

// createUser inserts a new user with the entered email and its canonical key.
func (r *Repository) createUser(ctx context.Context, email, canonical string) (*userRow, error) {
	var u userRow
	err := r.pool.QueryRow(ctx,
		`INSERT INTO users (email, canonical_email) VALUES ($1, $2)
		 RETURNING id, email, is_created_profile, onboarding_step`, email, canonical,
	).Scan(&u.ID, &u.Email, &u.IsCreatedProfile, &u.OnboardingStep)
	if err != nil {
		return nil, fmt.Errorf("auth: create user: %w", err)
	}
	return &u, nil
}

// --- OTP ---

// upsertOTP stores a fresh code for the email, replacing any previous one and
// resetting the attempt counter (only the latest code is valid).
func (r *Repository) upsertOTP(ctx context.Context, email string, codeHash []byte, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO otp_codes (email, code_hash, expires_at, attempts, created_at)
		VALUES ($1, $2, $3, 0, now())
		ON CONFLICT (email) DO UPDATE
		SET code_hash = EXCLUDED.code_hash, expires_at = EXCLUDED.expires_at,
		    attempts = 0, created_at = now()`,
		email, codeHash, expiresAt)
	if err != nil {
		return fmt.Errorf("auth: upsert otp: %w", err)
	}
	return nil
}

// otpByEmail returns the active code for the email, or nil if none.
func (r *Repository) otpByEmail(ctx context.Context, email string) (*otpRow, error) {
	var o otpRow
	err := r.pool.QueryRow(ctx,
		`SELECT code_hash, expires_at, attempts FROM otp_codes WHERE email = $1`, email,
	).Scan(&o.CodeHash, &o.ExpiresAt, &o.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: otp by email: %w", err)
	}
	return &o, nil
}

// incrementOTPAttempts bumps and returns the attempt counter for the email.
func (r *Repository) incrementOTPAttempts(ctx context.Context, email string) (int, error) {
	var attempts int
	err := r.pool.QueryRow(ctx,
		`UPDATE otp_codes SET attempts = attempts + 1 WHERE email = $1 RETURNING attempts`, email,
	).Scan(&attempts)
	if err != nil {
		return 0, fmt.Errorf("auth: increment otp attempts: %w", err)
	}
	return attempts, nil
}

// deleteOTP removes the code for the email (after success or lockout).
func (r *Repository) deleteOTP(ctx context.Context, email string) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM otp_codes WHERE email = $1`, email); err != nil {
		return fmt.Errorf("auth: delete otp: %w", err)
	}
	return nil
}

// --- Send events (rate limiting) ---

func (r *Repository) insertSendEvent(ctx context.Context, email, ip string) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO otp_send_events (email, ip) VALUES ($1, $2)`, email, ip); err != nil {
		return fmt.Errorf("auth: insert send event: %w", err)
	}
	return nil
}

// lastSendAt returns the timestamp of the most recent send for the email.
func (r *Repository) lastSendAt(ctx context.Context, email string) (time.Time, bool, error) {
	var ts time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT created_at FROM otp_send_events WHERE email = $1 ORDER BY created_at DESC LIMIT 1`, email,
	).Scan(&ts)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("auth: last send at: %w", err)
	}
	return ts, true, nil
}

func (r *Repository) countSendsByEmailSince(ctx context.Context, email string, since time.Time) (int, error) {
	return r.countSends(ctx, "email", email, since)
}

func (r *Repository) countSendsByIPSince(ctx context.Context, ip string, since time.Time) (int, error) {
	return r.countSends(ctx, "ip", ip, since)
}

func (r *Repository) countSends(ctx context.Context, column, value string, since time.Time) (int, error) {
	var n int
	// column is a fixed internal literal ("email"/"ip"), never user input.
	q := fmt.Sprintf(`SELECT count(*) FROM otp_send_events WHERE %s = $1 AND created_at > $2`, column)
	if err := r.pool.QueryRow(ctx, q, value, since).Scan(&n); err != nil {
		return 0, fmt.Errorf("auth: count sends: %w", err)
	}
	return n, nil
}

// --- Sessions ---

// resetSession revokes all active sessions for the user and creates one new
// session (single-session login), atomically.
func (r *Repository) resetSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiresAt time.Time) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE refresh_sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
			userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO refresh_sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
			userID, tokenHash, expiresAt)
		return err
	})
}

// rotateSession revokes the current session and creates a replacement, atomically.
func (r *Repository) rotateSession(ctx context.Context, oldID, userID uuid.UUID, tokenHash []byte, expiresAt time.Time) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE refresh_sessions SET revoked_at = now() WHERE id = $1`, oldID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO refresh_sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
			userID, tokenHash, expiresAt)
		return err
	})
}

// sessionByHash returns the session for a refresh token hash, or nil if none.
func (r *Repository) sessionByHash(ctx context.Context, hash []byte) (*sessionRow, error) {
	var s sessionRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, expires_at, revoked_at FROM refresh_sessions WHERE token_hash = $1`, hash,
	).Scan(&s.ID, &s.UserID, &s.ExpiresAt, &s.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: session by hash: %w", err)
	}
	return &s, nil
}

// revokeUserSessions revokes all active sessions for the user (logout).
func (r *Repository) revokeUserSessions(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE refresh_sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID); err != nil {
		return fmt.Errorf("auth: revoke user sessions: %w", err)
	}
	return nil
}

func (r *Repository) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return fmt.Errorf("auth: tx: %w", err)
	}
	return tx.Commit(ctx)
}
