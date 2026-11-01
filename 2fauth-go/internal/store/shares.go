package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"2fauth/internal/db"
)

// Share scopes.
const (
	ShareScopeUser     = "user"
	ShareScopeAllUsers = "all_users"
)

// Share mirrors a twofaccount_shares row.
type Share struct {
	ID               int64
	TwoFAccountID    int64
	SharedWithUserID *int64
	Scope            string
	CreatedByUserID  int64
	CreatedAt        *time.Time
}

// ShareWithUserIDs returns the ids of the users an account is explicitly shared with.
func (s *Store) ShareWithUserIDs(accountID int64) ([]int64, error) {
	rows, err := s.DB.Query(`SELECT shared_with_user_id FROM twofaccount_shares
		WHERE twofaccount_id = ? AND scope = ? AND shared_with_user_id IS NOT NULL ORDER BY shared_with_user_id ASC`,
		accountID, ShareScopeUser)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ShareCreationTimes returns the creation date of explicit shares.
func (s *Store) ShareCreationTimes(accountID int64) map[int64]*time.Time {
	out := map[int64]*time.Time{}
	rows, err := s.DB.Query(`SELECT shared_with_user_id, created_at FROM twofaccount_shares
		WHERE twofaccount_id = ? AND scope = ? AND shared_with_user_id IS NOT NULL`, accountID, ShareScopeUser)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id      int64
			created db.NullString
		)
		if err := rows.Scan(&id, &created); err != nil {
			continue
		}
		if t, err := db.ParseTime(created.String); err == nil {
			out[id] = t
		}
	}
	return out
}

// IsSharedWithAll reports whether the account is shared with every user.
func (s *Store) IsSharedWithAll(accountID int64) bool {
	var n int64
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM twofaccount_shares WHERE twofaccount_id = ? AND scope = ?`,
		accountID, ShareScopeAllUsers).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// IsSharedWith reports whether the account is explicitly shared with the user.
func (s *Store) IsSharedWith(accountID, userID int64) bool {
	var n int64
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM twofaccount_shares
		WHERE twofaccount_id = ? AND scope = ? AND shared_with_user_id = ?`,
		accountID, ShareScopeUser, userID).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// AddShare creates an explicit share, returning true when it was created.
func (s *Store) AddShare(accountID, targetUserID, createdBy int64) (bool, error) {
	if s.IsSharedWith(accountID, targetUserID) {
		return false, nil
	}
	now := db.NowValue(s.DB.Dialect)
	_, err := s.DB.Exec(`INSERT INTO twofaccount_shares
		(twofaccount_id, shared_with_user_id, scope, created_by_user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, accountID, targetUserID, ShareScopeUser, createdBy, now, now)
	return err == nil, err
}

// AddShareWithAll shares the account with every user.
func (s *Store) AddShareWithAll(accountID, createdBy int64) error {
	if s.IsSharedWithAll(accountID) {
		return nil
	}
	now := db.NowValue(s.DB.Dialect)
	_, err := s.DB.Exec(`INSERT INTO twofaccount_shares
		(twofaccount_id, shared_with_user_id, scope, created_by_user_id, created_at, updated_at)
		VALUES (?, NULL, ?, ?, ?, ?)`, accountID, ShareScopeAllUsers, createdBy, now, now)
	return err
}

// RemoveShare deletes an explicit share.
func (s *Store) RemoveShare(accountID, targetUserID int64) error {
	_, err := s.DB.Exec(`DELETE FROM twofaccount_shares WHERE twofaccount_id = ? AND scope = ? AND shared_with_user_id = ?`,
		accountID, ShareScopeUser, targetUserID)
	return err
}

// RemoveAllShares deletes every explicit share of an account.
func (s *Store) RemoveAllShares(accountID int64) error {
	_, err := s.DB.Exec(`DELETE FROM twofaccount_shares WHERE twofaccount_id = ? AND scope = ?`,
		accountID, ShareScopeUser)
	return err
}

// RemoveShareWithAll deletes the all-users share of an account.
func (s *Store) RemoveShareWithAll(accountID int64) error {
	_, err := s.DB.Exec(`DELETE FROM twofaccount_shares WHERE twofaccount_id = ? AND scope = ?`,
		accountID, ShareScopeAllUsers)
	return err
}

// SearchRecipients returns users whose name contains (or email equals) the filter.
func (s *Store) SearchRecipients(excludeUserID int64, filter string) ([]*User, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		users, err := s.ListUsers()
		if err != nil {
			return nil, err
		}
		out := users[:0]
		for _, u := range users {
			if u.ID != excludeUserID {
				out = append(out, u)
			}
		}
		return out, nil
	}
	rows, err := s.DB.Query(`SELECT `+userColumns+` FROM users WHERE id != ? AND (LOWER(name) LIKE ? OR LOWER(email) = ?) ORDER BY name ASC`,
		excludeUserID, "%"+strings.ToLower(filter)+"%", strings.ToLower(filter))
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

// PruneInaccessibleOrders removes order rows of users who can no longer see the account.
func (s *Store) PruneInaccessibleOrders(accountID int64, ownerID int64, sharedUserIDs []int64) error {
	keep := map[int64]bool{ownerID: true}
	for _, id := range sharedUserIDs {
		keep[id] = true
	}
	rows, err := s.DB.Query(`SELECT DISTINCT user_id FROM twofaccount_user_orders WHERE twofaccount_id = ?`, accountID)
	if err != nil {
		return err
	}
	var stale []int64
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err == nil && !keep[uid] {
			stale = append(stale, uid)
		}
	}
	rows.Close()
	for _, uid := range stale {
		if _, err := s.DB.Exec(`DELETE FROM twofaccount_user_orders WHERE user_id = ? AND twofaccount_id = ?`, uid, accountID); err != nil {
			return err
		}
	}
	return nil
}

// getUserOrNil is a small helper for optional lookups.
func (s *Store) getUserOrNil(id *int64) *User {
	if id == nil {
		return nil
	}
	u, err := s.GetUser(*id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return nil
	}
	return u
}
