package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"2fauth/internal/crypt"
	"2fauth/internal/mail"
	"2fauth/internal/store"
)

const (
	// guardReverseProxy delegates authentication to a trusted reverse proxy.
	guardReverseProxy = "reverse-proxy-guard"
	// registerThrottle bounds the slow anonymous requests, per address and minute.
	registerThrottle = 10
)

// authenticatedPayload mirrors HasAuthenticatedPayload::authenticatedPayload().
func (s *Server) authenticatedPayload(c *ctx, message string, u *store.User) map[string]any {
	return map[string]any{
		"message":        message,
		"email_verified": u.EmailVerifiedAt != nil,
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
	if s.cfg.AuthenticationGuard == guardReverseProxy {
		jsonMessage(c.w, http.StatusForbidden, "Accounts are managed by the reverse proxy")
		return
	}
	// The very first account must always be possible, whatever the settings say.
	count, err := s.st.CountUsers()
	if err != nil {
		s.srvError(c, err)
		return
	}
	if count > 0 && (s.settings.Bool("disableRegistration") || s.ssoOnly()) {
		jsonMessage(c.w, http.StatusForbidden, "forbidden")
		return
	}
	// Hashing is deliberately slow: without a limit this endpoint is a CPU sink.
	if ok, retry := s.throttle.Allow("register|"+s.clientIP(c.r), registerThrottle, time.Minute); !ok {
		jsonMessage(c.w, http.StatusTooManyRequests,
			"Too many attempts. Please try again in "+strconv.Itoa(retry)+" seconds.")
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
	v.password("password", password)
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

	if count > 0 {
		if message, blocked := s.registrationBlocked(email); blocked {
			jsonMessage(c.w, http.StatusUnprocessableEntity, message)
			return
		}
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

	if err := s.signIn(c, user); err != nil {
		s.srvError(c, err)
		return
	}
	_ = s.st.CreateAuthLog(user.ID, s.clientIP(c.r), c.r.UserAgent(), "web-guard", "register", true)
	s.sendVerificationMail(user)

	jsonResponse(c.w, http.StatusCreated, s.authenticatedPayload(c, "account created", user))
}

// registrationBlocked applies the restrictRegistration policy. The policy fails
// closed: once it is on, an address is refused unless the configuration positively
// accepts it.
//
//	restrictList  addresses, or "@domain" entries, separated by a line break, "|" or ","
//	restrictRule  "allow" or empty: the list is an allow list
//	              "deny": the list is a block list
//	              anything else: a regular expression the address may also match
func (s *Server) registrationBlocked(email string) (string, bool) {
	if !s.settings.Bool("restrictRegistration") {
		return "", false
	}
	const refused = "Registration is not allowed for this email address"
	lowered := strings.ToLower(strings.TrimSpace(email))
	listed := false
	for _, entry := range strings.FieldsFunc(strings.ToLower(s.settings.Str("restrictList")), func(r rune) bool {
		return r == '\n' || r == '\r' || r == '|' || r == ','
	}) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if entry == lowered || (strings.HasPrefix(entry, "@") && strings.HasSuffix(lowered, entry)) {
			listed = true
			break
		}
	}
	rule := strings.TrimSpace(s.settings.Str("restrictRule"))
	switch strings.ToLower(rule) {
	case "", "allow":
		if listed {
			return "", false
		}
		return refused, true
	case "deny":
		if listed {
			return refused, true
		}
		return "", false
	}
	if listed {
		return "", false
	}
	pattern, err := regexp.Compile(rule)
	if err != nil {
		slog.Warn("restrictRule is not a valid regular expression: registrations are refused", "error", err)
		return refused, true
	}
	if pattern.MatchString(lowered) {
		return "", false
	}
	return refused, true
}

// ssoOnly reports whether password sign-in is reserved to administrators.
func (s *Server) ssoOnly() bool {
	return s.settings.Bool("enableSso") && s.settings.Bool("useSsoOnly")
}

// login handles POST /user/login.
func (s *Server) login(c *ctx) {
	if s.cfg.AuthenticationGuard == guardReverseProxy {
		jsonMessage(c.w, http.StatusForbidden, "Sign in through the reverse proxy")
		return
	}
	v := newValidator(c)
	email := v.required("email")
	v.email("email", email)
	password := v.required("password")
	if v.respond() {
		return
	}

	// Three independent budgets: one address guessing one account, one address
	// spraying many accounts, and many addresses guessing one account.
	ip := s.clientIP(c.r)
	lowered := strings.ToLower(email)
	pairKey := ip + "|" + lowered
	limit := s.cfg.LoginThrottle
	for _, budget := range []struct {
		key string
		max int
	}{{pairKey, limit}, {"ip|" + ip, limit * 6}, {"email|" + lowered, limit * 6}} {
		if ok, retry := s.throttle.Allow(budget.key, budget.max, time.Minute); !ok {
			jsonMessage(c.w, http.StatusTooManyRequests,
				"Too many login attempts. Please try again in "+strconv.Itoa(retry)+" seconds.")
			return
		}
	}

	user, err := s.st.GetUserByEmail(email)
	if err != nil {
		// Same cost as a wrong password, so timing does not reveal the address.
		crypt.BurnPasswordCheck(password)
		writeUnauthorized(c.w)
		return
	}
	if !crypt.CheckPassword(user.Password, password) {
		_ = s.st.CreateAuthLog(user.ID, ip, c.r.UserAgent(), "web-guard", "password", false)
		s.notifyFailedLogin(c, user)
		writeUnauthorized(c.w)
		return
	}
	// Administrators keep the password as a way back in when the provider is down.
	if s.ssoOnly() && !user.IsAdmin {
		jsonMessage(c.w, http.StatusForbidden, "Single sign-on is required on this instance")
		return
	}

	s.throttle.Clear(pairKey)
	if err := s.signIn(c, user); err != nil {
		s.srvError(c, err)
		return
	}
	s.st.TouchLastSeen(user.ID)
	s.notifyNewDevice(c, user, "password")
	_ = s.st.CreateAuthLog(user.ID, ip, c.r.UserAgent(), "web-guard", "password", true)

	jsonResponse(c.w, http.StatusOK, s.authenticatedPayload(c, "authenticated", user))
}

// logout handles GET and POST /user/logout. The GET form is what the upstream SPA
// calls: it must prove it comes from the application, like any unsafe request.
func (s *Server) logout(c *ctx) {
	if c.user == nil {
		writeUnauthorized(c.w)
		return
	}
	if !c.viaToken && !s.csrfValid(c.r, c.session) {
		jsonResponse(c.w, statusCSRFMismatch, map[string]any{"message": "CSRF token mismatch."})
		return
	}
	s.st.CloseAuthLog(c.user.ID)
	if c.session.SID != "" {
		_ = s.st.DeleteSession(c.session.SID)
	}
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

// passwordResetTTL is how long a reset link stays valid.
const passwordResetTTL = 60 * time.Minute

// passwordLost handles POST /user/password/lost. The answer and its timing do not
// depend on the address: the lookup and the delivery happen in the background.
func (s *Server) passwordLost(c *ctx) {
	v := newValidator(c)
	email := v.required("email")
	v.email("email", email)
	if v.respond() {
		return
	}
	if ok, retry := s.throttle.Allow("lost|"+s.clientIP(c.r), registerThrottle, time.Minute); !ok {
		jsonMessage(c.w, http.StatusTooManyRequests,
			"Too many attempts. Please try again in "+strconv.Itoa(retry)+" seconds.")
		return
	}
	// One mail per address per quarter of an hour: the form must not be a way to
	// flood somebody's mailbox.
	if ok, _ := s.throttle.Allow("lost-mail|"+strings.ToLower(email), 3, 15*time.Minute); ok {
		s.runMailJob(func() { s.sendPasswordReset(email) })
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"message": "We have emailed your password reset link.",
	})
}

// sendPasswordReset issues a reset token and mails the link.
func (s *Server) sendPasswordReset(email string) {
	if !s.mailSettings().Enabled() {
		return
	}
	user, err := s.st.GetUserByEmail(email)
	if err != nil || user.OauthProvider != "" {
		return
	}
	token := crypt.RandomString(64)
	if err := s.st.SavePasswordReset(user.Email, crypt.SHA256Hex(token)); err != nil {
		slog.Error("cannot store a password reset", "error", err)
		return
	}
	link := s.publicURL("/password/reset") + "?" + url.Values{"token": {token}, "email": {user.Email}}.Encode()
	s.deliver(mail.KindResetPassword, user, map[string]string{
		"Email":          user.Email,
		"ResetURL":       link,
		"ExpiresMinutes": strconv.Itoa(int(passwordResetTTL.Minutes())),
	})
}

// passwordReset handles POST /user/password/reset.
func (s *Server) passwordReset(c *ctx) {
	v := newValidator(c)
	email := v.required("email")
	v.email("email", email)
	token := v.required("token")
	password := v.required("password")
	v.confirmed("password", password)
	v.password("password", password)
	if v.respond() {
		return
	}
	if ok, retry := s.throttle.Allow("reset|"+s.clientIP(c.r), s.cfg.LoginThrottle, time.Minute); !ok {
		jsonMessage(c.w, http.StatusTooManyRequests,
			"Too many attempts. Please try again in "+strconv.Itoa(retry)+" seconds.")
		return
	}
	invalid := func() {
		v.fail("email", "This password reset token is invalid.")
		v.respond()
	}
	user, err := s.st.GetUserByEmail(email)
	if err != nil {
		crypt.BurnPasswordCheck(password)
		invalid()
		return
	}
	stored, age, err := s.st.FindPasswordReset(user.Email)
	if err != nil || age > passwordResetTTL ||
		subtle.ConstantTimeCompare([]byte(stored), []byte(crypt.SHA256Hex(token))) != 1 {
		invalid()
		return
	}
	hash, err := crypt.HashPassword(password)
	if err != nil {
		s.srvError(c, err)
		return
	}
	if err := s.st.UpdatePassword(user.ID, hash); err != nil {
		s.srvError(c, err)
		return
	}
	_ = s.st.DeletePasswordReset(user.Email)
	// Following the link proved control of the mailbox.
	_ = s.st.MarkEmailVerified(user.ID)
	s.revokeCredentials(user.ID, "")
	s.notifyPasswordChanged(c, user)
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"message": "Your password has been reset.",
	})
}

