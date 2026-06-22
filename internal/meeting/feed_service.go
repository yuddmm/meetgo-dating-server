package meeting

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
)

const kmPerDegLat = 111.045

// Feed returns a keyset page of nearby active ads. Empty when the viewer has no
// stored location or no (completed) profile.
func (s *Service) Feed(ctx context.Context, userID uuid.UUID, f feedFilters) (feedResponse, error) {
	empty := feedResponse{Items: []feedItem{}}

	profileID, created, found, err := s.repo.profileForUser(ctx, userID)
	if err != nil {
		return feedResponse{}, err
	}
	if !found || !created {
		return empty, nil
	}

	// Pin the viewer's origin for the whole scroll session: first page reads the
	// live stored geo; later pages reuse the origin carried in the cursor, so
	// distances/ordering stay stable even if the viewer moves mid-scroll.
	var lat0, lng0 float64
	if f.Cursor != nil {
		lat0, lng0 = f.Cursor.Lat, f.Cursor.Lng
	} else {
		var hasGeo bool
		lat0, lng0, hasGeo, err = s.repo.viewerGeo(ctx, userID)
		if err != nil {
			return feedResponse{}, err
		}
		if !hasGeo {
			return empty, nil
		}
	}

	latMin, latMax, lngMin, lngMax := boundingBox(lat0, lng0, f.Radius)
	rows, err := s.repo.feedPage(ctx, feedQuery{
		Lat0: lat0, Lng0: lng0,
		LatMin: latMin, LatMax: latMax, LngMin: lngMin, LngMax: lngMax,
		ViewerProfileID: profileID,
		Gender:          f.Gender,
		Goal:            f.Goal,
		AgeMin:          f.AgeMin,
		AgeMax:          f.AgeMax,
		TagIDs:          f.TagIDs,
		Radius:          f.Radius,
		Sort:            f.Sort,
		Cursor:          f.Cursor,
		Limit:           f.Limit,
	})
	if err != nil {
		return feedResponse{}, err
	}
	if len(rows) == 0 {
		return empty, nil
	}

	adIDs := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		adIDs[i] = r.AdID
	}
	tagsByAd, err := s.repo.tagsByAdIDs(ctx, adIDs)
	if err != nil {
		return feedResponse{}, err
	}

	now := time.Now()
	items := make([]feedItem, len(rows))
	for i, r := range rows {
		tags := tagsByAd[r.AdID]
		if tags == nil {
			tags = []Tag{}
		}
		items[i] = feedItem{
			ID:          r.AdID,
			Description: r.Description,
			Tags:        tags,
			DistanceKm:  distanceKm(r.Dist),
			Author: feedAuthor{
				ID:        r.AuthorProfileID,
				Name:      r.Name,
				Age:       ageFromBirth(r.BirthDate, now),
				Gender:    r.Gender,
				AvatarURL: r.AvatarURL,
			},
		}
	}

	resp := feedResponse{Items: items}
	if len(rows) == f.Limit {
		resp.NextCursor = nextCursor(f.Sort, rows[len(rows)-1], lat0, lng0)
	}
	return resp, nil
}

func nextCursor(sort string, last feedRow, lat0, lng0 float64) *string {
	c := feedCursor{Sort: sort, ID: last.AdID.String(), Lat: lat0, Lng: lng0}
	if sort == sortDate {
		ts := last.CreatedAt.UTC().Format(time.RFC3339Nano)
		c.Ts = &ts
	} else {
		d := last.Dist
		c.Dist = &d
	}
	s := encodeCursor(c)
	return &s
}

// boundingBox returns a lat/lng box around (lat0,lng0) for the radius (km),
// used as a cheap index prefilter before the exact haversine distance.
func boundingBox(lat0, lng0, radiusKm float64) (latMin, latMax, lngMin, lngMax float64) {
	dLat := radiusKm / kmPerDegLat
	cosLat := math.Cos(lat0 * math.Pi / 180)
	if cosLat < 0.01 {
		cosLat = 0.01 // guard near the poles
	}
	dLng := radiusKm / (kmPerDegLat * cosLat)
	return lat0 - dLat, lat0 + dLat, lng0 - dLng, lng0 + dLng
}
