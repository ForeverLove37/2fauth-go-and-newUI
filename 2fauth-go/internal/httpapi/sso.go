package httpapi

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"2fauth/internal/config"
	"2fauth/internal/crypt"
	"2fauth/internal/oidc"
	"2fauth/internal/store"
)

// OpenID Connect single sign-on.
//
// The feature is configured twice: OIDC_* environment variables provide the
// deployment defaults, while an administrator may override every value through
// the openid.* runtime settings (persisted in the options table). The client
// secret is always stored encrypted with the application key, prefixed with
// "v1:" so encrypted and plain values can be told apart.

const (
	// openIDProvider is the identifier used for the OpenID Connect provider in
	// the routes (/socialite/*/openid) and in oauth_identities.provider.
	openIDProvider = "openid"

	// oidcTransactionCookie carries the state/nonce/verifier of an in-flight
	// authorization request.
	oidcTransactionCookie = "oidc_tx"

	// oidcTransactionTTL is the lifetime of the transaction cookie, in seconds.
	oidcTransactionTTL = 600

	// encryptedSecretPrefix marks a runtime secret that is encrypted at rest.
	encryptedSecretPrefix = "v1:"
)

// Runtime setting names (admin editable, they win over the environment).
const (
	settingOpenIDEnabled      = "openid.enabled"
	settingOpenIDIssuer       = "openid.issuer"
	settingOpenIDClientID     = "openid.client_id"
	settingOpenIDClientSecret = "openid.client_secret"
	settingOpenIDScopes       = "openid.scopes"
	settingOpenIDLabel        = "openid.label"
	settingOpenIDAutoRegister = "openid.auto_register"
	settingOpenIDAutoRedirect = "openid.auto_redirect"
)

// openIDSettings is the effective OpenID Connect configuration.
type openIDSettings struct {
	Enabled      bool
	Issuer       string
	ClientID     string
	ClientSecret string
	Scopes       string
	Label        string
	AutoRegister bool
	AutoRedirect bool
}

// scopeList splits the scopes setting into the list the IdP expects.
func (o openIDSettings) scopeList() []string {
	fields := strings.Fields(o.Scopes)
	if len(fields) == 0 {
		return strings.Fields(config.DefaultOpenIDScopes)
	}
	return fields
}

// ssoSettings resolves the effective configuration: a runtime setting when it is
// present, the environment value otherwise, and the built-in default last.
func (s *Server) ssoSettings() openIDSettings {
	eff := openIDSettings{
		Enabled:      s.cfg.OpenIDEnabled,
		Issuer:       s.cfg.OpenIDIssuer,
		ClientID:     s.cfg.OpenIDClientID,
		ClientSecret: s.cfg.OpenIDClientSecret,
		Scopes:       s.cfg.OpenIDScopes,
		Label:        s.cfg.OpenIDButtonLabel,
		AutoRegister: s.cfg.OpenIDAutoRegister,
		AutoRedirect: s.cfg.OpenIDAutoRedirect,
	}
	if v, ok := s.settingBool(settingOpenIDEnabled); ok {
		eff.Enabled = v
	}
	if v, ok := s.settingString(settingOpenIDIssuer); ok {
		eff.Issuer = v
	}
	if v, ok := s.settingString(settingOpenIDClientID); ok {
		eff.ClientID = v
	}
	if v, ok := s.settingString(settingOpenIDClientSecret); ok {
		eff.ClientSecret = s.decryptStoredSecret(v)
	}
	if v, ok := s.settingString(settingOpenIDScopes); ok {
		eff.Scopes = v
	}
	if v, ok := s.settingString(settingOpenIDLabel); ok {
		eff.Label = v
	}
	if v, ok := s.settingBool(settingOpenIDAutoRegister); ok {
		eff.AutoRegister = v
	}
	if v, ok := s.settingBool(settingOpenIDAutoRedirect); ok {
		eff.AutoRedirect = v
	}

	eff.Issuer = oidc.NormalizeIssuer(eff.Issuer)
	eff.ClientID = strings.TrimSpace(eff.ClientID)
	if strings.TrimSpace(eff.Scopes) == "" {
		eff.Scopes = config.DefaultOpenIDScopes
	}
	if strings.TrimSpace(eff.Label) == "" {
		eff.Label = config.DefaultOpenIDLabel
	}
	// Sign-in is only possible with the switch on, an issuer and a client id.
	eff.Enabled = eff.Enabled && eff.Issuer != "" && eff.ClientID != ""
	return eff
}

