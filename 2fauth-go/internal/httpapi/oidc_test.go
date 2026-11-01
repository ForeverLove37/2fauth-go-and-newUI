package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"2fauth/internal/oidc"
)

// fakeIdP starts a minimal OpenID Connect provider with discovery, token and
// userinfo endpoints, and returns it with its issuer URL.
func fakeIdP(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 srv.URL,
			"authorization_endpoint": srv.URL + "/authorize",
			"token_endpoint":         srv.URL + "/token",
			"userinfo_endpoint":      srv.URL + "/userinfo",
			"end_session_endpoint":   srv.URL + "/logout",
			"jwks_uri":               srv.URL + "/jwks",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.PostFormValue("code") != "test" || r.PostFormValue("grant_type") != "authorization_code" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_grant"})
			return
		}
		if r.PostFormValue("code_verifier") == "" {
			http.Error(w, "PKCE verifier missing", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "idp-access-token",
			"refresh_token": "idp-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer idp-access-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":                "idp-user-1",
			"email":              "sso@example.com",
			"email_verified":     true,
			"name":               "SSO User",
			"preferred_username": "ssouser",
		})
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// enableOpenID configures the OpenID Connect provider directly on the config so
// that no environment variable is needed.
func enableOpenID(srv *Server, issuer, secret string) {
	srv.cfg.OpenIDEnabled = true
	srv.cfg.OpenIDIssuer = issuer
	srv.cfg.OpenIDClientID = "2fauth-test"
	srv.cfg.OpenIDClientSecret = secret
	srv.cfg.OpenIDAutoRegister = true
	srv.cfg.OpenIDAutoRedirect = false
}

// responseCookies returns the cookies of a recorder keyed by name.
func responseCookies(rec *httptest.ResponseRecorder) map[string]*http.Cookie {
	out := map[string]*http.Cookie{}
	for _, c := range rec.Result().Cookies() {
		out[c.Name] = c
	}
	return out
}

// startLogin runs the SSO redirect and returns the state of a fresh transaction
// together with the recorder that set the oidc_tx cookie.
func startLogin(t *testing.T, c *client) (string, *httptest.ResponseRecorder) {
	t.Helper()
	rec := c.do(http.MethodGet, "/socialite/redirect/openid?format=json", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("redirect = %d (%s)", rec.Code, rec.Body.String())
	}
	rawURL, _ := jsonBody(t, rec)["url"].(string)
	authURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse authorize url %q: %v", rawURL, err)
	}
	return authURL.Query().Get("state"), rec
}