// revokeCredentials ends the sessions (but keep) and the personal access tokens of
// a user. Whoever used the previous password must not stay signed in.
func (s *Server) revokeCredentials(userID int64, keep string) {
	if err := s.st.DeleteUserSessions(userID, keep); err != nil {
		slog.Error("cannot end the sessions of a user", "error", err)
	}
	if err := s.st.RevokeAllTokens(userID); err != nil {
		slog.Error("cannot revoke the tokens of a user", "error", err)
	}
}

// emailToken is the content of an e-mail verification link.
type emailToken struct {
	Purpose string `json:"purpose"`
	UserID  int64  `json:"uid"`
	Email   string `json:"email"`
	Expires int64  `json:"exp"`
}

const (
	emailTokenPurpose = "email-verify"
	emailTokenTTL     = 24 * time.Hour
)

// sendVerificationMail mails a confirmation link for the address of the user.
func (s *Server) sendVerificationMail(user *store.User) {
	if user.EmailVerifiedAt != nil || !s.mailSettings().Enabled() {
		return
	}
	raw, err := json.Marshal(emailToken{
		Purpose: emailTokenPurpose,
		UserID:  user.ID,
		Email:   user.Email,
		Expires: time.Now().Add(emailTokenTTL).Unix(),
	})
	if err != nil {
		return
	}
	token, err := s.enc.EncryptString(string(raw))
	if err != nil {
		return
	}
	s.sendAsync(mail.KindVerifyEmail, user, map[string]string{
		"Email":        user.Email,
		"VerifyURL":    s.publicURL("/user/email/verify") + "?" + url.Values{"token": {token}}.Encode(),
		"ExpiresHours": strconv.Itoa(int(emailTokenTTL.Hours())),
	})
}

