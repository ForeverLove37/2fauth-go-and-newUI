package store

import (
	"database/sql"
	"errors"
	"time"

	"2fauth/internal/db"
)

// SSHKey is a public key allowed to fetch OTP codes from the command line. What it
// reaches is the intersection of its scope and what its owner can see right now.
type SSHKey struct {
	ID          int64
	UserID      int64
	Name        string
	PublicKey   string
	Fingerprint string
	// AllAccounts opens every account of the owner; otherwise only the listed ones.
	AllAccounts bool
	AccountIDs  []int64
	GroupIDs    []int64
	LastUsedAt  *int64
	LastUsedIP  string
	CreatedAt   int64
}

const sshKeyColumns = `id, user_id, name, public_key, fingerprint, all_accounts, last_used_at, last_used_ip, created_at`

func (s *Store) scanSSHKey(scan func(...any) error) (*SSHKey, error) {
	var (
		key    SSHKey
		last   sql.NullInt64
		lastIP db.NullString
	)
	if err := scan(&key.ID, &key.UserID, &key.Name, &key.PublicKey, &key.Fingerprint, &key.AllAccounts, &last, &lastIP, &key.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if last.Valid {
		key.LastUsedAt = &last.Int64
	}
	key.LastUsedIP = lastIP.String
	var err error
	if key.AccountIDs, err = s.sshKeyScope(`SELECT twofaccount_id FROM ssh_key_accounts WHERE key_id = ?`, key.ID); err != nil {
		return nil, err
	}
	if key.GroupIDs, err = s.sshKeyScope(`SELECT group_id FROM ssh_key_groups WHERE key_id = ?`, key.ID); err != nil {
		return nil, err
	}
	return &key, nil
}

func (s *Store) sshKeyScope(query string, keyID int64) ([]int64, error) {
	rows, err := s.DB.Query(query, keyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CreateSSHKey stores a key and its scope.
func (s *Store) CreateSSHKey(key *SSHKey) (*SSHKey, error) {
	id, err := s.DB.InsertID(`INSERT INTO ssh_keys (user_id, name, public_key, fingerprint, all_accounts, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, key.UserID, key.Name, key.PublicKey, key.Fingerprint, false, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	if err := s.setSSHKeyScope(id, key.AllAccounts, key.AccountIDs, key.GroupIDs); err != nil {
		_, _ = s.DB.Exec(`DELETE FROM ssh_keys WHERE id = ?`, id)
		return nil, err
	}
	return s.GetSSHKey(id)
}

// setSSHKeyScope replaces a scope. The steps are ordered so that a failure half way
// leaves the key with less access, never more.
func (s *Store) setSSHKeyScope(id int64, all bool, accounts, groups []int64) error {
	if _, err := s.DB.Exec(`UPDATE ssh_keys SET all_accounts = ? WHERE id = ?`, false, id); err != nil {
		return err
	}
	if _, err := s.DB.Exec(`DELETE FROM ssh_key_accounts WHERE key_id = ?`, id); err != nil {
		return err
	}
	if _, err := s.DB.Exec(`DELETE FROM ssh_key_groups WHERE key_id = ?`, id); err != nil {
		return err
	}
	if all {
		_, err := s.DB.Exec(`UPDATE ssh_keys SET all_accounts = ? WHERE id = ?`, true, id)
		return err
	}
	for _, account := range accounts {
		if _, err := s.DB.Exec(`INSERT INTO ssh_key_accounts (key_id, twofaccount_id) VALUES (?, ?)`, id, account); err != nil {
			return err
		}
	}
	for _, group := range groups {
		if _, err := s.DB.Exec(`INSERT INTO ssh_key_groups (key_id, group_id) VALUES (?, ?)`, id, group); err != nil {
			return err
		}
	}
	return nil
}

// UpdateSSHKey renames a key and replaces its scope.
func (s *Store) UpdateSSHKey(key *SSHKey) error {
	if _, err := s.DB.Exec(`UPDATE ssh_keys SET name = ? WHERE id = ?`, key.Name, key.ID); err != nil {
		return err
	}
	return s.setSSHKeyScope(key.ID, key.AllAccounts, key.AccountIDs, key.GroupIDs)
}

// GetSSHKey returns a key by id.
func (s *Store) GetSSHKey(id int64) (*SSHKey, error) {
	return s.scanSSHKey(s.DB.QueryRow(`SELECT `+sshKeyColumns+` FROM ssh_keys WHERE id = ?`, id).Scan)
}

// FindSSHKeyByFingerprint returns the key with this fingerprint.
func (s *Store) FindSSHKeyByFingerprint(fingerprint string) (*SSHKey, error) {
	return s.scanSSHKey(s.DB.QueryRow(`SELECT `+sshKeyColumns+` FROM ssh_keys WHERE fingerprint = ?`, fingerprint).Scan)
}

// ListSSHKeys lists the keys of a user, oldest first.
func (s *Store) ListSSHKeys(userID int64) ([]*SSHKey, error) {
	rows, err := s.DB.Query(`SELECT id FROM ssh_keys WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]*SSHKey, 0, len(ids))
	for _, id := range ids {
		key, err := s.GetSSHKey(id)
		if err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, nil
}

// DeleteSSHKey removes a key of a user and reports whether there was one.
func (s *Store) DeleteSSHKey(id, userID int64) (bool, error) {
	if _, err := s.DB.Exec(`DELETE FROM ssh_key_accounts WHERE key_id IN (SELECT id FROM ssh_keys WHERE id = ? AND user_id = ?)`, id, userID); err != nil {
		return false, err
	}
	if _, err := s.DB.Exec(`DELETE FROM ssh_key_groups WHERE key_id IN (SELECT id FROM ssh_keys WHERE id = ? AND user_id = ?)`, id, userID); err != nil {
		return false, err
	}
	res, err := s.DB.Exec(`DELETE FROM ssh_keys WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// DeleteUserSSHKeys removes every key of a user.
func (s *Store) DeleteUserSSHKeys(userID int64) error {
	keys, err := s.ListSSHKeys(userID)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if _, err := s.DeleteSSHKey(key.ID, userID); err != nil {
			return err
		}
	}
	return nil
}

// TouchSSHKey records the last use of a key.
func (s *Store) TouchSSHKey(id int64, ip string) error {
	_, err := s.DB.Exec(`UPDATE ssh_keys SET last_used_at = ?, last_used_ip = ? WHERE id = ?`, time.Now().Unix(), ip, id)
	return err
}
