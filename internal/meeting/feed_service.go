package meeting

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Feed returns a keyset page of nearby/city ads. Empty when the viewer has no
// completed profile, or (radius mode) no location.
func (s *Service) Feed(ctx context.Context, userID uuid.UUID, f feedFilters) (feedResponse, error) {
	empty := feedResponse{Items: []feedItem{}}

	profileID, created, found, err := s.repo.profileForUser(ctx, userID)
	if err != nil {
		return feedResponse{}, err
	}
	if !found || !created {
		return empty, nil
	}

	mode := "radius"
	if f.CityID != nil {
		mode = "city"
	}

	// Resolve sort + pinned viewer origin: from the cursor on later pages
	// (self-describing), from the DB + request on the first page.
	var (
		cursor       *feedCursor
		sort         = f.Sort
		hasPt        bool
		vlat, vlng   float64
	)
	if f.CursorRaw != "" {
		c, cerr := decodeCursor(f.CursorRaw)
		if cerr != nil {
			return feedResponse{}, cerr
		}
		cursor = c
		sort = c.Sort
		hasPt, vlat, vlng = c.HasPoint, c.Lat, c.Lng
	} else {
		vlat, vlng, hasPt, err = s.repo.viewerEffectivePoint(ctx, userID)
		if err != nil {
			return feedResponse{}, err
		}
		if !hasPt {
			sort = sortDate // can't sort by distance without an origin
		}
	}

	// Radius mode is meaningless without the viewer's location.
	if mode == "radius" && !hasPt {
		return empty, nil
	}

	q := feedQuery{
		ViewerProfileID: profileID,
		HasViewerPoint:  hasPt,
		ViewerLat:       vlat,
		ViewerLng:       vlng,
		Mode:            mode,
		RadiusMeters:    f.Radius * 1000,
		Gender:          f.Gender,
		Goal:            f.Goal,
		AgeMin:          f.AgeMin,
		AgeMax:          f.AgeMax,
		TagIDs:          f.TagIDs,
		Sort:            sort,
		Cursor:          cursor,
		Limit:           f.Limit,
	}
	if f.CityID != nil {
		q.CityID = *f.CityID
	}

	rows, err := s.repo.feedPage(ctx, q)
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
		var dist *int
		if r.DistKm != nil {
			d := distanceKm(*r.DistKm)
			dist = &d
		}
		var city *cityRef
		if r.CityID != nil && r.CityName != nil {
			city = &cityRef{ID: *r.CityID, Name: *r.CityName}
		}
		items[i] = feedItem{
			ID:          r.AdID,
			Description: r.Description,
			Tags:        tags,
			City:        city,
			DistanceKm:  dist,
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
		resp.NextCursor = nextCursor(sort, rows[len(rows)-1], hasPt, vlat, vlng)
	}
	return resp, nil
}

func nextCursor(sort string, last feedRow, hasPt bool, vlat, vlng float64) *string {
	c := feedCursor{Sort: sort, ID: last.AdID.String(), HasPoint: hasPt, Lat: vlat, Lng: vlng}
	if sort == sortDate {
		ts := last.CreatedAt.UTC().Format(time.RFC3339Nano)
		c.Ts = &ts
	} else if last.DistKm != nil {
		c.Dist = last.DistKm
	}
	s := encodeCursor(c)
	return &s
}
