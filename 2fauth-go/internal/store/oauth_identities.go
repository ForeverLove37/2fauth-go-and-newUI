package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"2fauth/internal/db"
)

// OAuthIdentity links a user to an external identity provider account. The pair
// (provider, subject) is unique so a subject resolves to exactly one user.
type OAuthIdentity struct {
	ID        int64
	UserID    int64
	Provider  string
	Subject   string
	Email     string
	CreatedAt *time.Time
	UpdatedAt *time.Time
}

const oauthIdentityColumns = `id, user_id, provider, subject, email, created_at, updated_at`

func scanOAuthIdentity(scan func(...any) error) (*OAuthIdentity, error) {
	var (
		identity OAuthIdentity
		userID   db.NullInt
		email    db.NullString
		created  db.NullString
		updated  db.NullString
	)
	if err := scan(&identity.ID, &userID, &identity.Provider, &identity.Subject, &email, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	identity.UserID = userID.Int
	identity.Email = email.String
	if t, err := db.ParseTime(created.String); err == nil {
		identity.CreatedAt = t
	}
	if t, err := db.ParseTime(updated.String); err == nil {
		identity.UpdatedAt = t
	}
	return &identity, nil
}

// FindOAuthIdentity resolves a (provider, subject) pair to its identity.
func (s *Store) FindOAuthIdentity(provider, subject string) (*OAuthIdentity, error) {
	row := s.DB.QueryRow(`SELECT `+oauthIdentityColumns+` FROM oauth_identities WHERE provider = ? AND subject = ?`,
		strings.TrimSpace(provider), subject)
	return scanOAuthIdentity(row.Scan)
}

// FindOAuthIdentitiesByUser returns every identity linked to a user.
func (s *Store) FindOAuthIdentitiesByUser(userID int64) ([]*OAuthIdentity, error) {
	rows, err := s.DB.Query(`SELECT `+oauthIdentityColumns+` FROM oauth_identities WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*OAuthIdentity
	for rows.Next() {
		identity, err := scanOAuthIdentity(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, identity)
	}
	return out, rows.Err()
}

// CreateOAuthIdentity links an external account to a user.
func (s *Store) CreateOAuthIdentity(userID int64, provider, subject, email string) error {
	now := db.NowValue(s.DB.Dialect)
	_, err := s.DB.Exec(`INSERT INTO oauth_identities (user_id, provider, subject, email, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		userID, strings.TrimSpace(provider), subject, strings.ToLower(strings.TrimSpace(email)), now, now)
	return err
}

// DeleteOAuthIdentitiesByUser removes every identity of a user.
func (s *Store) DeleteOAuthIdentitiesByUser(userID int64) error {
	_, err := s.DB.Exec(`DELETE FROM oauth_identities WHERE user_id = ?`, userID)
	return err
}
