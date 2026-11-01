// Package oidc implements the parts of OpenID Connect the application needs:
// provider discovery, the authorization-code flow with PKCE, token exchange and
// userinfo retrieval. It is provider agnostic and works with Keycloak, Authentik,
// Auth0, Okta, Google, Microsoft Entra ID, GitLab and Authelia alike.
package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNoUserInfoEndpoint is returned by UserInfo when the provider document does
// not advertise a userinfo_endpoint and the token itself carries no claims.
var ErrNoUserInfoEndpoint = errors.New("oidc: the provider has no userinfo_endpoint")

// ErrIssuerMismatch is returned when the discovery document announces another
// issuer than the one that was requested.
var ErrIssuerMismatch = errors.New("oidc: issuer mismatch in discovery document")

const (
	// discoveryTimeout bounds every outbound request.
	defaultTimeout = 10 * time.Second
	// cacheTTL is how long a provider document is trusted.
	cacheTTL = 30 * time.Minute
	// minVerifierLen and maxVerifierLen bound the PKCE code verifier (RFC 7636).
	minVerifierLen = 43
	maxVerifierLen = 128
)

// Provider is the subset of an OpenID Connect discovery document the app uses.
type Provider struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// TokenSet is the result of a successful code exchange.
type TokenSet struct {
	AccessToken  string
	IDToken      string
	RefreshToken string
	Expiry       time.Duration
}

// HTTPClient performs every outbound request. It honours HTTP_PROXY, HTTPS_PROXY
// and NO_PROXY, keeps certificate verification enabled and gives up after 10s.
// Tests (or an embedder) may replace it; a nil value falls back to the default.
var HTTPClient = defaultHTTPClient()

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: defaultTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			// Certificate verification stays enabled: InsecureSkipVerify is never set.
			TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
			ForceAttemptHTTP2: true,
			MaxIdleConns:      20,
			IdleConnTimeout:   90 * time.Second,
		},
	}
}

func httpClient() *http.Client {
	if HTTPClient != nil {
		return HTTPClient
	}
	return defaultHTTPClient()
}

// ---------------------------------------------------------------------------
// discovery
// ---------------------------------------------------------------------------

type cacheEntry struct {
	provider *Provider
	expires  time.Time
}

var (
	cacheMu sync.Mutex
	cache   = map[string]cacheEntry{}
)

// Discover fetches the provider document of issuer and caches it for 30 minutes.
// It tries the OpenID Connect well-known location first and falls back to the
// OAuth 2.0 authorization server metadata location.
func Discover(ctx context.Context, issuer string) (*Provider, error) {
	key := NormalizeIssuer(issuer)
	if key == "" {
		return nil, errors.New("oidc: empty issuer")
	}

	cacheMu.Lock()
	entry, ok := cache[key]
	cacheMu.Unlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.provider, nil
	}

	provider, err := discoverUncached(ctx, key)
	if err != nil {
		return nil, err
	}

	cacheMu.Lock()
	cache[key] = cacheEntry{provider: provider, expires: time.Now().Add(cacheTTL)}
	cacheMu.Unlock()
	return provider, nil
}

// ResetCache drops every cached provider document (used by tests).
func ResetCache() {
	cacheMu.Lock()
	cache = map[string]cacheEntry{}
	cacheMu.Unlock()
}

// NormalizeIssuer trims the surrounding whitespace and any trailing slash.
func NormalizeIssuer(issuer string) string {
	return strings.TrimRight(strings.TrimSpace(issuer), "/")
}

func discoverUncached(ctx context.Context, issuer string) (*Provider, error) {
	paths := []string{
		"/.well-known/openid-configuration",
		"/.well-known/oauth-authorization-server",
	}
	errs := make([]string, 0, len(paths))
	for _, path := range paths {
		provider, err := fetchProvider(ctx, issuer+path, issuer)
		if err == nil {
			return provider, nil
		}
		errs = append(errs, err.Error())
	}
	return nil, fmt.Errorf("oidc: discovery failed for %s: %s", issuer, strings.Join(errs, "; "))
}

func fetchProvider(ctx context.Context, endpoint, issuer string) (*Provider, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", endpoint, err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%s: unexpected status %d", endpoint, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", endpoint, err)
	}
	var provider Provider
	if err := json.Unmarshal(body, &provider); err != nil {
		return nil, fmt.Errorf("%s: invalid discovery document: %w", endpoint, err)
	}
	docIssuer := NormalizeIssuer(provider.Issuer)
	if docIssuer != "" && docIssuer != issuer {
		return nil, fmt.Errorf("%w: got %q, want %q", ErrIssuerMismatch, docIssuer, issuer)
	}
	if provider.AuthorizationEndpoint == "" {
		return nil, fmt.Errorf("%s: discovery document has no authorization_endpoint", endpoint)
	}
	if provider.TokenEndpoint == "" {
		return nil, fmt.Errorf("%s: discovery document has no token_endpoint", endpoint)
	}
	if provider.Issuer == "" {
		provider.Issuer = issuer
	}
	return &provider, nil
}

// ---------------------------------------------------------------------------
// authorization-code flow
// ---------------------------------------------------------------------------

