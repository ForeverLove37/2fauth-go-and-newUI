package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"2fauth/internal/crypt"
	"2fauth/internal/store"
)

// apiUserPayload mirrors UserResource::toArray().
func (s *Server) apiUserPayload(u *store.User) map[string]any {
	return map[string]any{
		"id":                     u.ID,
		"name":                   u.Name,
		"email":                  u.Email,
		"oauth_provider":         nullableString(u.OauthProvider),
		"authenticated_by_proxy": s.cfg.AuthenticationGuard == guardReverseProxy,
		"email_verified":         u.EmailVerifiedAt != nil,
		"preferences":            s.st.PreferencesWithDefaults(u),
		"appSettings":            s.settings.ForRegularUser(),
		"is_admin":               u.IsAdmin,
	}
}

// apiUser handles GET /api/v1/user.
func (s *Server) apiUser(c *ctx) {
	jsonData(c.w, http.StatusOK, s.apiUserPayload(c.user))
}

// updateUser handles PUT /user.
func (s *Server) updateUser(c *ctx) {
	if s.cfg.AuthenticationGuard == guardReverseProxy || c.user.OauthProvider != "" {
		jsonMessage(c.w, http.StatusBadRequest, "This account is managed by an external provider and cannot be modified")
		return
	}

	v := newValidator(c)
	name := v.required("name")
	v.maxLen("name", name, 191)
	email := v.required("email")
	v.maxLen("email", email, 191)
	v.email("email", email)
	password := v.required("password")
	if v.respond() {
		return
	}

	if !crypt.CheckPassword(c.user.Password, password) {
		jsonMessage(c.w, http.StatusBadRequest, "The current password does not match")
		return
	}

	if existing, err := s.st.GetUserByName(name); err == nil && existing.ID != c.user.ID {
		jsonMessage(c.w, http.StatusUnprocessableEntity, "The name has already been taken.")
		return
	}
	if existing, err := s.st.GetUserByEmail(email); err == nil && existing.ID != c.user.ID {
		jsonMessage(c.w, http.StatusUnprocessableEntity, "The email has already been taken.")
		return
	}

	emailChanged := false
	if !s.cfg.IsDemoApp {
		changed, err := s.st.UpdateUsernameEmail(c.user.ID, name, email)
		if err != nil {
			jsonMessage(c.w, http.StatusInternalServerError, "server error")
			return
		}
		emailChanged = changed
	}
	updated, err := s.st.GetUser(c.user.ID)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	c.user = updated
	if emailChanged {
		s.sendVerificationMail(updated)
	}
	jsonData(c.w, http.StatusOK, s.apiUserPayload(updated))
}

// deleteUser handles DELETE /user.
func (s *Server) deleteUser(c *ctx) {
	v := newValidator(c)
	password := v.optional("password")
	if !s.confirmIdentity(c, password) {
		return
	}
	if c.user.IsAdmin {
		admins, _ := s.st.CountAdmins()
		if admins <= 1 {
			jsonMessage(c.w, http.StatusBadRequest, "The only administrator of the app cannot be deleted")
			return
		}
	}
	if err := s.st.DeleteUser(c.user.ID); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	c.session = newSession()
	jsonNoContent(c.w)
}

// updatePassword handles PATCH /user/password.
func (s *Server) updatePassword(c *ctx) {
	if c.user.OauthProvider != "" {
		jsonMessage(c.w, http.StatusBadRequest, "This account is managed by an external provider and cannot be modified")
		return
	}
	v := newValidator(c)
	current := v.required("currentPassword")
	password := v.required("password")
	v.confirmed("password", password)
	v.password("password", password)
	if v.respond() {
		return
	}
	if !crypt.CheckPassword(c.user.Password, current) {
		jsonMessage(c.w, http.StatusBadRequest, "The current password does not match")
		return
	}
	if s.cfg.IsDemoApp {
		jsonMessage(c.w, http.StatusOK, "Password successfully changed")
		return
	}
	hash, err := crypt.HashPassword(password)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	if err := s.st.UpdatePassword(c.user.ID, hash); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	s.revokeCredentials(c.user.ID, c.session.SID)
	s.notifyPasswordChanged(c, c.user)
	jsonMessage(c.w, http.StatusOK, "Password successfully changed")
}

// reauthWindow is how recent a sign-in must be to replace a password confirmation,
// for accounts managed by an identity provider that have no usable password.
const reauthWindow = 15 * time.Minute

