package store

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"2fauth/internal/db"
)

// Account mirrors the twofaccounts table.
type Account struct {
	ID        int64
	UserID    int64
	GroupID   *int64
	Service   string
	LegacyURI string
	Account   string
	Icon      string
	OTPType   string
	Secret    string
	Algorithm string
	Digits    int
	Period    *int
	Counter   *int64
	CreatedAt *time.Time
	UpdatedAt *time.Time

	// ImportIcon carries a base64 icon read from an import file; it is not a column.
	ImportIcon string
}

// HOTP reports whether the account uses counter based OTP.
func (a *Account) HOTP() bool { return a.OTPType == "hotp" }

const accountColumns = `id, user_id, group_id, service, legacy_uri, account, icon, otp_type, secret, algorithm, digits, period, counter, created_at, updated_at`

func (s *Store) scanAccount(scan func(...any) error) (*Account, error) {
	var (
		a       Account
		userID  db.NullInt
		groupID db.NullInt
		service db.NullString
		legacy  db.NullString
		account db.NullString
		icon    db.NullString
		otpType db.NullString
		secret  db.NullString
		alg     db.NullString
		digits  db.NullInt
		period  db.NullInt
		counter db.NullInt
		created db.NullString
		updated db.NullString
	)
	if err := scan(&a.ID, &userID, &groupID, &service, &legacy, &account, &icon, &otpType,
		&secret, &alg, &digits, &period, &counter, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	a.UserID = userID.Int
	if groupID.Valid {
		v := groupID.Int
		a.GroupID = &v
	}
	a.Service = service.String
	a.LegacyURI = legacy.String
	a.Account = account.String
	a.Icon = icon.String
	a.OTPType = otpType.String
	a.Secret = s.openSecret(a.ID, secret.String)
	a.Algorithm = alg.String
	a.Digits = int(digits.Int)
	if period.Valid {
		v := int(period.Int)
		a.Period = &v
	}
	if counter.Valid {
		v := counter.Int
		a.Counter = &v
	}
	var t *time.Time
	var err error
	if t, err = db.ParseTime(created.String); err == nil {
		a.CreatedAt = t
	}
	if t, err = db.ParseTime(updated.String); err == nil {
		a.UpdatedAt = t
	}
	return &a, nil
}

// GetAccount returns an account by id, regardless of ownership.
func (s *Store) GetAccount(id int64) (*Account, error) {
	row := s.DB.QueryRow(`SELECT `+accountColumns+` FROM twofaccounts WHERE id = ?`, id)
	return s.scanAccount(row.Scan)
}

// VisibleScope describes the sharing rules applied when listing accounts.
type VisibleScope struct {
	EnableSharing        bool
	AllUsersSharingScope bool
}

// ListVisibleAccounts lists the accounts a user may see, sorted by their custom order.
func (s *Store) ListVisibleAccounts(userID int64, scope VisibleScope, ids []int64) ([]*Account, error) {
	where := `WHERE t.user_id = ?`
	args := []any{userID}

	if scope.EnableSharing {
		sharedCond := `EXISTS (SELECT 1 FROM twofaccount_shares sh WHERE sh.twofaccount_id = t.id AND sh.scope = 'user' AND sh.shared_with_user_id = ?)`
		args = append(args, userID)
		if scope.AllUsersSharingScope {
			sharedCond += ` OR EXISTS (SELECT 1 FROM twofaccount_shares sh2 WHERE sh2.twofaccount_id = t.id AND sh2.scope = 'all_users')`
		}
		where = `WHERE (t.user_id = ? OR ` + sharedCond + `)`
	}

	if len(ids) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		where += ` AND t.id IN (` + placeholders + `)`
		for _, id := range ids {
			args = append(args, id)
		}
	}

	rows, err := s.DB.Query(`SELECT `+prefixColumns("t", accountColumns)+` FROM twofaccounts t `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Account
	for rows.Next() {
		a, err := s.scanAccount(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.sortForUser(out, userID)
}

func prefixColumns(alias, columns string) string {
	parts := strings.Split(columns, ",")
	for i, p := range parts {
		parts[i] = alias + "." + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

// sortForUser applies the stored per-user ordering (position, then id).
func (s *Store) sortForUser(accounts []*Account, userID int64) ([]*Account, error) {
	if len(accounts) == 0 {
		return accounts, nil
	}
	positions := map[int64]int64{}
	rows, err := s.DB.Query(`SELECT twofaccount_id, position FROM twofaccount_user_orders WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, pos int64
		if err := rows.Scan(&id, &pos); err != nil {
			rows.Close()
			return nil, err
		}
		positions[id] = pos
	}
	rows.Close()

	key := func(a *Account) int64 {
		if p, ok := positions[a.ID]; ok {
			return p
		}
		return 1_000_000_000 + a.ID
	}
	sort.SliceStable(accounts, func(i, j int) bool { return key(accounts[i]) < key(accounts[j]) })
	return accounts, nil
}

// OrderedVisibleIDs returns the visible account ids in display order.
func (s *Store) OrderedVisibleIDs(userID int64, scope VisibleScope) ([]int64, error) {
	accounts, err := s.ListVisibleAccounts(userID, scope, nil)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(accounts))
	for _, a := range accounts {
		ids = append(ids, a.ID)
	}
	return ids, nil
}

// AccountGroupID returns the group the account is assigned to for that user.
func (s *Store) AccountGroupID(a *Account, userID int64) (int64, error) {
	if a.GroupID == nil {
		return 0, nil
	}
	var gid int64
	err := s.DB.QueryRow(`SELECT id FROM "groups" WHERE id = ? AND user_id = ?`, *a.GroupID, userID).Scan(&gid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return gid, nil
}

// CreateAccount inserts a new account.
func (s *Store) CreateAccount(a *Account) (*Account, error) {
	now := db.NowValue(s.DB.Dialect)
	sealed, err := s.sealSecret(a.Secret)
	if err != nil {
		return nil, err
	}
	id, err := s.DB.InsertID(`INSERT INTO twofaccounts
		(user_id, group_id, service, legacy_uri, account, icon, otp_type, secret, algorithm, digits, period, counter, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.UserID, nullableInt(a.GroupID), a.Service, a.LegacyURI, a.Account, a.Icon, a.OTPType, sealed,
		a.Algorithm, a.Digits, nullableInt(toInt64Ptr(a.Period)), nullableBigInt(a.Counter), now, now)
	if err != nil {
		return nil, err
	}
	return s.GetAccount(id)
}

// UpdateAccount persists every mutable field of an account.
func (s *Store) UpdateAccount(a *Account) error {
	sealed, err := s.sealSecret(a.Secret)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`UPDATE twofaccounts SET group_id = ?, service = ?, legacy_uri = ?, account = ?, icon = ?,
		otp_type = ?, secret = ?, algorithm = ?, digits = ?, period = ?, counter = ?, updated_at = ? WHERE id = ?`,
		nullableInt(a.GroupID), a.Service, a.LegacyURI, a.Account, a.Icon, a.OTPType, sealed, a.Algorithm,
		a.Digits, nullableInt(toInt64Ptr(a.Period)), nullableBigInt(a.Counter), db.NowValue(s.DB.Dialect), a.ID)
	return err
}

// MissingIcon names an account waiting for an icon.
type MissingIcon struct {
	ID      int64
	UserID  int64
	Service string
}

// AccountsMissingIcon lists the accounts that have a service name but no icon, except
// those whose owner removed the icon on purpose.
func (s *Store) AccountsMissingIcon() ([]MissingIcon, error) {
	rows, err := s.DB.Query(`SELECT id, user_id, service FROM twofaccounts
		WHERE (icon IS NULL OR icon = '') AND icon_opt_out = ? AND service IS NOT NULL AND service <> '' AND user_id IS NOT NULL`, false)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MissingIcon
	for rows.Next() {
		var m MissingIcon
		if err := rows.Scan(&m.ID, &m.UserID, &m.Service); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetMissingIcon gives an account an icon, unless it got one in the meantime.
func (s *Store) SetMissingIcon(id int64, icon string) error {
	_, err := s.DB.Exec(`UPDATE twofaccounts SET icon = ? WHERE id = ? AND (icon IS NULL OR icon = '')`, icon, id)
	return err
}

// SetIconOptOut records whether the owner removed the icon on purpose.
func (s *Store) SetIconOptOut(id int64, optOut bool) error {
	_, err := s.DB.Exec(`UPDATE twofaccounts SET icon_opt_out = ? WHERE id = ?`, optOut, id)
	return err
}

// AdvanceCounter moves an HOTP counter forward only when it still holds the value
// the code was computed from, so two concurrent requests never share a code.
func (s *Store) AdvanceCounter(id, from, to int64) (bool, error) {
	res, err := s.DB.Exec(`UPDATE twofaccounts SET counter = ?, updated_at = ? WHERE id = ? AND counter = ?`,
		to, db.NowValue(s.DB.Dialect), id, from)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// DeleteAccount removes one account.
func (s *Store) DeleteAccount(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM twofaccounts WHERE id = ?`, id)
	return err
}

// DeleteAccounts removes several accounts.
func (s *Store) DeleteAccounts(ids []int64) error {
	for _, id := range ids {
		if err := s.DeleteAccount(id); err != nil {
			return err
		}
	}
	return nil
}

// WithdrawAccounts removes the accounts from their group.
func (s *Store) WithdrawAccounts(ids []int64, userID int64) error {
	for _, id := range ids {
		if _, err := s.DB.Exec(`UPDATE twofaccounts SET group_id = NULL, updated_at = ? WHERE id = ? AND user_id = ?`,
			db.NowValue(s.DB.Dialect), id, userID); err != nil {
			return err
		}
	}
	return nil
}

// AssignAccountToGroup moves an account into one of the user's groups.
func (s *Store) AssignAccountToGroup(accountID, groupID, userID int64) error {
	var gid int64
	err := s.DB.QueryRow(`SELECT id FROM "groups" WHERE id = ? AND user_id = ?`, groupID, userID).Scan(&gid)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`UPDATE twofaccounts SET group_id = ?, updated_at = ? WHERE id = ?`,
		gid, db.NowValue(s.DB.Dialect), accountID)
	return err
}

// CountOwnedAccounts counts the accounts owned by a user.
func (s *Store) CountOwnedAccounts(userID int64) (int64, error) {
	var n int64
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM twofaccounts WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

// CountSharedAccounts counts accounts shared with the user.
func (s *Store) CountSharedAccounts(userID int64, scope VisibleScope) (int64, error) {
	if !scope.EnableSharing {
		return 0, nil
	}
	query := `SELECT COUNT(DISTINCT t.id) FROM twofaccounts t
		WHERE t.user_id != ? AND (EXISTS (SELECT 1 FROM twofaccount_shares sh WHERE sh.twofaccount_id = t.id AND sh.scope = 'user' AND sh.shared_with_user_id = ?)`
	args := []any{userID, userID}
	if scope.AllUsersSharingScope {
		query += ` OR EXISTS (SELECT 1 FROM twofaccount_shares sh2 WHERE sh2.twofaccount_id = t.id AND sh2.scope = 'all_users')`
	}
	query += `)`
	var n int64
	err := s.DB.QueryRow(query, args...).Scan(&n)
	return n, err
}

// IsFavorite reports whether the user flagged the account as favorite.
func (s *Store) IsFavorite(accountID, userID int64) bool {
	var n int64
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM twofaccount_user_favorites WHERE user_id = ? AND twofaccount_id = ?`,
		userID, accountID).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// FavoriteAccountIDs returns the favorite account ids of a user.
func (s *Store) FavoriteAccountIDs(userID int64) map[int64]bool {
	out := map[int64]bool{}
	rows, err := s.DB.Query(`SELECT twofaccount_id FROM twofaccount_user_favorites WHERE user_id = ?`, userID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			out[id] = true
		}
	}
	return out
}

// ToggleFavorite sets the favorite flag of an account for a user.
func (s *Store) ToggleFavorite(accountID, userID int64, favorite bool) error {
	if favorite {
		var n int64
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM twofaccount_user_favorites WHERE user_id = ? AND twofaccount_id = ?`,
			userID, accountID).Scan(&n); err == nil && n > 0 {
			return nil
		}
		now := db.NowValue(s.DB.Dialect)
		_, err := s.DB.Exec(`INSERT INTO twofaccount_user_favorites (user_id, twofaccount_id, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			userID, accountID, now, now)
		return err
	}
	_, err := s.DB.Exec(`DELETE FROM twofaccount_user_favorites WHERE user_id = ? AND twofaccount_id = ?`, userID, accountID)
	return err
}

// SaveOrderForUser persists the display order, keeping unseen accounts at the end.
func (s *Store) SaveOrderForUser(userID int64, orderedIDs []int64, scope VisibleScope) ([]int64, error) {
	current, err := s.OrderedVisibleIDs(userID, scope)
	if err != nil {
		return nil, err
	}
	visible := map[int64]bool{}
	for _, id := range current {
		visible[id] = true
	}
	final := make([]int64, 0, len(current))
	seen := map[int64]bool{}
	for _, id := range orderedIDs {
		if visible[id] && !seen[id] {
			final = append(final, id)
			seen[id] = true
		}
	}
	for _, id := range current {
		if !seen[id] {
			final = append(final, id)
		}
	}

	if _, err := s.DB.Exec(`DELETE FROM twofaccount_user_orders WHERE user_id = ?`, userID); err != nil {
		return nil, err
	}
	now := db.NowValue(s.DB.Dialect)
	for i, id := range final {
		if _, err := s.DB.Exec(`INSERT INTO twofaccount_user_orders (user_id, twofaccount_id, position, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)`, userID, id, i+1, now, now); err != nil {
			return nil, err
		}
	}
	return final, nil
}

// TransferOwnership moves an account to another user.
func (s *Store) TransferOwnership(accountID, newOwnerID int64) error {
	if _, err := s.DB.Exec(`UPDATE twofaccounts SET user_id = ?, group_id = NULL, updated_at = ? WHERE id = ?`,
		newOwnerID, db.NowValue(s.DB.Dialect), accountID); err != nil {
		return err
	}
	// A share was a decision of the previous owner: it does not survive the transfer.
	_, err := s.DB.Exec(`DELETE FROM twofaccount_shares WHERE twofaccount_id = ?`, accountID)
	return err
}

// OwnedAccounts lists the accounts owned by a user.
func (s *Store) OwnedAccounts(userID int64) ([]*Account, error) {
	rows, err := s.DB.Query(`SELECT `+accountColumns+` FROM twofaccounts WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Account
	for rows.Next() {
		a, err := s.scanAccount(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func nullableInt(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableBigInt(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func toInt64Ptr(v *int) *int64 {
	if v == nil {
		return nil
	}
	n := int64(*v)
	return &n
}