// AuthCodeURL builds the authorization endpoint URL of the login redirect.
func AuthCodeURL(p *Provider, clientID, redirectURI string, scopes []string, state, nonce, codeChallenge string) string {
	if p == nil || p.AuthorizationEndpoint == "" {
		return ""
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	if len(scopes) > 0 {
		q.Set("scope", strings.Join(scopes, " "))
	}
	if state != "" {
		q.Set("state", state)
	}
	if nonce != "" {
		q.Set("nonce", nonce)
	}
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")

	sep := "?"
	if strings.Contains(p.AuthorizationEndpoint, "?") {
		sep = "&"
	}
	return p.AuthorizationEndpoint + sep + q.Encode()
}

// tokenResponse is the RFC 6749 token endpoint answer.
type tokenResponse struct {
	AccessToken      string  `json:"access_token"`
	IDToken          string  `json:"id_token"`
	RefreshToken     string  `json:"refresh_token"`
	TokenType        string  `json:"token_type"`
	ExpiresIn        flexInt `json:"expires_in"`
	Error            string  `json:"error"`
	ErrorDescription string  `json:"error_description"`
}

// flexInt accepts both a JSON number and a quoted number (some providers).
type flexInt int64

// UnmarshalJSON implements json.Unmarshaler.
func (f *flexInt) UnmarshalJSON(b []byte) error {
	raw := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if raw == "" || raw == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		// ignore unparsable values instead of failing the whole exchange
		*f = 0
		return nil
	}
	*f = flexInt(n)
	return nil
}

// Exchange redeems an authorization code for tokens. The client secret is sent
// with the client_secret_post method first and, when the provider rejects it
// with a 401, the call is retried using HTTP Basic authentication.
func Exchange(ctx context.Context, p *Provider, clientID, clientSecret, code, redirectURI, codeVerifier string) (*TokenSet, error) {
	if p == nil || p.TokenEndpoint == "" {
		return nil, errors.New("oidc: the provider has no token_endpoint")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", clientID)
	if redirectURI != "" {
		form.Set("redirect_uri", redirectURI)
	}
	if codeVerifier != "" {
		form.Set("code_verifier", codeVerifier)
	}

	tokens, status, err := postTokenRequest(ctx, p.TokenEndpoint, form, clientID, clientSecret, false)
	if err == nil {
		return tokens, nil
	}
	if status == http.StatusUnauthorized && clientSecret != "" {
		tokens, _, retryErr := postTokenRequest(ctx, p.TokenEndpoint, form, clientID, clientSecret, true)
		return tokens, retryErr
	}
	return nil, err
}

func postTokenRequest(ctx context.Context, endpoint string, form url.Values, clientID, clientSecret string, basic bool) (*TokenSet, int, error) {
	payload := url.Values{}
	for k, v := range form {
		payload[k] = append([]string(nil), v...)
	}
	if clientSecret != "" && !basic {
		payload.Set("client_secret", clientSecret)
	}

	reqCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, strings.NewReader(payload.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if basic {
		req.SetBasicAuth(url.QueryEscape(clientID), url.QueryEscape(clientSecret))
	}

	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("oidc: token exchange failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("oidc: token exchange failed: %w", err)
	}

	var tr tokenResponse
	_ = json.Unmarshal(body, &tr)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		detail := strings.TrimSpace(tr.Error)
		if tr.ErrorDescription != "" {
			detail = strings.TrimSpace(detail + ": " + tr.ErrorDescription)
		}
		if detail == "" {
			detail = strings.TrimSpace(string(body))
			if len(detail) > 200 {
				detail = detail[:200]
			}
		}
		return nil, resp.StatusCode, fmt.Errorf("oidc: token endpoint answered %d (%s)", resp.StatusCode, detail)
	}
	if tr.AccessToken == "" && tr.IDToken == "" {
		return nil, resp.StatusCode, errors.New("oidc: token endpoint returned no token")
	}

	return &TokenSet{
		AccessToken:  tr.AccessToken,
		IDToken:      tr.IDToken,
		RefreshToken: tr.RefreshToken,
		Expiry:       time.Duration(tr.ExpiresIn) * time.Second,
	}, resp.StatusCode, nil
}

// UserInfo returns the claims of the signed-in user. When the provider does not
// advertise a userinfo_endpoint the claims are read from the token itself (the
// ID token payload), which is enough for account linking.
func UserInfo(ctx context.Context, p *Provider, accessToken string) (map[string]any, error) {
	if p == nil {
		return nil, errors.New("oidc: no provider")
	}
	if p.UserInfoEndpoint == "" {
		if claims, err := ParseClaims(accessToken); err == nil {
			return claims, nil
		}
		return nil, ErrNoUserInfoEndpoint
	}
	if ctx == nil {
		ctx = context.Background()
	}

	reqCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, p.UserInfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc: userinfo request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("oidc: userinfo request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("oidc: userinfo endpoint answered %d", resp.StatusCode)
	}
	var claims map[string]any
	if err := json.Unmarshal(body, &claims); err != nil {
		return nil, fmt.Errorf("oidc: invalid userinfo document: %w", err)
	}
	if claims == nil {
		return nil, errors.New("oidc: empty userinfo document")
	}
	return claims, nil
}

// ParseClaims decodes the payload of a JWT without verifying its signature. It
// is only used to read the identity claims after a successful code exchange,
// never to make a trust decision on its own.
func ParseClaims(token string) (map[string]any, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("oidc: empty token")
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, errors.New("oidc: not a JWT")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		if raw, err = base64.URLEncoding.DecodeString(parts[1]); err != nil {
			return nil, fmt.Errorf("oidc: invalid JWT payload: %w", err)
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, fmt.Errorf("oidc: invalid JWT payload: %w", err)
	}
	return claims, nil
}

// PKCE returns a fresh code verifier and its S256 challenge (RFC 7636).
func PKCE() (verifier, challenge string) {
	verifier = base64.RawURLEncoding.EncodeToString(randomBytes(32))
	if len(verifier) < minVerifierLen {
		verifier = verifier + strings.Repeat("A", minVerifierLen-len(verifier))
	}
	if len(verifier) > maxVerifierLen {
		verifier = verifier[:maxVerifierLen]
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}
