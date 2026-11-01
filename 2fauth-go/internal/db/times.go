package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// TimeLayout is the storage layout used for every timestamp column.
const TimeLayout = "2006-01-02 15:04:05"

// timeLayouts are the layouts accepted when scanning timestamps back.
var timeLayouts = []string{
	TimeLayout,
	"2006-01-02T15:04:05Z",
	"2006-01-02 15:04:05.999999",
	"2006-01-02T15:04:05",
	time.RFC3339Nano,
	time.RFC3339,
}

// NowValue renders the current time for storage.
func NowValue(dialect string) string {
	return time.Now().UTC().Format(TimeLayout)
}

// NullableValue converts an optional time into a driver friendly value.
func NullableValue(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(TimeLayout)
}

// ParseTime parses a stored timestamp value.
func ParseTime(v any) (*time.Time, error) {
	if v == nil {
		return nil, nil
	}
	switch t := v.(type) {
	case time.Time:
		return &t, nil
	case []byte:
		return parseTimeString(string(t))
	case string:
		return parseTimeString(t)
	}
	return nil, fmt.Errorf("unsupported time value %T", v)
}

func parseTimeString(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "null") {
		return nil, nil
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			utc := t.UTC()
			return &utc, nil
		}
	}
	return nil, fmt.Errorf("cannot parse timestamp %q", s)
}

// NullTime scans a nullable timestamp.
type NullTime struct {
	Time  time.Time
	Valid bool
}

// Scan implements sql.Scanner.
func (n *NullTime) Scan(value any) error {
	t, err := ParseTime(value)
	if err != nil {
		return err
	}
	if t == nil {
		n.Valid = false
		return nil
	}
	n.Time = *t
	n.Valid = true
	return nil
}

// RFC3339 renders the stored time as an ISO string for JSON payloads.
func (n NullTime) RFC3339() string {
	if !n.Valid {
		return ""
	}
	return n.Time.Format(time.RFC3339)
}

// JSON renders the value as a JSON string, or null.
func (n NullTime) JSON() any {
	if !n.Valid {
		return nil
	}
	return n.Time.UTC().Format("2006-01-02T15:04:05.000000Z")
}

// NullString is a nullable string scanner helper.
type NullString struct {
	String string
	Valid  bool
}

// Scan implements sql.Scanner.
func (n *NullString) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		n.Valid = false
	case []byte:
		n.String, n.Valid = string(v), true
	case string:
		n.String, n.Valid = v, true
	case time.Time:
		n.String, n.Valid = v.Format(TimeLayout), true
	default:
		n.String, n.Valid = fmt.Sprint(v), true
	}
	return nil
}

// Ptr returns a pointer to the value, or nil.
func (n NullString) Ptr() *string {
	if !n.Valid {
		return nil
	}
	s := n.String
	return &s
}

// NullInt is a nullable int scanner helper.
type NullInt struct {
	Int   int64
	Valid bool
}

// Scan implements sql.Scanner.
func (n *NullInt) Scan(value any) error {
	if value == nil {
		n.Valid = false
		return nil
	}
	switch v := value.(type) {
	case int64:
		n.Int, n.Valid = v, true
	case int32:
		n.Int, n.Valid = int64(v), true
	case int:
		n.Int, n.Valid = int64(v), true
	case float64:
		n.Int, n.Valid = int64(v), true
	case []byte:
		return n.scanInt(string(v))
	case string:
		return n.scanInt(v)
	case bool:
		if v {
			n.Int = 1
		}
		n.Valid = true
	}
	return nil
}

func (n *NullInt) scanInt(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		n.Valid = false
		return nil
	}
	var parsed int64
	if _, err := fmt.Sscan(s, &parsed); err != nil {
		return err
	}
	n.Int, n.Valid = parsed, true
	return nil
}

// BoolToInt converts a boolean to the storage representation.
func BoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// IntToBool converts a stored integer/boolean to a boolean.
func IntToBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case int64:
		return t != 0
	case []byte:
		s := strings.ToLower(string(t))
		return s == "1" || s == "true" || s == "t"
	case string:
		s := strings.ToLower(t)
		return s == "1" || s == "true" || s == "t"
	}
	return false
}

var _ sql.Scanner = (*NullTime)(nil)
