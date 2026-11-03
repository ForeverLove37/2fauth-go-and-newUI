package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Errors returned while verifying an ID token.
var (
	ErrIDTokenInvalid = errors.New("oidc: invalid id token")
	ErrNoSigningKey   = errors.New("oidc: no signing key matches the id token")
)

const (
	// clockSkew tolerates a small clock difference with the provider.
	clockSkew = 60 * time.Second
	// jwksTTL is how long a key set is trusted.
	jwksTTL = time.Hour
	// jwksRefreshInterval limits the refreshes triggered by an unknown key id.
	jwksRefreshInterval = time.Minute
	// minRSABits refuses keys that are too weak to verify anything.
	minRSABits = 2048
)

// VerifyIDToken checks the signature and the claims of an ID token as required by
// OpenID Connect Core 3.1.3.7 and returns its claims. It accepts only the algorithms
// listed below ("none" is never accepted), and the HMAC family only when the client
// has a secret to verify with.
func VerifyIDToken(ctx context.Context, p *Provider, raw, clientID, clientSecret, nonce string) (map[string]any, error) {
	if p == nil {
		return nil, errors.New("oidc: no provider")
	}
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: not a signed JWT", ErrIDTokenInvalid)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodeSegment(parts[0], &header); err != nil {
		return nil, fmt.Errorf("%w: header: %v", ErrIDTokenInvalid, err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: signature encoding", ErrIDTokenInvalid)
	}
	signed := []byte(parts[0] + "." + parts[1])

	switch header.Alg {
	case "HS256", "HS384", "HS512":
		if clientSecret == "" {
			return nil, fmt.Errorf("%w: %s needs a client secret", ErrIDTokenInvalid, header.Alg)
		}
		if err := verifyHMAC(header.Alg, []byte(clientSecret), signed, signature); err != nil {
			return nil, err
		}
	case "RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA":
		key, err := signingKey(ctx, p, header.Kid, header.Alg)
		if err != nil {
			return nil, err
		}
		if err := verifySignature(header.Alg, key, signed, signature); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: algorithm %q is not accepted", ErrIDTokenInvalid, header.Alg)
	}

	var claims map[string]any
	if err := decodeSegment(parts[1], &claims); err != nil {
		return nil, fmt.Errorf("%w: payload: %v", ErrIDTokenInvalid, err)
	}
	if err := checkClaims(claims, p.Issuer, clientID, nonce, time.Now()); err != nil {
		return nil, err
	}
	return claims, nil
}

