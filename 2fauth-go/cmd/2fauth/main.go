// Command 2fauth is a PHP-free reimplementation of the 2FAuth server.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"2fauth/internal/config"
	"2fauth/internal/crypt"
	"2fauth/internal/db"
	"2fauth/internal/httpapi"
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
		os.Exit(runCommand(basePath, os.Args[1]))
	}

	cfg := config.Load(basePath)

	if cfg.Key == "" {
		log.Fatal("APP_KEY is not set: generate one with `2fauth key:generate` and add it to your .env file")
	}

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

	st := store.New(conn, cfg)
	settings := services.NewSettingService(st, cfg)

	enc, err := crypt.New(cfg.Key)
	if err != nil {
		log.Fatalf("invalid APP_KEY: %v", err)
	}

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

	addr := listenAddr()
	srv := &http.Server{
		Addr:              addr,
		Handler:           server,
		ReadHeaderTimeout: 10 * time.Second,
	}

	fmt.Printf("2FAuth %s (Go) listening on %s\n", cfg.Version, addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

// listenAddr resolves the listening address from the environment.
func listenAddr() string {
	for _, key := range []string{"LISTEN_ADDR", "SERVER_PORT", "PORT"} {
		if v := os.Getenv(key); v != "" {
			if strings.HasPrefix(v, ":") || strings.Contains(v, ":") {
				return v
			}
			return ":" + v
		}
	}
	return ":8000"
}

// dsnFor builds the driver name and data source name from the configuration.
func dsnFor(cfg *config.Config) (string, string, error) {
	switch strings.ToLower(cfg.DBConnection) {
	case "", "sqlite":
		path := cfg.DatabasePath
		if path == "" {
			path = filepath.Join(cfg.BasePath, "database", "database.sqlite")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", "", err
		}
		return "sqlite", path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", nil
	case "mysql", "mariadb":
		port := cfg.DBPort
		if port == "" {
			port = "3306"
		}
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=UTC",
			cfg.DBUsername, cfg.DBPassword, defaultHost(cfg.DBHost), port, cfg.DBDatabase)
		return "mysql", dsn, nil
	case "pgsql", "postgres", "postgresql":
		port := cfg.DBPort
		if port == "" {
			port = "5432"
		}
		dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			defaultHost(cfg.DBHost), port, cfg.DBUsername, cfg.DBPassword, cfg.DBDatabase)
		return "pgx", dsn, nil
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
func runCommand(basePath, command string) int {
	switch command {
	case "key:generate":
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
			if strings.HasPrefix(line, "APP_KEY=") {
				lines[i] = "APP_KEY=" + key
				replaced = true
			}
		}
		if !replaced {
			lines = append(lines, "APP_KEY="+key)
		}
		if err := os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "unable to write .env: %v\n", err)
			return 1
		}
		fmt.Println("Application key set successfully.")
		return 0
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
