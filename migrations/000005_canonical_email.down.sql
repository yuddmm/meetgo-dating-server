-- 000005_canonical_email (down)
DROP INDEX IF EXISTS users_canonical_email_key;
ALTER TABLE users DROP COLUMN IF EXISTS canonical_email;
