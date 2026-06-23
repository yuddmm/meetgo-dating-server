// Package geo stores users' location and serves the cities gazetteer. Raw
// coordinates are write-only via the API (never returned); the backend computes
// distance. City is derived from GPS (reverse-geocode) or a manual override.
package geo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the persistence layer (PostGIS).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

type geoRow struct {
	Mode         string
	GpsLat       *float64
	GpsLng       *float64
	GpsUpdatedAt *time.Time
}

// currentGeo returns the user's location row, or nil if none exists.
func (r *Repository) currentGeo(ctx context.Context, userID uuid.UUID) (*geoRow, error) {
	var g geoRow
	err := r.pool.QueryRow(ctx,
		`SELECT mode, gps_lat, gps_lng, gps_updated_at FROM user_geo WHERE user_id = $1`, userID,
	).Scan(&g.Mode, &g.GpsLat, &g.GpsLng, &g.GpsUpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("geo: current: %w", err)
	}
	return &g, nil
}

// nearestCityID returns the closest city to a point (PostGIS KNN), or nil if the
// gazetteer is empty.
func (r *Repository) nearestCityID(ctx context.Context, lat, lng float64) (*uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM cities ORDER BY location <-> ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography LIMIT 1`,
		lng, lat,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("geo: nearest city: %w", err)
	}
	return &id, nil
}

// applyGPS upserts the GPS fix. Effective point/city are recomputed only in AUTO
// mode; MANUAL keeps its chosen city.
func (r *Repository) applyGPS(ctx context.Context, userID uuid.UUID, lat, lng float64, accuracy *float64, resolvedCity *uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_geo (user_id, mode, gps_lat, gps_lng, gps_accuracy, gps_updated_at,
		                      effective_point, resolved_city_id, updated_at)
		VALUES ($1, 'AUTO', $2, $3, $4, now(),
		        ST_SetSRID(ST_MakePoint($3, $2), 4326)::geography, $5, now())
		ON CONFLICT (user_id) DO UPDATE SET
		    gps_lat = $2, gps_lng = $3, gps_accuracy = $4, gps_updated_at = now(), updated_at = now(),
		    effective_point = CASE WHEN user_geo.mode = 'MANUAL' THEN user_geo.effective_point
		                           ELSE ST_SetSRID(ST_MakePoint($3, $2), 4326)::geography END,
		    resolved_city_id = CASE WHEN user_geo.mode = 'MANUAL' THEN user_geo.resolved_city_id
		                            ELSE $5 END`,
		userID, lat, lng, accuracy, resolvedCity)
	if err != nil {
		return fmt.Errorf("geo: apply gps: %w", err)
	}
	return nil
}

// setManual switches to MANUAL on the given city; returns false if the city is unknown.
func (r *Repository) setManual(ctx context.Context, userID, cityID uuid.UUID) (bool, error) {
	if exists, err := r.cityExists(ctx, cityID); err != nil || !exists {
		return false, err
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_geo (user_id, mode, manual_city_id, effective_point, resolved_city_id, updated_at)
		VALUES ($1, 'MANUAL', $2, (SELECT location FROM cities WHERE id = $2), $2, now())
		ON CONFLICT (user_id) DO UPDATE SET
		    mode = 'MANUAL', manual_city_id = $2,
		    effective_point = (SELECT location FROM cities WHERE id = $2),
		    resolved_city_id = $2, updated_at = now()`,
		userID, cityID)
	if err != nil {
		return false, fmt.Errorf("geo: set manual: %w", err)
	}
	return true, nil
}

// setAuto switches to AUTO; effective point/city recomputed from the stored GPS
// (resolvedCity = nearest to GPS, or nil).
func (r *Repository) setAuto(ctx context.Context, userID uuid.UUID, resolvedCity *uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE user_geo SET mode = 'AUTO', manual_city_id = NULL,
		    effective_point = CASE WHEN gps_lat IS NOT NULL
		        THEN ST_SetSRID(ST_MakePoint(gps_lng, gps_lat), 4326)::geography ELSE NULL END,
		    resolved_city_id = $2, updated_at = now()
		WHERE user_id = $1`, userID, resolvedCity)
	if err != nil {
		return fmt.Errorf("geo: set auto: %w", err)
	}
	return nil
}

func (r *Repository) cityExists(ctx context.Context, cityID uuid.UUID) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cities WHERE id = $1)`, cityID).Scan(&exists); err != nil {
		return false, fmt.Errorf("geo: city exists: %w", err)
	}
	return exists, nil
}

// modeAndCity returns the user's current mode and effective city (for responses).
func (r *Repository) modeAndCity(ctx context.Context, userID uuid.UUID) (string, *cityDTO, error) {
	var mode string
	var c cityDTO
	var name, region *string
	var cid *uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT ug.mode, c.id, COALESCE(c.name_ru, c.name), c.region
		FROM user_geo ug LEFT JOIN cities c ON c.id = ug.resolved_city_id
		WHERE ug.user_id = $1`, userID).Scan(&mode, &cid, &name, &region)
	if errors.Is(err, pgx.ErrNoRows) {
		return "AUTO", nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("geo: mode and city: %w", err)
	}
	if cid == nil {
		return mode, nil, nil
	}
	c.ID, c.Name, c.Region = *cid, *name, region
	return mode, &c, nil
}

// searchCities returns cities matching q (substring), most populous first.
func (r *Repository) searchCities(ctx context.Context, q string, limit int) ([]cityDTO, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, COALESCE(name_ru, name), region FROM cities
		WHERE name ILIKE '%' || $1 || '%' OR name_ru ILIKE '%' || $1 || '%'
		ORDER BY population DESC LIMIT $2`, q, limit)
	if err != nil {
		return nil, fmt.Errorf("geo: search cities: %w", err)
	}
	defer rows.Close()
	out := make([]cityDTO, 0, limit)
	for rows.Next() {
		var c cityDTO
		if err := rows.Scan(&c.ID, &c.Name, &c.Region); err != nil {
			return nil, fmt.Errorf("geo: scan city: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
