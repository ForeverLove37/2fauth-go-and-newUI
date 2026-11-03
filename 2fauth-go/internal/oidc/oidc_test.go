package oidc

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// idpStub starts a fake provider and returns it with its server.
func idpStub(t *testing.T) (*httptest.Server, *Provider) {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
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
		if r.PostFormValue("grant_type") != "authorization_code" {
			http.Error(w, "bad grant", http.StatusBadRequest)
			return
		}
		if r.PostFormValue("code") != "test-code" {
			writeJSON(t, w, map[string]any{"error": "invalid_grant", "error_description": "bad code"})
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.PostFormValue("client_secret") != "secret" {
			http.Error(w, "bad client", http.StatusUnauthorized)
			return
		}
		writeJSON(t, w, map[string]any{
			"access_token":  "access-1",
			"id_token":      fakeIDToken(t, map[string]any{"sub": "abc", "email": "sso@example.com"}),
			"refresh_token": "refresh-1",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-1" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		writeJSON(t, w, map[string]any{"sub": "abc", "email": "sso@example.com", "name": "SSO User"})
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	provider, err := Discover(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	return srv, provider
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func fakeIDToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}

func TestDiscover(t *testing.T) {
	ResetCache()
	srv, provider := idpStub(t)

	if provider.Issuer != srv.URL {
		t.Errorf("issuer = %q, want %q", provider.Issuer, srv.URL)
	}
	if provider.AuthorizationEndpoint != srv.URL+"/authorize" {
		t.Errorf("authorization_endpoint = %q", provider.AuthorizationEndpoint)
	}
	if provider.TokenEndpoint != srv.URL+"/token" {
		t.Errorf("token_endpoint = %q", provider.TokenEndpoint)
	}
	if provider.UserInfoEndpoint != srv.URL+"/userinfo" {
		t.Errorf("userinfo_endpoint = %q", provider.UserInfoEndpoint)
	}
	if provider.EndSessionEndpoint != srv.URL+"/logout" {
		t.Errorf("end_session_endpoint = %q", provider.EndSessionEndpoint)
	}
	if provider.JWKSURI != srv.URL+"/jwks" {
		t.Errorf("jwks_uri = %q", provider.JWKSURI)
	}

	// A trailing slash is tolerated and the second call is served from the cache.
	again, err := Discover(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatalf("Discover with trailing slash: %v", err)
	}
	if again.Issuer != provider.Issuer {
		t.Errorf("cached issuer = %q", again.Issuer)
	}
}

func TestDiscoverFallbackAndMismatch(t *testing.T) {
	ResetCache()

	// Only the RFC 8414 metadata document exists.
	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/oauth-authorization-server" {
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, map[string]any{
			"issuer":                 "http://ignored-issuer.invalid",
			"authorization_endpoint": "https://idp.example.com/authorize",
			"token_endpoint":         "https://idp.example.com/token",
		})
	}))
	defer legacy.Close()
	if _, err := Discover(context.Background(), legacy.URL); err == nil {
		t.Fatal("expected an issuer mismatch error")
	}

	// A document announcing the requested issuer is accepted through the fallback.
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/oauth-authorization-server" {
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, map[string]any{
			"issuer":                 goodIssuer(r),
			"authorization_endpoint": "https://idp.example.com/authorize",
			"token_endpoint":         "https://idp.example.com/token",
		})
	}))
	defer good.Close()
	if _, err := Discover(context.Background(), good.URL); err != nil {
		t.Fatalf("fallback discovery: %v", err)
	}
}

