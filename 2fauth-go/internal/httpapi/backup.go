package httpapi

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/scrypt"

	"2fauth/internal/crypt"
)

// encryptedExportFormat identifies an encrypted 2FAuth export. docs/IMPORT.md
// describes the format, so a backup can be opened without this application.
const encryptedExportFormat = "2fauth-encrypted-v1"

// Cost of the key derivation of new exports (argon2id, RFC 9106 second recommendation).
const (
	exportArgonTime   = 3
	exportArgonMemory = 64 * 1024 // KiB
	exportArgonThread = 4
)

// Limits on what an imported file may ask for: the parameters come from the file, and
// a crafted one must not be able to allocate gigabytes or run for minutes.
const (
	maxArgonTime     = 10
	maxArgonMemory   = 512 * 1024 // KiB
	maxArgonThreads  = 16
	maxScryptN       = 1 << 20
	maxScryptR       = 32
	maxScryptP       = 16
	twoFASIterations = 10000
)

// minExportPassphrase is the shortest passphrase accepted for an export.
const minExportPassphrase = 12

// exportEncrypted handles POST /api/v1/twofaccounts/export: the passphrase travels
// in the body, never in a URL that could end up in a log.
func (s *Server) exportEncrypted(c *ctx) {
	v := newValidator(c)
	passphrase := v.required("passphrase")
	if passphrase != "" && len([]rune(passphrase)) < minExportPassphrase {
		v.fail("passphrase", "The passphrase must be at least 12 characters.")
	}
	if v.respond() {
		return
	}
	accounts, ok := s.accountsToExport(c, v.optional("ids"))
	if !ok {
		return
	}
	plain, err := json.Marshal(s.exportDocument(accounts))
	if err != nil {
		s.srvError(c, err)
		return
	}
	sealed, err := sealExport(plain, passphrase)
	if err != nil {
		s.srvError(c, err)
		return
	}
	sealed["app"] = "2fauth_v" + s.cfg.Version
	sealed["datetime"] = time.Now().UTC().Format("2006-01-02T15:04:05.000000Z")
	jsonResponse(c.w, http.StatusOK, sealed)
}

// sealExport encrypts an export with a key derived from the passphrase.
func sealExport(plain []byte, passphrase string) (map[string]any, error) {
	salt := make([]byte, 16)
	nonce := make([]byte, 12)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	release := crypt.AcquireKDF()
	key := argon2.IDKey([]byte(passphrase), salt, exportArgonTime, exportArgonMemory, exportArgonThread, 32)
	release()
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"format":    encryptedExportFormat,
		"encrypted": true,
		"kdf": map[string]any{
			"name":        "argon2id",
			"salt":        base64.StdEncoding.EncodeToString(salt),
			"time":        exportArgonTime,
			"memory_kib":  exportArgonMemory,
			"parallelism": exportArgonThread,
		},
		"cipher": map[string]any{
			"name":  "aes-256-gcm",
			"nonce": base64.StdEncoding.EncodeToString(nonce),
		},
		"ciphertext": base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, plain, []byte(encryptedExportFormat))),
	}, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// decryptExport opens an encrypted 2FAuth export.
func decryptExport(doc map[string]any, passphrase string) ([]byte, error) {
	if passphrase == "" {
		return nil, errPassphraseRequired
	}
	kdf, _ := doc["kdf"].(map[string]any)
	cipherInfo, _ := doc["cipher"].(map[string]any)
	if str(kdf["name"]) != "argon2id" || str(cipherInfo["name"]) != "aes-256-gcm" {
		return nil, errInvalidPayload
	}
	salt, err1 := base64.StdEncoding.DecodeString(str(kdf["salt"]))
	nonce, err2 := base64.StdEncoding.DecodeString(str(cipherInfo["nonce"]))
	sealed, err3 := base64.StdEncoding.DecodeString(str(doc["ciphertext"]))
	if err := errors.Join(err1, err2, err3); err != nil {
		return nil, errInvalidPayload
	}
	time, memory, threads := numOr(kdf["time"], 0), numOr(kdf["memory_kib"], 0), numOr(kdf["parallelism"], 0)
	if time < 1 || time > maxArgonTime || memory < 8 || memory > maxArgonMemory || threads < 1 || threads > maxArgonThreads {
		return nil, errInvalidPayload
	}
	release := crypt.AcquireKDF()
	key := argon2.IDKey([]byte(passphrase), salt, uint32(time), uint32(memory), uint8(threads), 32)
	release()
	gcm, err := newGCM(key)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return nil, errInvalidPayload
	}
	plain, err := gcm.Open(nil, nonce, sealed, []byte(encryptedExportFormat))
	if err != nil {
		return nil, errPassphraseWrong
	}
	return plain, nil
}

