package store

import (
	"database/sql"
	"errors"
	"strconv"
	"time"

	"2fauth/internal/db"
)

// WebAuthnCredential mirrors the Laravel webauthn_credentials table.
type WebAuthnCredential struct {
	ID        string
	UserID    int64
	Name      string
	Counter   int64
	CreatedAt time.Time
}

// ListWebAuthnCredentials returns the credentials of a user.
func (s *Store) ListWebAuthnCredentials(userID int64) ([]*WebAuthnCredential, error) {
	rows, err := s.DB.Query(
		s.DB.Rebind(`SELECT id, alias, counter, created_at FROM webauthn_credentials WHERE user_id = ? ORDER BY id`),
		strconv.FormatInt(userID, 10),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*WebAuthnCredential
	for rows.Next() {
		var (
			c       WebAuthnCredential
			id      db.NullString
			alias   db.NullString
			counter sql.NullInt64
			created db.NullString
		)
		if err := rows.Scan(&id, &alias, &counter, &created); err != nil {
			return nil, err
		}
		c.ID = id.String
		c.UserID = userID
		c.Name = alias.String
		if c.Name == "" {
			c.Name = "passkey"
		}
		c.Counter = counter.Int64
		if t, err := db.ParseTime(created.String); err == nil && t != nil {
			c.CreatedAt = *t
		} else {
			c.CreatedAt = time.Now()
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// CountWebAuthnCredentials counts the credentials of a user.
func (s *Store) CountWebAuthnCredentials(userID int64) (int, error) {
	var n int
	err := s.DB.QueryRow(
		s.DB.Rebind(`SELECT COUNT(*) FROM webauthn_credentials WHERE user_id = ?`),
		strconv.FormatInt(userID, 10),
	).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

// RenameWebAuthnCredential renames a credential owned by the user.
func (s *Store) RenameWebAuthnCredential(id string, userID int64, name string) bool {
	res, err := s.DB.Exec(
		s.DB.Rebind(`UPDATE webauthn_credentials SET alias = ? WHERE id = ? AND user_id = ?`),
		name, id, strconv.FormatInt(userID, 10),
	)
	if err != nil {
		return false
	}
	affected, _ := res.RowsAffected()
	return affected > 0
}

// DeleteWebAuthnCredential deletes a credential owned by the user.
func (s *Store) DeleteWebAuthnCredential(id string, userID int64) bool {
	res, err := s.DB.Exec(
		s.DB.Rebind(`DELETE FROM webauthn_credentials WHERE id = ? AND user_id = ?`),
		id, strconv.FormatInt(userID, 10),
	)
	if err != nil {
		return false
	}
	affected, _ := res.RowsAffected()
	return affected > 0
}

// DeleteWebAuthnCredentials deletes every credential of a user.
func (s *Store) DeleteWebAuthnCredentials(userID int64) error {
	_, err := s.DB.Exec(
		s.DB.Rebind(`DELETE FROM webauthn_credentials WHERE user_id = ?`),
		strconv.FormatInt(userID, 10),
	)
	return err
}

// HasWebAuthnCredentials reports whether the user registered a credential.
func (s *Store) HasWebAuthnCredentials(userID int64) bool {
	n, err := s.CountWebAuthnCredentials(userID)
	return err == nil && n > 0
}
