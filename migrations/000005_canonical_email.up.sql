-- 000005_canonical_email (up)
-- De-duplication key to block multi-accounting via Gmail dot/plus tricks.
-- We still send OTP to the address the user entered (users.email); identity is
-- matched on canonical_email (lowercased, plus-stripped, Gmail dots folded).

ALTER TABLE users ADD COLUMN canonical_email TEXT;

-- Backfill existing rows (app computes the real canonical for new inserts).
UPDATE users SET canonical_email = lower(email) WHERE canonical_email IS NULL;

ALTER TABLE users ALTER COLUMN canonical_email SET NOT NULL;
CREATE UNIQUE INDEX users_canonical_email_key ON users (canonical_email);
