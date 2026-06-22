-- 000003_profile (up)
-- Profile/onboarding slice: interests reference (seeded), onboarding step on
-- users, profiles and their interests join.

-- Interests reference (НСИ {id, value}); value is the stable key, i18n on client.
CREATE TABLE interests (
    id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    value TEXT NOT NULL UNIQUE
);

-- Seed (~50 keys, see docs/interests.md). Idempotent on value.
INSERT INTO interests (value) VALUES
    ('COFFEE'), ('TEA'), ('WINE'), ('CRAFT_BEER'), ('COCKTAILS'), ('BRUNCH'),
    ('STREET_FOOD'), ('COOKING'), ('BAKING'), ('WALKS'), ('HIKING'), ('CYCLING'),
    ('RUNNING'), ('GYM'), ('YOGA'), ('SWIMMING'), ('SKIING'), ('SNOWBOARDING'),
    ('SKATEBOARDING'), ('CLIMBING'), ('FOOTBALL'), ('BASKETBALL'), ('TENNIS'),
    ('DANCING'), ('MUSIC'), ('CONCERTS'), ('LIVE_GIGS'), ('KARAOKE'), ('MOVIES'),
    ('THEATRE'), ('STANDUP'), ('ART'), ('EXHIBITIONS'), ('MUSEUMS'),
    ('PHOTOGRAPHY'), ('READING'), ('BOARD_GAMES'), ('VIDEO_GAMES'),
    ('TABLE_TENNIS'), ('BILLIARDS'), ('BOWLING'), ('TRAVEL'), ('ROAD_TRIPS'),
    ('CAMPING'), ('FISHING'), ('PETS'), ('VOLUNTEERING'), ('MEDITATION'),
    ('LANGUAGES'), ('COWORKING'), ('STARTUPS'), ('TECH')
ON CONFLICT (value) DO NOTHING;

-- Onboarding progress is owned here; auth echoes it. Invariant:
-- is_created_profile === (onboarding_step = 'DONE').
ALTER TABLE users
    ADD COLUMN onboarding_step TEXT NOT NULL DEFAULT 'BASICS'
        CHECK (onboarding_step IN ('BASICS', 'ABOUT', 'PHOTOS', 'DONE'));

CREATE TABLE profiles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL UNIQUE REFERENCES users (id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    gender      TEXT        NOT NULL CHECK (gender IN ('MALE', 'FEMALE')),
    birth_date  DATE        NOT NULL,
    city        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    dating_goal TEXT        CHECK (dating_goal IN ('FRIENDSHIP', 'LONG_TERM_RELATIONSHIP', 'DATES', 'NOTHING_SERIOUS')),
    height_cm   REAL,
    weight_kg   REAL,
    show_zodiac BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE profile_interests (
    profile_id  UUID NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    interest_id UUID NOT NULL REFERENCES interests (id) ON DELETE RESTRICT,
    PRIMARY KEY (profile_id, interest_id)
);