// confirmIdentity checks the password confirmation of a sensitive action. An account
// that signs in through a provider has no password it knows: it must have signed in
// recently instead. On failure the response has already been written.
func (s *Server) confirmIdentity(c *ctx, password string) bool {
	if password != "" && crypt.CheckPassword(c.user.Password, password) {
		return true
	}
	if c.user.OauthProvider != "" || s.hasOAuthIdentity(c.user.ID) {
		if c.session != nil && c.session.SID != "" && password == "" {
			row, err := s.st.GetSession(c.session.SID)
			if err == nil && time.Since(time.Unix(row.CreatedAt, 0)) <= reauthWindow {
				return true
			}
			jsonMessage(c.w, http.StatusForbidden, "Sign in again to confirm this action")
			return false
		}
	}
	jsonMessage(c.w, http.StatusBadRequest, "The current password does not match")
	return false
}

func (s *Server) hasOAuthIdentity(userID int64) bool {
	identities, err := s.st.FindOAuthIdentitiesByUser(userID)
	return err == nil && len(identities) > 0
}

// allPreferences handles GET /api/v1/user/preferences.
func (s *Server) allPreferences(c *ctx) {
	prefs := s.st.PreferencesWithDefaults(c.user)
	out := make([]map[string]any, 0, len(prefs))
	for _, key := range preferenceOrder(s) {
		value, ok := prefs[key]
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"key":    key,
			"value":  value,
			"locked": s.preferenceLocked(key),
		})
	}
	// additional preferences stored outside the defaults
	for key, value := range prefs {
		if s.inPreferenceOrder(key) {
			continue
		}
		out = append(out, map[string]any{"key": key, "value": value, "locked": false})
	}
	jsonResponse(c.w, http.StatusOK, out)
}

// showPreference handles GET /api/v1/user/preferences/{preferenceName}.
func (s *Server) showPreference(c *ctx) {
	name := c.param("preferenceName")
	prefs := s.st.PreferencesWithDefaults(c.user)
	value, ok := prefs[name]
	if !ok {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"key":    name,
		"value":  value,
		"locked": s.preferenceLocked(name),
	})
}

// setPreference handles PUT /api/v1/user/preferences/{preferenceName}.
func (s *Server) setPreference(c *ctx) {
	name := c.param("preferenceName")
	prefs := s.st.PreferencesWithDefaults(c.user)
	if _, ok := prefs[name]; !ok {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return
	}
	if s.preferenceLocked(name) {
		jsonMessage(c.w, http.StatusForbidden, "This preference is locked and cannot be changed")
		return
	}
	v := newValidator(c)
	value := v.c.json()["value"]
	if v.has("value") == false {
		v.fail("value", "The value field is required.")
	}
	if v.respond() {
		return
	}

	stored := c.user.Preferences
	if stored == nil {
		stored = map[string]any{}
	}
	stored[name] = value
	if err := s.st.SavePreferences(c.user.ID, stored); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonResponse(c.w, http.StatusCreated, map[string]any{"key": name, "value": value})
}

func (s *Server) preferenceLocked(name string) bool {
	for _, locked := range s.cfg.LockedPreferences {
		if locked == name {
			return true
		}
	}
	return false
}

// preferenceOrder returns the preference names in declaration order.
func preferenceOrder(s *Server) []string {
	return s.cfg.PreferenceOrder()
}

