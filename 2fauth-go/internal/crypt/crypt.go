// Package crypt implements the Laravel-compatible primitives 2FAuth relies on:
// AES-256-CBC payload encryption with an HMAC-SHA256 MAC (Crypt::encryptString /
// Crypt::decryptString), bcrypt password hashing and Laravel's random string helper.
package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidMAC is returned when the payload MAC does not match.
var ErrInvalidMAC = errors.New("invalid mac")

// ErrInvalidPayload is returned when the payload is not a valid Laravel payload.
var ErrInvalidPayload = errors.New("invalid payload")

// Encrypter mirrors Illuminate\Encryption\Encrypter with the AES-256-CBC cipher.
type Encrypter struct {
	key []byte
}

// New builds an Encrypter from an APP_KEY value ("base64:..." or a 32 byte string).
func New(appKey string) (*Encrypter, error) {
	key := appKey
	if strings.HasPrefix(key, "base64:") {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(key, "base64:"))
		if err != nil {
			return nil, fmt.Errorf("invalid base64 APP_KEY: %w", err)
		}
		key = string(raw)
	} else if len(key) > 32 {
		key = key[:32]
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("APP_KEY must be 32 bytes for AES-256-CBC, got %d", len(key))
	}
	return &Encrypter{key: []byte(key)}, nil
}

// GenerateKey returns a fresh APP_KEY in Laravel's "base64:..." format.
func GenerateKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "base64:" + base64.StdEncoding.EncodeToString(b)
}

type payload struct {
	IV    string `json:"iv"`
	Value string `json:"value"`
	Mac   string `json:"mac"`
	Tag   string `json:"tag"`
}

// EncryptString mirrors Crypt::encryptString().
func (e *Encrypter) EncryptString(plain string) (string, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}
	iv := make([]byte, block.BlockSize())
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	// PKCS#7 padding
	data := []byte(plain)
	pad := block.BlockSize() - len(data)%block.BlockSize()
	data = append(data, bytesRepeat(byte(pad), pad)...)
	cipherText := make([]byte, len(data))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(cipherText, data)

	p := payload{
		IV:    base64.StdEncoding.EncodeToString(iv),
		Value: base64.StdEncoding.EncodeToString(cipherText),
		Mac:   "",
		Tag:   "",
	}
	p.Mac = e.hash(p.IV, p.Value, p.Tag)

	js, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(js), nil
}

// DecryptString mirrors Crypt::decryptString().
func (e *Encrypter) DecryptString(encrypted string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", ErrInvalidPayload
	}
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", ErrInvalidPayload
	}
	if p.IV == "" || p.Value == "" || p.Mac == "" {
		// tolerate a plaintext value so mixed installs keep working
		if !strings.HasPrefix(encrypted, "eyJ") {
			return encrypted, nil
		}
		return "", ErrInvalidPayload
	}
	expected := e.hash(p.IV, p.Value, p.Tag)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(p.Mac)) != 1 {
		return "", ErrInvalidMAC
	}

	iv, err := base64.StdEncoding.DecodeString(p.IV)
	if err != nil {
		return "", ErrInvalidPayload
	}
	cipherText, err := base64.StdEncoding.DecodeString(p.Value)
	if err != nil {
		return "", ErrInvalidPayload
	}
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}
	if len(iv) != block.BlockSize() || len(cipherText) == 0 || len(cipherText)%block.BlockSize() != 0 {
		return "", ErrInvalidPayload
	}
	plain := make([]byte, len(cipherText))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, cipherText)

	// strip PKCS#7 padding
	n := int(plain[len(plain)-1])
	if n == 0 || n > block.BlockSize() || n > len(plain) {
		return "", ErrInvalidPayload
	}
	for _, b := range plain[len(plain)-n:] {
		if int(b) != n {
			return "", ErrInvalidPayload
		}
	}
	return string(plain[:len(plain)-n]), nil
}

// IsEncrypted reports whether v looks like a Laravel encrypted payload.
func (e *Encrypter) IsEncrypted(v string) bool {
	if v == "" {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return false
	}
	var p payload
	if json.Unmarshal(raw, &p) != nil {
		return false
	}
	return p.IV != "" && p.Value != "" && p.Mac != ""
}

func (e *Encrypter) hash(iv, value, tag string) string {
	mac := hmac.New(sha256.New, e.key)
	mac.Write([]byte(iv))
	mac.Write([]byte(value))
	mac.Write([]byte(tag))
	return hex.EncodeToString(mac.Sum(nil))
}

// HashPassword returns a bcrypt hash compatible with Laravel's Hash::make().
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), 12)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword verifies a password against a Laravel bcrypt hash.
func CheckPassword(hashed, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain)) == nil
}

const laravelAlpha = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// RandomString mirrors Str::random(): a base62 string of length n.
func RandomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	out := make([]byte, n)
	for i, v := range b {
		out[i] = laravelAlpha[int(v)%len(laravelAlpha)]
	}
	return string(out)
}

// RandomBytes returns cryptographically secure random bytes.
func RandomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// SHA256Hex returns the hex digest of s.
func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// HMACSHA256Hex returns a hex HMAC-SHA256 digest.
func HMACSHA256Hex(key, msg string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

// --- minimal PHP serialization support (legacy encrypted fields) ---

// UnserializeString parses a PHP serialized scalar (string/int/bool/float/null).
func UnserializeString(s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "N;" {
		return nil, nil
	}
	if len(s) < 2 {
		return nil, fmt.Errorf("invalid serialized value")
	}
	switch s[0] {
	case 'b':
		if s == "b:1;" {
			return true, nil
		}
		if s == "b:0;" {
			return false, nil
		}
		return nil, fmt.Errorf("invalid serialized bool")
	case 'i':
		n, err := strconv.Atoi(strings.TrimSuffix(s[2:], ";"))
		return n, err
	case 'd':
		f, err := strconv.ParseFloat(strings.TrimSuffix(s[2:], ";"), 64)
		return f, err
	case 's':
		colon := strings.Index(s, ":")
		if colon < 0 {
			return nil, fmt.Errorf("invalid serialized string")
		}
		rest := s[colon+1:]
		q := strings.Index(rest, ":\"")
		if q < 0 {
			return nil, fmt.Errorf("invalid serialized string")
		}
		end := strings.LastIndex(rest, "\";")
		if end < 0 {
			return nil, fmt.Errorf("invalid serialized string")
		}
		return rest[q+2 : end], nil
	}
	return nil, fmt.Errorf("unsupported serialized type %q", s[0])
}

// SerializeString produces PHP's serialized form for a scalar value.
func SerializeString(v any) string {
	switch t := v.(type) {
	case nil:
		return "N;"
	case string:
		return fmt.Sprintf("s:%d:\"%s\";", len(t), t)
	case bool:
		if t {
			return "b:1;"
		}
		return "b:0;"
	case int:
		return fmt.Sprintf("i:%d;", t)
	case int64:
		return fmt.Sprintf("i:%d;", t)
	case float64:
		return "d:" + strconv.FormatFloat(t, 'f', -1, 64) + ";"
	}
	return fmt.Sprintf("s:%d:\"%v\";", len(fmt.Sprint(v)), v)
}