func TestOIDCConfigEndpoint(t *testing.T) {
	oidc.ResetCache()
	srv := testServer(t)
	idp := fakeIdP(t)
	c := &client{t: t, srv: srv}

	// SSO is disabled by default.
	rec := c.do(http.MethodGet, "/api/v1/sso/config", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/sso/config = %d", rec.Code)
	}
	payload := jsonBody(t, rec)
	if payload["enabled"] != false {
		t.Errorf("enabled = %v, want false", payload["enabled"])
	}
	if payload["discovery"] != nil {
		t.Errorf("discovery = %v, want null", payload["discovery"])
	}
	if payload["provider"] != "openid" {
		t.Errorf("provider = %v", payload["provider"])
	}
	if payload["start_url"] != "/socialite/redirect/openid" {
		t.Errorf("start_url = %v", payload["start_url"])
	}

	// The front-end bootstrap reports the same (disabled) state.
	if upstream := jsonBody(t, c.do(http.MethodGet, "/api/v1/frontend-config", "", nil)); upstream["sso"] != nil {
		sso, _ := upstream["sso"].(map[string]any)
		if sso["openid"] != false {
			t.Errorf("frontend-config sso.openid = %v, want false", sso["openid"])
		}
	} else {
		t.Fatal("frontend-config has no sso block")
	}

	// Enable the provider on the test configuration.
	enableOpenID(srv, idp.URL, "s3cr3t-keep-private")

	rec = c.do(http.MethodGet, "/api/v1/sso/config", "", nil)
	payload = jsonBody(t, rec)
	if payload["enabled"] != true {
		t.Fatalf("enabled = %v (%s)", payload["enabled"], rec.Body.String())
	}
	if payload["discovery"] == nil {
		t.Fatal("discovery is null while enabled")
	}
	discovery, _ := payload["discovery"].(map[string]any)
	if discovery["issuer"] != idp.URL {
		t.Errorf("discovery issuer = %v", discovery["issuer"])
	}
	if discovery["authorization_endpoint"] != idp.URL+"/authorize" {
		t.Errorf("discovery authorization_endpoint = %v", discovery["authorization_endpoint"])
	}
	if discovery["end_session_endpoint"] != idp.URL+"/logout" {
		t.Errorf("discovery end_session_endpoint = %v", discovery["end_session_endpoint"])
	}
	if payload["label"] != "Sign in with SSO" {
		t.Errorf("label = %v", payload["label"])
	}
	redirectURI, _ := payload["redirect_uri"].(string)
	if !strings.HasSuffix(redirectURI, "/socialite/callback/openid") {
		t.Errorf("redirect_uri = %q", redirectURI)
	}
	if payload["error"] != nil {
		t.Errorf("error = %v, want null", payload["error"])
	}
	// The secret must never be part of the public answer.
	if strings.Contains(rec.Body.String(), "s3cr3t-keep-private") {
		t.Fatalf("the public SSO config leaks the client secret: %s", rec.Body.String())
	}
	if upstream := jsonBody(t, c.do(http.MethodGet, "/api/v1/frontend-config", "", nil)); upstream["sso"] != nil {
		sso, _ := upstream["sso"].(map[string]any)
		if sso["openid"] != true {
			t.Errorf("frontend-config sso.openid = %v, want true", sso["openid"])
		}
	}

	// A broken issuer keeps the feature disabled but explains why.
	srv.cfg.OpenIDIssuer = "http://127.0.0.1:1/"
	oidc.ResetCache()
	payload = jsonBody(t, c.do(http.MethodGet, "/api/v1/sso/config", "", nil))
	if payload["enabled"] != false {
		t.Errorf("enabled with a broken issuer = %v", payload["enabled"])
	}
	if message, _ := payload["error"].(string); message == "" {
		t.Errorf("error = %v, want a discovery failure message", payload["error"])
	}
}