// sendEmailVerification handles POST /user/email/verification.
func (s *Server) sendEmailVerification(c *ctx) {
	if c.user.EmailVerifiedAt != nil {
		jsonMessage(c.w, http.StatusOK, "This address is already verified")
		return
	}
	if !s.mailSettings().Enabled() {
		jsonMessage(c.w, http.StatusUnprocessableEntity, "E-mail delivery is not configured")
		return
	}
	if ok, retry := s.throttle.Allow("verify-mail|"+strconv.FormatInt(c.user.ID, 10), 3, 15*time.Minute); !ok {
		jsonMessage(c.w, http.StatusTooManyRequests,
			"Too many attempts. Please try again in "+strconv.Itoa(retry)+" seconds.")
		return
	}
	s.sendVerificationMail(c.user)
	jsonMessage(c.w, http.StatusOK, "Verification e-mail sent")
}

// verifyEmail handles GET /user/email/verify, the target of the e-mail link.
func (s *Server) verifyEmail(c *ctx) {
	result := "invalid"
	if plain, err := s.enc.DecryptString(c.query("token")); err == nil {
		var token emailToken
		if json.Unmarshal([]byte(plain), &token) == nil && token.Purpose == emailTokenPurpose && time.Now().Unix() < token.Expires {
			// The address must still be the one the link was sent to.
			if user, err := s.st.GetUser(token.UserID); err == nil && strings.EqualFold(user.Email, token.Email) {
				if s.st.MarkEmailVerified(user.ID) == nil {
					result = "ok"
				}
			}
		}
	}
	s.ssoRedirect(c, s.baseURL()+"/settings?tab=account&email_verified="+result)
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
	if !s.patPolicyAllows() {
		jsonMessage(c.w, http.StatusForbidden, "Personal access tokens are not supported when SSO only is enabled")
		return false
	}
	if s.cfg.AuthenticationGuard == guardReverseProxy {
		jsonMessage(c.w, http.StatusForbidden, "Personal access tokens are not supported behind a reverse proxy")
		return false
	}
	return true
}

