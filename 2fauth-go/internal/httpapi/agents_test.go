package httpapi

import (
	"strings"
	"testing"

	"2fauth/internal/config"
)

func TestAgentGuideIsPublicAndDescribesThisInstance(t *testing.T) {
	srv := testServerWith(t, func(c *config.Config) { c.URL = "https://2fa.example.org" })
	startSSH(t, srv)
	_, port, _ := strings.Cut(srv.cfg.SSHListenAddr, ":")
	anonymous := newClient(t, srv)

	for _, path := range []string{"/agents.md", "/llms.txt"} {
		rec := anonymous.do("GET", path, "", nil)
		if rec.Code != 200 {
			t.Fatalf("%s = %d", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
			t.Fatalf("%s content type %q", path, ct)
		}
		body := rec.Body.String()
		for _, want := range []string{
			"https://2fa.example.org/api/v1/twofaccounts/12/otp",
			"otp@2fa.example.org",
			"-p " + port + " ",
			srv.sshHostFingerprint,
			"| Available here | yes | yes |",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s lacks %q", path, want)
			}
		}
		if strings.Contains(body, "<no value>") {
			t.Errorf("%s has an unfilled template value", path)
		}
	}
}

func TestAgentGuideFollowsTheConfiguration(t *testing.T) {
	srv := testServerWith(t, func(c *config.Config) {
		c.URL = "https://2fa.example.org"
		c.SSHPublicAddr = "otp.example.org:2200"
	})
	startSSH(t, srv)
	body := newClient(t, srv).do("GET", "/agents.md", "", nil).Body.String()
	if !strings.Contains(body, "otp@otp.example.org") || !strings.Contains(body, `"-p", "2200"`) {
		t.Fatal("SSH_PUBLIC_ADDR is not used")
	}

	// Without the SSH service the guide says so instead of giving a dead address.
	off := testServerWith(t, func(c *config.Config) { c.URL = "https://2fa.example.org" })
	body = newClient(t, off).do("GET", "/agents.md", "", nil).Body.String()
	if !strings.Contains(body, "The SSH service is disabled on this server") || strings.Contains(body, "otp@2fa.example.org whoami") {
		t.Fatal("a disabled SSH service is described as available")
	}
}