// settingString reads a runtime setting as a string. It reports false when the
// setting was never stored, so that the environment value applies.
func (s *Server) settingString(name string) (string, bool) {
	raw := s.settings.Get(name)
	if raw == nil {
		return "", false
	}
	switch v := raw.(type) {
	case string:
		return v, true
	case bool:
		if v {
			return "1", true
		}
		return "", true
	default:
		return toString(raw), true
	}
}

// settingBool reads a runtime setting as a boolean.
func (s *Server) settingBool(name string) (bool, bool) {
	raw := s.settings.Get(name)
	if raw == nil {
		return false, false
	}
	switch v := raw.(type) {
	case bool:
		return v, true
	case string:
		return parseBoolInput(v), true
	case int:
		return v != 0, true
	case int64:
		return v != 0, true
	case float64:
		return v != 0, true
	}
	return false, false
}

// decryptStoredSecret returns the usable client secret of a stored value. Values
// written by this build carry the "v1:" prefix; Laravel payloads written by the
// PHP implementation are detected too. Anything else is treated as plain text
// (the environment variable case).
func (s *Server) decryptStoredSecret(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, encryptedSecretPrefix) {
		plain, err := s.enc.DecryptString(strings.TrimPrefix(raw, encryptedSecretPrefix))
		if err != nil {
			slog.Warn("cannot decrypt the stored OpenID Connect client secret", "error", err)
			return ""
		}
		return plain
	}
	if s.enc.IsEncrypted(raw) {
		if plain, err := s.enc.DecryptString(raw); err == nil {
			return plain
		}
	}
	return raw
}

// encryptStoredSecret seals a client secret for storage.
func (s *Server) encryptStoredSecret(plain string) (string, error) {
	encrypted, err := s.enc.EncryptString(plain)
	if err != nil {
		return "", err
	}
	return encryptedSecretPrefix + encrypted, nil
}

// ---------------------------------------------------------------------------
// URLs
// ---------------------------------------------------------------------------

// publicBaseURL returns the absolute mount point of the application, without a
// trailing slash (e.g. "https://2fauth.example.com/2fauth"). APP_URL is used
// when it holds an absolute http(s) URL, the request otherwise.
func (s *Server) publicBaseURL(c *ctx) string {
	base := ""
	if raw := strings.TrimRight(strings.TrimSpace(s.cfg.URL), "/"); isAbsoluteHTTPURL(raw) {
		base = raw
		if sub := s.baseURL(); sub != "" && !strings.HasSuffix(base, sub) {
			base += sub
		}
	} else if c != nil && c.r != nil {
		scheme := "http"
		if c.r.TLS != nil || s.forceHTTPS || s.cfg.SessionSecure {
			scheme = "https"
		}
		base = scheme + "://" + c.r.Host + s.baseURL()
	}
	return strings.TrimRight(base, "/")
}

// redirectURI is the absolute callback URL registered at the provider.
func (s *Server) redirectURI(c *ctx) string {
	return s.publicBaseURL(c) + "/socialite/callback/" + openIDProvider
}

// startURL is the path of the SSO entry point.
func (s *Server) ssoStartURL() string {
	return "/socialite/redirect/" + openIDProvider
}

// loginErrorURL is where a failed SSO attempt lands, with a reason code.
func (s *Server) loginErrorURL(code string) string {
	return s.baseURL() + "/login?sso_error=" + url.QueryEscape(code)
}

// isAbsoluteHTTPURL reports whether raw is an absolute http(s) URL.
func isAbsoluteHTTPURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}

// ---------------------------------------------------------------------------
// GET /api/v1/sso/config (public)
// ---------------------------------------------------------------------------

// discoveryPayload renders the parts of a provider document the front-end shows.
func discoveryPayload(p *oidc.Provider) map[string]any {
	return map[string]any{
		"issuer":                 p.Issuer,
		"authorization_endpoint": p.AuthorizationEndpoint,
		"token_endpoint":         p.TokenEndpoint,
		"userinfo_endpoint":      p.UserInfoEndpoint,
		"end_session_endpoint":   p.EndSessionEndpoint,
	}
}

