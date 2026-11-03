// Command 2fauth is a PHP-free reimplementation of the 2FAuth server.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"

	"2fauth/internal/config"
	"2fauth/internal/crypt"
	"2fauth/internal/db"
	"2fauth/internal/httpapi"
	"2fauth/internal/mail"
	"2fauth/internal/services"
	"2fauth/internal/store"
)

func main() {
	basePath := os.Getenv("APP_BASE_PATH")
	if basePath == "" {
		basePath = "."
	}
	if abs, err := filepath.Abs(basePath); err == nil {
		basePath = abs
	}

	// Handle artisan-like commands before starting the server.
	if len(os.Args) > 1 {
		os.Exit(runCommand(basePath, os.Args[1], os.Args[2:]))
	}

	cfg := config.Load(basePath)

	if cfg.Key == "" {
		log.Fatal("APP_KEY is not set: generate one with `2fauth key:generate` and add it to your .env file")
	}
	warnIfWorldReadable(filepath.Join(basePath, ".env"))

	dialect, dsn, err := dsnFor(cfg)
	if err != nil {
		log.Fatalf("database configuration error: %v", err)
	}

	conn, err := db.Open(dialect, dsn)
	if err != nil {
		log.Fatalf("unable to connect to the database: %v", err)
	}
	defer conn.Close()

	if err := conn.Migrate(); err != nil {
		log.Fatalf("unable to migrate the database: %v", err)
	}
	if dialect == "sqlite" {
		hardenSQLite(cfg)
	}

	st := store.New(conn, cfg)
	enc, err := crypt.New(cfg.Key)
	if err != nil {
		log.Fatalf("invalid APP_KEY: %v", err)
	}
	st.SetCipher(enc)
	if sealed, err := st.EncryptExistingSecrets(); err != nil {
		log.Fatalf("unable to encrypt the stored OTP secrets: %v", err)
	} else if sealed > 0 {
		slog.Info("OTP secrets stored in clear text are now encrypted", "count", sealed)
	}
	settings := services.NewSettingService(st, cfg)

	secure := strings.HasPrefix(strings.ToLower(cfg.URL), "https://")
	server, err := httpapi.New(httpapi.Options{
		Config:   cfg,
		Store:    st,
		Settings: settings,
		Enc:      enc,
		HTTPS:    secure,
	})
	if err != nil {
		log.Fatalf("unable to start the server: %v", err)
	}

	addr := cfg.ListenAddr
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("unable to listen on %s: %v", addr, err)
	}
	allowed, err := httpapi.ParseIPSet(cfg.AllowedClientIPs)
	if err != nil {
		log.Fatalf("ALLOWED_CLIENT_IPS: %v", err)
	}
	if !allowed.Empty() {
		// Refused before TLS and HTTP: nothing of the application is reachable.
		listener = &httpapi.AllowListener{Listener: listener, Allowed: allowed, OnRefused: func(remote string) {
			slog.Warn("connection refused by ALLOWED_CLIENT_IPS", "remote", remote)
		}}
	}

	srv := &http.Server{
		Handler:           server,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go runJanitor(ctx, st, cfg)

	useTLS := cfg.TLSCertFile != "" && cfg.TLSKeyFile != ""
	if (cfg.TLSCertFile != "") != (cfg.TLSKeyFile != "") {
		log.Fatal("TLS_CERT_FILE and TLS_KEY_FILE must be set together")
	}
	errs := make(chan error, 1)
	go func() {
		if useTLS {
			srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
			errs <- srv.ServeTLS(listener, cfg.TLSCertFile, cfg.TLSKeyFile)
			return
		}
		errs <- srv.Serve(listener)
	}()
	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	fmt.Printf("2FAuth %s (Go) listening on %s://%s\n", cfg.Version, scheme, addr)

	select {
	case err := <-errs:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
		server.WaitForMail()
	}
}

// runJanitor removes what has expired: finished sessions and old audit logs.
func runJanitor(ctx context.Context, st *store.Store, cfg *config.Config) {
	sweep := func() {
		now := time.Now()
		idle := now.Add(-time.Duration(cfg.SessionLifetime) * time.Minute).Unix()
		created := now.Add(-time.Duration(cfg.SessionMaxAge) * time.Minute).Unix()
		if err := st.PurgeSessions(idle, created); err != nil {
			slog.Warn("cannot purge the expired sessions", "error", err)
		}
		if err := st.PurgeAuthLogs(cfg.AuthLogRetentionTime); err != nil {
			slog.Warn("cannot purge the authentication logs", "error", err)
		}
		if err := st.PurgeOtpLogs(cfg.OtpLogRetentionTime); err != nil {
			slog.Warn("cannot purge the OTP logs", "error", err)
		}
	}
	sweep()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}

