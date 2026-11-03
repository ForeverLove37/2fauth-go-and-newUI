package store

import (
	"database/sql"
	"errors"
	"time"

	"2fauth/internal/crypt"
	"2fauth/internal/db"
)

// UserSession is the server-side record of a signed-in browser. The cookie only
// names it, so signing out, changing a password or an administrator reset really
// end the session: replaying an old cookie finds nothing.
type UserSession struct {
	ID           string
	UserID       int64
	CreatedAt    int64
	LastActivity int64
}

// CreateSession opens a session for the user and returns its id.
func (s *Store) CreateSession(userID int64, ip, userAgent string) (string, error) {
	id := crypt.RandomString(48)
	now := time.Now().Unix()
	_, err := s.DB.Exec(`INSERT INTO user_sessions (id, user_id, ip_address, user_agent, created_at, last_activity)
		VALUES (?, ?, ?, ?, ?, ?)`, id, userID, ip, userAgent, now, now)
	if err != nil {
		return "", err
	}
	return id, nil
}

// GetSession returns a session by id.
func (s *Store) GetSession(id string) (*UserSession, error) {
	var out UserSession
	err := s.DB.QueryRow(`SELECT id, user_id, created_at, last_activity FROM user_sessions WHERE id = ?`, id).
		Scan(&out.ID, &out.UserID, &out.CreatedAt, &out.LastActivity)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// TouchSession records the activity of a session.
func (s *Store) TouchSession(id string, at int64) {
	_, _ = s.DB.Exec(`UPDATE user_sessions SET last_activity = ? WHERE id = ?`, at, id)
}

// DeleteSession ends one session.
func (s *Store) DeleteSession(id string) error {
	_, err := s.DB.Exec(`DELETE FROM user_sessions WHERE id = ?`, id)
	return err
}

// DeleteUserSessions ends every session of a user but the optional one to keep.
func (s *Store) DeleteUserSessions(userID int64, keep string) error {
	_, err := s.DB.Exec(`DELETE FROM user_sessions WHERE user_id = ? AND id != ?`, userID, keep)
	return err
}

// PurgeSessions removes sessions idle or older than the given limits.
func (s *Store) PurgeSessions(idleBefore, createdBefore int64) error {
	_, err := s.DB.Exec(`DELETE FROM user_sessions WHERE last_activity < ? OR created_at < ?`, idleBefore, createdBefore)
	return err
}

// ---------------------------------------------------------------------------
// password resets
// ---------------------------------------------------------------------------

// SavePasswordReset replaces the pending reset of an address.
func (s *Store) SavePasswordReset(email, tokenHash string) error {
	if err := s.DeletePasswordReset(email); err != nil {
		return err
	}
	_, err := s.DB.Exec(`INSERT INTO password_resets (email, token, created_at) VALUES (?, ?, ?)`,
		email, tokenHash, db.NowValue(s.DB.Dialect))
	return err
}

// FindPasswordReset returns the pending token hash of an address and its age.
func (s *Store) FindPasswordReset(email string) (string, time.Duration, error) {
	var token string
	var created db.NullString
	err := s.DB.QueryRow(`SELECT token, created_at FROM password_resets WHERE email = ?`, email).Scan(&token, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	if err != nil {
		return "", 0, err
	}
	t, err := db.ParseTime(created.String)
	if err != nil || t == nil {
		return "", 0, ErrNotFound
	}
	return token, time.Since(*t), nil
}

// DeletePasswordReset removes the pending reset of an address.
func (s *Store) DeletePasswordReset(email string) error {
	_, err := s.DB.Exec(`DELETE FROM password_resets WHERE email = ?`, email)
	return err
}

// ---------------------------------------------------------------------------
// icon ownership
// ---------------------------------------------------------------------------

// AddIconUpload records that a user uploaded (or fetched) an icon.
func (s *Store) AddIconUpload(name string, userID int64) error {
	if s.IconUploadedBy(name, userID) {
		return nil
	}
	_, err := s.DB.Exec(`INSERT INTO icon_uploads (name, user_id, created_at) VALUES (?, ?, ?)`, name, userID, time.Now().Unix())
	return err
}

// IconUploadedBy reports whether the user uploaded the icon.
func (s *Store) IconUploadedBy(name string, userID int64) bool {
	var n int64
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM icon_uploads WHERE name = ? AND user_id = ?`, name, userID).Scan(&n)
	return err == nil && n > 0
}

// CountIconUploads returns how many icons a user uploaded.
func (s *Store) CountIconUploads(userID int64) int64 {
	var n int64
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM icon_uploads WHERE user_id = ?`, userID).Scan(&n); err != nil {
		return 0
	}
	return n
}

// RemoveIconUpload forgets one user's claim on an icon.
func (s *Store) RemoveIconUpload(name string, userID int64) error {
	_, err := s.DB.Exec(`DELETE FROM icon_uploads WHERE name = ? AND user_id = ?`, name, userID)
	return err
}

// IconUsedByOthers reports whether another user still needs the icon, either as
// an uploader or through one of their accounts.
func (s *Store) IconUsedByOthers(name string, userID int64) bool {
	var uploads, accounts int64
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM icon_uploads WHERE name = ? AND user_id != ?`, name, userID).Scan(&uploads); err != nil {
		return true
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM twofaccounts WHERE icon = ? AND user_id != ?`, name, userID).Scan(&accounts); err != nil {
		return true
	}
	return uploads > 0 || accounts > 0
}

// IconUsedBy reports whether one of the user's accounts displays the icon.
func (s *Store) IconUsedBy(name string, userID int64) bool {
	var n int64
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM twofaccounts WHERE icon = ? AND user_id = ?`, name, userID).Scan(&n)
	return err == nil && n > 0
}