// ssoConfig handles GET /api/v1/sso/config. It never exposes the client secret.
func (s *Server) ssoConfig(c *ctx) {
	eff := s.ssoSettings()

	payload := map[string]any{
		"enabled":       false,
		"provider":      openIDProvider,
		"label":         eff.Label,
		"auto_redirect": eff.AutoRedirect,
		"auto_register": eff.AutoRegister,
		"issuer":        eff.Issuer,
		"redirect_uri":  s.redirectURI(c),
		"start_url":     s.ssoStartURL(),
		"discovery":     nil,
		"error":         nil,
	}
	if !eff.Enabled {
		// Explain an incomplete configuration; a provider that is simply
		// switched off (or not configured at all) reports no error.
		if eff.Issuer != "" && eff.ClientID == "" {
			payload["error"] = "The OpenID Connect client id is not configured."
		}
		jsonResponse(c.w, http.StatusOK, payload)
		return
	}

	provider, err := oidc.Discover(c.r.Context(), eff.Issuer)
	if err != nil {
		slog.Warn("OpenID Connect discovery failed", "issuer", eff.Issuer, "error", err)
		payload["error"] = err.Error()
		jsonResponse(c.w, http.StatusOK, payload)
		return
	}
	payload["enabled"] = true
	payload["discovery"] = discoveryPayload(provider)
	jsonResponse(c.w, http.StatusOK, payload)
}

// ---------------------------------------------------------------------------
// /socialite/redirect/{driver} and /socialite/callback/{driver}
// ---------------------------------------------------------------------------

// oidcTransaction is the state kept in the oidc_tx cookie between the redirect
// and the callback (encrypted with the application key).
type oidcTransaction struct {
	State    string `json:"state"`
	Nonce    string `json:"nonce"`
	Verifier string `json:"verifier"`
	Created  int64  `json:"created"`
}

// socialiteRedirect handles GET /socialite/redirect/{driver}.
func (s *Server) socialiteRedirect(c *ctx) {
	if c.param("driver") != openIDProvider {
		jsonMessage(c.w, http.StatusNotImplemented, "SSO is not configured")
		return
	}
	s.beginOpenIDLogin(c)
}

// socialiteCallback handles GET /socialite/callback/{driver}.
func (s *Server) socialiteCallback(c *ctx) {
	if c.param("driver") != openIDProvider {
		jsonMessage(c.w, http.StatusNotImplemented, "SSO is not configured")
		return
	}
	s.finishOpenIDLogin(c)
}

// beginOpenIDLogin starts the authorization-code flow with PKCE.
func (s *Server) beginOpenIDLogin(c *ctx) {
	eff := s.ssoSettings()
	if !eff.Enabled {
		s.ssoFail(c, "provider")
		return
	}

	provider, err := oidc.Discover(c.r.Context(), eff.Issuer)
	if err != nil {
		slog.Warn("OpenID Connect discovery failed", "issuer", eff.Issuer, "error", err)
		s.ssoFail(c, "provider")
		return
	}

	state := hex.EncodeToString(crypt.RandomBytes(32))
	nonce := crypt.RandomString(32)
	verifier, challenge := oidc.PKCE()

	tx := oidcTransaction{State: state, Nonce: nonce, Verifier: verifier, Created: time.Now().Unix()}
	if err := s.storeTransaction(c, tx); err != nil {
		slog.Error("cannot store the OpenID Connect transaction", "error", err)
		s.ssoFail(c, "server")
		return
	}

	authURL := oidc.AuthCodeURL(provider, eff.ClientID, s.redirectURI(c), eff.scopeList(), state, nonce, challenge)
	if authURL == "" {
		s.ssoFail(c, "provider")
		return
	}

	if strings.EqualFold(strings.TrimSpace(c.query("format")), "json") {
		jsonResponse(c.w, http.StatusOK, map[string]any{"url": authURL})
		return
	}
	s.ssoRedirect(c, authURL)
}

