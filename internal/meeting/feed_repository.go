package meeting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// viewerEffectivePoint returns the viewer's effective location (lat,lng), or
// has=false if they have none.
func (r *Repository) viewerEffectivePoint(ctx context.Context, userID uuid.UUID) (lat, lng float64, has bool, err error) {
	err = r.pool.QueryRow(ctx,
		`SELECT ST_Y(effective_point::geometry), ST_X(effective_point::geometry)
		 FROM user_geo WHERE user_id = $1 AND effective_point IS NOT NULL`, userID,
	).Scan(&lat, &lng)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, fmt.Errorf("meeting: viewer point: %w", err)
	}
	return lat, lng, true, nil
}

type feedQuery struct {
	ViewerProfileID uuid.UUID
	HasViewerPoint  bool
	ViewerLat       float64
	ViewerLng       float64
	Mode            string // "radius" | "city"
	RadiusMeters    float64
	CityID          uuid.UUID
	Gender, Goal    *string
	AgeMin, AgeMax  int
	TagIDs          []uuid.UUID
	Sort            string
	Cursor          *feedCursor
	Limit           int
}

type feedRow struct {
	AdID            uuid.UUID
	Description     string
	CreatedAt       time.Time
	AuthorProfileID uuid.UUID
	Name            string
	BirthDate       time.Time
	Gender          string
	AvatarURL       *string
	CityID          *uuid.UUID
	CityName        *string
	DistKm          *float64
}

// feedPage runs one keyset page of the discovery query (PostGIS).
// Distance/city come from the author's effective location (user_geo + cities);
// meeting_ads stores no location.
func (r *Repository) feedPage(ctx context.Context, q feedQuery) ([]feedRow, error) {
	args := []any{
		q.ViewerProfileID,         // $1
		q.ViewerLng, q.ViewerLat,  // $2, $3
		q.HasViewerPoint,          // $4
		q.Gender, q.Goal,          // $5, $6
		q.AgeMin, q.AgeMax,        // $7, $8
		len(q.TagIDs),             // $9
		uuidStrings(q.TagIDs),     // $10
	}

	modeWhere := ""
	switch q.Mode {
	case "city":
		args = append(args, q.CityID) // $11
		modeWhere = `AND aug.resolved_city_id = $11`
	default: // radius
		args = append(args, q.RadiusMeters) // $11
		modeWhere = `AND ST_DWithin(ST_SetSRID(ST_MakePoint($2, $3), 4326)::geography, aug.effective_point, $11)`
	}
	args = append(args, q.Limit) // $12

	sql := `
WITH cand AS (
    SELECT a.id, a.description, a.created_at,
           p.id AS author_profile_id, p.name, p.birth_date, p.gender, ph.url AS avatar_url,
           ac.id AS city_id, COALESCE(ac.name_ru, ac.name) AS city_name,
           CASE WHEN $4 THEN ST_Distance(ST_SetSRID(ST_MakePoint($2, $3), 4326)::geography,
                                         aug.effective_point) / 1000.0 END AS dist_km
    FROM meeting_ads a
    JOIN profiles p ON p.id = a.author_profile_id
    JOIN user_geo aug ON aug.user_id = p.user_id
    LEFT JOIN cities ac ON ac.id = aug.resolved_city_id
    LEFT JOIN photos ph ON ph.profile_id = p.id AND ph.position = 0
    WHERE a.status = 'ACTIVE' AND a.expires_at > now()
      AND a.author_profile_id <> $1
      AND aug.effective_point IS NOT NULL
      AND NOT EXISTS (SELECT 1 FROM meeting_candidates c
                      WHERE c.meeting_ad_id = a.id AND c.profile_id = $1)
      AND ($5::text IS NULL OR p.gender = $5)
      AND ($6::text IS NULL OR p.dating_goal = $6)
      AND date_part('year', age(p.birth_date)) BETWEEN $7 AND $8
      AND ($9 = 0 OR EXISTS (SELECT 1 FROM meeting_ad_tags mt
                             WHERE mt.meeting_ad_id = a.id AND mt.tag_id = ANY($10::uuid[])))
      ` + modeWhere + `
)
SELECT id, description, created_at, author_profile_id, name, birth_date, gender,
       avatar_url, city_id, city_name, dist_km
FROM cand
WHERE TRUE`

	switch q.Sort {
	case sortDate:
		if q.Cursor != nil {
			ts, _ := time.Parse(time.RFC3339Nano, derefStr(q.Cursor.Ts))
			args = append(args, ts, uuid.MustParse(q.Cursor.ID)) // $13, $14
			sql += ` AND (created_at < $13 OR (created_at = $13 AND id < $14))`
		}
		sql += ` ORDER BY created_at DESC, id DESC LIMIT $12`
	default: // distance
		if q.Cursor != nil {
			args = append(args, derefF64(q.Cursor.Dist), uuid.MustParse(q.Cursor.ID)) // $13, $14
			sql += ` AND (dist_km > $13 OR (dist_km = $13 AND id > $14))`
		}
		sql += ` ORDER BY dist_km ASC, id ASC LIMIT $12`
	}

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("meeting: feed query: %w", err)
	}
	defer rows.Close()

	out := make([]feedRow, 0, q.Limit)
	for rows.Next() {
		var f feedRow
		if err := rows.Scan(&f.AdID, &f.Description, &f.CreatedAt, &f.AuthorProfileID,
			&f.Name, &f.BirthDate, &f.Gender, &f.AvatarURL, &f.CityID, &f.CityName, &f.DistKm); err != nil {
			return nil, fmt.Errorf("meeting: scan feed: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// tagsByAdIDs returns tags grouped per ad id (batched, no N+1).
func (r *Repository) tagsByAdIDs(ctx context.Context, adIDs []uuid.UUID) (map[uuid.UUID][]Tag, error) {
	res := make(map[uuid.UUID][]Tag, len(adIDs))
	if len(adIDs) == 0 {
		return res, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT mt.meeting_ad_id, t.id, t.value
		FROM meeting_ad_tags mt JOIN meeting_tags t ON t.id = mt.tag_id
		WHERE mt.meeting_ad_id = ANY($1::uuid[])
		ORDER BY t.value`, uuidStrings(adIDs))
	if err != nil {
		return nil, fmt.Errorf("meeting: feed tags: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var adID uuid.UUID
		var t Tag
		if err := rows.Scan(&adID, &t.ID, &t.Value); err != nil {
			return nil, fmt.Errorf("meeting: scan feed tag: %w", err)
		}
		res[adID] = append(res[adID], t)
	}
	return res, rows.Err()
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefF64(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
