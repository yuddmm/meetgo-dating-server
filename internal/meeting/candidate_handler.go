package meeting

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// Like godoc
//
//	@Summary	Like an ad
//	@Tags		meetings
//	@Security	BearerAuth
//	@Param		id	path	string	true	"ad id"
//	@Success	201
//	@Failure	404	{object}	object
//	@Failure	409	{object}	object
//	@Failure	422	{object}	object
//	@Router		/meeting-ad/{id}/like [post]
func (h *Handler) Like(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	adID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, errAdNotFound)
		return
	}
	if err := h.svc.Like(r.Context(), userID, adID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, nil)
}

// ListLikes godoc
//
//	@Summary	Incoming likes on my active ad
//	@Tags		meetings
//	@Security	BearerAuth
//	@Produce	json
//	@Success	200	{array}	likeResponse
//	@Router		/likes [get]
func (h *Handler) ListLikes(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.ListLikes(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// AcceptLike godoc
//
//	@Summary	Accept a like (opens a chat)
//	@Tags		meetings
//	@Security	BearerAuth
//	@Produce	json
//	@Param		id	path		string	true	"like id"
//	@Success	200	{object}	acceptResponse
//	@Failure	404	{object}	object
//	@Failure	409	{object}	object
//	@Router		/likes/{id}/accept [post]
func (h *Handler) AcceptLike(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, errLikeNotFound)
		return
	}
	resp, err := h.svc.Accept(r.Context(), userID, id)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// RejectLike godoc
//
//	@Summary	Reject a like
//	@Tags		meetings
//	@Security	BearerAuth
//	@Param		id	path	string	true	"like id"
//	@Success	204
//	@Failure	404	{object}	object
//	@Failure	409	{object}	object
//	@Router		/likes/{id}/reject [post]
func (h *Handler) RejectLike(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, errLikeNotFound)
		return
	}
	if err := h.svc.Reject(r.Context(), userID, id); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.NoContent(w)
}