// finishOpenIDLogin validates the callback, exchanges the code and signs the
// resolved user in.
func (s *Server) finishOpenIDLogin(c *ctx) {
	eff := s.ssoSettings()
	if !eff.Enabled {
		s.ssoFail(c, "provider")
		return
	}

	if errCode := c.query("error"); errCode != "" {
		// A user pressing "cancel" at the IdP answers error=access_denied.
		if errCode == "access_denied" {
			s.ssoFail(c, "cancelled")
			return
		}
		slog.Warn("OpenID Connect provider returned an error", "error", errCode, "description", c.query("error_description"))
		s.ssoFail(c, "provider")
		return
	}

	tx, ok := s.loadTransaction(c)
	if !ok || tx.State == "" || tx.State != c.query("state") {
		s.ssoFail(c, "state")
		return
	}
	if time.Since(time.Unix(tx.Created, 0)) > oidcTransactionTTL*time.Second {
		s.ssoFail(c, "state")
		return
	}

	code := c.query("code")
	if code == "" {
		s.ssoFail(c, "exchange")
		return
	}

	ctx := c.r.Context()
	provider, err := oidc.Discover(ctx, eff.Issuer)
	if err != nil {
		slog.Warn("OpenID Connect discovery failed", "issuer", eff.Issuer, "error", err)
		s.ssoFail(c, "provider")
		return
	}

	tokens, err := oidc.Exchange(ctx, provider, eff.ClientID, eff.ClientSecret, code, s.redirectURI(c), tx.Verifier)
	if err != nil {
		slog.Warn("OpenID Connect token exchange failed", "error", err)
		s.ssoFail(c, "exchange")
		return
	}

	claims, err := oidc.UserInfo(ctx, provider, tokens.AccessToken)
	if err != nil {
		// Fall back to the ID token claims when userinfo is unavailable.
		if claims, err = oidc.ParseClaims(tokens.IDToken); err != nil {
			slog.Warn("OpenID Connect userinfo request failed", "error", err)
			s.ssoFail(c, "userinfo")
			return
		}
	}

	user, failure := s.resolveOAuthUser(eff, claims)
	if user == nil {
		slog.Warn("OpenID Connect sign-in refused", "reason", failure, "subject", claimString(claims, "sub"))
		s.ssoFail(c, failure)
		return
	}

	// Same session bootstrap as a password login.
	c.session.UserID = user.ID
	c.session.regenerate()
	s.st.TouchLastSeen(user.ID)
	_ = s.st.CreateAuthLog(user.ID, clientIP(c.r), c.r.UserAgent(), "web-guard", openIDProvider, true)

	s.clearTransaction(c)
	s.ssoRedirect(c, s.baseURL()+"/")
}

// resolveOAuthUser maps the provider claims to a local account: known identity,
// then same e-mail, then auto-registration. It returns a failure code on error.
func (s *Server) resolveOAuthUser(eff openIDSettings, claims map[string]any) (*store.User, string) {
	subject := claimString(claims, "sub")
	email := claimString(claims, "email")
	if subject == "" {
		return nil, "userinfo"
	}
	if email == "" {
		return nil, "email"
	}

	// 1. an identity already linked to a user
	if identity, err := s.st.FindOAuthIdentity(openIDProvider, subject); err == nil {
		if user, err := s.st.GetUser(identity.UserID); err == nil {
			return user, ""
		}
		return nil, "account"
	}

	// 2. an existing account using the same (case insensitive) e-mail
	if user, err := s.st.GetUserByEmail(email); err == nil {
		if err := s.st.CreateOAuthIdentity(user.ID, openIDProvider, subject, email); err != nil {
			slog.Warn("cannot link the OpenID Connect identity", "error", err)
			return nil, "server"
		}
		return user, ""
	} else if !errors.Is(err, store.ErrNotFound) {
		slog.Warn("cannot look up the user by email", "error", err)
		return nil, "account"
	}

	// 3. auto-registration
	if !eff.AutoRegister || s.settings.Bool("disableRegistration") {
		return nil, "register_disabled"
	}

	name := claimString(claims, "name")
	if name == "" {
		name = claimString(claims, "preferred_username")
	}
	if name == "" {
		name = emailLocalPart(email)
	}
	name = s.availableUsername(name)

	hash, err := crypt.HashPassword(crypt.RandomString(32))
	if err != nil {
		slog.Error("cannot hash the generated password", "error", err)
		return nil, "server"
	}
	user, err := s.st.CreateUser(name, email, hash)
	if err != nil {
		slog.Warn("cannot create the SSO user", "error", err)
		return nil, "server"
	}
	if err := s.st.CreateOAuthIdentity(user.ID, openIDProvider, subject, email); err != nil {
		slog.Warn("cannot link the OpenID Connect identity", "error", err)
		return nil, "server"
	}
	return user, ""
}