// goodIssuer renders the issuer matching the request host.
func goodIssuer(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func TestAuthCodeURL(t *testing.T) {
	p := &Provider{AuthorizationEndpoint: "https://idp.example.com/authorize?foo=bar"}
	raw := AuthCodeURL(p, "client-1", "https://app.example.com/socialite/callback/openid",
		[]string{"openid", "profile", "email"}, "the-state", "the-nonce", "the-challenge")

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Host != "idp.example.com" || parsed.Path != "/authorize" {
		t.Fatalf("url = %q", raw)
	}
	q := parsed.Query()
	want := map[string]string{
		"response_type":         "code",
		"client_id":             "client-1",
		"redirect_uri":          "https://app.example.com/socialite/callback/openid",
		"scope":                 "openid profile email",
		"state":                 "the-state",
		"nonce":                 "the-nonce",
		"code_challenge":        "the-challenge",
		"code_challenge_method": "S256",
		"foo":                   "bar",
	}
	for key, value := range want {
		if q.Get(key) != value {
			t.Errorf("%s = %q, want %q", key, q.Get(key), value)
		}
	}
	if AuthCodeURL(nil, "c", "r", nil, "s", "n", "ch") != "" {
		t.Error("AuthCodeURL(nil) should be empty")
	}
}

func TestExchange(t *testing.T) {
	ResetCache()
	_, provider := idpStub(t)

	tokens, err := Exchange(context.Background(), provider, "client", "secret", "test-code",
		"https://app.example.com/socialite/callback/openid", "verifier")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if tokens.AccessToken != "access-1" {
		t.Errorf("access token = %q", tokens.AccessToken)
	}
	if tokens.RefreshToken != "refresh-1" {
		t.Errorf("refresh token = %q", tokens.RefreshToken)
	}
	if tokens.IDToken == "" {
		t.Error("no id token")
	}
	if tokens.Expiry != 3600*1000*1000*1000 {
		t.Errorf("expiry = %v", tokens.Expiry)
	}

	// A rejected code surfaces as an error.
	if _, err := Exchange(context.Background(), provider, "client", "secret", "wrong", "https://x/y", "v"); err == nil {
		t.Fatal("expected an error for an invalid code")
	}
}

func TestExchangeFallsBackToBasicAuth(t *testing.T) {
	ResetCache()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeJSON(t, w, map[string]any{
				"issuer":                 srv.URL,
				"authorization_endpoint": srv.URL + "/authorize",
				"token_endpoint":         srv.URL + "/token",
			})
		case "/token":
			_ = r.ParseForm()
			user, pass, ok := r.BasicAuth()
			if !ok || user == "" {
				http.Error(w, "client_secret_basic required", http.StatusUnauthorized)
				return
			}
			if pass != "secret" {
				http.Error(w, "bad secret", http.StatusUnauthorized)
				return
			}
			writeJSON(t, w, map[string]any{"access_token": "from-basic", "expires_in": "120"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	provider, err := Discover(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	tokens, err := Exchange(context.Background(), provider, "client", "secret", "code", "https://x/y", "v")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if tokens.AccessToken != "from-basic" {
		t.Errorf("access token = %q", tokens.AccessToken)
	}
	if tokens.Expiry != 120*1000*1000*1000 {
		t.Errorf("expiry = %v", tokens.Expiry)
	}
}

func TestUserInfo(t *testing.T) {
	ResetCache()
	_, provider := idpStub(t)

	claims, err := UserInfo(context.Background(), provider, "access-1")
	if err != nil {
		t.Fatalf("UserInfo: %v", err)
	}
	if claims["sub"] != "abc" || claims["email"] != "sso@example.com" {
		t.Errorf("claims = %v", claims)
	}
	if _, err := UserInfo(context.Background(), provider, "wrong-token"); err == nil {
		t.Fatal("expected an error for an invalid access token")
	}

	// An access token is never read as a source of claims.
	noUserInfo := &Provider{Issuer: "https://idp.example.com"}
	if _, err := UserInfo(context.Background(), noUserInfo, fakeIDToken(t, map[string]any{"sub": "xyz"})); !errors.Is(err, ErrNoUserInfoEndpoint) {
		t.Fatalf("UserInfo without an endpoint = %v", err)
	}
}

func TestPKCE(t *testing.T) {
	verifier, challenge := PKCE()
	if len(verifier) < minVerifierLen || len(verifier) > maxVerifierLen {
		t.Fatalf("verifier length = %d", len(verifier))
	}
	if strings.ContainsAny(verifier, "+/=") {
		t.Errorf("verifier is not base64url: %q", verifier)
	}
	sum := sha256.Sum256([]byte(verifier))
	if want := base64.RawURLEncoding.EncodeToString(sum[:]); challenge != want {
		t.Errorf("challenge = %q, want %q", challenge, want)
	}

	other, _ := PKCE()
	if other == verifier {
		t.Error("two PKCE calls returned the same verifier")
	}
}
