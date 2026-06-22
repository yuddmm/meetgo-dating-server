// Package httpx contains small HTTP helpers shared across domain modules:
// JSON responses and the unified error envelope (see docs/api.md).
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

// errorEnvelope is the unified error response shape:
//
//	{ "error": { "code": "...", "message": "...", "details": { ... } } }
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message,omitempty"`
	Details map[string]string `json:"details,omitempty"`
}

// JSON writes v as a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("httpx: encode response", slog.Any("error", err))
	}
}

// NoContent writes a 204 response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// DecodeJSON reads a size-limited JSON body into v, returning a 422
// VALIDATION_ERROR on malformed input. Unknown fields are rejected.
func DecodeJSON(r *http.Request, v any) *APIError {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20)) // 1 MiB
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return ValidationError(map[string]string{"body": "invalid JSON"})
	}
	return nil
}

// Error writes the unified error envelope. message is for logs/debug, not for
// direct display to the user; details carries per-field validation messages.
func Error(w http.ResponseWriter, status int, code, message string, details map[string]string) {
	JSON(w, status, errorEnvelope{Error: errorBody{
		Code:    code,
		Message: message,
		Details: details,
	}})
}
