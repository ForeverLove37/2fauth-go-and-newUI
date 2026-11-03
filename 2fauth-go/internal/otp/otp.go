// Package otp implements the OTP algorithms supported by 2FAuth (TOTP, HOTP and
// Steam TOTP) as well as otpauth:// provisioning URI handling.
package otp

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"hash"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Supported OTP types (mirrors App\Models\TwoFAccount constants).
const (
	TypeTOTP  = "totp"
	TypeHOTP  = "hotp"
	TypeSteam = "steamtotp"
)

// Supported digest algorithms.
const (
	SHA1   = "sha1"
	SHA256 = "sha256"
	SHA512 = "sha512"
	MD5    = "md5"
)

const (
	// DefaultPeriod is the TOTP time step in seconds.
	DefaultPeriod = 30
	// DefaultDigits is the number of digits of a generated code.
	DefaultDigits = 6
	// DefaultCounter is the initial HOTP counter.
	DefaultCounter = 0
	// DefaultAlgorithm is the default digest.
	DefaultAlgorithm = SHA1
	// MinDigits and MaxDigits bound the length of a code.
	MinDigits = 5
	MaxDigits = 10
	// MaxPeriod bounds the TOTP time step (one week).
	MaxPeriod = 604800
	// MaxLabelLength bounds the service and account names.
	MaxLabelLength = 255
)

// Errors returned by this package.
var (
	ErrInvalidSecret      = errors.New("invalid secret")
	ErrInvalidOTPType     = errors.New("unsupported otp type")
	ErrInvalidParameter   = errors.New("invalid otp parameter")
	ErrInvalidURI         = errors.New("invalid otpauth uri")
	ErrMissingLabel       = errors.New("uri must contain a label")
	ErrUndecipherableText = errors.New("secret cannot be deciphered")
)

// Account holds the parameters needed to generate codes.
type Account struct {
	Type      string
	Secret    string // base32, case insensitive
	Algorithm string
	Digits    int
	Period    *int
	Counter   *int64
	Service   string
	Account   string
}

var steamAlphabet = []byte("23456789BCDFGHJKMNPQRTVWXY")

// SanitizeSecret normalises a base32 secret: strips spaces and padding, upper cases.
func SanitizeSecret(secret string) string {
	s := strings.ToUpper(strings.TrimSpace(secret))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.TrimRight(s, "=")
	return s
}

// PadSecret pads a base32 secret to a multiple of 8 characters, as 2FAuth does
// before persisting it.
func PadSecret(secret string) string {
	if secret == "" {
		return ""
	}
	upper := strings.ToUpper(secret)
	pad := int(math.Ceil(float64(len(upper))/8.0))*8 - len(upper)
	return upper + strings.Repeat("=", pad)
}

// DecodeBase32 decodes a base32 (RFC 4648) secret.
func DecodeBase32(secret string) ([]byte, error) {
	s := SanitizeSecret(secret)
	if s == "" {
		return nil, ErrInvalidSecret
	}
	var out []byte
	var buf, bits uint32
	for i := 0; i < len(s); i++ {
		v := b32Value(s[i])
		if v < 0 {
			return nil, ErrInvalidSecret
		}
		buf = buf<<5 | uint32(v)
		bits += 5
		if bits >= 8 {
			bits -= 8
			out = append(out, byte(buf>>bits))
		}
	}
	if len(out) == 0 {
		return nil, ErrInvalidSecret
	}
	return out, nil
}

// EncodeBase32 encodes raw bytes to an unpadded upper case base32 string.
func EncodeBase32(data []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	var sb strings.Builder
	var buf, bits uint32
	for _, b := range data {
		buf = buf<<8 | uint32(b)
		bits += 8
		for bits >= 5 {
			bits -= 5
			sb.WriteByte(alphabet[(buf>>bits)&0x1f])
		}
	}
	if bits > 0 {
		sb.WriteByte(alphabet[(buf<<(5-bits))&0x1f])
	}
	return sb.String()
}

func b32Value(c byte) int {
	switch {
	case c >= 'A' && c <= 'Z':
		return int(c - 'A')
	case c >= '2' && c <= '7':
		return int(c-'2') + 26
	}
	return -1
}

func digest(alg string) (func() hash.Hash, error) {
	switch strings.ToLower(alg) {
	case "", SHA1:
		return sha1.New, nil
	case SHA256:
		return sha256.New, nil
	case SHA512:
		return sha512.New, nil
	case MD5:
		return md5.New, nil
	}
	return nil, ErrInvalidParameter
}