// patPolicyAllows reports whether personal access tokens are accepted at all: an
// existing token must not be a way around the SSO-only policy.
func (s *Server) patPolicyAllows() bool {
	return !s.ssoOnly() || s.settings.Bool("allowPatWhileSsoOnly")
}

// userFromProxy resolves the identity asserted by a trusted reverse proxy, creating
// the account on first use. Headers from any other peer are ignored.
func (s *Server) userFromProxy(r *http.Request) *store.User {
	if !s.trustedProxies.Contains(peerAddr(r.RemoteAddr)) {
		return nil
	}
	name := strings.TrimSpace(r.Header.Get(s.cfg.AuthProxyHeaderForUser))
	if name == "" {
		return nil
	}
	email := ""
	if s.cfg.AuthProxyHeaderForMail != "" {
		email = strings.ToLower(strings.TrimSpace(r.Header.Get(s.cfg.AuthProxyHeaderForMail)))
	}
	if email == "" {
		email = strings.ToLower(name) + "@proxy.invalid"
	}
	if user, err := s.st.GetUserByEmail(email); err == nil {
		return user
	}
	if user, err := s.st.GetUserByName(name); err == nil {
		return user
	}
	hash, err := crypt.HashPassword(crypt.RandomString(40))
	if err != nil {
		return nil
	}
	user, err := s.st.CreateUser(name, email, hash)
	if err != nil {
		slog.Warn("cannot create the account of a proxied user", "error", err)
		return nil
	}
	_ = s.st.SetOAuthProvider(user.ID, "reverse-proxy", name)
	return user
}

// deprecatedUserName handles GET /api/v1/user/name.
func (s *Server) deprecatedUserName(c *ctx) {
	c.w.Header().Set("Deprecation", time.Date(2023, 3, 21, 0, 0, 0, 0, time.UTC).Format(http.TimeFormat))
	jsonResponse(c.w, http.StatusOK, map[string]any{"deprecation": true})
}

// createdAtJSON renders an optional time like Eloquent does.
func createdAtJSON(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format("2006-01-02T15:04:05.000000Z")
}
