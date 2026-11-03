package store

import (
	"time"

	"2fauth/internal/db"
)

// OtpLog mirrors an otp_logs row.
type OtpLog struct {
	ID             int64
	RequesterID    *int64
	RequesterName  string
	RequesterEmail string
	OwnerID        *int64
	OwnerName      string
	OwnerEmail     string
	TwoFAccountID  int64
	IPAddress      string
	OTPType        string
	Counter        *int64
	GeneratedAt    time.Time
}

// CreateOtpLog appends an OTP generation log entry.
func (s *Store) CreateOtpLog(l *OtpLog) error {
	_, err := s.DB.Exec(`INSERT INTO otp_logs
		(requester_id, requester_name, requester_email, owner_id, owner_name, owner_email,
		 twofaccount_id, ip_address, otp_type, counter, generated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.RequesterID, l.RequesterName, l.RequesterEmail, l.OwnerID, l.OwnerName, l.OwnerEmail,
		l.TwoFAccountID, l.IPAddress, l.OTPType, l.Counter, l.GeneratedAt.UTC().Format(db.TimeLayout))
	return err
}

// ListOtpLogs returns the OTP logs of an account, optionally limited by period (months).
func (s *Store) ListOtpLogs(accountID int64, periodMonths int, limit int) ([]*OtpLog, error) {
	query := `SELECT id, requester_id, requester_name, requester_email, owner_id, owner_name, owner_email,
		twofaccount_id, ip_address, otp_type, counter, generated_at FROM otp_logs WHERE twofaccount_id = ?`
	args := []any{accountID}
	if periodMonths > 0 {
		cutoff := time.Now().AddDate(0, -periodMonths, 0)
		query += ` AND generated_at >= ?`
		args = append(args, cutoff.UTC().Format(db.TimeLayout))
	}
	query += ` ORDER BY generated_at DESC`
	if limit > 0 {
		query += ` LIMIT ` + itoa(int64(limit))
	}
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*OtpLog
	for rows.Next() {
		var (
			l         OtpLog
			requester db.NullInt
			owner     db.NullInt
			counter   db.NullInt
			genAt     db.NullString
		)
		if err := rows.Scan(&l.ID, &requester, &l.RequesterName, &l.RequesterEmail, &owner, &l.OwnerName,
			&l.OwnerEmail, &l.TwoFAccountID, &l.IPAddress, &l.OTPType, &counter, &genAt); err != nil {
			return nil, err
		}
		if requester.Valid {
			v := requester.Int
			l.RequesterID = &v
		}
		if owner.Valid {
			v := owner.Int
			l.OwnerID = &v
		}
		if counter.Valid {
			v := counter.Int
			l.Counter = &v
		}
		if t, err := db.ParseTime(genAt.String); err == nil && t != nil {
			l.GeneratedAt = *t
		}
		out = append(out, &l)
	}
	return out, rows.Err()
}

// PurgeOtpLogs removes entries older than the retention period (in days).
func (s *Store) PurgeOtpLogs(retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	_, err := s.DB.Exec(`DELETE FROM otp_logs WHERE generated_at < ?`, cutoff.UTC().Format(db.TimeLayout))
	return err
}

// AuthLog mirrors an auth_logs row.
type AuthLog struct {
	ID              int64
	UserID          *int64
	IPAddress       string
	UserAgent       string
	LoginAt         *time.Time
	LoginSuccessful bool
	LogoutAt        *time.Time
	ClearedByUser   bool
	Guard           string
	LoginMethod     string
}

// CreateAuthLog inserts an authentication log entry.
func (s *Store) CreateAuthLog(userID int64, ip, userAgent, guard, method string, success bool) error {
	now := time.Now().UTC()
	var logoutAt any
	if !success {
		logoutAt = now.Format(db.TimeLayout)
	}
	_, err := s.DB.Exec(`INSERT INTO auth_logs
		(authenticatable_type, authenticatable_id, ip_address, user_agent, login_at, login_successful,
		 logout_at, cleared_by_user, guard, login_method)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		map[bool]string{true: "App\\Models\\User", false: "App\\Models\\User"}[true], userID, ip, userAgent,
		now.Format(db.TimeLayout), success, logoutAt, false, guard, method)
	return err
}

// CloseAuthLog records the logout time of the latest session.
func (s *Store) CloseAuthLog(userID int64) {
	// Two statements: MySQL cannot select from the table an UPDATE is modifying.
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM auth_logs WHERE authenticatable_id = ? AND login_successful = ? AND logout_at IS NULL
		ORDER BY login_at DESC LIMIT 1`, userID, true).Scan(&id)
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(db.TimeLayout)
	_, _ = s.DB.Exec(`UPDATE auth_logs SET logout_at = ? WHERE id = ?`, now, id)
}

// SeenDevice reports whether the user already signed in from this address and
// browser. It must be called before the current sign-in is logged.
func (s *Store) SeenDevice(userID int64, ip, userAgent string) bool {
	var n int64
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM auth_logs WHERE authenticatable_id = ? AND login_successful = ?
		AND ip_address = ? AND user_agent = ?`, userID, true, ip, userAgent).Scan(&n)
	return err != nil || n > 0
}

// CountSuccessfulLogins returns how many times the user signed in.
func (s *Store) CountSuccessfulLogins(userID int64) int64 {
	var n int64
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM auth_logs WHERE authenticatable_id = ? AND login_successful = ?`,
		userID, true).Scan(&n); err != nil {
		return 0
	}
	return n
}

// ListAuthLogs returns the authentication history of a user.
func (s *Store) ListAuthLogs(userID int64, periodMonths int, limit int) ([]*AuthLog, error) {
	query := `SELECT id, authenticatable_id, ip_address, user_agent, login_at, login_successful, logout_at,
		cleared_by_user, guard, login_method FROM auth_logs WHERE authenticatable_id = ?`
	args := []any{userID}
	if periodMonths > 0 {
		cutoff := time.Now().AddDate(0, -periodMonths, 0)
		query += ` AND login_at >= ?`
		args = append(args, cutoff.UTC().Format(db.TimeLayout))
	}
	query += ` ORDER BY login_at DESC`
	if limit > 0 {
		query += ` LIMIT ` + itoa(int64(limit))
	}
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AuthLog
	for rows.Next() {
		var (
			l       AuthLog
			uid     db.NullInt
			ip      db.NullString
			ua      db.NullString
			loginAt db.NullString
			success any
			logout  db.NullString
			cleared any
			guard   db.NullString
			method  db.NullString
		)
		if err := rows.Scan(&l.ID, &uid, &ip, &ua, &loginAt, &success, &logout, &cleared, &guard, &method); err != nil {
			return nil, err
		}
		if uid.Valid {
			v := uid.Int
			l.UserID = &v
		}
		l.IPAddress = ip.String
		l.UserAgent = ua.String
		l.LoginSuccessful = db.IntToBool(success)
		l.ClearedByUser = db.IntToBool(cleared)
		l.Guard = guard.String
		l.LoginMethod = method.String
		if t, err := db.ParseTime(loginAt.String); err == nil {
			l.LoginAt = t
		}
		if t, err := db.ParseTime(logout.String); err == nil {
			l.LogoutAt = t
		}
		out = append(out, &l)
	}
	return out, rows.Err()
}

// PurgeAuthLogs removes expired authentication logs.
func (s *Store) PurgeAuthLogs(retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	_, err := s.DB.Exec(`DELETE FROM auth_logs WHERE login_at < ?`, cutoff.UTC().Format(db.TimeLayout))
	return err
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
