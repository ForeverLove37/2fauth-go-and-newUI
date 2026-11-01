package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"2fauth/internal/crypt"
	"2fauth/internal/store"
)

// authenticatedPayload mirrors HasAuthenticatedPayload::authenticatedPayload().
func (s *Server) authenticatedPayload(c *ctx, message string, u *store.User) map[string]any {
	return map[string]any{
		"message":        message,
		"id":             u.ID,
		"name":           u.Name,
		"email":          u.Email,
		"oauth_provider": nullableString(u.OauthProvider),
		"preferences":    s.st.PreferencesWithDefaults(u),
		"appSettings":    s.settings.ForRegularUser(),
		"is_admin":       u.IsAdmin,
	}
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// register handles POST /user.
func (s *Server) register(c *ctx) {
	if s.settings.Bool("disableRegistration") {
		jsonMessage(c.w, http.StatusForbidden, "forbidden")
		return
	}

	v := newValidator(c)
	name := v.required("name")
	v.maxLen("name", name, 191)
	email := v.required("email")
	v.maxLen("email", email, 191)
	v.email("email", email)
	password := v.required("password")
	v.confirmed("password", password)
	if len(password) > 0 && len(password) < 8 {
		v.fail("password", "The password field must be at least 8 characters.")
	}
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

	if message, blocked := s.registrationBlocked(email); blocked {
		jsonMessage(c.w, http.StatusUnprocessableEntity, message)
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

	c.session.UserID = user.ID
	c.session.regenerate()
	_ = s.st.CreateAuthLog(user.ID, clientIP(c.r), c.r.UserAgent(), "web-guard", "register", true)

	jsonResponse(c.w, http.StatusCreated, s.authenticatedPayload(c, "account created", user))
}

// registrationBlocked applies the restrictRegistration policy.
func (s *Server) registrationBlocked(email string) (string, bool) {
	if !s.settings.Bool("restrictRegistration") {
		return "", false
	}
	rule := s.settings.Str("restrictRule")
	if rule == "" {
		return "", false
	}
	list := strings.Split(strings.ToLower(s.settings.Str("restrictList")), "\n")
	lowered := strings.ToLower(strings.TrimSpace(email))
	if rule == "allow" {
		for _, entry := range list {
			if entry = strings.TrimSpace(entry); entry != "" && entry == lowered {
				return "", false
			}
		}
		return "Registration is not allowed for this email address", true
	}
	for _, entry := range list {
		if entry = strings.TrimSpace(entry); entry != "" && entry == lowered {
			return "Registration is not allowed for this email address", true
		}
	}
	return "", false
}

// login handles POST /user/login.
func (s *Server) login(c *ctx) {
	v := newValidator(c)
	email := v.required("email")
	v.email("email", email)
	password := v.required("password")
	if v.respond() {
		return
	}

	key := clientIP(c.r) + "|" + strings.ToLower(email)
	window := time.Minute
	if ok, retry := s.throttle.Allow(key, s.cfg.LoginThrottle, window); !ok {
		jsonMessage(c.w, http.StatusTooManyRequests,
			"Too many login attempts. Please try again in "+strconv.Itoa(retry)+" seconds.")
		return
	}

	user, err := s.st.GetUserByEmail(email)
	if err != nil || !crypt.CheckPassword(user.Password, password) {
		if user != nil {
			_ = s.st.CreateAuthLog(user.ID, clientIP(c.r), c.r.UserAgent(), "web-guard", "password", false)
		}
		writeUnauthorized(c.w)
		return
	}

	s.throttle.Clear(key)
	c.session.UserID = user.ID
	c.session.regenerate()
	s.st.TouchLastSeen(user.ID)
	_ = s.st.CreateAuthLog(user.ID, clientIP(c.r), c.r.UserAgent(), "web-guard", "password", true)

	jsonResponse(c.w, http.StatusOK, s.authenticatedPayload(c, "authenticated", user))
}

// logout handles GET /user/logout.
func (s *Server) logout(c *ctx) {
	if c.user == nil {
		writeUnauthorized(c.w)
		return
	}
	userID := c.user.ID
	s.st.CloseAuthLog(userID)
	c.session = newSession()
	jsonMessage(c.w, http.StatusOK, "signed out")
}

// refreshCSRF handles GET /refresh-csrf.
func (s *Server) refreshCSRF(c *ctx) {
	jsonResponse(c.w, http.StatusOK, c.session.CSRFToken)
}

// manifestJSON serves the web app manifest.
func (s *Server) manifestJSON(c *ctx) {
	base := s.baseURL()
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"short_name":  s.cfg.AppName,
		"name":        s.cfg.AppName,
		"start_url":   base + "/",
		"scope":       "/",
		"display":     "standalone",
		"theme_color": "#242424",
		"icons": []map[string]any{{
			"src":   base + "/favicon_lg.png",
			"type":  "image/png",
			"sizes": "180x180",
		}},
		"shortcuts": []map[string]any{
			{"name": "New 2FA", "url": base + "/start", "description": "Add a new 2FA account by flashing a QR code or filling out a form"},
			{"name": "Import 2FAs", "url": base + "/account/import", "description": "Import 2FA accounts previously exported from another 2FA app"},
			{"name": "Settings", "url": base + "/settings/options", "description": "Manage your 2FAuth user settings"},
		},
	})
}

