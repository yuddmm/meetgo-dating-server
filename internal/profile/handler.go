package profile

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/yuddmm/meetgo-dating-server/internal/auth"
	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// Handler exposes the profile/onboarding endpoints.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes registers the profile routes (expected under an authenticated group).
func (h *Handler) Routes(r chi.Router) {
	r.Get("/me/profile", h.Get)
	r.Put("/me/profile/basics", h.Basics)
	r.Put("/me/profile/about", h.About)

	// Photos (step 3). "/order" is registered before "/{id}".
	r.Post("/me/profile/photos", h.UploadPhoto)
	r.Patch("/me/profile/photos/order", h.ReorderPhotos)
	r.Patch("/me/profile/photos/{id}", h.SetCrop)
	r.Delete("/me/profile/photos/{id}", h.DeletePhoto)

	r.Post("/me/profile/complete", h.Complete)
}

// userID extracts the authenticated user id (middleware guarantees presence).
func (h *Handler) userID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := auth.UserID(r.Context())
	if !ok {
		httpx.WriteError(w, errUnauthorized)
	}
	return id, ok
}

// Get godoc
//
//	@Summary	Get my profile
//	@Tags		profile
//	@Security	BearerAuth
//	@Produce	json
//	@Success	200	{object}	profileResponse
//	@Failure	404	{object}	object
//	@Router		/me/profile [get]
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		httpx.WriteError(w, errUnauthorized)
		return
	}
	resp, err := h.svc.Get(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// Basics godoc
//
//	@Summary	Onboarding step 1 — basics
//	@Tags		profile
//	@Security	BearerAuth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		basicsRequest	true	"basics"
//	@Success	200		{object}	profileEnvelope
//	@Failure	422		{object}	object
//	@Router		/me/profile/basics [put]
func (h *Handler) Basics(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		httpx.WriteError(w, errUnauthorized)
		return
	}
	var req basicsRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	resp, err := h.svc.SaveBasics(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// About godoc
//
//	@Summary	Onboarding step 2 — about
//	@Tags		profile
//	@Security	BearerAuth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		aboutRequest	true	"about"
//	@Success	200		{object}	profileEnvelope
//	@Failure	409		{object}	object
//	@Failure	422		{object}	object
//	@Router		/me/profile/about [put]
func (h *Handler) About(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		httpx.WriteError(w, errUnauthorized)
		return
	}
	var req aboutRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	resp, err := h.svc.SaveAbout(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}