// decryptAegis opens an encrypted Aegis vault: the passphrase derives (scrypt) the key
// of a password slot, which holds the master key of the database (both AES-256-GCM).
func decryptAegis(doc map[string]any, db, passphrase string) ([]byte, error) {
	if passphrase == "" {
		return nil, errPassphraseRequired
	}
	header, _ := doc["header"].(map[string]any)
	params, _ := header["params"].(map[string]any)
	slots, _ := header["slots"].([]any)
	var master []byte
	found := false
	for _, item := range slots {
		slot, _ := item.(map[string]any)
		if numOr(slot["type"], 0) != 1 {
			continue // biometric slots cannot be opened with a passphrase
		}
		found = true
		n, r, p := numOr(slot["n"], 0), numOr(slot["r"], 0), numOr(slot["p"], 0)
		if n < 2 || n > maxScryptN || n&(n-1) != 0 || r < 1 || r > maxScryptR || p < 1 || p > maxScryptP {
			return nil, errInvalidPayload
		}
		salt, err := hex.DecodeString(str(slot["salt"]))
		if err != nil {
			return nil, errInvalidPayload
		}
		release := crypt.AcquireKDF()
		key, err := scrypt.Key([]byte(passphrase), salt, n, r, p, 32)
		release()
		if err != nil {
			return nil, errInvalidPayload
		}
		keyParams, _ := slot["key_params"].(map[string]any)
		if opened, err := openAegisGCM(key, str(slot["key"]), keyParams); err == nil {
			master = opened
			break
		}
	}
	if !found {
		return nil, errInvalidPayload
	}
	if master == nil {
		return nil, errPassphraseWrong
	}
	raw, err := base64.StdEncoding.DecodeString(db)
	if err != nil {
		return nil, errInvalidPayload
	}
	plain, err := openAegisGCM(master, hex.EncodeToString(raw), params)
	if err != nil {
		return nil, errPassphraseWrong
	}
	return plain, nil
}

func openAegisGCM(key []byte, hexCipher string, params map[string]any) ([]byte, error) {
	nonce, err1 := hex.DecodeString(str(params["nonce"]))
	tag, err2 := hex.DecodeString(str(params["tag"]))
	data, err3 := hex.DecodeString(hexCipher)
	if err := errors.Join(err1, err2, err3); err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errInvalidPayload
	}
	return gcm.Open(nil, nonce, append(data, tag...), nil)
}

// decrypt2FAS opens the "servicesEncrypted" field of a 2FAS backup, three base64
// parts separated by a colon: ciphertext with tag, salt and nonce.
func decrypt2FAS(field, passphrase string) ([]byte, error) {
	if passphrase == "" {
		return nil, errPassphraseRequired
	}
	parts := strings.Split(field, ":")
	if len(parts) != 3 {
		return nil, errInvalidPayload
	}
	sealed, err1 := base64.StdEncoding.DecodeString(parts[0])
	salt, err2 := base64.StdEncoding.DecodeString(parts[1])
	nonce, err3 := base64.StdEncoding.DecodeString(parts[2])
	if err := errors.Join(err1, err2, err3); err != nil {
		return nil, errInvalidPayload
	}
	release := crypt.AcquireKDF()
	key, err := pbkdf2.Key(sha256.New, passphrase, salt, twoFASIterations, 32)
	release()
	if err != nil {
		return nil, errInvalidPayload
	}
	gcm, err := newGCM(key)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return nil, errInvalidPayload
	}
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, errPassphraseWrong
	}
	return plain, nil
}
