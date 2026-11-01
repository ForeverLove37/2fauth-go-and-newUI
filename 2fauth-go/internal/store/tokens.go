package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"2fauth/internal/crypt"
	"2fauth/internal/db"
)

// Token mirrors an oauth_access_tokens row (Passport compatible).
type Token struct {
	ID        string
	UserID    int64
	ClientID  int64
	Name      string
	Scopes    []string
	Revoked   bool
	CreatedAt *time.Time
	UpdatedAt *time.Time
	ExpiresAt *time.Time
}

// CreateToken issues a new personal access token and returns its plain value.
func (s *Store) CreateToken(userID int64, name string, scopes []string) (*Token, string, error) {
	plain := crypt.RandomString(40)
	// Passport identifies access tokens with a random string primary key.
	id := crypt.RandomString(80)
	now := db.NowValue(s.DB.Dialect)
	rawScopes, _ := json.Marshal(scopes)
	_, err := s.DB.Exec(`INSERT INTO oauth_access_tokens
		(id, user_id, client_id, name, scopes, revoked, token_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, userID, 1, name, string(rawScopes), false, crypt.SHA256Hex(plain), now, now)
	if err != nil {
		return nil, "", err
	}
	tok, err := s.GetToken(id, userID)
	if err != nil {
		return nil, "", err
	}
	return tok, plain, nil
}

func (s *Store) scanToken(scan func(...any) error) (*Token, error) {
	var (
		t       Token
		userID  db.NullInt
		client  db.NullInt
		name    db.NullString
		scopes  db.NullString
		revoked any
		created db.NullString
		updated db.NullString
		expires db.NullString
	)
	if err := scan(&t.ID, &userID, &client, &name, &scopes, &revoked, &created, &updated, &expires); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	t.UserID = userID.Int
	t.ClientID = client.Int
	t.Name = name.String
	t.Revoked = db.IntToBool(revoked)
	if scopes.Valid && scopes.String != "" {
		_ = json.Unmarshal([]byte(scopes.String), &t.Scopes)
	}
	if t.Scopes == nil {
		t.Scopes = []string{}
	}
	var tm *time.Time
	var err error
	if tm, err = db.ParseTime(created.String); err == nil {
		t.CreatedAt = tm
	}
	if tm, err = db.ParseTime(updated.String); err == nil {
		t.UpdatedAt = tm
	}
	if tm, err = db.ParseTime(expires.String); err == nil {
		t.ExpiresAt = tm
	}
	return &t, nil
}

const tokenColumns = `id, user_id, client_id, name, scopes, revoked, created_at, updated_at, expires_at`

// ListTokens returns the personal access tokens of a user.
func (s *Store) ListTokens(userID int64) ([]*Token, error) {
	rows, err := s.DB.Query(`SELECT `+tokenColumns+` FROM oauth_access_tokens WHERE user_id = ? AND revoked = ? ORDER BY id DESC`,
		userID, false)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Token
	for rows.Next() {
		tok, err := s.scanToken(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, tok)
	}
	return out, rows.Err()
}

// GetToken returns one token owned by the user.
func (s *Store) GetToken(id string, userID int64) (*Token, error) {
	row := s.DB.QueryRow(`SELECT `+tokenColumns+` FROM oauth_access_tokens WHERE id = ? AND user_id = ?`, id, userID)
	return s.scanToken(row.Scan)
}

// DeleteToken revokes a token.
func (s *Store) DeleteToken(id string, userID int64) bool {
	res, err := s.DB.Exec(`DELETE FROM oauth_access_tokens WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

// RevokeAllTokens revokes every token of a user.
func (s *Store) RevokeAllTokens(userID int64) error {
	_, err := s.DB.Exec(`DELETE FROM oauth_access_tokens WHERE user_id = ?`, userID)
	return err
}

// UserFromToken resolves a plain personal access token to its owner.
func (s *Store) UserFromToken(plain string) (*User, error) {
	var userID db.NullInt
	err := s.DB.QueryRow(`SELECT user_id FROM oauth_access_tokens WHERE token_hash = ? AND revoked = ?`,
		crypt.SHA256Hex(plain), false).Scan(&userID)
	if err != nil {
		return nil, ErrNotFound
	}
	return s.GetUser(userID.Int)
}
