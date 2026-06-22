package httpx

import "net/http"

// APIError is a transport-agnostic error that maps to an HTTP status and a
// machine-readable code from the contract (docs/api.md). Domain services return
// it; handlers render it via WriteError.
type APIError struct {
	Status  int
	Code    string
	Message string
	Details map[string]string
}

func (e *APIError) Error() string { return e.Code }

// NewError builds an APIError without per-field details.
func NewError(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

// ValidationError builds a 422 VALIDATION_ERROR with per-field details.
func ValidationError(details map[string]string) *APIError {
	return &APIError{
		Status:  http.StatusUnprocessableEntity,
		Code:    "VALIDATION_ERROR",
		Message: "validation failed",
		Details: details,
	}
}

// WriteError renders an *APIError using the unified envelope, falling back to a
// generic 500 for any other error.
func WriteError(w http.ResponseWriter, err error) {
	if apiErr, ok := err.(*APIError); ok {
		Error(w, apiErr.Status, apiErr.Code, apiErr.Message, apiErr.Details)
		return
	}
	Error(w, http.StatusInternalServerError, "INTERNAL", "internal error", nil)
}