func (s *Server) inPreferenceOrder(key string) bool {
	for _, k := range s.cfg.PreferenceOrder() {
		if k == key {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// administration
// ---------------------------------------------------------------------------

// userManagerPayload mirrors UserManagerResource.
func (s *Server) userManagerPayload(u *store.User) map[string]any {
	payload := s.apiUserPayload(u)
	count, _ := s.st.CountOwnedAccounts(u.ID)
	tokens, _ := s.st.ListTokens(u.ID)
	credentials, _ := s.st.CountWebAuthnCredentials(u.ID)

	payload["twofaccounts_count"] = count
	payload["last_seen_at"] = humanDiff(u.LastSeenAt)
	payload["created_at"] = humanDiff(u.CreatedAt)
	payload["password_reset"] = nil
	payload["valid_personal_access_tokens"] = len(tokens)
	payload["webauthn_credentials"] = credentials
	return payload
}

// listUsers handles GET /api/v1/users.
func (s *Server) listUsers(c *ctx) {
	users, err := s.st.ListUsers()
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, s.userManagerPayload(u))
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{"info": out})
}

// showUser handles GET /api/v1/users/{user}.
func (s *Server) showUser(c *ctx) {
	u, ok := s.lookupUserParam(c)
	if !ok {
		return
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{"info": s.userManagerPayload(u)})
}

// storeUser handles POST /api/v1/users.
func (s *Server) storeUser(c *ctx) {
	v := newValidator(c)
	name := v.required("name")
	v.maxLen("name", name, 191)
	email := v.required("email")
	v.maxLen("email", email, 191)
	v.email("email", email)
	password := v.required("password")
	v.password("password", password)
	isAdmin, _ := v.boolean("is_admin")
	if v.has("name") {
		if _, err := s.st.GetUserByName(name); err == nil {
			v.fail("name", "The name has already been taken.")
		}
	}
	if v.has("email") {
		if _, err := s.st.GetUserByEmail(email); err == nil {
			v.fail("email", "The email has already been taken.")
		}
	}
	if v.respond() {
		return
	}

	hash, err := crypt.HashPassword(password)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	user, err := s.st.CreateUser(name, email, hash)
	if err != nil {
		jsonMessage(c.w, http.StatusUnprocessableEntity, "The name has already been taken.")
		return
	}
	if isAdmin {
		_ = s.st.SetAdmin(user.ID, true)
		user, _ = s.st.GetUser(user.ID)
	}
	jsonResponse(c.w, http.StatusCreated, map[string]any{"info": s.userManagerPayload(user)})
}

// destroyUser handles DELETE /api/v1/users/{user}.
func (s *Server) destroyUser(c *ctx) {
	u, ok := s.lookupUserParam(c)
	if !ok {
		return
	}
	if u.IsAdmin {
		admins, _ := s.st.CountAdmins()
		if admins <= 1 {
			jsonMessage(c.w, http.StatusForbidden, "The only administrator of the app cannot be deleted")
			return
		}
	}
	if err := s.st.DeleteUser(u.ID); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonNoContent(c.w)
}

// resetUserPassword handles PATCH /api/v1/users/{user}/password/reset.
func (s *Server) resetUserPassword(c *ctx) {
	u, ok := s.lookupUserParam(c)
	if !ok {
		return
	}
	newPassword := crypt.RandomString(16)
	hash, err := crypt.HashPassword(newPassword)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	if err := s.st.UpdatePassword(u.ID, hash); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	s.revokeCredentials(u.ID, "")
	jsonResponse(c.w, http.StatusOK, map[string]any{"password": newPassword})
}

// promoteUser handles PATCH /api/v1/users/{user}/promote.
func (s *Server) promoteUser(c *ctx) {
	u, ok := s.lookupUserParam(c)
	if !ok {
		return
	}
	v := newValidator(c)
	isAdmin, present := v.boolean("is_admin")
	if !present {
		v.fail("is_admin", "The is_admin field is required.")
	}
	if v.respond() {
		return
	}
	if !isAdmin && u.IsAdmin {
		admins, _ := s.st.CountAdmins()
		if admins <= 1 {
			jsonMessage(c.w, http.StatusForbidden, "The only administrator of the app cannot be demoted")
			return
		}
	}
	if err := s.st.SetAdmin(u.ID, isAdmin); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	updated, _ := s.st.GetUser(u.ID)
	jsonResponse(c.w, http.StatusOK, map[string]any{"info": s.userManagerPayload(updated)})
}

// revokeUserPATs handles DELETE /api/v1/users/{user}/pats.
func (s *Server) revokeUserPATs(c *ctx) {
	u, ok := s.lookupUserParam(c)
	if !ok {
		return
	}
	if err := s.st.RevokeAllTokens(u.ID); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonNoContent(c.w)
}

// revokeUserCredentials handles DELETE /api/v1/users/{user}/credentials.
func (s *Server) revokeUserCredentials(c *ctx) {
	u, ok := s.lookupUserParam(c)
	if !ok {
		return
	}
	if err := s.st.DeleteWebAuthnCredentials(u.ID); err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	jsonNoContent(c.w)
}

// userAuthentications handles GET /api/v1/users/{user}/authentications.
func (s *Server) userAuthentications(c *ctx) {
	u, ok := s.lookupUserParam(c)
	if !ok {
		return
	}
	period, _ := parseIntQS(c.query("period"))
	limit, _ := parseIntQS(c.query("limit"))
	logs, err := s.st.ListAuthLogs(u.ID, period, limit)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	tz := s.userTimezone(c.user)
	out := make([]map[string]any, 0, len(logs))
	for _, l := range logs {
		browser, platform, device := parseUserAgent(l.UserAgent)
		out = append(out, map[string]any{
			"id":               l.ID,
			"ip_address":       l.IPAddress,
			"user_agent":       l.UserAgent,
			"browser":          browser,
			"platform":         platform,
			"device":           device,
			"login_at":         dayDateTime(l.LoginAt, tz),
			"logout_at":        dayDateTime(l.LogoutAt, tz),
			"login_successful": l.LoginSuccessful,
			"duration":         durationBetween(l.LoginAt, l.LogoutAt),
			"login_method":     nullableString(l.LoginMethod),
		})
	}
	jsonCollection(c.w, http.StatusOK, out)
}

// lookupUserParam resolves the {user} route parameter.
func (s *Server) lookupUserParam(c *ctx) (*store.User, bool) {
	raw := c.param("user")
	var id int64
	if _, err := fmt.Sscan(raw, &id); err != nil {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return nil, false
	}
	u, err := s.st.GetUser(id)
	if err != nil {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return nil, false
	}
	return u, true
}

// userTimezone returns the timezone preference of a user.
func (s *Server) userTimezone(u *store.User) string {
	if u == nil {
		return s.cfg.Timezone
	}
	prefs := s.st.PreferencesWithDefaults(u)
	if tz, ok := prefs["timezone"].(string); ok && tz != "" {
		return tz
	}
	return s.cfg.Timezone
}

// humanDiff renders a Carbon-like diffForHumans string.
func humanDiff(t *time.Time) string {
	if t == nil {
		return ""
	}
	d := time.Since(*t)
	future := d < 0
	if future {
		d = -d
	}
	var value string
	var unit string
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		value, unit = fmt.Sprintf("%d", int(d.Minutes())), "minute"
	case d < 24*time.Hour:
		value, unit = fmt.Sprintf("%d", int(d.Hours())), "hour"
	case d < 30*24*time.Hour:
		value, unit = fmt.Sprintf("%d", int(d.Hours()/24)), "day"
	case d < 365*24*time.Hour:
		value, unit = fmt.Sprintf("%d", int(d.Hours()/24/30)), "month"
	default:
		value, unit = fmt.Sprintf("%d", int(d.Hours()/24/365)), "year"
	}
	plural := ""
	if value != "1" {
		plural = "s"
	}
	if future {
		return "in " + value + " " + unit + plural
	}
	return value + " " + unit + plural + " ago"
}