// hotpValue computes the RFC 4226 HOTP integer for the given counter.
func hotpValue(key []byte, counter uint64, newHash func() hash.Hash) (uint32, error) {
	msg := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		msg[i] = byte(counter & 0xff)
		counter >>= 8
	}
	mac := hmac.New(newHash, key)
	if _, err := mac.Write(msg); err != nil {
		return 0, err
	}
	sum := mac.Sum(nil)
	offset := int(sum[len(sum)-1] & 0x0f)
	// RFC 4226 truncation assumes a digest of at least 19 bytes. MD5 only has 16:
	// bytes past the end count as zero, which is what OTPHP computes for them.
	at := func(i int) uint32 {
		if i < len(sum) {
			return uint32(sum[i])
		}
		return 0
	}
	return (at(offset)&0x7f)<<24 | at(offset+1)<<16 | at(offset+2)<<8 | at(offset+3), nil
}

// Code returns the code for the account at the given unix timestamp.
func Code(a Account, ts int64) (string, error) {
	ts = NormalizeTime(ts)
	switch a.Type {
	case TypeTOTP, "":
		return totpCode(a, ts)
	case TypeSteam:
		return steamCode(a, ts)
	case TypeHOTP:
		counter := int64(DefaultCounter)
		if a.Counter != nil {
			counter = *a.Counter
		}
		return hotpCode(a, uint64(counter))
	}
	return "", ErrInvalidOTPType
}

func totpCode(a Account, ts int64) (string, error) {
	period := DefaultPeriod
	if a.Period != nil && *a.Period > 0 {
		period = *a.Period
	}
	digits := a.Digits
	if digits <= 0 {
		digits = DefaultDigits
	}
	key, err := DecodeBase32(a.Secret)
	if err != nil {
		return "", err
	}
	newHash, err := digest(a.Algorithm)
	if err != nil {
		return "", err
	}
	bin, err := hotpValue(key, uint64(ts/int64(period)), newHash)
	if err != nil {
		return "", err
	}
	return formatCode(bin, digits), nil
}

func hotpCode(a Account, counter uint64) (string, error) {
	digits := a.Digits
	if digits <= 0 {
		digits = DefaultDigits
	}
	key, err := DecodeBase32(a.Secret)
	if err != nil {
		return "", err
	}
	newHash, err := digest(a.Algorithm)
	if err != nil {
		return "", err
	}
	bin, err := hotpValue(key, counter, newHash)
	if err != nil {
		return "", err
	}
	return formatCode(bin, digits), nil
}

// steamCode renders the Steam Guard alphabet code (5 chars).
func steamCode(a Account, ts int64) (string, error) {
	key, err := DecodeBase32(a.Secret)
	if err != nil {
		return "", err
	}
	bin, err := hotpValue(key, uint64(ts/DefaultPeriod), sha1.New)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for i := 0; i < 5; i++ {
		sb.WriteByte(steamAlphabet[bin%uint32(len(steamAlphabet))])
		bin /= uint32(len(steamAlphabet))
	}
	return sb.String(), nil
}

func formatCode(bin uint32, digits int) string {
	if digits > MaxDigits {
		// Only reachable through rows stored by an older build.
		digits = MaxDigits
	}
	mod := uint64(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, uint64(bin)%mod)
}

// Period returns the effective TOTP period.
func Period(a Account) int {
	if a.Period != nil && *a.Period > 0 {
		return *a.Period
	}
	if a.Type == TypeSteam {
		return DefaultPeriod
	}
	return DefaultPeriod
}

// ExpiresIn mirrors OTPHP TOTP::expiresIn(): seconds left before a new code.
func ExpiresIn(a Account, ts int64) int {
	period := Period(a)
	return period - int(NormalizeTime(ts)%int64(period))
}

// NormalizeTime validates 2FAuth's `at` query parameter, which is truncated to
// seconds and must stay within the current TOTP window when provided.
func NormalizeTime(ts int64) int64 {
	if ts <= 0 {
		return time.Now().Unix()
	}
	return ts
}

// NextPassword mirrors the `next_password` field: the code of the following window.
func NextPassword(a Account, ts int64) (string, error) {
	return Code(a, ts+int64(ExpiresIn(a, ts))+2)
}

