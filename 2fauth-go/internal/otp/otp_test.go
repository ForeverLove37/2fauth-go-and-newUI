package otp

import (
	"strings"
	"testing"
)

// Secret from RFC 4226/6238 test vectors (ASCII "12345678901234567890").
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

// RFC 6238 uses longer shared secrets for SHA-256 and SHA-512.
const (
	rfcSecret256 = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZA"
	rfcSecret512 = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNA"
)

// secretFor returns the RFC 6238 secret matching an algorithm.
func secretFor(alg string) string {
	switch alg {
	case SHA256:
		return rfcSecret256
	case SHA512:
		return rfcSecret512
	default:
		return rfcSecret
	}
}

func ptrInt(v int) *int     { return &v }
func ptrI64(v int64) *int64 { return &v }

func totpAcc(alg string, digits int) Account {
	return Account{
		Type:      TypeTOTP,
		Secret:    secretFor(alg),
		Algorithm: alg,
		Digits:    digits,
		Period:    ptrInt(30),
	}
}

// TestTOTPRFC6238Vectors checks the published RFC 6238 test vectors.
func TestTOTPRFC6238Vectors(t *testing.T) {
	cases := []struct {
		at     int64
		alg    string
		digits int
		want   string
	}{
		{59, SHA1, 8, "94287082"},
		{59, SHA256, 8, "46119246"},
		{59, SHA512, 8, "90693936"},
		{1111111109, SHA1, 8, "07081804"},
		{1111111111, SHA1, 8, "14050471"},
		{1234567890, SHA1, 8, "89005924"},
		{2000000000, SHA1, 8, "69279037"},
		{20000000000, SHA1, 8, "65353130"},
	}
	for _, tc := range cases {
		acc := totpAcc(tc.alg, tc.digits)
		code, err := Code(acc, tc.at)
		if err != nil {
			t.Fatalf("Code(%s,%d): %v", tc.alg, tc.at, err)
		}
		if code != tc.want {
			t.Errorf("TOTP %s at %d = %s, want %s", tc.alg, tc.at, code, tc.want)
		}
	}
}

// TestHOTPRFC4226Vectors checks the published RFC 4226 counter vectors.
func TestHOTPRFC4226Vectors(t *testing.T) {
	want := []string{"755224", "287082", "359152", "969429", "338314"}
	for counter, expected := range want {
		acc := Account{
			Type:      TypeHOTP,
			Secret:    rfcSecret,
			Algorithm: SHA1,
			Digits:    6,
			Counter:   ptrI64(int64(counter)),
		}
		code, err := Code(acc, 0)
		if err != nil {
			t.Fatalf("Code(counter=%d): %v", counter, err)
		}
		if code != expected {
			t.Errorf("HOTP counter %d = %s, want %s", counter, code, expected)
		}
	}
}

func TestSteamCodeFormat(t *testing.T) {
	acc := Account{Type: TypeSteam, Secret: rfcSecret, Digits: 5, Algorithm: SHA1, Period: ptrInt(30)}
	code, err := Code(acc, 1111111109)
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if len(code) != 5 {
		t.Fatalf("steam code %q is not 5 characters long", code)
	}
	for _, r := range code {
		if !strings.ContainsRune(string(steamAlphabet), r) {
			t.Errorf("steam code %q contains %q which is outside the Steam alphabet", code, r)
		}
	}
}

func TestParseAndBuildURI(t *testing.T) {
	uri := "otpauth://totp/ACME%20Co:john.doe@email.com?secret=" + rfcSecret +
		"&issuer=ACME%20Co&algorithm=SHA256&digits=8&period=60"
	acc, err := ParseURI(uri)
	if err != nil {
		t.Fatalf("ParseURI: %v", err)
	}
	if acc.Service != "ACME Co" || acc.Account != "john.doe@email.com" {
		t.Errorf("label parsed as %q/%q", acc.Service, acc.Account)
	}
	if acc.Algorithm != SHA256 || acc.Digits != 8 || acc.Period == nil || *acc.Period != 60 {
		t.Errorf("parameters parsed as %s/%d/%v", acc.Algorithm, acc.Digits, acc.Period)
	}

	rebuilt := BuildURI(acc)
	again, err := ParseURI(rebuilt)
	if err != nil {
		t.Fatalf("ParseURI(rebuilt): %v", err)
	}
	if again.Secret != acc.Secret || again.Service != acc.Service || again.Account != acc.Account {
		t.Errorf("round trip lost data: %+v", again)
	}
}

func TestMalformedURIs(t *testing.T) {
	for _, uri := range []string{
		"http://example.com",
		"otpauth://totp/?secret=ABC",
	} {
		if _, err := ParseURI(uri); err == nil {
			t.Errorf("ParseURI(%q) did not fail", uri)
		}
	}
}

func TestExpiresIn(t *testing.T) {
	acc := totpAcc(SHA1, 6)
	if got := ExpiresIn(acc, 30); got != 30 {
		t.Errorf("ExpiresIn(30) = %d, want 30", got)
	}
	if got := ExpiresIn(acc, 1); got != 29 {
		t.Errorf("ExpiresIn(1) = %d, want 29", got)
	}
	acc.Period = nil
	if got := ExpiresIn(acc, 0); got < 1 || got > DefaultPeriod {
		t.Errorf("ExpiresIn with no period and no timestamp = %d, want 1..%d", got, DefaultPeriod)
	}
}
