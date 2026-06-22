package auth

import (
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/yuddmm/meetgo-dating-server/internal/platform/httpx"
)

// GeoResolver resolves a client IP to an ISO country code ("" when unknown).
type GeoResolver interface {
	Country(ip string) string
}

// Handler exposes the auth HTTP endpoints.
type Handler struct {
	svc     *Service
	tokens  *TokenService
	geo     GeoResolver
	devMode bool
}

// NewHandler constructs a Handler. geo may be nil (geo resolution disabled).
func NewHandler(svc *Service, tokens *TokenService, geo GeoResolver, devMode bool) *Handler {
	return &Handler{svc: svc, tokens: tokens, geo: geo, devMode: devMode}
}

// resolveCountry returns the request's country code. In dev the X-Debug-Country
// header overrides GeoIP (for testing / client simulation).
func (h *Handler) resolveCountry(r *http.Request) string {
	if h.devMode {
		if c := r.Header.Get("X-Debug-Country"); c != "" {
			return strings.ToUpper(c)
		}
	}
	if h.geo == nil {
		return ""
	}
	return h.geo.Country(clientIP(r))
}

// Middleware exposes the bearer-auth middleware so other modules can protect
// their routes with the same authenticator.
func (h *Handler) Middleware(next http.Handler) http.Handler {
	return h.tokens.Middleware(next)
}

// Routes mounts the auth routes onto r (expected to be the /api/v1 group).
func (h *Handler) Routes(r chi.Router) {
	r.Route("/auth", func(r chi.Router) {
		r.Post("/send_code", h.SendCode)
		r.Post("/check_code", h.CheckCode)
		r.Post("/refresh", h.Refresh)

		r.Group(func(r chi.Router) {
			r.Use(h.tokens.Middleware)
			r.Post("/logout", h.Logout)
		})
	})

	r.Group(func(r chi.Router) {
		r.Use(h.tokens.Middleware)
		r.Get("/me", h.Me)
	})
}

// SendCode godoc
//
//	@Summary	Send OTP code to email
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		sendCodeRequest	true	"email"
//	@Success	200		{object}	sendCodeResponse
//	@Failure	422		{object}	object
//	@Failure	429		{object}	object
//	@Router		/auth/send_code [post]
func (h *Handler) SendCode(w http.ResponseWriter, r *http.Request) {
	var req sendCodeRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	email, verr := validateEmail(req.Email)
	if verr != nil {
		httpx.WriteError(w, verr)
		return
	}
	resp, err := h.svc.SendCode(r.Context(), email, clientIP(r), h.resolveCountry(r))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// CheckCode godoc
//
//	@Summary	Verify OTP code and issue tokens
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		checkCodeRequest	true	"email + code"
//	@Success	200		{object}	tokenResponse
//	@Failure	401		{object}	object
//	@Failure	422		{object}	object
//	@Router		/auth/check_code [post]
func (h *Handler) CheckCode(w http.ResponseWriter, r *http.Request) {
	var req checkCodeRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	email, verr := validateEmail(req.Email)
	if verr != nil {
		httpx.WriteError(w, verr)
		return
	}
	if verr := validateCode(req.Code); verr != nil {
		httpx.WriteError(w, verr)
		return
	}
	resp, err := h.svc.CheckCode(r.Context(), email, req.Code)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// Refresh godoc
//
//	@Summary	Rotate token pair
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		refreshRequest	true	"refreshToken"
//	@Success	200		{object}	tokenResponse
//	@Failure	401		{object}	object
//	@Router		/auth/refresh [post]
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if req.RefreshToken == "" {
		httpx.WriteError(w, errInvalidRefresh)
		return
	}
	resp, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// Logout godoc
//
//	@Summary	Revoke the current session
//	@Tags		auth
//	@Security	BearerAuth
//	@Success	204
//	@Failure	401	{object}	object
//	@Router		/auth/logout [post]
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserID(r.Context())
	if !ok {
		httpx.WriteError(w, errUnauthorized)
		return
	}
	if err := h.svc.Logout(r.Context(), userID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.NoContent(w)
}

// Me godoc
//
//	@Summary	Current account identity
//	@Tags		auth
//	@Security	BearerAuth
//	@Produce	json
//	@Success	200	{object}	meResponse
//	@Failure	401	{object}	object
//	@Router		/me [get]
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserID(r.Context())
	if !ok {
		httpx.WriteError(w, errUnauthorized)
		return
	}
	resp, err := h.svc.Me(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// clientIP returns the client IP (chi RealIP has already resolved proxies).
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
