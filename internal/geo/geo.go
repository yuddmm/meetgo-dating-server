// Package geo stores users' device location for distance-based discovery.
// Coordinates are write-only via the API: they are never returned (the backend
// computes distance), preventing address disclosure from a stolen token.
package geo

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuddmm/meetgo-dating-server/internal/auth"
	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// Repository persists user locations (one row per user, upsert).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Upsert sets the user's current location.
func (r *Repository) Upsert(ctx context.Context, userID uuid.UUID, lat, lng float64) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_geo (user_id, lat, lng, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (user_id) DO UPDATE
		SET lat = EXCLUDED.lat, lng = EXCLUDED.lng, updated_at = now()`,
		userID, lat, lng)
	if err != nil {
		return fmt.Errorf("geo: upsert: %w", err)
	}
	return nil
}

type updateRequest struct {
	Lat *float64 `json:"lat"`
	Lng *float64 `json:"lng"`
}

// Handler exposes the location endpoint.
type Handler struct {
	repo *Repository
}

// NewHandler constructs a Handler.
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// Routes registers geo routes (expected under an authenticated group).
func (h *Handler) Routes(r chi.Router) {
	r.Post("/my-geo", h.Update)
}

// Update godoc
//
//	@Summary	Update my current location (write-only; never returned)
//	@Tags		geo
//	@Security	BearerAuth
//	@Accept		json
//	@Param		body	body	updateRequest	true	"lat/lng"
//	@Success	204
//	@Failure	422	{object}	object
//	@Router		/my-geo [post]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		httpx.WriteError(w, httpx.NewError(http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid access token"))
		return
	}
	var req updateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if verr := validateCoords(req); verr != nil {
		httpx.WriteError(w, verr)
		return
	}
	if err := h.repo.Upsert(r.Context(), userID, *req.Lat, *req.Lng); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.NoContent(w)
}

func validateCoords(req updateRequest) *httpx.APIError {
	details := map[string]string{}
	if req.Lat == nil || *req.Lat < -90 || *req.Lat > 90 {
		details["lat"] = "required, -90..90"
	}
	if req.Lng == nil || *req.Lng < -180 || *req.Lng > 180 {
		details["lng"] = "required, -180..180"
	}
	if len(details) > 0 {
		return httpx.ValidationError(details)
	}
	return nil
}
