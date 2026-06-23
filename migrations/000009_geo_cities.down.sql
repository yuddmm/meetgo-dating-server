-- 000009_geo_cities (down)
ALTER TABLE profiles ADD COLUMN city TEXT NOT NULL DEFAULT '';

DROP TABLE IF EXISTS user_geo;
CREATE TABLE user_geo (
    user_id    UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    lat        DOUBLE PRECISION NOT NULL,
    lng        DOUBLE PRECISION NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX user_geo_lat_lng_idx ON user_geo (lat, lng);

DROP TABLE IF EXISTS cities;
-- extensions left installed intentionally.
