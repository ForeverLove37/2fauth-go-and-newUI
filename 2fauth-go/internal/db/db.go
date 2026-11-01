// Package db provides the database connection, schema creation and small query
// helpers used by the 2FAuth Go backend. The schema mirrors the final state of
// the Laravel migrations shipped with 2FAuth v9.
package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// DB wraps *sql.DB with dialect helpers.
type DB struct {
	*sql.DB
	Dialect string // sqlite | mysql | pgsql
}

// Open connects to the configured database.
func Open(dialect, dsn string) (*DB, error) {
	var driver string
	switch dialect {
	case "sqlite":
		driver = "sqlite"
	case "mysql":
		driver = "mysql"
	case "pgsql":
		driver = "pgx"
	default:
		return nil, fmt.Errorf("unsupported database connection %q", dialect)
	}
	conn, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(25)
	if err := conn.Ping(); err != nil {
		return nil, err
	}
	return &DB{DB: conn, Dialect: dialect}, nil
}

// Rebind rewrites `?` placeholders to the dialect specific form.
func (d *DB) Rebind(query string) string {
	if d.Dialect != "pgsql" {
		return query
	}
	var sb strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			sb.WriteString(fmt.Sprintf("$%d", n))
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// Exec runs a statement with dialect rebinding.
func (d *DB) Exec(query string, args ...any) (sql.Result, error) {
	return d.DB.Exec(d.Rebind(query), args...)
}

// Query runs a query with dialect rebinding.
func (d *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.DB.Query(d.Rebind(query), args...)
}

// QueryRow runs a single row query with dialect rebinding.
func (d *DB) QueryRow(query string, args ...any) *sql.Row {
	return d.DB.QueryRow(d.Rebind(query), args...)
}

func (d *DB) autoincrement() string {
	if d.Dialect == "pgsql" {
		return "BIGSERIAL PRIMARY KEY"
	}
	if d.Dialect == "mysql" {
		return "BIGINT AUTO_INCREMENT PRIMARY KEY"
	}
	return "INTEGER PRIMARY KEY AUTOINCREMENT"
}

func (d *DB) text() string {
	if d.Dialect == "mysql" {
		return "TEXT"
	}
	return "TEXT"
}

// schema returns the ordered DDL statements of the final schema.
func (d *DB) schema() []string {
	pk := d.autoincrement()
	t := d.text()
	return []string{
		`CREATE TABLE IF NOT EXISTS users (
			id ` + pk + `,
			name VARCHAR(255) NOT NULL UNIQUE,
			email VARCHAR(255) NOT NULL UNIQUE,
			email_verified_at TIMESTAMP NULL,
			password VARCHAR(255) NOT NULL,
			remember_token VARCHAR(100) NULL,
			created_at TIMESTAMP NULL,
			updated_at TIMESTAMP NULL,
			last_seen_at TIMESTAMP NULL,
			is_admin BOOLEAN NOT NULL DEFAULT FALSE,
			preferences ` + t + ` NULL,
			oauth_provider VARCHAR(100) NULL,
			oauth_id VARCHAR(200) NULL
		)`,

		`CREATE TABLE IF NOT EXISTS password_resets (
			email VARCHAR(255) NOT NULL,
			token VARCHAR(255) NOT NULL,
			created_at TIMESTAMP NULL
		)`,
		`CREATE INDEX IF NOT EXISTS password_resets_email_index ON password_resets (email)`,

		`CREATE TABLE IF NOT EXISTS options (
			id ` + pk + `,
			key VARCHAR(255) NOT NULL UNIQUE,
			value ` + t + ` NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS groups (
			id ` + pk + `,
			user_id BIGINT NULL,
			name VARCHAR(255) NOT NULL,
			order_column INTEGER NULL,
			show_in_chips BOOLEAN NOT NULL DEFAULT 1,
			created_at TIMESTAMP NULL,
			updated_at TIMESTAMP NULL,
			CONSTRAINT groups_user_id_foreign FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
		)`,

		`CREATE TABLE IF NOT EXISTS twofaccounts (
			id ` + pk + `,
			user_id BIGINT NULL,
			group_id BIGINT NULL,
			service ` + t + ` NULL,
			legacy_uri ` + t + ` NULL,
			account ` + t + ` NULL,
			icon VARCHAR(255) NULL,
			otp_type VARCHAR(10) NOT NULL,
			secret ` + t + ` NOT NULL,
			algorithm VARCHAR(20) NOT NULL DEFAULT 'sha1',
			digits SMALLINT NOT NULL DEFAULT 6,
			period INTEGER NULL,
			counter BIGINT NULL,
			created_at TIMESTAMP NULL,
			updated_at TIMESTAMP NULL,
			CONSTRAINT twofaccounts_user_id_foreign FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
			CONSTRAINT twofaccounts_group_id_foreign FOREIGN KEY (group_id) REFERENCES groups (id) ON DELETE SET NULL
		)`,

		`CREATE TABLE IF NOT EXISTS twofaccount_shares (
			id ` + pk + `,
			twofaccount_id BIGINT NOT NULL,
			shared_with_user_id BIGINT NULL,
			scope VARCHAR(24) NOT NULL,
			created_by_user_id BIGINT NOT NULL,
			created_at TIMESTAMP NULL,
			updated_at TIMESTAMP NULL,
			CONSTRAINT twofaccount_shares_twofaccount_id_foreign FOREIGN KEY (twofaccount_id) REFERENCES twofaccounts (id) ON DELETE CASCADE,
			CONSTRAINT twofaccount_shares_shared_with_user_id_foreign FOREIGN KEY (shared_with_user_id) REFERENCES users (id) ON DELETE CASCADE,
			CONSTRAINT twofaccount_shares_created_by_user_id_foreign FOREIGN KEY (created_by_user_id) REFERENCES users (id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS twofaccount_shares_account_scope_idx ON twofaccount_shares (twofaccount_id, scope)`,
		`CREATE INDEX IF NOT EXISTS twofaccount_shares_shared_with_idx ON twofaccount_shares (shared_with_user_id)`,

		`CREATE TABLE IF NOT EXISTS twofaccount_user_orders (
			id ` + pk + `,
			user_id BIGINT NOT NULL,
			twofaccount_id BIGINT NOT NULL,
			position INTEGER NOT NULL,
			created_at TIMESTAMP NULL,
			updated_at TIMESTAMP NULL,
			CONSTRAINT twofaccount_user_orders_user_id_foreign FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
			CONSTRAINT twofaccount_user_orders_twofaccount_id_foreign FOREIGN KEY (twofaccount_id) REFERENCES twofaccounts (id) ON DELETE CASCADE
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS twofaccount_user_orders_unique_scope ON twofaccount_user_orders (user_id, twofaccount_id)`,

		`CREATE TABLE IF NOT EXISTS twofaccount_user_favorites (
			id ` + pk + `,
			user_id BIGINT NOT NULL,
			twofaccount_id BIGINT NOT NULL,
			created_at TIMESTAMP NULL,
			updated_at TIMESTAMP NULL,
			CONSTRAINT twofaccount_user_favorites_user_id_foreign FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
			CONSTRAINT twofaccount_user_favorites_twofaccount_id_foreign FOREIGN KEY (twofaccount_id) REFERENCES twofaccounts (id) ON DELETE CASCADE
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS twofaccount_user_favorites_unique_scope ON twofaccount_user_favorites (user_id, twofaccount_id)`,

		`CREATE TABLE IF NOT EXISTS icons (
			name VARCHAR(255) NOT NULL PRIMARY KEY,
			content ` + t + ` NOT NULL,
			created_at TIMESTAMP NULL,
			updated_at TIMESTAMP NULL
		)`,

		`CREATE TABLE IF NOT EXISTS otp_logs (
			id ` + pk + `,
			requester_id BIGINT NULL,
			requester_name VARCHAR(255) NOT NULL,
			requester_email VARCHAR(255) NOT NULL,
			owner_id BIGINT NULL,
			owner_name VARCHAR(255) NOT NULL,
			owner_email VARCHAR(255) NOT NULL,
			twofaccount_id BIGINT NOT NULL,
			ip_address VARCHAR(45) NOT NULL,
			otp_type VARCHAR(10) NOT NULL,
			counter BIGINT NULL,
			generated_at TIMESTAMP NOT NULL,
			CONSTRAINT otp_logs_requester_foreign FOREIGN KEY (requester_id) REFERENCES users (id) ON DELETE SET NULL,
			CONSTRAINT otp_logs_owner_foreign FOREIGN KEY (owner_id) REFERENCES users (id) ON DELETE SET NULL,
			CONSTRAINT otp_logs_twofaccount_foreign FOREIGN KEY (twofaccount_id) REFERENCES twofaccounts (id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS otp_logs_requester_idx ON otp_logs (requester_id)`,
		`CREATE INDEX IF NOT EXISTS otp_logs_twofaccount_idx ON otp_logs (twofaccount_id)`,
		`CREATE INDEX IF NOT EXISTS otp_logs_generated_at_idx ON otp_logs (generated_at)`,

		`CREATE TABLE IF NOT EXISTS auth_logs (
			id ` + pk + `,
			authenticatable_type VARCHAR(255) NULL,
			authenticatable_id BIGINT NULL,
			ip_address VARCHAR(45) NULL,
			user_agent ` + t + ` NULL,
			login_at TIMESTAMP NULL,
			login_successful BOOLEAN NOT NULL DEFAULT FALSE,
			logout_at TIMESTAMP NULL,
			cleared_by_user BOOLEAN NOT NULL DEFAULT FALSE,
			guard VARCHAR(40) NULL,
			login_method VARCHAR(40) NULL,
			CONSTRAINT auth_logs_authenticatable_foreign FOREIGN KEY (authenticatable_id) REFERENCES users (id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS auth_logs_login_at_idx ON auth_logs (login_at)`,

		`CREATE TABLE IF NOT EXISTS webauthn_credentials (
			id VARCHAR(510) NOT NULL PRIMARY KEY,
			authenticatable_type VARCHAR(255) NULL,
			authenticatable_id BIGINT NULL,
			user_id VARCHAR(255) NULL,
			alias VARCHAR(255) NULL,
			counter BIGINT NULL,
			rp_id VARCHAR(255) NOT NULL,
			origin VARCHAR(255) NOT NULL,
			transports ` + t + ` NULL,
			aaguid VARCHAR(255) NULL,
			public_key ` + t + ` NOT NULL,
			attestation_format VARCHAR(255) NOT NULL DEFAULT 'none',
			certificates ` + t + ` NULL,
			disabled_at TIMESTAMP NULL,
			created_at TIMESTAMP NULL,
			updated_at TIMESTAMP NULL
		)`,

		`CREATE TABLE IF NOT EXISTS oauth_access_tokens (
			id VARCHAR(100) NOT NULL PRIMARY KEY,
			user_id BIGINT NULL,
			client_id BIGINT NOT NULL DEFAULT 1,
			name VARCHAR(255) NULL,
			scopes ` + t + ` NULL,
			revoked BOOLEAN NOT NULL DEFAULT FALSE,
			token_hash VARCHAR(64) NULL,
			created_at TIMESTAMP NULL,
			updated_at TIMESTAMP NULL,
			expires_at TIMESTAMP NULL
		)`,
		`CREATE INDEX IF NOT EXISTS oauth_access_tokens_user_id_idx ON oauth_access_tokens (user_id)`,
		`CREATE INDEX IF NOT EXISTS oauth_access_tokens_token_hash_idx ON oauth_access_tokens (token_hash)`,

		`CREATE TABLE IF NOT EXISTS oauth_identities (
			id ` + pk + `,
			user_id BIGINT NOT NULL,
			provider VARCHAR(40) NOT NULL,
			subject VARCHAR(255) NOT NULL,
			email VARCHAR(255) NULL,
			created_at TIMESTAMP NULL,
			updated_at TIMESTAMP NULL,
			CONSTRAINT oauth_identities_user_id_foreign FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS oauth_identities_provider_subject_unique ON oauth_identities (provider, subject)`,
		`CREATE INDEX IF NOT EXISTS oauth_identities_user_id_idx ON oauth_identities (user_id)`,

		`CREATE TABLE IF NOT EXISTS migrations (
			id INTEGER PRIMARY KEY,
			migration VARCHAR(255) NOT NULL,
			batch INTEGER NOT NULL
		)`,
	}
}

// Migrate creates the schema if needed and applies additive upgrades.
func (d *DB) Migrate() error {
	for _, stmt := range d.schema() {
		if _, err := d.DB.Exec(stmt); err != nil {
			return fmt.Errorf("schema: %w\nstatement: %s", err, stmt)
		}
	}
	// Additive upgrades for databases created by earlier builds.
	upgrades := []string{
		`ALTER TABLE groups ADD COLUMN show_in_chips BOOLEAN NOT NULL DEFAULT 1`,
	}
	for _, stmt := range upgrades {
		_, _ = d.DB.Exec(stmt) // fails when the column already exists
	}
	return nil
}

// Now returns the current time formatted for the dialect's timestamp columns.
func (d *DB) Now() any {
	return NowValue(d.Dialect)
}
