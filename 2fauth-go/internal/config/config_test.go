package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnv(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDefaultsAreSafe(t *testing.T) {
	c := Load(t.TempDir())
	if c.DBConnection != "sqlite" {
		t.Errorf("DBConnection = %q", c.DBConnection)
	}
	if c.SessionLifetime != 120 {
		t.Errorf("SessionLifetime = %d", c.SessionLifetime)
	}
	if !c.ContentSecurityPolicy || !c.MailVerifyTLS || c.OpenIDTrustUnverifiedEmail {
		t.Errorf("config = %+v", c)
	}
	if enabled, _ := c.Settings["useEncryption"].(bool); !enabled {
		t.Error("useEncryption is off by default")
	}
	if c.ListenAddr != ":8000" {
		t.Errorf("ListenAddr = %q", c.ListenAddr)
	}
}

func TestListenAddrComesFromTheFileToo(t *testing.T) {
	c := Load(writeEnv(t, "LISTEN_ADDR=192.168.1.237:8444\nALLOWED_CLIENT_IPS=192.168.1.118\nTLS_CERT_FILE=c.pem\nTLS_KEY_FILE=k.pem\n"))
	if c.ListenAddr != "192.168.1.237:8444" || c.AllowedClientIPs != "192.168.1.118" || c.TLSCertFile != "c.pem" || c.TLSKeyFile != "k.pem" {
		t.Errorf("config = %+v", c)
	}
	if c := Load(writeEnv(t, "PORT=9000\n")); c.ListenAddr != ":9000" {
		t.Errorf("PORT: ListenAddr = %q", c.ListenAddr)
	}
	t.Setenv("LISTEN_ADDR", "127.0.0.1:1234")
	if c := Load(writeEnv(t, "LISTEN_ADDR=192.168.1.237:8444\n")); c.ListenAddr != "127.0.0.1:1234" {
		t.Errorf("the environment must win: %q", c.ListenAddr)
	}
}

func TestRelativeUIPathIsResolved(t *testing.T) {
	c := Load(writeEnv(t, "UI_ASSETS_PATH=./dist\n"))
	if !filepath.IsAbs(c.UIPath) {
		t.Errorf("UIPath = %q", c.UIPath)
	}
}
