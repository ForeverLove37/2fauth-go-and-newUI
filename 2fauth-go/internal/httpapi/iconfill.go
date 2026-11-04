package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"2fauth/internal/services"
)

const (
	// iconFillInterval is how often accounts still lacking an icon are looked at.
	iconFillInterval = time.Hour
	// iconMissRetry is how long a service no library knows is left alone.
	iconMissRetry = 6 * time.Hour
)

// iconFiller gives accounts their official icon in the background: a lookup can take
// seconds, far too long for a request, let alone an import of hundreds of accounts.
// The panel polls the accounts anyway, so the icons simply appear.
type iconFiller struct {
	wake chan struct{}

	mu     sync.Mutex
	found  map[string]string    // service -> icon file
	missed map[string]time.Time // service -> time of the last miss
}

func newIconFiller() *iconFiller {
	return &iconFiller{
		wake:   make(chan struct{}, 1),
		found:  map[string]string{},
		missed: map[string]time.Time{},
	}
}

// wakeIconFiller asks for a pass over the accounts without blocking the caller.
func (s *Server) wakeIconFiller() {
	select {
	case s.iconFill.wake <- struct{}{}:
	default:
	}
}

// RunIconFiller looks for missing icons until the context ends.
func (s *Server) RunIconFiller(ctx context.Context) {
	ticker := time.NewTicker(iconFillInterval)
	defer ticker.Stop()
	for {
		s.fillIcons(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.iconFill.wake:
		}
	}
}

// fillIcons is one pass. It returns the number of accounts that got an icon.
func (s *Server) fillIcons(ctx context.Context) int {
	missing, err := s.st.AccountsMissingIcon()
	if err != nil {
		slog.Warn("cannot list the accounts without an icon", "error", err)
		return 0
	}
	wanted := map[int64]bool{}
	filled := 0
	for _, account := range missing {
		if ctx.Err() != nil {
			break
		}
		allowed, known := wanted[account.UserID]
		if !known {
			allowed = s.wantsOfficialIcons(account.UserID)
			wanted[account.UserID] = allowed
		}
		if !allowed {
			continue
		}
		icon := s.resolveIcon(ctx, account.Service)
		if icon == "" {
			continue
		}
		if err := s.st.AddIconUpload(icon, account.UserID); err != nil {
			slog.Warn("cannot record an icon", "error", err)
			continue
		}
		if err := s.st.SetMissingIcon(account.ID, icon); err != nil {
			slog.Warn("cannot save an icon", "error", err)
			continue
		}
		filled++
	}
	return filled
}

// wantsOfficialIcons reads the preference of the owner. Looking an icon up tells the
// icon library the name of the service, so it can be turned off.
func (s *Server) wantsOfficialIcons(userID int64) bool {
	user, err := s.st.GetUser(userID)
	if err != nil {
		return false
	}
	switch value := s.st.PreferencesWithDefaults(user)["getOfficialIcons"].(type) {
	case bool:
		return value
	case string:
		return value != "" && value != "0" && !strings.EqualFold(value, "false")
	case float64:
		return value != 0
	}
	return true
}

// resolveIcon finds the icon of a service, once per service.
func (s *Server) resolveIcon(ctx context.Context, service string) string {
	key := strings.ToLower(strings.TrimSpace(service))
	fill := s.iconFill

	fill.mu.Lock()
	icon, found := fill.found[key]
	missedAt, missed := fill.missed[key]
	fill.mu.Unlock()

	if found && s.icons.Exists(icon) {
		return icon
	}
	if missed && time.Since(missedAt) < iconMissRetry {
		return ""
	}

	res, err := s.icons.FetchIcon(ctx, service)
	fill.mu.Lock()
	defer fill.mu.Unlock()
	if err != nil {
		if !errors.Is(err, services.ErrIconNotFound) {
			slog.Warn("cannot store an icon", "service", service, "error", err)
		}
		fill.missed[key] = time.Now()
		return ""
	}
	fill.found[key] = res.Name
	delete(fill.missed, key)
	return res.Name
}

// fillMissingIcons handles POST /api/v1/twofaccounts/icons/fill: try again now, even
// for services that were not found a moment ago.
func (s *Server) fillMissingIcons(c *ctx) {
	s.iconFill.mu.Lock()
	s.iconFill.missed = map[string]time.Time{}
	s.iconFill.mu.Unlock()

	missing, err := s.st.AccountsMissingIcon()
	if err != nil {
		s.srvError(c, err)
		return
	}
	pending := 0
	for _, account := range missing {
		if account.UserID == c.user.ID {
			pending++
		}
	}
	s.wakeIconFiller()
	jsonResponse(c.w, http.StatusOK, map[string]any{"pending": pending})
}