// dayDateTime renders a Carbon::toDayDateTimeString equivalent (UTC respected).
func dayDateTime(t *time.Time, tz string) any {
	if t == nil {
		return nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	return t.In(loc).Format("Mon, Jan 2, 2006 3:04 PM")
}

// durationBetween renders the session duration in absolute human form.
func durationBetween(from, to *time.Time) any {
	if from == nil || to == nil {
		return nil
	}
	return humanDiffFrom(from, to)
}

func humanDiffFrom(from, to *time.Time) string {
	d := to.Sub(*from)
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d second%s", int(d.Seconds()), plural(int(d.Seconds())))
	case d < time.Hour:
		return fmt.Sprintf("%d minute%s", int(d.Minutes()), plural(int(d.Minutes())))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hour%s", int(d.Hours()), plural(int(d.Hours())))
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%d day%s", days, plural(days))
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// parseUserAgent extracts browser/platform/device from a raw user agent string.
func parseUserAgent(ua string) (browser, platform, device string) {
	lower := strings.ToLower(ua)
	switch {
	case strings.Contains(lower, "firefox"):
		browser = "Firefox"
	case strings.Contains(lower, "edg/"):
		browser = "Edge"
	case strings.Contains(lower, "chrome"):
		browser = "Chrome"
	case strings.Contains(lower, "safari"):
		browser = "Safari"
	case strings.Contains(lower, "curl"):
		browser = "curl"
	default:
		browser = "Unknown"
	}
	switch {
	case strings.Contains(lower, "windows"):
		platform = "Windows"
	case strings.Contains(lower, "android"):
		platform = "Android"
	case strings.Contains(lower, "iphone"), strings.Contains(lower, "ipad"), strings.Contains(lower, "mac os x"), strings.Contains(lower, "macintosh"):
		platform = "Apple"
	case strings.Contains(lower, "linux"):
		platform = "Linux"
	default:
		platform = "Unknown"
	}
	switch {
	case strings.Contains(lower, "mobile"):
		device = "mobile"
	case strings.Contains(lower, "tablet"):
		device = "tablet"
	default:
		device = "desktop"
	}
	return browser, platform, device
}
