-- 000007_meetings (up)
-- Meeting ads: tags reference (seeded), ads (one active per profile), ad↔tags join.

CREATE TABLE meeting_tags (
    id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    value TEXT NOT NULL UNIQUE
);

-- Seed (~50 keys, see docs/meeting-tags.md). Idempotent on value.
INSERT INTO meeting_tags (value) VALUES
    ('DATE'), ('CINEMA'), ('PARK_WALK'), ('CITY_WALK'), ('EMBANKMENT_WALK'),
    ('PHOTO_WALK'), ('COFFEE'), ('BREAKFAST'), ('BRUNCH'), ('LUNCH'), ('DINNER'),
    ('BAR_HOP'), ('COCKTAIL_BAR'), ('WINE_TASTING'), ('CRAFT_BEER'), ('PICNIC'),
    ('EXHIBITION'), ('MUSEUM'), ('THEATRE'), ('CONCERT'), ('STANDUP_SHOW'),
    ('KARAOKE'), ('NIGHTCLUB'), ('DANCING'), ('BOARD_GAMES'), ('BOWLING'),
    ('BILLIARDS'), ('KARTING'), ('ESCAPE_ROOM'), ('ICE_SKATING'), ('ROLLER_SKATING'),
    ('BIKE_RIDE'), ('RUN_TOGETHER'), ('GYM_TOGETHER'), ('YOGA_SESSION'), ('SWIMMING'),
    ('HIKING'), ('CAMPING'), ('FISHING'), ('ROAD_TRIP'), ('BEACH'), ('SHOPPING'),
    ('MARKET'), ('FOOD_COURT'), ('COWORKING'), ('WORK_FROM_CAFE'), ('STUDY_TOGETHER'),
    ('LANGUAGE_EXCHANGE'), ('ART_WORKSHOP'), ('COOKING_TOGETHER'), ('GAMING'),
    ('SPORTS_MATCH')
ON CONFLICT (value) DO NOTHING;

CREATE TABLE meeting_ads (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    author_profile_id UUID        NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    description       TEXT        NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'ACTIVE'
                          CHECK (status IN ('ACTIVE', 'EXPIRED', 'CLOSED')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at        TIMESTAMPTZ NOT NULL
);

-- One active ad per profile (partial unique; expired/closed don't count).
CREATE UNIQUE INDEX meeting_ads_one_active_idx
    ON meeting_ads (author_profile_id) WHERE status = 'ACTIVE';
CREATE INDEX meeting_ads_status_idx ON meeting_ads (status);

CREATE TABLE meeting_ad_tags (
    meeting_ad_id UUID NOT NULL REFERENCES meeting_ads (id) ON DELETE CASCADE,
    tag_id        UUID NOT NULL REFERENCES meeting_tags (id) ON DELETE RESTRICT,
    PRIMARY KEY (meeting_ad_id, tag_id)
);