// availableUsername returns name, or a numbered variant when it is taken.
func (s *Server) availableUsername(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "sso-user"
	}
	if len([]rune(name)) > 180 {
		name = string([]rune(name)[:180])
	}
	candidate := name
	for i := 2; i < 100; i++ {
		if _, err := s.st.GetUserByName(candidate); err != nil {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", name, i)
	}
	return fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
}

// claimString reads a string claim.
func claimString(claims map[string]any, name string) string {
	raw, ok := claims[name]
	if !ok || raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(toString(raw))
}

// emailLocalPart returns the part of an email address before the "@".
func emailLocalPart(email string) string {
	if at := strings.Index(email, "@"); at > 0 {
		return email[:at]
	}
	return email
}

// ssoFail abandons the flow: the transaction is dropped and the browser lands
// on the login screen with a reason code.
func (s *Server) ssoFail(c *ctx, code string) {
	s.clearTransaction(c)
	s.ssoRedirect(c, s.loginErrorURL(code))
}

// secureCookie reports whether cookies may be flagged Secure.
func (s *Server) secureCookie(c *ctx) bool {
	if s.cfg.SessionSecure || s.isHTTPS() {
		return true
	}
	return c != nil && c.r != nil && c.r.TLS != nil
}

// ssoRedirect answers with a 302 to target.
func (s *Server) ssoRedirect(c *ctx, target string) {
	c.w.Header().Set("Location", target)
	c.w.WriteHeader(http.StatusFound)
}

// ---------------------------------------------------------------------------
// transaction cookie
// ---------------------------------------------------------------------------

// storeTransaction seals tx into the oidc_tx cookie.
func (s *Server) storeTransaction(c *ctx, tx oidcTransaction) error {
	raw, err := json.Marshal(tx)
	if err != nil {
		return err
	}
	encrypted, err := s.enc.EncryptString(string(raw))
	if err != nil {
		return err
	}
	http.SetCookie(c.w, &http.Cookie{
		Name:     oidcTransactionCookie,
		Value:    encrypted,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookie(c),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   oidcTransactionTTL,
	})
	return nil
}

// loadTransaction reads and decrypts the oidc_tx cookie.
func (s *Server) loadTransaction(c *ctx) (oidcTransaction, bool) {
	var tx oidcTransaction
	cookie, err := c.r.Cookie(oidcTransactionCookie)
	if err != nil || cookie.Value == "" {
		return tx, false
	}
	plain, err := s.enc.DecryptString(cookie.Value)
	if err != nil {
		return tx, false
	}
	if err := json.Unmarshal([]byte(plain), &tx); err != nil {
		return tx, false
	}
	return tx, true
}

// clearTransaction removes the oidc_tx cookie.
func (s *Server) clearTransaction(c *ctx) {
	http.SetCookie(c.w, &http.Cookie{
		Name:     oidcTransactionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookie(c),
		SameSite: http.SameSiteLaxMode,
	})
}

// ---------------------------------------------------------------------------
// admin API
// ---------------------------------------------------------------------------

// ssoSettingsPayload is the admin representation of the SSO configuration.
func (s *Server) ssoSettingsPayload(c *ctx) map[string]any {
	eff := s.ssoSettings()
	return map[string]any{
		"enabled":           eff.Enabled,
		"issuer":            eff.Issuer,
		"client_id":         eff.ClientID,
		"client_secret_set": eff.ClientSecret != "",
		"scopes":            eff.Scopes,
		"label":             eff.Label,
		"auto_register":     eff.AutoRegister,
		"auto_redirect":     eff.AutoRedirect,
		"redirect_uri":      s.redirectURI(c),
	}
}

// showSSOSettings handles GET /api/v1/sso/settings.
func (s *Server) showSSOSettings(c *ctx) {
	jsonResponse(c.w, http.StatusOK, s.ssoSettingsPayload(c))
}

