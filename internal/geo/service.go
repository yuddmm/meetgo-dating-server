package geo

import (
	"context"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// GPS-quality knobs (defend against laggy fixes).
const (
	accuracyMaxMeters = 10000           // ignore fixes worse than 10 km
	jumpMaxKm         = 300             // implausible jump distance...
	jumpWindow        = 15 * time.Minute // ...within this window → ignore
	citiesLimit       = 20
)

var (
	errCityNotFound = httpx.NewError(http.StatusNotFound, "CITY_NOT_FOUND", "city not found")
	errUnauthorized = httpx.NewError(http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid access token")
)

// Service implements the geo use-cases.
type Service struct {
	repo *Repository
}

// NewService constructs a Service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// UpdateGPS stores a GPS fix, ignoring low-accuracy or implausible-jump fixes.
// Effective city is recomputed (AUTO); MANUAL keeps its chosen city.
func (s *Service) UpdateGPS(ctx context.Context, userID uuid.UUID, req updateGeoRequest) *httpx.APIError {
	if req.Lat == nil || *req.Lat < -90 || *req.Lat > 90 ||
		req.Lng == nil || *req.Lng < -180 || *req.Lng > 180 {
		return httpx.ValidationError(map[string]string{"lat": "required, -90..90", "lng": "required, -180..180"})
	}
	lat, lng := *req.Lat, *req.Lng

	// Drop obviously bad fixes (keeps the last good location).
	if req.Accuracy != nil && *req.Accuracy > accuracyMaxMeters {
		return nil
	}
	cur, err := s.repo.currentGeo(ctx, userID)
	if err != nil {
		return internalErr(err)
	}
	if cur != nil && cur.GpsLat != nil && cur.GpsLng != nil && cur.GpsUpdatedAt != nil &&
		time.Since(*cur.GpsUpdatedAt) < jumpWindow &&
		haversineKm(*cur.GpsLat, *cur.GpsLng, lat, lng) > jumpMaxKm {
		return nil // implausible jump → ignore
	}

	nearest, err := s.repo.nearestCityID(ctx, lat, lng)
	if err != nil {
		return internalErr(err)
	}
	if err := s.repo.applyGPS(ctx, userID, lat, lng, req.Accuracy, nearest); err != nil {
		return internalErr(err)
	}
	return nil
}

// SetLocation switches AUTO/MANUAL and returns the resulting effective city.
func (s *Service) SetLocation(ctx context.Context, userID uuid.UUID, req setLocationRequest) (locationResponse, *httpx.APIError) {
	switch strings.ToUpper(req.Mode) {
	case "MANUAL":
		if req.CityID == nil {
			return locationResponse{}, httpx.ValidationError(map[string]string{"cityId": "required for MANUAL"})
		}
		cityID, err := uuid.Parse(*req.CityID)
		if err != nil {
			return locationResponse{}, errCityNotFound
		}
		ok, e := s.repo.setManual(ctx, userID, cityID)
		if e != nil {
			return locationResponse{}, internalErr(e)
		}
		if !ok {
			return locationResponse{}, errCityNotFound
		}
	case "AUTO":
		cur, e := s.repo.currentGeo(ctx, userID)
		if e != nil {
			return locationResponse{}, internalErr(e)
		}
		var nearest *uuid.UUID
		if cur != nil && cur.GpsLat != nil && cur.GpsLng != nil {
			if nearest, e = s.repo.nearestCityID(ctx, *cur.GpsLat, *cur.GpsLng); e != nil {
				return locationResponse{}, internalErr(e)
			}
		}
		if e := s.repo.setAuto(ctx, userID, nearest); e != nil {
			return locationResponse{}, internalErr(e)
		}
	default:
		return locationResponse{}, httpx.ValidationError(map[string]string{"mode": "must be AUTO or MANUAL"})
	}

	mode, city, e := s.repo.modeAndCity(ctx, userID)
	if e != nil {
		return locationResponse{}, internalErr(e)
	}
	return locationResponse{Mode: mode, City: city}, nil
}

// SearchCities returns cities matching q for the picker.
func (s *Service) SearchCities(ctx context.Context, q string) ([]cityDTO, *httpx.APIError) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []cityDTO{}, nil
	}
	items, err := s.repo.searchCities(ctx, q, citiesLimit)
	if err != nil {
		return nil, internalErr(err)
	}
	return items, nil
}

func internalErr(err error) *httpx.APIError {
	return httpx.NewError(http.StatusInternalServerError, "INTERNAL", err.Error())
}

// haversineKm returns the great-circle distance between two points in km.
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return r * 2 * math.Asin(math.Sqrt(a))
}
