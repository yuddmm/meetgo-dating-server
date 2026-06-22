-- 000003_profile (down)
DROP TABLE IF EXISTS profile_interests;
DROP TABLE IF EXISTS profiles;
ALTER TABLE users DROP COLUMN IF EXISTS onboarding_step;
DROP TABLE IF EXISTS interests;