// updateSSOSettings handles PUT /api/v1/sso/settings.
func (s *Server) updateSSOSettings(c *ctx) {
	v := newValidator(c)

	issuer := ""
	if v.has("issuer") {
		issuer = strings.TrimSpace(v.optional("issuer"))
		if issuer != "" && !isAbsoluteHTTPURL(issuer) {
			v.fail("issuer", "The issuer field must be a valid absolute URL.")
		}
	}
	if v.respond() {
		return
	}

	save := func(name string, value any) bool {
		if err := s.settings.Set(name, value); err != nil {
			s.srvError(c, err)
			return false
		}
		return true
	}

	if v.has("issuer") {
		if !save(settingOpenIDIssuer, oidc.NormalizeIssuer(issuer)) {
			return
		}
	}
	if v.has("client_id") {
		if !save(settingOpenIDClientID, strings.TrimSpace(v.optional("client_id"))) {
			return
		}
	}
	if v.has("scopes") {
		scopes := strings.TrimSpace(v.optional("scopes"))
		if scopes == "" {
			scopes = config.DefaultOpenIDScopes
		}
		if !save(settingOpenIDScopes, scopes) {
			return
		}
	}
	if v.has("label") {
		label := strings.TrimSpace(v.optional("label"))
		if label == "" {
			label = config.DefaultOpenIDLabel
		}
		if !save(settingOpenIDLabel, label) {
			return
		}
	}
	if enabled, present := v.boolean("enabled"); present {
		if !save(settingOpenIDEnabled, enabled) {
			return
		}
	}
	if autoRegister, present := v.boolean("auto_register"); present {
		if !save(settingOpenIDAutoRegister, autoRegister) {
			return
		}
	}
	if autoRedirect, present := v.boolean("auto_redirect"); present {
		if !save(settingOpenIDAutoRedirect, autoRedirect) {
			return
		}
	}

	clearSecret, _ := v.boolean("clear_secret")
	secret := strings.TrimSpace(v.optional("client_secret"))
	switch {
	case clearSecret:
		if err := s.settings.Delete(settingOpenIDClientSecret); err != nil {
			s.srvError(c, err)
			return
		}
	case secret != "":
		encrypted, err := s.encryptStoredSecret(secret)
		if err != nil {
			s.srvError(c, err)
			return
		}
		if !save(settingOpenIDClientSecret, encrypted) {
			return
		}
	}

	jsonResponse(c.w, http.StatusOK, s.ssoSettingsPayload(c))
}

// testSSO handles POST /api/v1/sso/test.
func (s *Server) testSSO(c *ctx) {
	eff := s.ssoSettings()
	if eff.Issuer == "" {
		jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
			"ok":    false,
			"error": "No OpenID Connect issuer is configured.",
		})
		return
	}
	provider, err := oidc.Discover(c.r.Context(), eff.Issuer)
	if err != nil {
		slog.Warn("OpenID Connect discovery failed", "issuer", eff.Issuer, "error", err)
		jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"ok":        true,
		"discovery": discoveryPayload(provider),
	})
}

// ssoPreset is one entry of the provider catalogue.
type ssoPreset struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Issuer string `json:"issuer"`
	Scopes string `json:"scopes"`
	Hint   string `json:"hint"`
}

// ssoPresets is the static catalogue of supported providers.
var ssoPresets = []ssoPreset{
	{
		ID:     "keycloak",
		Name:   "Keycloak",
		Issuer: "https://keycloak.example.com/realms/{realm}",
		Scopes: "openid profile email",
		Hint:   "Replace {realm} with your realm name. The issuer is <base url>/realms/<realm>; the client must be confidential with PKCE (S256) enabled. GET /api/v1/sso/keycloak-client exports an importable client.",
	},
	{
		ID:     "authentik",
		Name:   "Authentik",
		Issuer: "https://authentik.example.com/application/o/{slug}",
		Scopes: "openid profile email",
		Hint:   "Replace {slug} with the slug of the OAuth2/OpenID provider configured in Authentik; the issuer must end with a trailing slash in some versions, 2FAuth tolerates both.",
	},
	{
		ID:     "auth0",
		Name:   "Auth0",
		Issuer: "https://{tenant}.eu.auth0.com/",
		Scopes: "openid profile email",
		Hint:   "Replace {tenant} (and the region) with your Auth0 domain. Register the callback URL as an Allowed Callback URL of a Regular Web Application.",
	},
	{
		ID:     "okta",
		Name:   "Okta",
		Issuer: "https://{domain}.okta.com/oauth2/default",
		Scopes: "openid profile email",
		Hint:   "Replace {domain} with your Okta organization; use the authorization server issuer (…/oauth2/default or …/oauth2/<id>).",
	},
	{
		ID:     "google",
		Name:   "Google",
		Issuer: "https://accounts.google.com",
		Scopes: "openid profile email",
		Hint:   "Create an OAuth client id of type \"Web application\" in the Google Cloud console and add the callback URL to the authorized redirect URIs.",
	},
	{
		ID:     "entra",
		Name:   "Microsoft Entra ID",
		Issuer: "https://login.microsoftonline.com/{tenant}/v2.0",
		Scopes: "openid profile email",
		Hint:   "Replace {tenant} with your tenant id (or \"common\" for multi-tenant). The v2.0 endpoint requires the scopes above and a client secret.",
	},
	{
		ID:     "gitlab",
		Name:   "GitLab",
		Issuer: "https://gitlab.com",
		Scopes: "openid profile email",
		Hint:   "For self-managed instances use the instance URL (https://gitlab.example.com). Create an application with the callback URL and the scopes openid, profile and email.",
	},
	{
		ID:     "authelia",
		Name:   "Authelia",
		Issuer: "https://auth.example.com",
		Scopes: "openid profile email",
		Hint:   "The issuer is the Authelia portal URL; the client must be declared with client_secret and PKCE enabled in configuration.yml.",
	},
}

