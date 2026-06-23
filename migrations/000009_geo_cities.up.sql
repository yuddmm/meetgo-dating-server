-- 000009_geo_cities (up)
-- PostGIS-based geo: cities gazetteer (geonames), user effective location, city
-- derived (no manual city in onboarding). meeting_ads is NOT changed — ad distance
-- and city come from the author's effective location.

CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Cities gazetteer (imported from geonames by cmd/geonames-import).
CREATE TABLE cities (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    geoname_id BIGINT      NOT NULL UNIQUE,
    name       TEXT        NOT NULL,           -- international/ascii name
    name_ru    TEXT,                           -- localized (Cyrillic) if available
    country    TEXT        NOT NULL,           -- ISO country code (e.g. RU)
    region     TEXT,                           -- admin1
    population INTEGER     NOT NULL DEFAULT 0,
    location   geography(Point, 4326) NOT NULL -- centroid
);

CREATE INDEX cities_location_gix ON cities USING gist (location);
CREATE INDEX cities_name_trgm ON cities USING gin (name gin_trgm_ops, name_ru gin_trgm_ops);

-- User location: GPS (raw) + mode + manual override + effective point/city.
DROP TABLE IF EXISTS user_geo;
CREATE TABLE user_geo (
    user_id          UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    mode             TEXT NOT NULL DEFAULT 'AUTO' CHECK (mode IN ('AUTO', 'MANUAL')),
    manual_city_id   UUID REFERENCES cities (id),
    gps_lat          DOUBLE PRECISION,
    gps_lng          DOUBLE PRECISION,
    gps_accuracy     REAL,
    gps_updated_at   TIMESTAMPTZ,
    effective_point  geography(Point, 4326),   -- manual centroid (MANUAL) or GPS point (AUTO)
    resolved_city_id UUID REFERENCES cities (id),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX user_geo_effective_gix ON user_geo USING gist (effective_point);
CREATE INDEX user_geo_resolved_city_idx ON user_geo (resolved_city_id);

-- City is no longer entered in onboarding; it is derived from effective location.
ALTER TABLE profiles DROP COLUMN city;
