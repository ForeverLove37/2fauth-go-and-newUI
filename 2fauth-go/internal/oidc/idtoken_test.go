package oidc

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type signer struct {
	alg  string
	sign func(input []byte) []byte
	jwk  map[string]any
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func rsaSigner(t *testing.T) signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return signer{"RS256", func(input []byte) []byte {
		sum := sha256.Sum256(input)
		sig, err := rsa.SignPKCS1v15(rand.Reader, key, 5 /* crypto.SHA256 */, sum[:])
		if err != nil {
			t.Fatal(err)
		}
		return sig
	}, map[string]any{"kty": "RSA", "kid": "k", "n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes())}}
}

func ecSigner(t *testing.T) signer {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := key.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return signer{"ES256", func(input []byte) []byte {
		sum := sha256.Sum256(input)
		r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
		if err != nil {
			t.Fatal(err)
		}
		raw := make([]byte, 64)
		r.FillBytes(raw[:32])
		s.FillBytes(raw[32:])
		return raw
	}, map[string]any{"kty": "EC", "kid": "k", "crv": "P-256", "x": b64(pub[1:33]), "y": b64(pub[33:65])}}
}

func edSigner(t *testing.T) signer {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return signer{"EdDSA", func(input []byte) []byte { return ed25519.Sign(priv, input) },
		map[string]any{"kty": "OKP", "kid": "k", "crv": "Ed25519", "x": b64(pub)}}
}

func token(t *testing.T, s signer, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]any{"alg": s.alg, "kid": "k"})
	payload, _ := json.Marshal(claims)
	input := b64(header) + "." + b64(payload)
	return input + "." + b64(s.sign([]byte(input)))
}

func goodClaims() map[string]any {
	return map[string]any{
		"iss": "https://idp.test", "aud": "client", "sub": "user-1", "nonce": "n0nce",
		"exp": float64(time.Now().Add(time.Minute).Unix()), "iat": float64(time.Now().Unix()),
	}
}

func jwksServer(t *testing.T, keys ...map[string]any) *Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	t.Cleanup(srv.Close)
	ResetCache()
	return &Provider{Issuer: "https://idp.test", JWKSURI: srv.URL}
}

func TestVerifyIDTokenAlgorithms(t *testing.T) {
	for _, s := range []signer{rsaSigner(t), ecSigner(t), edSigner(t)} {
		p := jwksServer(t, s.jwk)
		claims, err := VerifyIDToken(context.Background(), p, token(t, s, goodClaims()), "client", "", "n0nce")
		if err != nil {
			t.Errorf("%s: %v", s.alg, err)
			continue
		}
		if claims["sub"] != "user-1" {
			t.Errorf("%s: claims = %v", s.alg, claims)
		}
	}
}

func TestVerifyIDTokenHMACNeedsTheClientSecret(t *testing.T) {
	s := signer{alg: "HS256", sign: func(input []byte) []byte {
		mac := hmac.New(sha256.New, []byte("client-secret"))
		mac.Write(input)
		return mac.Sum(nil)
	}}
	p := &Provider{Issuer: "https://idp.test"}
	if _, err := VerifyIDToken(context.Background(), p, token(t, s, goodClaims()), "client", "client-secret", "n0nce"); err != nil {
		t.Fatalf("HS256: %v", err)
	}
	if _, err := VerifyIDToken(context.Background(), p, token(t, s, goodClaims()), "client", "other", "n0nce"); err == nil {
		t.Fatal("a wrong HMAC key was accepted")
	}
	if _, err := VerifyIDToken(context.Background(), p, token(t, s, goodClaims()), "client", "", "n0nce"); err == nil {
		t.Fatal("HS256 was accepted without a client secret")
	}
}

