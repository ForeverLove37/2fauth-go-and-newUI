package crypt

import "testing"

// laravelKey is a fixed 32 byte key used to check compatibility with Laravel's
// Crypt::encryptString() output format.
const laravelKey = "base64:AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="

// laravelCiphertext was produced by an independent implementation of Laravel's
// Crypt format (AES-256-CBC, PKCS#7, JSON payload {iv, value, mac, tag}, HMAC
// over iv.value.tag) for the payload below.
const laravelCiphertext = "eyJpdiI6IkFBRUNBd1FGQmdjSUNRb0xEQTBPRHc9PSIsInZhbHVlIjoiTkNJa0tjYlNQUzYyanhWdnEyOTBLdmxsZVF0Q1kydEdGdmpHYkpNVGJ3bkRpd3E4QnFLN1BoS0tacTFSS3p6U3czbUtsRCt5M1duK1Q3Nis3RDZra0E9PSIsIm1hYyI6ImY2NWQzMmU3MzJmYWY4YjU2YTFiNDMzZGEwZDkyZjk3ZDI4NGJkMmFmM2JmMmQzZTUwZGE5MmU3Zjk1NWNlZDkiLCJ0YWciOiIifQ=="

const laravelPlaintext = `{"id":42,"name":"alice","secret":"JBSWY3DPEHPK3PXP"}`

func TestDecryptLaravelPayload(t *testing.T) {
	enc, err := New(laravelKey)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := enc.DecryptString(laravelCiphertext)
	if err != nil {
		t.Fatalf("DecryptString: %v", err)
	}
	if got != laravelPlaintext {
		t.Fatalf("decrypted payload = %q, want %q", got, laravelPlaintext)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	enc, err := New(laravelKey)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, plain := range []string{"", "a", laravelPlaintext, "clé-éàü"} {
		ciphertext, err := enc.EncryptString(plain)
		if err != nil {
			t.Fatalf("EncryptString(%q): %v", plain, err)
		}
		if plain != "" && !enc.IsEncrypted(ciphertext) {
			t.Errorf("IsEncrypted(%q) = false", ciphertext)
		}
		got, err := enc.DecryptString(ciphertext)
		if err != nil {
			t.Fatalf("DecryptString: %v", err)
		}
		if got != plain {
			t.Errorf("round trip = %q, want %q", got, plain)
		}
	}
}

func TestDecryptRejectsTamperedPayload(t *testing.T) {
	enc, _ := New(laravelKey)
	if _, err := enc.DecryptString("bm90LWEtbGFyYXZlbC1wYXlsb2Fk"); err == nil {
		t.Fatal("expected an error for a non-Laravel payload")
	}
}

func TestGenerateKey(t *testing.T) {
	key := GenerateKey()
	enc, err := New(key)
	if err != nil {
		t.Fatalf("New(%q): %v", key, err)
	}
	ciphertext, err := enc.EncryptString("secret")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	if got, _ := enc.DecryptString(ciphertext); got != "secret" {
		t.Fatalf("round trip = %q", got)
	}
}

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !CheckPassword(hash, "password123") {
		t.Error("CheckPassword rejected the right password")
	}
	if CheckPassword(hash, "password124") {
		t.Error("CheckPassword accepted a wrong password")
	}
}