// warnIfWorldReadable flags a .env file others can read: APP_KEY decrypts everything.
func warnIfWorldReadable(path string) {
	if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o077 != 0 {
		slog.Warn("the .env file holds APP_KEY and is readable by other users: run chmod 600 on it", "path", path, "mode", info.Mode().Perm().String())
	}
}

// hardenSQLite keeps the database files private to the service account.
func hardenSQLite(cfg *config.Config) {
	path := sqlitePath(cfg)
	_ = os.Chmod(filepath.Dir(path), 0o700)
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Chmod(path+suffix, 0o600)
	}
}

func sqlitePath(cfg *config.Config) string {
	if cfg.DatabasePath != "" {
		return cfg.DatabasePath
	}
	return filepath.Join(cfg.BasePath, "database", "database.sqlite")
}

// dsnFor builds the driver name and data source name from the configuration.
func dsnFor(cfg *config.Config) (string, string, error) {
	switch strings.ToLower(cfg.DBConnection) {
	case "", "sqlite":
		path := sqlitePath(cfg)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return "", "", err
		}
		return "sqlite", path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", nil
	case "mysql", "mariadb":
		port := cfg.DBPort
		if port == "" {
			port = "3306"
		}
		// FormatDSN escapes what the credentials may contain.
		dsn := mysql.NewConfig()
		dsn.User = cfg.DBUsername
		dsn.Passwd = cfg.DBPassword
		dsn.Net = "tcp"
		dsn.Addr = net.JoinHostPort(defaultHost(cfg.DBHost), port)
		dsn.DBName = cfg.DBDatabase
		dsn.ParseTime = true
		dsn.Loc = time.UTC
		dsn.TLSConfig = cfg.DBTLS
		dsn.Params = map[string]string{"charset": "utf8mb4"}
		return "mysql", dsn.FormatDSN(), nil
	case "pgsql", "postgres", "postgresql":
		port := cfg.DBPort
		if port == "" {
			port = "5432"
		}
		// A URL, so that a password with spaces or quotes stays one value.
		dsn := url.URL{
			Scheme:   "postgres",
			User:     url.UserPassword(cfg.DBUsername, cfg.DBPassword),
			Host:     net.JoinHostPort(defaultHost(cfg.DBHost), port),
			Path:     "/" + cfg.DBDatabase,
			RawQuery: url.Values{"sslmode": {cfg.DBSSLMode}}.Encode(),
		}
		return "pgsql", dsn.String(), nil
	default:
		return "", "", fmt.Errorf("unsupported DB_CONNECTION %q", cfg.DBConnection)
	}
}

func defaultHost(host string) string {
	if host == "" {
		return "127.0.0.1"
	}
	return host
}

