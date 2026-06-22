-- 000006_user_geo (up)
-- User location for distance-based discovery. Coordinates are stored server-side
-- only and never returned via API (backend computes distance). One row per user,
-- updated by POST /my-geo (app open + every ~10 min).

CREATE TABLE user_geo (
    user_id    UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    lat        DOUBLE PRECISION NOT NULL,
    lng        DOUBLE PRECISION NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Bounding-box prefilter for distance queries.
CREATE INDEX user_geo_lat_lng_idx ON user_geo (lat, lng);
