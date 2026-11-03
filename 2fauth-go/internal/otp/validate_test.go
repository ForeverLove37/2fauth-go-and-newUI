package otp

import (
	"crypto/hmac"
	"crypto/md5"
	"encoding/binary"
	"strings"
	"testing"
)

func TestTenDigitCodesDoNotOverflow(t *testing.T) {
	// 10^10 does not fit in 32 bits: RFC 4226 keeps the whole 31-bit value here.
	if got := formatCode(2147483647, 10); got != "2147483647" {
		t.Fatalf("formatCode = %s", got)
	}
	if got := formatCode(1410065408, 10); got != "1410065408" {
		t.Fatalf("formatCode = %s", got)
	}
	if got := formatCode(7, 10); got != "0000000007" {
		t.Fatalf("formatCode = %s", got)
	}
}

func TestStoredOutOfRangeDigitsCannotPanic(t *testing.T) {
	if got := formatCode(123456, 32); len(got) != MaxDigits {
		t.Fatalf("formatCode = %s", got)
	}
}

func TestMD5NeverPanics(t *testing.T) {
	a := Account{Type: TypeHOTP, Secret: "JBSWY3DPEHPK3PXP", Algorithm: MD5, Digits: 6}
	short := 0
	for counter := int64(0); counter < 2000; counter++ {
		c := counter
		a.Counter = &c
		code, err := Code(a, 1)
		if err != nil {
			t.Fatalf("counter %d: %v", counter, err)
		}
		if len(code) != 6 {
			t.Fatalf("counter %d: code %q", counter, code)
		}
		key, _ := DecodeBase32(a.Secret)
		msg := make([]byte, 8)
		binary.BigEndian.PutUint64(msg, uint64(counter))
		mac := hmac.New(md5.New, key)
		mac.Write(msg)
		if mac.Sum(nil)[15]&0x0f > 12 {
			short++
		}
	}
	if short == 0 {
		t.Fatal("the test never reached the offsets past the end of the digest")
	}
}

func TestParseURIRefusesOutOfRangeValues(t *testing.T) {
	for _, query := range []string{"digits=32", "digits=4", "digits=11", "period=0", "period=-5", "period=99999999", "counter=-1"} {
		if _, err := ParseURI("otpauth://totp/a:b?secret=JBSWY3DPEHPK3PXP&" + query); err == nil {
			t.Errorf("%s was accepted", query)
		}
	}
}

func TestDecodeBase32IsStrict(t *testing.T) {
	for _, secret := range []string{"JBSWY3DP01", "!!!", "ABC 8"} {
		if _, err := DecodeBase32(secret); err == nil {
			t.Errorf("%q was accepted", secret)
		}
	}
}

func TestValidate(t *testing.T) {
	period := 30
	good := Account{Type: TypeTOTP, Secret: "JBSWY3DPEHPK3PXP", Algorithm: SHA1, Digits: 6, Period: &period}
	if err := Validate(good); err != nil {
		t.Fatalf("Validate(good) = %v", err)
	}
	bad := map[string]func(*Account){
		"type":    func(a *Account) { a.Type = "motp" },
		"secret":  func(a *Account) { a.Secret = "not base32!" },
		"empty":   func(a *Account) { a.Secret = "" },
		"digits":  func(a *Account) { a.Digits = 32 },
		"alg":     func(a *Account) { a.Algorithm = "sha3" },
		"service": func(a *Account) { a.Service = strings.Repeat("x", 300) },
	}
	for name, change := range bad {
		a := good
		change(&a)
		if err := Validate(a); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