// showSSOPresets handles GET /api/v1/sso/presets.
func (s *Server) showSSOPresets(c *ctx) {
	out := make([]map[string]any, 0, len(ssoPresets))
	for _, preset := range ssoPresets {
		out = append(out, map[string]any{
			"id":     preset.ID,
			"name":   preset.Name,
			"issuer": preset.Issuer,
			"scopes": preset.Scopes,
			"hint":   preset.Hint,
		})
	}
	jsonResponse(c.w, http.StatusOK, out)
}

// keycloakNameRE restricts the identifiers injected into the generated files.
var keycloakNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// realmFromIssuer extracts the realm of a Keycloak issuer (.../realms/<realm>).
func realmFromIssuer(issuer string) string {
	parsed, err := url.Parse(oidc.NormalizeIssuer(issuer))
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i := 0; i+1 < len(segments); i++ {
		if segments[i] == "realms" {
			return segments[i+1]
		}
	}
	return ""
}

// keycloakClientObject builds the importable Keycloak client representation.
func keycloakClientObject(clientID, baseURL, redirectURI, secret string) map[string]any {
	return map[string]any{
		"clientId":                  clientID,
		"name":                      clientID,
		"description":               "2FAuth - two-factor authentication manager",
		"rootUrl":                   baseURL,
		"baseUrl":                   baseURL,
		"enabled":                   true,
		"alwaysDisplayInConsole":    true,
		"clientAuthenticatorType":   "client-secret",
		"secret":                    secret,
		"redirectUris":              []string{redirectURI, baseURL + "/*"},
		"webOrigins":                []string{baseURL},
		"standardFlowEnabled":       true,
		"implicitFlowEnabled":       false,
		"directAccessGrantsEnabled": false,
		"serviceAccountsEnabled":    false,
		"publicClient":              false,
		"frontchannelLogout":        true,
		"protocol":                  "openid-connect",
		"fullScopeAllowed":          true,
		"defaultClientScopes":       []string{"web-origins", "acr", "profile", "roles", "email"},
		"optionalClientScopes":      []string{"address", "phone", "offline_access", "microprofile-jwt"},
		"attributes": map[string]string{
			"pkce.code.challenge.method":          "S256",
			"post.logout.redirect.uris":           baseURL + "/*",
			"access.token.signed.response.alg":    "RS256",
			"backchannel.logout.session.required": "true",
		},
	}
}

