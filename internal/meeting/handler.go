package meeting

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/yuddmm/meetgo-dating-server/internal/auth"
	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// Handler exposes the meeting-ad endpoints.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes registers meeting routes (expected under an authenticated group).
// "/meeting-ad/me" is static and matches before "/meeting-ad/{id}".
func (h *Handler) Routes(r chi.Router) {
	r.Get("/meeting-tags", h.ListTags)
	r.Get("/meeting-list", h.Feed)
	r.Post("/meeting-ad", h.Create)
	r.Get("/meeting-ad/me", h.GetMine)
	r.Put("/meeting-ad/me", h.UpdateMine)
	r.Delete("/meeting-ad/me", h.DeleteMine)
	r.Get("/meeting-ad/{id}", h.GetByID)

	// Likes (candidates)
	r.Post("/meeting-ad/{id}/like", h.Like)
	r.Get("/likes", h.ListLikes)
	r.Post("/likes/{id}/accept", h.AcceptLike)
	r.Post("/likes/{id}/reject", h.RejectLike)
}

// ListTags godoc
//
//	@Summary	List meeting tags reference
//	@Tags		meetings
//	@Security	BearerAuth
//	@Produce	json
//	@Success	200	{array}	Tag
//	@Router		/meeting-tags [get]
func (h *Handler) ListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.svc.ListTags(r.Context())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, tags)
}

// Create godoc
//
//	@Summary	Create my meeting ad
//	@Tags		meetings
//	@Security	BearerAuth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		adRequest	true	"ad"
//	@Success	201		{object}	adResponse
//	@Failure	403		{object}	object
//	@Failure	409		{object}	object
//	@Failure	422		{object}	object
//	@Router		/meeting-ad [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	var req adRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	resp, err := h.svc.Create(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, resp)
}

// GetMine godoc
//
//	@Summary	Get my active ad
//	@Tags		meetings
//	@Security	BearerAuth
//	@Produce	json
//	@Success	200	{object}	adResponse
//	@Failure	404	{object}	object
//	@Router		/meeting-ad/me [get]
func (h *Handler) GetMine(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.GetMine(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// GetByID godoc
//
//	@Summary	Get an ad by id
//	@Tags		meetings
//	@Security	BearerAuth
//	@Produce	json
//	@Param		id	path		string	true	"ad id"
//	@Success	200	{object}	adResponse
//	@Failure	404	{object}	object
//	@Router		/meeting-ad/{id} [get]
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.userID(w, r); !ok {
		return
	}
	adID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, errAdNotFound)
		return
	}
	resp, err := h.svc.GetByID(r.Context(), adID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// UpdateMine godoc
//
//	@Summary	Edit my active ad (description + tags)
//	@Tags		meetings
//	@Security	BearerAuth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		adRequest	true	"ad"
//	@Success	200		{object}	adResponse
//	@Failure	404		{object}	object
//	@Failure	422		{object}	object
//	@Router		/meeting-ad/me [put]
func (h *Handler) UpdateMine(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	var req adRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	resp, err := h.svc.UpdateMine(r.Context(), userID, req)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// DeleteMine godoc
//
//	@Summary	Remove my active ad
//	@Tags		meetings
//	@Security	BearerAuth
//	@Success	204
//	@Failure	404	{object}	object
//	@Router		/meeting-ad/me [delete]
func (h *Handler) DeleteMine(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteMine(r.Context(), userID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) userID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := auth.UserID(r.Context())
	if !ok {
		httpx.WriteError(w, errUnauthorized)
	}
	return id, ok
}