func TestVerifyIDTokenRejections(t *testing.T) {
	s := ecSigner(t)
	other := ecSigner(t)
	p := jwksServer(t, s.jwk)
	cases := map[string]string{
		"wrong issuer":    token(t, s, patch(goodClaims(), "iss", "https://evil.test")),
		"no issuer":       token(t, s, patch(goodClaims(), "iss", nil)),
		"wrong audience":  token(t, s, patch(goodClaims(), "aud", "other")),
		"no audience":     token(t, s, patch(goodClaims(), "aud", nil)),
		"several aud":     token(t, s, patch(goodClaims(), "aud", []any{"client", "other"})),
		"wrong azp":       token(t, s, patch(patch(goodClaims(), "aud", []any{"client", "x"}), "azp", "x")),
		"expired":         token(t, s, patch(goodClaims(), "exp", float64(time.Now().Add(-time.Hour).Unix()))),
		"no expiry":       token(t, s, patch(goodClaims(), "exp", nil)),
		"future iat":      token(t, s, patch(goodClaims(), "iat", float64(time.Now().Add(time.Hour).Unix()))),
		"wrong nonce":     token(t, s, patch(goodClaims(), "nonce", "other")),
		"no nonce":        token(t, s, patch(goodClaims(), "nonce", nil)),
		"no subject":      token(t, s, patch(goodClaims(), "sub", "")),
		"another key":     token(t, s, goodClaims())[:0] + token(t, other, goodClaims()),
		"not a JWT":       "abc.def",
		"unsigned":        unsigned(goodClaims()),
		"empty signature": dropSignature(token(t, s, goodClaims())),
	}
	for name, raw := range cases {
		if _, err := VerifyIDToken(context.Background(), p, raw, "client", "", "n0nce"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestVerifyIDTokenRefusesWeakRSAKeys(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	k := jwk{Kty: "RSA", N: b64(key.N.Bytes()), E: b64(big.NewInt(int64(key.E)).Bytes())}
	if _, err := k.publicKey(); err == nil {
		t.Fatal("a 1024-bit RSA key was accepted")
	}
}

func TestSigningKeyRefreshesAfterRotation(t *testing.T) {
	first, second := ecSigner(t), ecSigner(t)
	current := first.jwk
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{current}})
	}))
	defer srv.Close()
	ResetCache()
	p := &Provider{Issuer: "https://idp.test", JWKSURI: srv.URL}
	if _, err := VerifyIDToken(context.Background(), p, token(t, first, goodClaims()), "client", "", "n0nce"); err != nil {
		t.Fatalf("first key: %v", err)
	}
	// Same kid, new key material: the cached key cannot verify it, which is an
	// invalid signature rather than a reason to accept.
	current = second.jwk
	if _, err := VerifyIDToken(context.Background(), p, token(t, second, goodClaims()), "client", "", "n0nce"); !errors.Is(err, ErrIDTokenInvalid) {
		t.Fatalf("stale key = %v", err)
	}
}

func TestDiscoverRefusesDowngrade(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 srv.URL,
			"authorization_endpoint": srv.URL + "/a",
			"token_endpoint":         "http://clear.test/token",
		})
	}))
	defer srv.Close()
	ResetCache()
	previous := HTTPClient
	HTTPClient = srv.Client()
	defer func() { HTTPClient = previous }()
	if _, err := Discover(context.Background(), srv.URL); err == nil {
		t.Fatal("a clear-text token endpoint was accepted for an HTTPS issuer")
	}
}

func TestDiscoverRemembersFailures(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	ResetCache()
	for i := 0; i < 5; i++ {
		if _, err := Discover(context.Background(), srv.URL); err == nil {
			t.Fatal("expected a failure")
		}
	}
	if hits != 2 { // the two well-known locations of the first attempt
		t.Fatalf("provider was called %d times, want 2", hits)
	}
}

func patch(claims map[string]any, name string, value any) map[string]any {
	if value == nil {
		delete(claims, name)
		return claims
	}
	claims[name] = value
	return claims
}

func unsigned(claims map[string]any) string {
	payload, _ := json.Marshal(claims)
	return b64([]byte(`{"alg":"none"}`)) + "." + b64(payload) + "."
}

func dropSignature(raw string) string {
	for i := len(raw) - 1; i >= 0; i-- {
		if raw[i] == '.' {
			return raw[:i+1]
		}
	}
	return raw
}