// runCommand implements the small set of maintenance commands the app exposes.
func runCommand(basePath, command string, args []string) int {
	switch command {
	case "key:generate":
		return generateKey(basePath, len(args) > 0 && args[0] == "--force")
	case "user:reset-password":
		return resetPassword(basePath, args)
	case "mail:preview":
		return previewMail(args)
	case "migrate":
		cfg := config.Load(basePath)
		dialect, dsn, err := dsnFor(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		conn, err := db.Open(dialect, dsn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		defer conn.Close()
		if err := conn.Migrate(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		fmt.Println("Database migrated successfully.")
		return 0
	case "config:cache", "route:cache", "event:cache", "view:cache", "optimize", "config:clear", "optimize:clear", "cache:clear":
		// Nothing is cached at build time in this implementation.
		fmt.Println("Nothing to cache: configuration is read at start-up.")
		return 0
	case "--version", "-v", "version":
		cfg := config.Load(basePath)
		fmt.Println(cfg.Version)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", command)
		return 2
	}
}

// generateKey writes a new APP_KEY. Replacing an existing key makes every encrypted
// value unreadable, so it needs --force and keeps a copy of the previous file.
func generateKey(basePath string, force bool) int {
	key := crypt.GenerateKey()
	envPath := filepath.Join(basePath, ".env")
	content, err := os.ReadFile(envPath)
	if err != nil {
		fmt.Println(key)
		return 0
	}
	lines := strings.Split(string(content), "\n")
	replaced := false
	for i, line := range lines {
		if !strings.HasPrefix(line, "APP_KEY=") {
			continue
		}
		if strings.TrimSpace(strings.TrimPrefix(line, "APP_KEY=")) != "" {
			if !force {
				fmt.Fprintln(os.Stderr, "APP_KEY is already set. It decrypts your OTP secrets: replacing it makes them unreadable.")
				fmt.Fprintln(os.Stderr, "Run `2fauth key:generate --force` only if you really mean to.")
				return 1
			}
			backup := envPath + ".bak-" + time.Now().Format("20060102-150405")
			if err := os.WriteFile(backup, content, 0o600); err != nil {
				fmt.Fprintf(os.Stderr, "unable to back up .env: %v\n", err)
				return 1
			}
			fmt.Printf("The previous file was kept as %s.\n", backup)
		}
		lines[i] = "APP_KEY=" + key
		replaced = true
	}
	if !replaced {
		lines = append(lines, "APP_KEY="+key)
	}
	if err := os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "unable to write .env: %v\n", err)
		return 1
	}
	// WriteFile keeps the mode of an existing file.
	_ = os.Chmod(envPath, 0o600)
	fmt.Println("Application key set successfully.")
	return 0
}

// resetPassword is the way back in when no administrator can sign in. It prints a
// new random password and ends the sessions and the tokens of the account.
func resetPassword(basePath string, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: 2fauth user:reset-password <email>")
		return 2
	}
	cfg := config.Load(basePath)
	dialect, dsn, err := dsnFor(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	conn, err := db.Open(dialect, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer conn.Close()
	if err := conn.Migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	st := store.New(conn, cfg)
	user, err := st.GetUserByEmail(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "no account uses this address")
		return 1
	}
	password := crypt.RandomString(24)
	hash, err := crypt.HashPassword(password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	if err := errors.Join(
		st.UpdatePassword(user.ID, hash),
		st.SetOAuthProvider(user.ID, "", ""),
		st.DeleteUserSessions(user.ID, ""),
		st.RevokeAllTokens(user.ID),
	); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	fmt.Println(password)
	return 0
}

// previewMail renders every e-mail with sample values, as files to look at.
func previewMail(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: 2fauth mail:preview <directory> <base url>")
		return 2
	}
	if err := os.MkdirAll(args[0], 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	base := strings.TrimRight(args[1], "/")
	at := time.Date(2026, 11, 3, 9, 41, 0, 0, time.UTC).Format("2006-01-02 15:04") + " (Asia/Shanghai)"
	samples := map[string]map[string]string{
		mail.KindResetPassword:   {"Email": "lin@2fauth-sample.org", "ResetURL": base + "/password/reset?token=SAMPLE-TOKEN-0123456789&email=lin%402fauth-sample.org", "ExpiresMinutes": "60"},
		mail.KindVerifyEmail:     {"Email": "lin@2fauth-sample.org", "VerifyURL": base + "/user/email/verify?token=SAMPLE-TOKEN-0123456789", "ExpiresHours": "24"},
		mail.KindNewDevice:       {"Email": "lin@2fauth-sample.org", "Time": at, "IP": "192.168.1.118", "Browser": "Chrome", "Platform": "Windows", "Method": "password", "SecurityURL": base + "/settings?tab=account"},
		mail.KindFailedLogin:     {"Email": "lin@2fauth-sample.org", "Time": at, "IP": "192.168.1.118", "Browser": "Firefox", "Platform": "Linux", "SecurityURL": base + "/settings?tab=account"},
		mail.KindPasswordChanged: {"Email": "lin@2fauth-sample.org", "Time": at, "IP": "192.168.1.118", "ResetURL": base + "/password/lost"},
		mail.KindAccountShared:   {"OwnerName": "周一鸣", "Label": "GitHub", "Account": "ops@2fauth-sample.org", "AppURL": base + "/"},
		mail.KindTransferred:     {"OwnerName": "周一鸣", "Label": "Cloudflare", "Account": "", "AppURL": base + "/"},
		mail.KindTest:            {"Server": "smtp.2fauth-sample.org:587", "Encryption": "tls", "Time": at, "AdminName": "周一鸣"},
	}
	brand := mail.Brand{AppName: "2FAuth", AppURL: base + "/"}
	for kind, vars := range samples {
		for _, lang := range []string{"zh-CN", "en"} {
			msg, err := mail.Render(kind, lang, brand, "lin@2fauth-sample.org", vars)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				return 1
			}
			stem := filepath.Join(args[0], kind+"."+lang)
			text := "Subject: " + msg.Subject + "\nPreheader: " + msg.Preheader + "\n\n" + msg.Text
			if err := errors.Join(os.WriteFile(stem+".html", []byte(msg.HTML), 0o644), os.WriteFile(stem+".txt", []byte(text), 0o644)); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				return 1
			}
		}
	}
	return 0
}
