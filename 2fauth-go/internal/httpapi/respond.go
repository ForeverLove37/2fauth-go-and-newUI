package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// jsonResponse writes v as JSON with the given status code.
func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// jsonData wraps a single resource like a Laravel JsonResource ({"data": ...}).
func jsonData(w http.ResponseWriter, status int, v any) {
	jsonResponse(w, status, map[string]any{"data": v})
}

// jsonCollection wraps a collection like a Laravel ResourceCollection.
func jsonCollection(w http.ResponseWriter, status int, items any) {
	jsonResponse(w, status, map[string]any{"data": items})
}

// jsonNoContent writes an empty 204 response.
func jsonNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// jsonMessage writes {"message": ...} with the given status.
func jsonMessage(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]any{"message": message})
}

// ValidationError is a Laravel-like 422 payload.
type ValidationError struct {
	Errors  map[string][]string
	Message string
}

// Error implements the error interface.
func (v *ValidationError) Error() string { return v.Message }

// newValidationError builds a ValidationError with a single field message.
func newValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Errors:  map[string][]string{field: {message}},
		Message: message,
	}
}

// addError appends a message to a field.
func (v *ValidationError) addError(field, message string) *ValidationError {
	if v.Errors == nil {
		v.Errors = map[string][]string{}
	}
	v.Errors[field] = append(v.Errors[field], message)
	if v.Message == "" {
		v.Message = message
	}
	return v
}

// writeError renders a ValidationError as Laravel does.
func writeError(w http.ResponseWriter, err *ValidationError) {
	jsonResponse(w, http.StatusUnprocessableEntity, map[string]any{
		"message": err.Message,
		"errors":  err.Errors,
	})
}

// decodeJSON parses a JSON body, tolerating an empty body.
func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(dst); err != nil {
		if err.Error() == "EOF" {
			return nil
		}
		return err
	}
	return nil
}

// parseBoolInput mimics PHP's filter_var($v, FILTER_VALIDATE_BOOLEAN).
func parseBoolInput(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}

// parseIDParam parses a numeric route parameter.
func parseIDParam(raw string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
}