// keycloakCLIScript renders a short kcadm.sh snippet importing the client.
func keycloakCLIScript(realm, clientJSON, issuer string) string {
	server := ""
	if parsed, err := url.Parse(oidc.NormalizeIssuer(issuer)); err == nil && parsed.Host != "" {
		server = parsed.Scheme + "://" + parsed.Host
	}
	if server == "" {
		server = "https://keycloak.example.com"
	}
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\n")
	b.WriteString("# Import the 2FAuth client into Keycloak with kcadm.sh.\n")
	b.WriteString("# Usage: KC_USER=<admin> ./keycloak-client.sh\n")
	b.WriteString("set -euo pipefail\n\n")
	b.WriteString("KC_SERVER=\"${KC_SERVER:-" + server + "}\"\n")
	b.WriteString("KC_REALM=\"${KC_REALM:-" + realm + "}\"\n")
	b.WriteString("KC_USER=\"${KC_USER:-admin}\"\n")
	b.WriteString("KC_PASSWORD=\"${KC_PASSWORD:-}\"\n\n")
	b.WriteString("cat > keycloak-client.json <<'JSON'\n")
	b.WriteString(clientJSON)
	if !strings.HasSuffix(clientJSON, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("JSON\n\n")
	b.WriteString("kcadm.sh config credentials --server \"$KC_SERVER\" --realm master --user \"$KC_USER\" --password \"$KC_PASSWORD\"\n")
	b.WriteString("kcadm.sh create clients -r \"$KC_REALM\" -f keycloak-client.json\n")
	return b.String()
}

// keycloakClient handles GET /api/v1/sso/keycloak-client.
func (s *Server) keycloakClient(c *ctx) {
	eff := s.ssoSettings()

	format := strings.ToLower(strings.TrimSpace(c.query("format")))
	if format == "" {
		format = "client"
	}
	v := newValidator(c)
	v.in("format", format, "client", "partial", "cli")

	clientID := strings.TrimSpace(c.query("client_id"))
	if clientID == "" {
		clientID = eff.ClientID
	}
	if clientID == "" {
		clientID = "2fauth"
	}
	realm := strings.TrimSpace(c.query("realm"))
	if realm == "" {
		realm = realmFromIssuer(eff.Issuer)
	}
	if realm == "" {
		realm = "2fauth"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(c.query("base_url")), "/")
	if baseURL == "" {
		baseURL = s.publicBaseURL(c)
	}

	if !keycloakNameRE.MatchString(clientID) {
		v.fail("client_id", "The client_id may only contain letters, digits, dots, dashes and underscores.")
	}
	if !keycloakNameRE.MatchString(realm) {
		v.fail("realm", "The realm may only contain letters, digits, dots, dashes and underscores.")
	}
	if !isAbsoluteHTTPURL(baseURL) {
		v.fail("base_url", "The base_url field must be a valid absolute URL.")
	}
	if v.respond() {
		return
	}

	// Make the exported client immediately usable: generate a secret when none
	// is stored yet and persist it encrypted.
	secret := eff.ClientSecret
	generated := false
	if secret == "" {
		secret = crypt.RandomString(40)
		encrypted, err := s.encryptStoredSecret(secret)
		if err != nil {
			s.srvError(c, err)
			return
		}
		if err := s.settings.Set(settingOpenIDClientSecret, encrypted); err != nil {
			s.srvError(c, err)
			return
		}
		generated = true
	}

	redirectURI := baseURL + "/socialite/callback/" + openIDProvider
	client := keycloakClientObject(clientID, baseURL, redirectURI, secret)

	filename := "keycloak-" + clientID + ".json"
	var content string
	switch format {
	case "partial":
		filename = "keycloak-" + clientID + "-partial.json"
		content = prettyJSON(map[string]any{
			"realm":            realm,
			"ifResourceExists": "SKIP",
			"clients":          []any{client},
		})
	case "cli":
		filename = "keycloak-" + clientID + ".sh"
		content = keycloakCLIScript(realm, prettyJSON(client), eff.Issuer)
	default:
		content = prettyJSON(client)
	}

	notice := "The client was generated from the current configuration. Import it into the \"" + realm + "\" realm, then sign in with SSO."
	if generated {
		notice = "A new 40-character client secret was generated and stored encrypted. Keep the exported file private: it contains the secret."
	} else if format != "cli" {
		notice = "The stored client secret was reused. Keep the exported file private: it contains the secret."
	}

	jsonResponse(c.w, http.StatusOK, map[string]any{
		"filename":         filename,
		"format":           format,
		"content":          content,
		"secret_generated": generated,
		"notice":           notice,
	})
}

// prettyJSON renders v as indented JSON. Every value passed here is composed of
// strings, booleans and slices, so marshalling cannot fail.
func prettyJSON(v any) string {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// isSSOSettingKey reports whether name belongs to the SSO namespace, which is
// served by the dedicated /api/v1/sso endpoints only.
func isSSOSettingKey(name string) bool {
	return strings.HasPrefix(name, "openid.")
}

// openIDEnabled reports whether the OpenID Connect provider is usable; it is
// used by the public front-end bootstrap.
func (s *Server) openIDEnabled() bool {
	return s.ssoSettings().Enabled
}
