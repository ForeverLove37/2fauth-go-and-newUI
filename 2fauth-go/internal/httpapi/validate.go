package httpapi

import (
	"fmt"
	"net/http"
	netmail "net/mail"
	"strings"

	"2fauth/internal/crypt"
)

// validator collects Laravel-like validation errors for a request.
type validator struct {
	c    *ctx
	errs *ValidationError
}

func newValidator(c *ctx) *validator {
	return &validator{c: c, errs: &ValidationError{Errors: map[string][]string{}}}
}

func (v *validator) fail(field, message string) *validator {
	v.errs.addError(field, message)
	return v
}

// failed reports whether at least one rule failed.
func (v *validator) failed() bool {
	return len(v.errs.Errors) > 0
}

// respond writes the 422 payload when validation failed and reports true.
func (v *validator) respond() bool {
	if !v.failed() {
		return false
	}
	writeError(v.c.w, v.errs)
	return true
}

// required returns a required string field.
func (v *validator) required(field string) string {
	raw, ok := v.c.json()[field]
	if !ok && !v.c.r.URL.Query().Has(field) {
		v.fail(field, fmt.Sprintf("The %s field is required.", field))
		return ""
	}
	value := toString(raw)
	if strings.TrimSpace(value) == "" {
		v.fail(field, fmt.Sprintf("The %s field is required.", field))
		return ""
	}
	return value
}

// present requires the field to exist even when null.
func (v *validator) present(field string) (string, bool) {
	if !v.c.hasInput(field) {
		v.fail(field, fmt.Sprintf("The %s field must be present.", field))
		return "", false
	}
	return toString(v.c.json()[field]), true
}

// optional returns a string field, empty when absent.
func (v *validator) optional(field string) string {
	if !v.c.hasInput(field) {
		return ""
	}
	return toString(v.c.json()[field])
}

// has reports whether the field was provided.
func (v *validator) has(field string) bool { return v.c.hasInput(field) }

// maxLen enforces a maximum length.
func (v *validator) maxLen(field, value string, n int) *validator {
	if len([]rune(value)) > n {
		v.fail(field, fmt.Sprintf("The %s field must not be greater than %d characters.", field, n))
	}
	return v
}

// email validates an email address.
func (v *validator) email(field, value string) *validator {
	if value == "" {
		return v
	}
	parsed, err := netmail.ParseAddress(value)
	if err != nil || parsed.Address != strings.TrimSpace(value) {
		v.fail(field, fmt.Sprintf("The %s field must be a valid email address.", field))
	}
	return v
}

// password enforces the length a password can be hashed with.
func (v *validator) password(field, value string) *validator {
	if value == "" {
		return v
	}
	if len(value) < 8 {
		v.fail(field, fmt.Sprintf("The %s field must be at least 8 characters.", field))
	}
	if len(value) > crypt.MaxPasswordBytes {
		v.fail(field, fmt.Sprintf("The %s field must not be greater than %d bytes.", field, crypt.MaxPasswordBytes))
	}
	return v
}

// integer parses an optional integer field.
func (v *validator) integer(field string) (int, bool) {
	if !v.c.hasInput(field) {
		return 0, false
	}
	raw := toString(v.c.json()[field])
	if raw == "" {
		return 0, false
	}
	var n int
	if _, err := fmt.Sscan(strings.TrimSpace(raw), &n); err != nil {
		v.fail(field, fmt.Sprintf("The %s field must be an integer.", field))
		return 0, false
	}
	return n, true
}

// boolean parses a boolean field accepting 0/1/true/false.
func (v *validator) boolean(field string) (bool, bool) {
	if !v.c.hasInput(field) {
		return false, false
	}
	raw := toString(v.c.json()[field])
	return parseBoolInput(raw), true
}

// in restricts the value to an enumeration.
func (v *validator) in(field, value string, allowed ...string) *validator {
	if value == "" {
		return v
	}
	for _, a := range allowed {
		if a == value {
			return v
		}
	}
	v.fail(field, fmt.Sprintf("The selected %s is invalid.", field))
	return v
}

// between enforces an inclusive numeric range.
func (v *validator) between(field string, value, min, max int) *validator {
	if value < min || value > max {
		v.fail(field, fmt.Sprintf("The %s field must be between %d and %d.", field, min, max))
	}
	return v
}

// requireInteger parses a mandatory integer.
func (v *validator) requireInteger(field string) int {
	n, ok := v.integer(field)
	if !ok {
		if !v.c.hasInput(field) {
			v.fail(field, fmt.Sprintf("The %s field is required.", field))
		}
		return 0
	}
	return n
}

// confirmed checks the `_confirmation` companion field.
func (v *validator) confirmed(field, value string) *validator {
	if toString(v.c.json()[field+"_confirmation"]) != value {
		v.fail(field, "The "+field+" field confirmation does not match.")
	}
	return v
}

// ids parses a comma separated list of ids.
func parseIDList(raw string) ([]int64, bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, false
	}
	var out []int64
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		var n int64
		if _, err := fmt.Sscan(part, &n); err != nil || n <= 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// parseOptionalInt parses a query parameter as an integer.
func parseIntQS(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	var n int
	if _, err := fmt.Sscan(raw, &n); err != nil {
		return 0, false
	}
	return n, true
}

// writeUnauthorized writes the standard 401 body.
func writeUnauthorized(w http.ResponseWriter) {
	jsonMessage(w, http.StatusUnauthorized, "unauthorized")
}
