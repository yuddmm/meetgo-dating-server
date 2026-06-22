package profile

import (
	"io"
	"mime/multipart"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// UploadPhoto godoc
//
//	@Summary	Upload a profile photo (multipart field "photo")
//	@Tags		profile
//	@Security	BearerAuth
//	@Accept		mpfd
//	@Produce	json
//	@Param		photo	formData	file	true	"image (jpeg/png/webp, <=10MB)"
//	@Success	201		{object}	photoResponse
//	@Failure	409		{object}	object
//	@Failure	422		{object}	object
//	@Router		/me/profile/photos [post]
func (h *Handler) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes+(1<<20))
	if err := r.ParseMultipartForm(maxPhotoBytes); err != nil {
		httpx.WriteError(w, httpx.ValidationError(map[string]string{"photo": "invalid upload or too large"}))
		return
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		httpx.WriteError(w, httpx.ValidationError(map[string]string{"photo": "file is required"}))
		return
	}
	defer file.Close()
	if header.Size > maxPhotoBytes {
		httpx.WriteError(w, httpx.ValidationError(map[string]string{"photo": "max 10MB"}))
		return
	}

	contentType := detectContentType(file, header)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		httpx.WriteError(w, err)
		return
	}

	resp, err := h.svc.UploadPhoto(r.Context(), userID, file, header.Size, contentType)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, resp)
}

// SetCrop godoc
//
//	@Summary	Set/update a photo crop
//	@Tags		profile
//	@Security	BearerAuth
//	@Accept		json
//	@Produce	json
//	@Param		id		path		string			true	"photo id"
//	@Param		body	body		setCropRequest	true	"crop"
//	@Success	200		{object}	photoResponse
//	@Failure	404		{object}	object
//	@Router		/me/profile/photos/{id} [patch]
func (h *Handler) SetCrop(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	photoID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, errPhotoNotFound)
		return
	}
	var req setCropRequest
	if aerr := httpx.DecodeJSON(r, &req); aerr != nil {
		httpx.WriteError(w, aerr)
		return
	}
	resp, err := h.svc.SetCrop(r.Context(), userID, photoID, req.Crop)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// ReorderPhotos godoc
//
//	@Summary	Reorder photos (first = main)
//	@Tags		profile
//	@Security	BearerAuth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		reorderRequest	true	"order"
//	@Success	200		{array}		photoResponse
//	@Failure	422		{object}	object
//	@Router		/me/profile/photos/order [patch]
func (h *Handler) ReorderPhotos(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	var req reorderRequest
	if aerr := httpx.DecodeJSON(r, &req); aerr != nil {
		httpx.WriteError(w, aerr)
		return
	}
	ids, verr := validateReorder(req.Order)
	if verr != nil {
		httpx.WriteError(w, verr)
		return
	}
	resp, err := h.svc.Reorder(r.Context(), userID, ids)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// DeletePhoto godoc
//
//	@Summary	Delete a photo
//	@Tags		profile
//	@Security	BearerAuth
//	@Param		id	path	string	true	"photo id"
//	@Success	204
//	@Failure	404	{object}	object
//	@Router		/me/profile/photos/{id} [delete]
func (h *Handler) DeletePhoto(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	photoID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, errPhotoNotFound)
		return
	}
	if err := h.svc.DeletePhoto(r.Context(), userID, photoID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.NoContent(w)
}

// Complete godoc
//
//	@Summary	Finalize onboarding (>=2 photos)
//	@Tags		profile
//	@Security	BearerAuth
//	@Produce	json
//	@Success	200	{object}	completeResponse
//	@Failure	422	{object}	object
//	@Router		/me/profile/complete [post]
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Complete(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// detectContentType sniffs the file content, falling back to the declared
// multipart content type (the stdlib sniffer does not recognize webp).
func detectContentType(file multipart.File, header *multipart.FileHeader) string {
	buf := make([]byte, 512)
	n, _ := io.ReadFull(file, buf) // short read is fine; n bytes are valid
	ct := http.DetectContentType(buf[:n])
	if _, ok := allowedImageTypes[ct]; ok {
		return ct
	}
	if declared := header.Header.Get("Content-Type"); allowedImageTypes[declared] != "" {
		return declared
	}
	return ct
}