func decodeSegment(segment string, dst any) error {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

func checkClaims(claims map[string]any, issuer, clientID, nonce string, now time.Time) error {
	if iss, _ := claims["iss"].(string); NormalizeIssuer(iss) != NormalizeIssuer(issuer) || iss == "" {
		return fmt.Errorf("%w: issuer %q", ErrIDTokenInvalid, iss)
	}
	audiences, ok := audienceList(claims["aud"])
	if !ok || !contains(audiences, clientID) {
		return fmt.Errorf("%w: wrong audience", ErrIDTokenInvalid)
	}
	azp, hasAZP := claims["azp"].(string)
	if hasAZP && azp != clientID {
		return fmt.Errorf("%w: wrong authorized party", ErrIDTokenInvalid)
	}
	if len(audiences) > 1 && !hasAZP {
		return fmt.Errorf("%w: several audiences without azp", ErrIDTokenInvalid)
	}
	exp, ok := numericDate(claims["exp"])
	if !ok || now.After(exp.Add(clockSkew)) {
		return fmt.Errorf("%w: expired", ErrIDTokenInvalid)
	}
	if iat, ok := numericDate(claims["iat"]); ok && iat.After(now.Add(clockSkew)) {
		return fmt.Errorf("%w: issued in the future", ErrIDTokenInvalid)
	}
	if nbf, ok := numericDate(claims["nbf"]); ok && nbf.After(now.Add(clockSkew)) {
		return fmt.Errorf("%w: not valid yet", ErrIDTokenInvalid)
	}
	got, _ := claims["nonce"].(string)
	if nonce == "" || subtle.ConstantTimeCompare([]byte(got), []byte(nonce)) != 1 {
		return fmt.Errorf("%w: nonce mismatch", ErrIDTokenInvalid)
	}
	if sub, _ := claims["sub"].(string); strings.TrimSpace(sub) == "" {
		return fmt.Errorf("%w: no subject", ErrIDTokenInvalid)
	}
	return nil
}

func audienceList(v any) ([]string, bool) {
	switch t := v.(type) {
	case string:
		return []string{t}, t != ""
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, len(out) > 0
	}
	return nil, false
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func numericDate(v any) (time.Time, bool) {
	n, ok := v.(float64)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(int64(n), 0), true
}

// ---------------------------------------------------------------------------
// signatures
// ---------------------------------------------------------------------------

func hashFor(alg string) (func() hash.Hash, crypto.Hash) {
	switch alg[2:] {
	case "384":
		return sha512.New384, crypto.SHA384
	case "512":
		return sha512.New, crypto.SHA512
	}
	return sha256.New, crypto.SHA256
}

func verifyHMAC(alg string, key, signed, signature []byte) error {
	newHash, _ := hashFor(alg)
	mac := hmac.New(newHash, key)
	mac.Write(signed)
	if !hmac.Equal(mac.Sum(nil), signature) {
		return fmt.Errorf("%w: bad signature", ErrIDTokenInvalid)
	}
	return nil
}

func verifySignature(alg string, key any, signed, signature []byte) error {
	bad := fmt.Errorf("%w: bad signature", ErrIDTokenInvalid)
	if alg == "EdDSA" {
		pub, ok := key.(ed25519.PublicKey)
		if !ok || !ed25519.Verify(pub, signed, signature) {
			return bad
		}
		return nil
	}
	newHash, id := hashFor(alg)
	h := newHash()
	h.Write(signed)
	sum := h.Sum(nil)

	switch alg[:2] {
	case "RS":
		pub, ok := key.(*rsa.PublicKey)
		if !ok || rsa.VerifyPKCS1v15(pub, id, sum, signature) != nil {
			return bad
		}
	case "PS":
		pub, ok := key.(*rsa.PublicKey)
		if !ok || rsa.VerifyPSS(pub, id, sum, signature, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash}) != nil {
			return bad
		}
	case "ES":
		pub, ok := key.(*ecdsa.PublicKey)
		if !ok {
			return bad
		}
		size := (pub.Curve.Params().BitSize + 7) / 8
		if len(signature) != 2*size {
			return bad
		}
		r := new(big.Int).SetBytes(signature[:size])
		s := new(big.Int).SetBytes(signature[size:])
		if !ecdsa.Verify(pub, sum, r, s) {
			return bad
		}
	default:
		return bad
	}
	return nil
}

// ---------------------------------------------------------------------------
// key set
// ---------------------------------------------------------------------------

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

type parsedKey struct {
	kid string
	alg string
	key any
}

type jwksEntry struct {
	keys    []parsedKey
	fetched time.Time
}

var (
	jwksMu    sync.Mutex
	jwksCache = map[string]jwksEntry{}
)

// resetJWKS drops the cached key sets.
func resetJWKS() {
	jwksMu.Lock()
	jwksCache = map[string]jwksEntry{}
	jwksMu.Unlock()
}

// signingKey finds the key of a token, refreshing the set when the provider has
// rotated its keys since the last fetch.
func signingKey(ctx context.Context, p *Provider, kid, alg string) (any, error) {
	if p.JWKSURI == "" {
		return nil, fmt.Errorf("%w: the provider has no jwks_uri", ErrNoSigningKey)
	}
	jwksMu.Lock()
	entry, cached := jwksCache[p.JWKSURI]
	jwksMu.Unlock()

	if cached && time.Since(entry.fetched) < jwksTTL {
		if key := pickKey(entry.keys, kid, alg); key != nil {
			return key, nil
		}
		if time.Since(entry.fetched) < jwksRefreshInterval {
			return nil, ErrNoSigningKey
		}
	}
	keys, err := fetchJWKS(ctx, p.JWKSURI)
	if err != nil {
		return nil, err
	}
	jwksMu.Lock()
	jwksCache[p.JWKSURI] = jwksEntry{keys: keys, fetched: time.Now()}
	jwksMu.Unlock()
	if key := pickKey(keys, kid, alg); key != nil {
		return key, nil
	}
	return nil, ErrNoSigningKey
}

func pickKey(keys []parsedKey, kid, alg string) any {
	var match any
	count := 0
	for _, k := range keys {
		if !keyFitsAlg(k.key, alg) || (k.alg != "" && k.alg != alg) {
			continue
		}
		if kid != "" {
			if k.kid == kid {
				return k.key
			}
			continue
		}
		match = k.key
		count++
	}
	if kid == "" && count == 1 {
		return match
	}
	return nil
}

func keyFitsAlg(key any, alg string) bool {
	switch key.(type) {
	case *rsa.PublicKey:
		return strings.HasPrefix(alg, "RS") || strings.HasPrefix(alg, "PS")
	case *ecdsa.PublicKey:
		return strings.HasPrefix(alg, "ES")
	case ed25519.PublicKey:
		return alg == "EdDSA"
	}
	return false
}

func fetchJWKS(ctx context.Context, uri string) ([]parsedKey, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc: jwks request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc: jwks endpoint answered %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("oidc: jwks request failed: %w", err)
	}
	var doc struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("oidc: invalid jwks document: %w", err)
	}
	out := make([]parsedKey, 0, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		key, err := k.publicKey()
		if err != nil {
			continue // unknown key types are ignored, as RFC 7517 asks
		}
		out = append(out, parsedKey{kid: k.Kid, alg: k.Alg, key: key})
	}
	return out, nil
}

func (k jwk) publicKey() (any, error) {
	switch k.Kty {
	case "RSA":
		n, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, err
		}
		e, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil || len(e) == 0 || len(e) > 4 {
			return nil, errors.New("invalid exponent")
		}
		pub := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
		if pub.N.BitLen() < minRSABits {
			return nil, errors.New("RSA key too short")
		}
		return pub, nil
	case "EC":
		var curve elliptic.Curve
		switch k.Crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		default:
			return nil, errors.New("unsupported curve")
		}
		size := (curve.Params().BitSize + 7) / 8
		x, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil || len(x) != size {
			return nil, errors.New("invalid x coordinate")
		}
		y, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil || len(y) != size {
			return nil, errors.New("invalid y coordinate")
		}
		// ParseUncompressedPublicKey also rejects a point outside the curve.
		return ecdsa.ParseUncompressedPublicKey(curve, append(append([]byte{4}, x...), y...))
	case "OKP":
		if k.Crv != "Ed25519" {
			return nil, errors.New("unsupported curve")
		}
		x, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil || len(x) != ed25519.PublicKeySize {
			return nil, errors.New("invalid key")
		}
		return ed25519.PublicKey(x), nil
	}
	return nil, errors.New("unsupported key type")
}
