// Package store contains the data access layer mirroring the Eloquent models of
// 2FAuth v9.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"2fauth/internal/config"
	"2fauth/internal/db"
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("not found")

// Store is the data access entry point.
type Store struct {
	DB  *db.DB
	Cfg *config.Config
}

// New builds a Store.
func New(d *db.DB, cfg *config.Config) *Store {
	return &Store{DB: d, Cfg: cfg}
}

// User mirrors the users table.
type User struct {
	ID              int64
	Name            string
	Email           string
	EmailVerifiedAt *time.Time
	Password        string
	RememberToken   string
	CreatedAt       *time.Time
	UpdatedAt       *time.Time
	LastSeenAt      *time.Time
	IsAdmin         bool
	Preferences     map[string]any
	OauthProvider   string
	OauthID         string
}

const userColumns = `id, name, email, email_verified_at, password, remember_token, created_at, updated_at, last_seen_at, is_admin, preferences, oauth_provider, oauth_id`

func (s *Store) scanUser(scan func(...any) error) (*User, error) {
	var (
		u         User
		verified  db.NullString
		remember  db.NullString
		createdAt db.NullString
		updatedAt db.NullString
		lastSeen  db.NullString
		isAdmin   any
		prefsRaw  db.NullString
		oauthProv db.NullString
		oauthID   db.NullString
	)
	err := scan(&u.ID, &u.Name, &u.Email, &verified, &u.Password, &remember,
		&createdAt, &updatedAt, &lastSeen, &isAdmin, &prefsRaw, &oauthProv, &oauthID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.IsAdmin = db.IntToBool(isAdmin)
	u.RememberToken = remember.String
	u.OauthProvider = oauthProv.String
	u.OauthID = oauthID.String
	var t *time.Time
	if t, err = db.ParseTime(verified.String); err == nil {
		u.EmailVerifiedAt = t
	}
	if t, err = db.ParseTime(createdAt.String); err == nil {
		u.CreatedAt = t
	}
	if t, err = db.ParseTime(updatedAt.String); err == nil {
		u.UpdatedAt = t
	}
	if t, err = db.ParseTime(lastSeen.String); err == nil {
		u.LastSeenAt = t
	}
	u.Preferences = map[string]any{}
	if prefsRaw.Valid && strings.TrimSpace(prefsRaw.String) != "" {
		_ = json.Unmarshal([]byte(prefsRaw.String), &u.Preferences)
	}
	return &u, nil
}

// PreferencesWithDefaults returns the config defaults merged with the user preferences.
func (s *Store) PreferencesWithDefaults(u *User) map[string]any {
	out := map[string]any{}
	for k, v := range s.Cfg.DefaultPreferences {
		out[k] = v
	}
	for k, v := range u.Preferences {
		out[k] = v
	}
	return out
}

// GetUserByEmail looks a user up by email (case insensitive).
func (s *Store) GetUserByEmail(email string) (*User, error) {
	row := s.DB.QueryRow(`SELECT `+userColumns+` FROM users WHERE LOWER(email) = ?`, strings.ToLower(strings.TrimSpace(email)))
	return s.scanUser(row.Scan)
}

// GetUserByName looks a user up by nickname.
func (s *Store) GetUserByName(name string) (*User, error) {
	row := s.DB.QueryRow(`SELECT `+userColumns+` FROM users WHERE name = ?`, name)
	return s.scanUser(row.Scan)
}

// GetUser returns a user by id.
func (s *Store) GetUser(id int64) (*User, error) {
	row := s.DB.QueryRow(`SELECT `+userColumns+` FROM users WHERE id = ?`, id)
	return s.scanUser(row.Scan)
}

// ListUsers returns every user ordered by name.
func (s *Store) ListUsers() ([]*User, error) {
	rows, err := s.DB.Query(`SELECT ` + userColumns + ` FROM users ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u, err := s.scanUser(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CountUsers returns the number of registered users.
func (s *Store) CountUsers() (int64, error) {
	var n int64
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CountAdmins returns the number of administrators.
func (s *Store) CountAdmins() (int64, error) {
	var n int64
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin = ?`, true).Scan(&n)
	return n, err
}

// CreateUser inserts a user. The first registered user becomes an administrator.
func (s *Store) CreateUser(name, email, passwordHash string) (*User, error) {
	now := db.NowValue(s.DB.Dialect)
	count, err := s.CountUsers()
	if err != nil {
		return nil, err
	}
	isAdmin := count == 0
	res, err := s.DB.Exec(`INSERT INTO users (name, email, password, created_at, updated_at, is_admin, preferences)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		name, strings.ToLower(email), passwordHash, now, now, isAdmin, "{}")
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetUser(id)
}

// UpdateUsernameEmail persists a name/email change.
func (s *Store) UpdateUsernameEmail(id int64, name, email string) error {
	_, err := s.DB.Exec(`UPDATE users SET name = ?, email = ?, updated_at = ? WHERE id = ?`,
		name, strings.ToLower(email), db.NowValue(s.DB.Dialect), id)
	return err
}

// UpdatePassword persists a new password hash.
func (s *Store) UpdatePassword(id int64, hash string) error {
	_, err := s.DB.Exec(`UPDATE users SET password = ?, updated_at = ? WHERE id = ?`,
		hash, db.NowValue(s.DB.Dialect), id)
	return err
}

// SetAdmin toggles the administrator flag.
func (s *Store) SetAdmin(id int64, admin bool) error {
	_, err := s.DB.Exec(`UPDATE users SET is_admin = ?, updated_at = ? WHERE id = ?`,
		admin, db.NowValue(s.DB.Dialect), id)
	return err
}

// SavePreferences persists the whole preferences document.
func (s *Store) SavePreferences(id int64, prefs map[string]any) error {
	raw, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`UPDATE users SET preferences = ?, updated_at = ? WHERE id = ?`,
		string(raw), db.NowValue(s.DB.Dialect), id)
	return err
}

// TouchLastSeen updates the last_seen_at column.
func (s *Store) TouchLastSeen(id int64) {
	_, _ = s.DB.Exec(`UPDATE users SET last_seen_at = ? WHERE id = ?`, db.NowValue(s.DB.Dialect), id)
}

// DeleteUser removes a user and every owned record.
func (s *Store) DeleteUser(id int64) error {
	// Do not rely on ON DELETE CASCADE: SQLite only enforces foreign keys when
	// the connection enables the pragma.
	if err := s.DeleteOAuthIdentitiesByUser(id); err != nil {
		return err
	}
	_, err := s.DB.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

// FindUsersByIDs returns users matching the given ids.
func (s *Store) FindUsersByIDs(ids []int64) ([]*User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, v := range ids {
		args[i] = v
	}
	rows, err := s.DB.Query(`SELECT `+userColumns+` FROM users WHERE id IN (`+placeholders+`) ORDER BY name ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u, err := s.scanUser(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// options
// ---------------------------------------------------------------------------

// AllOptions returns the raw option rows.
func (s *Store) AllOptions() (map[string]string, error) {
	rows, err := s.DB.Query(`SELECT key, value FROM options`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SetOption upserts an option.
func (s *Store) SetOption(key, value string) error {
	if _, err := s.DB.Exec(`UPDATE options SET value = ? WHERE key = ?`, value, key); err == nil {
		var n int64
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM options WHERE key = ?`, key).Scan(&n); err == nil && n > 0 {
			return nil
		}
	}
	_, err := s.DB.Exec(`INSERT INTO options (key, value) VALUES (?, ?)`, key, value)
	if err != nil {
		// fall back to update for drivers returning an error on duplicate keys
		_, err = s.DB.Exec(`UPDATE options SET value = ? WHERE key = ?`, value, key)
	}
	return err
}

// DeleteOption removes an option.
func (s *Store) DeleteOption(key string) error {
	_, err := s.DB.Exec(`DELETE FROM options WHERE key = ?`, key)
	return err
}

// OptionExists reports whether an option row exists.
func (s *Store) OptionExists(key string) bool {
	var n int64
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM options WHERE key = ?`, key).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// ---------------------------------------------------------------------------
// groups
// ---------------------------------------------------------------------------

// Group mirrors the groups table.
type Group struct {
	ID            int64
	UserID        int64
	Name          string
	OrderColumn   *int64
	ShowInChips   bool
	CreatedAt     *time.Time
	UpdatedAt     *time.Time
	AccountsCount int64
}

const groupColumns = `id, user_id, name, order_column, show_in_chips, created_at, updated_at`

func scanGroup(scan func(...any) error) (*Group, error) {
	var (
		g       Group
		userID  db.NullInt
		order   db.NullInt
		chips   any
		created db.NullString
		updated db.NullString
	)
	if err := scan(&g.ID, &userID, &g.Name, &order, &chips, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	g.UserID = userID.Int
	g.ShowInChips = true
	if chips != nil {
		g.ShowInChips = db.IntToBool(chips)
	}
	if order.Valid {
		v := order.Int
		g.OrderColumn = &v
	}
	var t *time.Time
	var err error
	if t, err = db.ParseTime(created.String); err == nil {
		g.CreatedAt = t
	}
	if t, err = db.ParseTime(updated.String); err == nil {
		g.UpdatedAt = t
	}
	return &g, nil
}

// ListGroups returns the groups of a user with their account counts.
func (s *Store) ListGroups(userID int64) ([]*Group, error) {
	rows, err := s.DB.Query(`SELECT g.id, g.user_id, g.name, g.order_column, g.show_in_chips, g.created_at, g.updated_at,
			(SELECT COUNT(*) FROM twofaccounts t WHERE t.group_id = g.id AND t.user_id = g.user_id)
		FROM groups g WHERE g.user_id = ? ORDER BY g.order_column ASC, g.id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Group
	for rows.Next() {
		var (
			g       Group
			uid     db.NullInt
			order   db.NullInt
			chips   any
			created db.NullString
			updated db.NullString
			count   int64
		)
		if err := rows.Scan(&g.ID, &uid, &g.Name, &order, &chips, &created, &updated, &count); err != nil {
			return nil, err
		}
		g.UserID = uid.Int
		g.ShowInChips = chips == nil || db.IntToBool(chips)
		if order.Valid {
			v := order.Int
			g.OrderColumn = &v
		}
		var t *time.Time
		if t, err = db.ParseTime(created.String); err == nil {
			g.CreatedAt = t
		}
		if t, err = db.ParseTime(updated.String); err == nil {
			g.UpdatedAt = t
		}
		g.AccountsCount = count
		out = append(out, &g)
	}
	return out, rows.Err()
}

// GetGroup returns one group owned by the user.
func (s *Store) GetGroup(id, userID int64) (*Group, error) {
	row := s.DB.QueryRow(`SELECT `+groupColumns+` FROM groups WHERE id = ? AND user_id = ?`, id, userID)
	g, err := scanGroup(row.Scan)
	if err != nil {
		return nil, err
	}
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM twofaccounts WHERE group_id = ? AND user_id = ?`, id, userID).
		Scan(&g.AccountsCount); err != nil {
		return nil, err
	}
	return g, nil
}

// CreateGroup inserts a group at the end of the user's ordering.
func (s *Store) CreateGroup(userID int64, name string, showInChips bool) (*Group, error) {
	var maxOrder db.NullInt
	if err := s.DB.QueryRow(`SELECT MAX(order_column) FROM groups WHERE user_id = ?`, userID).Scan(&maxOrder); err != nil {
		return nil, err
	}
	next := maxOrder.Int + 1
	now := db.NowValue(s.DB.Dialect)
	res, err := s.DB.Exec(`INSERT INTO groups (user_id, name, order_column, show_in_chips, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, name, next, showInChips, now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetGroup(id, userID)
}

// UpdateGroup persists a group rename.
func (s *Store) UpdateGroup(id, userID int64, name string, showInChips *bool) error {
	if showInChips != nil {
		_, err := s.DB.Exec(`UPDATE groups SET name = ?, show_in_chips = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
			name, *showInChips, db.NowValue(s.DB.Dialect), id, userID)
		return err
	}
	_, err := s.DB.Exec(`UPDATE groups SET name = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		name, db.NowValue(s.DB.Dialect), id, userID)
	return err
}

// DeleteGroup removes a group; accounts fall back to orphans.
func (s *Store) DeleteGroup(id, userID int64) error {
	if _, err := s.DB.Exec(`UPDATE twofaccounts SET group_id = NULL WHERE group_id = ? AND user_id = ?`, id, userID); err != nil {
		return err
	}
	_, err := s.DB.Exec(`DELETE FROM groups WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

// SaveGroupOrder persists the ordering of a user's groups.
func (s *Store) SaveGroupOrder(userID int64, orderedIDs []int64) error {
	for i, id := range orderedIDs {
		if _, err := s.DB.Exec(`UPDATE groups SET order_column = ? WHERE id = ? AND user_id = ?`, i+1, id, userID); err != nil {
			return err
		}
	}
	return nil
}
