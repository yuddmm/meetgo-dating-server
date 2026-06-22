package meeting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type feedQuery struct {
	Lat0, Lng0                     float64
	LatMin, LatMax, LngMin, LngMax float64
	ViewerProfileID                uuid.UUID
	Gender, Goal                   *string
	AgeMin, AgeMax                 int
	TagIDs                         []uuid.UUID
	Radius                         float64
	Sort                           string
	Cursor                         *feedCursor
	Limit                          int
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
	Dist            float64
}

// viewerGeo returns the user's stored location, or found=false if none.
func (r *Repository) viewerGeo(ctx context.Context, userID uuid.UUID) (lat, lng float64, found bool, err error) {
	err = r.pool.QueryRow(ctx, `SELECT lat, lng FROM user_geo WHERE user_id = $1`, userID).Scan(&lat, &lng)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, fmt.Errorf("meeting: viewer geo: %w", err)
	}
	return lat, lng, true, nil
}

const feedBaseSQL = `
WITH cand AS (
    SELECT a.id, a.description, a.created_at,
           p.id AS author_profile_id, p.name, p.birth_date, p.gender, ph.url AS avatar_url,
           (6371 * 2 * asin(sqrt(
               power(sin(radians(g.lat - $1) / 2), 2) +
               cos(radians($1)) * cos(radians(g.lat)) *
               power(sin(radians(g.lng - $2) / 2), 2)
           ))) AS dist
    FROM meeting_ads a
    JOIN profiles p ON p.id = a.author_profile_id
    JOIN users au ON au.id = p.user_id
    JOIN user_geo g ON g.user_id = au.id
    LEFT JOIN photos ph ON ph.profile_id = p.id AND ph.position = 0
    WHERE a.status = 'ACTIVE' AND a.expires_at > now()
      AND a.author_profile_id <> $3
      AND g.lat BETWEEN $4 AND $5 AND g.lng BETWEEN $6 AND $7
      AND NOT EXISTS (SELECT 1 FROM meeting_candidates c
                      WHERE c.meeting_ad_id = a.id AND c.profile_id = $3)
      AND ($8::text IS NULL OR p.gender = $8)
      AND ($9::text IS NULL OR p.dating_goal = $9)
      AND date_part('year', age(p.birth_date)) BETWEEN $10 AND $11
      AND ($12 = 0 OR EXISTS (SELECT 1 FROM meeting_ad_tags mt
                              WHERE mt.meeting_ad_id = a.id AND mt.tag_id = ANY($13::uuid[])))
)
SELECT id, description, created_at, author_profile_id, name, birth_date, gender, avatar_url, dist
FROM cand
WHERE dist <= $14`

// feedPage runs one keyset page of the discovery query.
func (r *Repository) feedPage(ctx context.Context, q feedQuery) ([]feedRow, error) {
	args := []any{
		q.Lat0, q.Lng0, q.ViewerProfileID,
		q.LatMin, q.LatMax, q.LngMin, q.LngMax,
		q.Gender, q.Goal, q.AgeMin, q.AgeMax,
		len(q.TagIDs), uuidStrings(q.TagIDs), q.Radius, q.Limit,
	}

	sql := feedBaseSQL
	switch q.Sort {
	case sortDate:
		if q.Cursor != nil {
			ts, _ := time.Parse(time.RFC3339Nano, derefStr(q.Cursor.Ts))
			args = append(args, ts, uuid.MustParse(q.Cursor.ID))
			sql += ` AND (created_at < $16 OR (created_at = $16 AND id < $17))`
		}
		sql += ` ORDER BY created_at DESC, id DESC LIMIT $15`
	default: // distance
		if q.Cursor != nil {
			args = append(args, derefF64(q.Cursor.Dist), uuid.MustParse(q.Cursor.ID))
			sql += ` AND (dist > $16 OR (dist = $16 AND id > $17))`
		}
		sql += ` ORDER BY dist ASC, id ASC LIMIT $15`
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
			&f.Name, &f.BirthDate, &f.Gender, &f.AvatarURL, &f.Dist); err != nil {
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
