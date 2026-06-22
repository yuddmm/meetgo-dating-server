-- 000002_auth (up)
-- Auth slice: users, refresh sessions, OTP codes, OTP send events (rate-limit).
-- Emails are normalized to lowercase by the application before any query.

CREATE TABLE users (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email              TEXT        NOT NULL UNIQUE,
    is_created_profile BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Single-session: on login all prior sessions are revoked and one new row is created.
-- Revoked rows are kept (not deleted) so a re-used refresh token can be told apart
-- (SESSION_REVOKED) from an unknown one (INVALID_REFRESH).
CREATE TABLE refresh_sessions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash BYTEA       NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX refresh_sessions_token_hash_key ON refresh_sessions (token_hash);
CREATE INDEX refresh_sessions_user_id_idx ON refresh_sessions (user_id);

-- One active OTP per email: send_code upserts on the email, invalidating the previous code.
CREATE TABLE otp_codes (
    email      TEXT        PRIMARY KEY,
    code_hash  BYTEA       NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    attempts   INTEGER     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per send_code call; used for cooldown and per-email / per-IP rate limits.
CREATE TABLE otp_send_events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email      TEXT        NOT NULL,
    ip         TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX otp_send_events_email_created_idx ON otp_send_events (email, created_at);
CREATE INDEX otp_send_events_ip_created_idx ON otp_send_events (ip, created_at);