// ParseURI parses an otpauth:// provisioning URI.
func ParseURI(uri string) (Account, error) {
	acc := Account{Type: TypeTOTP, Algorithm: DefaultAlgorithm, Digits: DefaultDigits}
	// 2FAuth escapes '#' which appears in Azure AD labels
	sanitized := strings.ReplaceAll(strings.TrimSpace(uri), "#", "%23")
	u, err := url.Parse(sanitized)
	if err != nil {
		return acc, ErrInvalidURI
	}
	if !strings.EqualFold(u.Scheme, "otpauth") {
		return acc, ErrInvalidURI
	}

	switch strings.ToLower(u.Host) {
	case "totp":
		acc.Type = TypeTOTP
	case "hotp":
		acc.Type = TypeHOTP
	case "steam":
		acc.Type = TypeSteam
	default:
		return acc, ErrInvalidURI
	}

	label := strings.TrimPrefix(u.Path, "/")
	label, err = url.PathUnescape(label)
	if err != nil {
		return acc, ErrInvalidURI
	}
	label = strings.ReplaceAll(label, "%23", "#")
	if label == "" {
		return acc, ErrMissingLabel
	}
	// OTPHP splits "Issuer:account" keeping both parts
	if idx := strings.Index(label, ":"); idx >= 0 {
		acc.Service = strings.TrimSpace(label[:idx])
		acc.Account = strings.TrimSpace(label[idx+1:])
	} else {
		acc.Account = label
	}

	q := u.Query()
	secret := q.Get("secret")
	if secret == "" {
		return acc, ErrInvalidSecret
	}
	acc.Secret = SanitizeSecret(secret)

	if issuer := q.Get("issuer"); issuer != "" {
		acc.Service = issuer
	}
	if alg := q.Get("algorithm"); alg != "" {
		alg = strings.ToUpper(alg)
		switch alg {
		case "SHA1", "SHA256", "SHA512", "MD5":
			acc.Algorithm = strings.ToLower(alg)
		default:
			// OTPHP falls back to the default digest on unknown values
			acc.Algorithm = DefaultAlgorithm
		}
	}
	if d := q.Get("digits"); d != "" {
		n, err := strconv.Atoi(d)
		if err != nil || n < MinDigits || n > MaxDigits {
			return acc, ErrInvalidParameter
		}
		acc.Digits = n
	}
	if p := q.Get("period"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > MaxPeriod {
			return acc, ErrInvalidParameter
		}
		acc.Period = &n
	}
	if c := q.Get("counter"); c != "" {
		n, err := strconv.ParseInt(c, 10, 64)
		if err != nil || n < 0 {
			return acc, ErrInvalidParameter
		}
		acc.Counter = &n
	}
	if strings.EqualFold(q.Get("encoder"), "steam") || acc.Type == TypeSteam {
		acc.Type = TypeSteam
	}
	if acc.Type == TypeSteam {
		// enforceAsSteam()
		acc.Service = "Steam"
		acc.Digits = 5
		acc.Algorithm = SHA1
		period := DefaultPeriod
		acc.Period = &period
	}
	if acc.Type == TypeHOTP && acc.Counter == nil {
		c := int64(DefaultCounter)
		acc.Counter = &c
	}
	if acc.Type == TypeTOTP && acc.Period == nil {
		p := DefaultPeriod
		acc.Period = &p
	}
	return acc, nil
}

// Validate reports whether the account can actually generate codes. Every path
// that stores or computes an account (form, URI, import) goes through it, so a bad
// value is refused up front instead of failing later on every listing.
func Validate(a Account) error {
	switch a.Type {
	case TypeTOTP, TypeHOTP, TypeSteam:
	default:
		return ErrInvalidOTPType
	}
	if _, err := DecodeBase32(a.Secret); err != nil {
		return ErrInvalidSecret
	}
	if _, err := digest(a.Algorithm); err != nil {
		return ErrInvalidParameter
	}
	if a.Digits != 0 && (a.Digits < MinDigits || a.Digits > MaxDigits) {
		return ErrInvalidParameter
	}
	if a.Period != nil && (*a.Period < 1 || *a.Period > MaxPeriod) {
		return ErrInvalidParameter
	}
	if a.Counter != nil && *a.Counter < 0 {
		return ErrInvalidParameter
	}
	if len([]rune(a.Service)) > MaxLabelLength || len([]rune(a.Account)) > MaxLabelLength {
		return ErrInvalidParameter
	}
	return nil
}

// BuildURI renders the otpauth:// provisioning URI of an account, mirroring
// OTPHP's getProvisioningUri().
func BuildURI(a Account) string {
	scheme := a.Type
	label := a.Account
	if label == "" {
		label = a.Service
	}
	if a.Service != "" && a.Account != "" && !strings.EqualFold(a.Service, a.Account) {
		label = a.Service + ":" + a.Account
	}
	params := url.Values{}
	params.Set("secret", SanitizeSecret(a.Secret))
	if a.Service != "" {
		params.Set("issuer", a.Service)
	}
	if a.Type == TypeSteam {
		params.Set("encoder", "steam")
	}
	digits := a.Digits
	if digits == 0 {
		digits = DefaultDigits
	}
	params.Set("digits", strconv.Itoa(digits))
	alg := strings.ToUpper(a.Algorithm)
	if alg == "" {
		alg = strings.ToUpper(DefaultAlgorithm)
	}
	params.Set("algorithm", alg)
	if a.Type == TypeHOTP {
		counter := int64(DefaultCounter)
		if a.Counter != nil {
			counter = *a.Counter
		}
		params.Set("counter", strconv.FormatInt(counter, 10))
	} else {
		period := DefaultPeriod
		if a.Period != nil && *a.Period > 0 {
			period = *a.Period
		}
		params.Set("period", strconv.Itoa(period))
	}
	return fmt.Sprintf("otpauth://%s/%s?%s", scheme, url.PathEscape(label), params.Encode())
}
