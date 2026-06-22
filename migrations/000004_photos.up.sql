-- 000004_photos (up)
-- Profile photos (onboarding step 3). position 0 = main (with crop), others crop null.

CREATE TABLE photos (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id UUID        NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    object_key TEXT        NOT NULL, -- storage key, used for deletion
    url        TEXT        NOT NULL, -- public URL
    position   INTEGER     NOT NULL, -- 0 = main (avatar)
    crop       JSONB,                -- {x,y,size} normalized [0,1]; only for the main photo
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX photos_profile_position_key ON photos (profile_id, position);
CREATE INDEX photos_profile_idx ON photos (profile_id);
