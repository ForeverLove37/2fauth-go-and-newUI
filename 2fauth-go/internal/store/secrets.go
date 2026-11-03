package store

import (
	"log/slog"
)

// SecretCipher seals OTP secrets at rest. It is implemented by crypt.Encrypter and
// produces the same payloads as Laravel, so a database moved from PHP stays readable.
type SecretCipher interface {
	EncryptString(plain string) (string, error)
	DecryptString(encrypted string) (string, error)
	IsEncrypted(value string) bool
}

// SetCipher enables the encryption of OTP secrets.
func (s *Store) SetCipher(cipher SecretCipher) { s.cipher = cipher }

func (s *Store) sealSecret(plain string) (string, error) {
	if s.cipher == nil || plain == "" {
		return plain, nil
	}
	return s.cipher.EncryptString(plain)
}

// openSecret returns the usable secret of a stored value. A value that cannot be
// decrypted (another APP_KEY) reads as empty: the account then reports an invalid
// secret instead of showing a ciphertext to the user.
func (s *Store) openSecret(id int64, stored string) string {
	if s.cipher == nil || !s.cipher.IsEncrypted(stored) {
		return stored
	}
	plain, err := s.cipher.DecryptString(stored)
	if err != nil {
		slog.Warn("cannot decrypt an OTP secret: was APP_KEY changed?", "twofaccount_id", id)
		return ""
	}
	return plain
}

// EncryptExistingSecrets seals every secret still stored in clear text and returns
// how many rows it changed. It is safe to run at every start-up.
func (s *Store) EncryptExistingSecrets() (int, error) {
	if s.cipher == nil {
		return 0, nil
	}
	rows, err := s.DB.Query(`SELECT id, secret FROM twofaccounts`)
	if err != nil {
		return 0, err
	}
	type row struct {
		id     int64
		secret string
	}
	var pending []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.secret); err != nil {
			rows.Close()
			return 0, err
		}
		if r.secret != "" && !s.cipher.IsEncrypted(r.secret) {
			pending = append(pending, r)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, r := range pending {
		sealed, err := s.cipher.EncryptString(r.secret)
		if err != nil {
			return 0, err
		}
		if _, err := s.DB.Exec(`UPDATE twofaccounts SET secret = ? WHERE id = ?`, sealed, r.id); err != nil {
			return 0, err
		}
	}
	return len(pending), nil
}