func TestOIDCLoginFlow(t *testing.T) {
	oidc.ResetCache()
	srv := testServer(t)
	idp := fakeIdP(t)
	enableOpenID(srv, idp.URL, "s3cr3t-keep-private")

	c := &client{t: t, srv: srv}
	if rec := c.do(http.MethodGet, "/", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d", rec.Code)
	}

	// Start the flow in JSON mode to read the authorize URL back.
	rec := c.do(http.MethodGet, "/socialite/redirect/openid?format=json", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("redirect = %d (%s)", rec.Code, rec.Body.String())
	}
	rawURL, _ := jsonBody(t, rec)["url"].(string)
	if rawURL == "" {
		t.Fatalf("no url in %s", rec.Body.String())
	}
	authURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse authorize url: %v", err)
	}
	if authURL.Host != strings.TrimPrefix(idp.URL, "http://") {
		t.Errorf("authorize host = %q", authURL.Host)
	}
	query := authURL.Query()
	if query.Get("response_type") != "code" {
		t.Errorf("response_type = %q", query.Get("response_type"))
	}
	if query.Get("client_id") != "2fauth-test" {
		t.Errorf("client_id = %q", query.Get("client_id"))
	}
	if query.Get("scope") != "openid profile email" {
		t.Errorf("scope = %q", query.Get("scope"))
	}
	if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		t.Errorf("PKCE parameters = %v", query)
	}
	if query.Get("nonce") == "" {
		t.Error("no nonce in the authorize URL")
	}
	state := query.Get("state")
	if len(state) != 64 {
		t.Errorf("state = %q (want 32 bytes hex)", state)
	}

	// The transaction cookie must be HttpOnly, path-wide and short lived.
	tx, ok := responseCookies(rec)["oidc_tx"]
	if !ok {
		t.Fatalf("no oidc_tx cookie in %v", rec.Header())
	}
	if !tx.HttpOnly || tx.Path != "/" || tx.MaxAge != oidcTransactionTTL {
		t.Errorf("oidc_tx cookie = %+v", tx)
	}

	// A wrong state is rejected before anything is exchanged.
	rec = c.do(http.MethodGet, "/socialite/callback/openid?code=test&state=deadbeef", "", nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("bogus state = %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/login?sso_error=state" {
		t.Errorf("bogus state location = %q", location)
	}

	// The user cancelling at the provider is reported as such.
	state, _ = startLogin(t, c)
	rec = c.do(http.MethodGet, "/socialite/callback/openid?error=access_denied&state="+state, "", nil)
	if location := rec.Header().Get("Location"); location != "/login?sso_error=cancelled" {
		t.Errorf("cancelled location = %q", location)
	}

	// Complete the flow with a fresh transaction and the real state.
	state, rec = startLogin(t, c)
	rec = c.do(http.MethodGet, "/socialite/callback/openid?code=test&state="+state, "", nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("callback = %d (%s)", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "/" {
		t.Fatalf("callback location = %q", location)
	}
	cookies := responseCookies(rec)
	if sessionCookie, ok := cookies[sessionCookieName]; !ok || sessionCookie.Value == "" {
		t.Fatalf("no session cookie after the callback: %v", rec.Header())
	}
	if txCookie, ok := cookies["oidc_tx"]; ok && txCookie.Value != "" {
		t.Errorf("the oidc_tx cookie was not cleared: %v", txCookie.Value)
	}

	// The account was created from the userinfo claims and linked.
	user, err := srv.st.GetUserByEmail("sso@example.com")
	if err != nil {
		t.Fatalf("auto-created user: %v", err)
	}
	if user.Name != "SSO User" {
		t.Errorf("user name = %q", user.Name)
	}
	identity, err := srv.st.FindOAuthIdentity("openid", "idp-user-1")
	if err != nil {
		t.Fatalf("oauth identity: %v", err)
	}
	if identity.UserID != user.ID {
		t.Errorf("identity user = %d, want %d", identity.UserID, user.ID)
	}

	// The session is a full login.
	rec = c.do(http.MethodGet, "/api/v1/user", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/user = %d (%s)", rec.Code, rec.Body.String())
	}
	resource, _ := jsonBody(t, rec)["data"].(map[string]any)
	if resource["email"] != "sso@example.com" {
		t.Errorf("signed-in user = %v", resource["email"])
	}

	// A second login resolves through the stored identity (no new user).
	rec = c.do(http.MethodGet, "/socialite/redirect/openid?format=json", "", nil)
	again, _ := url.Parse(jsonBody(t, rec)["url"].(string))
	rec = c.do(http.MethodGet, "/socialite/callback/openid?code=test&state="+again.Query().Get("state"), "", nil)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("second login = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	users, err := srv.st.ListUsers()
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("users = %d, want 1 (the same account is reused)", len(users))
	}
}

func TestOIDCAdminEndpoints(t *testing.T) {
	oidc.ResetCache()
	srv := testServer(t)
	idp := fakeIdP(t)

	admin := &client{t: t, srv: srv}
	admin.do(http.MethodGet, "/", "", nil)
	if rec := admin.do(http.MethodPost, "/user",
		`{"name":"admin","email":"admin@example.com","password":"password123","password_confirmation":"password123"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("register admin = %d (%s)", rec.Code, rec.Body.String())
	}

	// Reading the settings before any change.
	rec := admin.do(http.MethodGet, "/api/v1/sso/settings", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET settings = %d (%s)", rec.Code, rec.Body.String())
	}
	payload := jsonBody(t, rec)
	if payload["enabled"] != false || payload["client_secret_set"] != false {
		t.Errorf("initial settings = %v", payload)
	}
	if payload["scopes"] != "openid profile email" || payload["label"] != "Sign in with SSO" {
		t.Errorf("initial defaults = %v", payload)
	}

	// An invalid issuer is rejected with the usual Laravel validation payload.
	rec = admin.do(http.MethodPut, "/api/v1/sso/settings", `{"issuer":"not-a-url"}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid issuer = %d (%s)", rec.Code, rec.Body.String())
	}
	errs, _ := jsonBody(t, rec)["errors"].(map[string]any)
	if _, ok := errs["issuer"]; !ok {
		t.Errorf("validation errors = %v", errs)
	}

	// Configure the provider (without a secret on purpose).
	rec = admin.do(http.MethodPut, "/api/v1/sso/settings",
		`{"enabled":true,"issuer":"`+idp.URL+`","client_id":"2fauth","auto_register":true,"auto_redirect":true,"label":"Login with IdP"}`,
		nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT settings = %d (%s)", rec.Code, rec.Body.String())
	}
	payload = jsonBody(t, rec)
	if payload["enabled"] != true {
		t.Errorf("enabled = %v (%s)", payload["enabled"], rec.Body.String())
	}
	if payload["issuer"] != idp.URL || payload["client_id"] != "2fauth" {
		t.Errorf("settings = %v", payload)
	}
	if payload["label"] != "Login with IdP" || payload["auto_redirect"] != true || payload["auto_register"] != true {
		t.Errorf("settings = %v", payload)
	}
	if payload["client_secret_set"] != false {
		t.Errorf("client_secret_set = %v, want false", payload["client_secret_set"])
	}

	// The effective config now answers enabled with the stored values.
	config := jsonBody(t, admin.do(http.MethodGet, "/api/v1/sso/config", "", nil))
	if config["enabled"] != true || config["label"] != "Login with IdP" || config["auto_redirect"] != true {
		t.Errorf("public config = %v", config)
	}

	// Discovery test endpoint.
	rec = admin.do(http.MethodPost, "/api/v1/sso/test", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/sso/test = %d (%s)", rec.Code, rec.Body.String())
	}
	if body := jsonBody(t, rec); body["ok"] != true || body["discovery"] == nil {
		t.Errorf("test payload = %v", body)
	}

	// Provider catalogue.
	rec = admin.do(http.MethodGet, "/api/v1/sso/presets", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("presets = %d", rec.Code)
	}
	var presets []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &presets); err != nil {
		t.Fatalf("presets payload: %v", err)
	}
	if len(presets) != 8 {
		t.Errorf("preset count = %d", len(presets))
	}
	seen := map[string]bool{}
	for _, preset := range presets {
		id, _ := preset["id"].(string)
		seen[id] = true
		if preset["issuer"] == "" || preset["name"] == "" || preset["scopes"] == "" || preset["hint"] == "" {
			t.Errorf("incomplete preset %v", preset)
		}
	}
	for _, id := range []string{"keycloak", "authentik", "auth0", "okta", "google", "entra", "gitlab", "authelia"} {
		if !seen[id] {
			t.Errorf("missing preset %q", id)
		}
	}

	// Keycloak client export: the secret is generated on first use.
	rec = admin.do(http.MethodGet, "/api/v1/sso/keycloak-client?format=client", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("keycloak client = %d (%s)", rec.Code, rec.Body.String())
	}
	export := jsonBody(t, rec)
	if export["secret_generated"] != true {
		t.Errorf("secret_generated = %v, want true", export["secret_generated"])
	}
	if export["format"] != "client" {
		t.Errorf("format = %v", export["format"])
	}
	filename, _ := export["filename"].(string)
	if filename != "keycloak-2fauth.json" {
		t.Errorf("filename = %q", filename)
	}
	if export["notice"] == "" {
		t.Error("no notice in the export")
	}
	content, _ := export["content"].(string)
	var kcClient map[string]any
	if err := json.Unmarshal([]byte(content), &kcClient); err != nil {
		t.Fatalf("client content is not JSON: %v", err)
	}
	if kcClient["clientId"] != "2fauth" {
		t.Errorf("clientId = %v", kcClient["clientId"])
	}
	if kcClient["secret"] == "" {
		t.Error("the generated client carries no secret")
	}
	redirectURIs, _ := kcClient["redirectUris"].([]any)
	if len(redirectURIs) != 2 || !strings.HasSuffix(redirectURIs[0].(string), "/socialite/callback/openid") {
		t.Errorf("redirectUris = %v", kcClient["redirectUris"])
	}
	attributes, _ := kcClient["attributes"].(map[string]any)
	if attributes["pkce.code.challenge.method"] != "S256" {
		t.Errorf("attributes = %v", attributes)
	}
	if kcClient["protocol"] != "openid-connect" || kcClient["publicClient"] != false {
		t.Errorf("client = %v", kcClient)
	}

	// The generated secret is persisted (encrypted) and reported.
	settings := jsonBody(t, admin.do(http.MethodGet, "/api/v1/sso/settings", "", nil))
	if settings["client_secret_set"] != true {
		t.Errorf("client_secret_set after export = %v", settings["client_secret_set"])
	}
	stored := srv.settings.Str("openid.client_secret")
	if !strings.HasPrefix(stored, encryptedSecretPrefix) {
		t.Errorf("stored secret is not marked as encrypted: %q", stored)
	}
	if strings.Contains(stored, kcClient["secret"].(string)) {
		t.Error("the client secret is stored in clear text")
	}
	if plain := srv.ssoSettings().ClientSecret; plain != kcClient["secret"] {
		t.Errorf("decrypted secret = %q, want %q", plain, kcClient["secret"])
	}

	// A second export reuses the stored secret.
	second := jsonBody(t, admin.do(http.MethodGet, "/api/v1/sso/keycloak-client?format=client", "", nil))
	if second["secret_generated"] != false {
		t.Errorf("second export secret_generated = %v", second["secret_generated"])
	}

	// Partial and CLI formats.
	partial := jsonBody(t, admin.do(http.MethodGet, "/api/v1/sso/keycloak-client?format=partial&realm=main", "", nil))
	var partialDoc map[string]any
	if err := json.Unmarshal([]byte(partial["content"].(string)), &partialDoc); err != nil {
		t.Fatalf("partial content is not JSON: %v", err)
	}
	if partialDoc["realm"] != "main" || partialDoc["ifResourceExists"] != "SKIP" {
		t.Errorf("partial doc = %v", partialDoc)
	}
	if clients, _ := partialDoc["clients"].([]any); len(clients) != 1 {
		t.Errorf("partial clients = %v", partialDoc["clients"])
	}

	rec = admin.do(http.MethodGet, "/api/v1/sso/keycloak-client?format=cli&client_id=2fauth-go", "", nil)
	cli := jsonBody(t, rec)
	if cli["format"] != "cli" {
		t.Errorf("cli format = %v", cli["format"])
	}
	script, _ := cli["content"].(string)
	for _, want := range []string{"kcadm.sh config credentials", "kcadm.sh create clients -r", "keycloak-client.json"} {
		if !strings.Contains(script, want) {
			t.Errorf("cli script misses %q:\n%s", want, script)
		}
	}

	// Invalid identifiers are refused instead of being injected into the script.
	rec = admin.do(http.MethodGet, "/api/v1/sso/keycloak-client?format=client&realm=bad-realm%3Brm%20-rf", "", nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("invalid realm = %d (%s)", rec.Code, rec.Body.String())
	}

	// Clearing the secret.
	rec = admin.do(http.MethodPut, "/api/v1/sso/settings", `{"clear_secret":true}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear secret = %d (%s)", rec.Code, rec.Body.String())
	}
	if body := jsonBody(t, rec); body["client_secret_set"] != false {
		t.Errorf("client_secret_set after clear = %v", body["client_secret_set"])
	}

	// A non administrator is refused on every admin SSO endpoint.
	sub := &client{t: t, srv: srv}
	sub.do(http.MethodGet, "/", "", nil)
	if rec := sub.do(http.MethodPost, "/user",
		`{"name":"bob","email":"bob@example.com","password":"password123","password_confirmation":"password123"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("register bob = %d (%s)", rec.Code, rec.Body.String())
	}
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/sso/settings"},
		{http.MethodPut, "/api/v1/sso/settings"},
		{http.MethodPost, "/api/v1/sso/test"},
		{http.MethodGet, "/api/v1/sso/presets"},
		{http.MethodGet, "/api/v1/sso/keycloak-client?format=client"},
	} {
		if rec := sub.do(route.method, route.path, "", nil); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", route.method, route.path, rec.Code)
		}
	}
	// The public config endpoint stays reachable.
	if rec := sub.do(http.MethodGet, "/api/v1/sso/config", "", nil); rec.Code != http.StatusOK {
		t.Errorf("public sso config = %d", rec.Code)
	}
}
