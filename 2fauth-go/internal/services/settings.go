// Package services holds the application services ported from Laravel.
package services

import (
	"encoding/json"
	"strings"
	"sync"

	"2fauth/internal/config"
	"2fauth/internal/store"
)

// SettingService mirrors App\Services\SettingService.
type SettingService struct {
	mu       sync.RWMutex
	st       *store.Store
	cfg      *config.Config
	settings map[string]any
}

// NewSettingService builds the service and loads the current values.
func NewSettingService(st *store.Store, cfg *config.Config) *SettingService {
	s := &SettingService{st: st, cfg: cfg}
	s.reload()
	return s
}

func (s *SettingService) reload() {
	values := map[string]any{}
	for k, v := range s.cfg.Settings {
		values[k] = v
	}
	options, err := s.st.AllOptions()
	if err == nil {
		for k, v := range options {
			values[k] = restoreType(v)
		}
	}
	s.mu.Lock()
	s.settings = values
	s.mu.Unlock()
}

// All returns every setting.
func (s *SettingService) All() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]any, len(s.settings))
	for k, v := range s.settings {
		out[k] = v
	}
	return out
}

// Keys returns the setting names in a stable order (defaults first, then extras).
func (s *SettingService) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	keys := make([]string, 0, len(s.settings))
	for _, k := range orderedDefaults() {
		if _, ok := s.settings[k]; ok {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	extras := make([]string, 0)
	for k := range s.settings {
		if !seen[k] {
			extras = append(extras, k)
		}
	}
	sortStrings(extras)
	return append(keys, extras...)
}

// Get returns a single setting value (nil when unknown).
func (s *SettingService) Get(name string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.settings[name]
	if !ok {
		return nil
	}
	return v
}

// Bool returns a boolean setting.
func (s *SettingService) Bool(name string) bool {
	v := s.Get(name)
	b, _ := v.(bool)
	return b
}

// Str returns a string setting.
func (s *SettingService) Str(name string) string {
	v := s.Get(name)
	s2, ok := v.(string)
	if !ok {
		return ""
	}
	return s2
}

// Set persists a setting, mirroring the boolean pattern used by 2FAuth.
func (s *SettingService) Set(name string, value any) error {
	if err := s.st.SetOption(name, replaceBoolean(value)); err != nil {
		return err
	}
	s.reload()
	return nil
}

// Delete removes a customised setting.
func (s *SettingService) Delete(name string) error {
	if err := s.st.DeleteOption(name); err != nil {
		return err
	}
	s.reload()
	return nil
}

// Exists reports whether the setting name is known.
func (s *SettingService) Exists(name string) bool {
	if _, ok := s.cfg.Settings[name]; ok {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.settings[name]
	return ok
}

// ForVisitor mirrors SettingService::forVisitor().
func (s *SettingService) ForVisitor() map[string]any {
	return map[string]any{
		"disableRegistration": s.Get("disableRegistration"),
		"enableSso":           s.Get("enableSso"),
		"useSsoOnly":          s.Get("useSsoOnly"),
	}
}

// ForRegularUser mirrors SettingService::forRegularUser().
func (s *SettingService) ForRegularUser() map[string]any {
	out := s.ForVisitor()
	out["enableSharing"] = s.Get("enableSharing")
	out["enableAllUsersSharingScope"] = s.Get("enableAllUsersSharingScope")
	out["allowPatWhileSsoOnly"] = s.Get("allowPatWhileSsoOnly")
	return out
}

// NullifiedKeys returns every known setting name in declaration order; used by
// the SPA bootstrap where all admin values are published as null.
func (s *SettingService) NullifiedKeys() []string {
	return orderedDefaults()
}

func orderedDefaults() []string {
	// Mirrors the order of config('2fauth.settings').
	return []string{
		"useEncryption",
		"checkForUpdate",
		"lastRadarScan",
		"latestRelease",
		"disableRegistration",
		"enableSso",
		"useSsoOnly",
		"allowPatWhileSsoOnly",
		"restrictRegistration",
		"restrictList",
		"restrictRule",
		"keepSsoRegistrationEnabled",
		"storeIconsInDatabase",
		"enableSharing",
		"enableAllUsersSharingScope",
	}
}

// replaceBoolean mirrors SettingService::replaceBoolean().
func replaceBoolean(value any) string {
	switch t := value.(type) {
	case bool:
		if t {
			return "{{1}}"
		}
		return "{{}}"
	case nil:
		return ""
	default:
		return toStr(value)
	}
}

// restoreType mirrors SettingService::restoreType().
func restoreType(raw string) any {
	if raw == "{{}}" {
		return false
	}
	if raw == "{{1}}" {
		return true
	}
	if isNumeric(raw) {
		if strings.Contains(raw, ".") {
			if f, ok := parseFloat(raw); ok {
				return f
			}
		}
		if n, ok := parseInt(raw); ok {
			return n
		}
	}
	return raw
}

func toStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case int:
		return itoa(int64(t))
	case int64:
		return itoa(t)
	case float64:
		return ftoa(t)
	}
	return ""
}

func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