// healthPage serves GET /up.
func (s *Server) healthPage(c *ctx) {
	c.w.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.w.WriteHeader(http.StatusOK)
	_, _ = c.w.Write([]byte(`<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>2FAuth - Health</title>
<link rel="stylesheet" href="` + s.baseURL() + `/healthcheck.css"></head>
<body><main><h1>2FAuth is up</h1><p>The application answered this health check correctly.</p></main></body></html>`))
}

// passwordLost handles POST /user/password/lost.
func (s *Server) passwordLost(c *ctx) {
	v := newValidator(c)
	email := v.required("email")
	v.email("email", email)
	if v.respond() {
		return
	}
	// Anti-enumeration: always answer the same way.
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"message": "We have emailed your password reset link.",
	})
}

// passwordReset handles POST /user/password/reset.
func (s *Server) passwordReset(c *ctx) {
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"message": "Your password has been reset.",
	})
}

// listPersonalAccessTokens handles GET /oauth/personal-access-tokens.
func (s *Server) listPersonalAccessTokens(c *ctx) {
	if !s.patAllowed(c) {
		return
	}
	tokens, err := s.st.ListTokens(c.user.ID)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	out := make([]map[string]any, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, s.tokenPayload(t))
	}
	jsonResponse(c.w, http.StatusOK, out)
}

func (s *Server) tokenPayload(t *store.Token) map[string]any {
	return map[string]any{
		"id":         t.ID,
		"user_id":    t.UserID,
		"client_id":  t.ClientID,
		"name":       t.Name,
		"scopes":     t.Scopes,
		"revoked":    t.Revoked,
		"created_at": createdAtJSON(t.CreatedAt),
		"updated_at": createdAtJSON(t.UpdatedAt),
		"expires_at": createdAtJSON(t.ExpiresAt),
	}
}

// createPersonalAccessToken handles POST /oauth/personal-access-tokens.
func (s *Server) createPersonalAccessToken(c *ctx) {
	if !s.patAllowed(c) {
		return
	}
	v := newValidator(c)
	name := v.required("name")
	v.maxLen("name", name, 255)
	if v.respond() {
		return
	}
	token, plain, err := s.st.CreateToken(c.user.ID, name, nil)
	if err != nil {
		s.srvError(c, err)
		return
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"accessToken":    plain,
		"accessTokenId":  token.ID,
		"plainTextToken": plain,
	})
}

// deletePersonalAccessToken handles DELETE /oauth/personal-access-tokens/{token_id}.
func (s *Server) deletePersonalAccessToken(c *ctx) {
	if !s.patAllowed(c) {
		return
	}
	if !s.st.DeleteToken(c.param("token_id"), c.user.ID) {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return
	}
	jsonNoContent(c.w)
}

// patAllowed enforces the SSO-only restriction on personal access tokens.
func (s *Server) patAllowed(c *ctx) bool {
	appSettings := s.settings
	if appSettings.Bool("enableSso") && appSettings.Bool("useSsoOnly") && !appSettings.Bool("allowPatWhileSsoOnly") {
		jsonMessage(c.w, http.StatusForbidden, "Personal access tokens are not supported when SSO only is enabled")
		return false
	}
	if s.cfg.AuthenticationGuard == "reverse-proxy-guard" {
		jsonMessage(c.w, http.StatusForbidden, "Personal access tokens are not supported behind a reverse proxy")
		return false
	}
	return true
}

// deprecatedUserName handles GET /api/v1/user/name.
func (s *Server) deprecatedUserName(c *ctx) {
	c.w.Header().Set("Deprecation", time.Date(2023, 3, 21, 0, 0, 0, 0, time.UTC).Format(http.TimeFormat))
	jsonResponse(c.w, http.StatusOK, map[string]any{"deprecation": true})
}

// clientIP returns the caller IP address.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[0])
	}
	host := r.RemoteAddr
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}
	return host
}

// createdAtJSON renders an optional time like Eloquent does.
func createdAtJSON(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format("2006-01-02T15:04:05.000000Z")
}
