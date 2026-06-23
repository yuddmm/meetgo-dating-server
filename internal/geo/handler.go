package geo

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/yuddmm/meetgo-dating-server/internal/auth"
	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// Handler exposes the geo/cities endpoints.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes registers geo routes (expected under an authenticated group).
func (h *Handler) Routes(r chi.Router) {
	r.Post("/my-geo", h.UpdateGeo)
	r.Get("/cities", h.SearchCities)
	r.Put("/me/location", h.SetLocation)
}

// UpdateGeo godoc
//
//	@Summary	Update my location (write-only; never returned)
//	@Tags		geo
//	@Security	BearerAuth
//	@Accept		json
//	@Param		body	body	updateGeoRequest	true	"lat/lng/accuracy"
//	@Success	204
//	@Failure	422	{object}	object
//	@Router		/my-geo [post]
func (h *Handler) UpdateGeo(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	var req updateGeoRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := h.svc.UpdateGPS(r.Context(), userID, req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.NoContent(w)
}

// SearchCities godoc
//
//	@Summary	Search cities (picker)
//	@Tags		geo
//	@Security	BearerAuth
//	@Produce	json
//	@Param		q	query	string	true	"query"
//	@Success	200	{array}	cityDTO
//	@Router		/cities [get]
func (h *Handler) SearchCities(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.userID(w, r); !ok {
		return
	}
	items, err := h.svc.SearchCities(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, items)
}

// SetLocation godoc
//
//	@Summary	Set location mode (AUTO/MANUAL city override)
//	@Tags		geo
//	@Security	BearerAuth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		setLocationRequest	true	"mode + cityId"
//	@Success	200		{object}	locationResponse
//	@Failure	404		{object}	object
//	@Failure	422		{object}	object
//	@Router		/me/location [put]
func (h *Handler) SetLocation(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	var req setLocationRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	resp, err := h.svc.SetLocation(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

func (h *Handler) userID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := auth.UserID(r.Context())
	if !ok {
		httpx.WriteError(w, errUnauthorized)
	}
	return id, ok
}
