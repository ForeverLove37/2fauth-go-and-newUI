package httpapi

import (
	"bytes"
	_ "embed"
	"net"
	"net/http"
	"net/url"
	"strings"
	"text/template"
)

//go:embed agents.md.tmpl
var agentGuideSource string

var agentGuideTemplate = template.Must(template.New("agents.md").Parse(agentGuideSource))

// agentGuide handles GET /agents.md and /llms.txt: how a program gets a code from this
// instance, with its real addresses filled in. It is public, like the login page: it
// grants nothing, and an agent reads it before it has any credential.
func (s *Server) agentGuide(c *ctx) {
	base := s.publicBaseURL(c)
	data := map[string]any{
		"AppName":         s.cfg.AppName,
		"Version":         s.cfg.Version,
		"Base":            base,
		"Tokens":          s.patPolicyAllows() && s.cfg.AuthenticationGuard != guardReverseProxy,
		"SSH":             s.cfg.SSHListenAddr != "",
		"SSHHost":         "",
		"SSHPort":         "",
		"SSHPortArg":      "",
		"SSHPortFlag":     "",
		"HostFingerprint": "",
	}
	if s.cfg.SSHListenAddr != "" {
		host, port := s.sshPublicAddr(base)
		data["SSHHost"] = host
		data["SSHPort"] = port
		if port != "22" {
			data["SSHPortArg"] = port
			data["SSHPortFlag"] = "-p " + port + " "
		}
		s.sshMu.RLock()
		data["HostFingerprint"] = s.sshHostFingerprint
		s.sshMu.RUnlock()
	}

	var out bytes.Buffer
	if err := agentGuideTemplate.Execute(&out, data); err != nil {
		s.srvError(c, err)
		return
	}
	h := c.w.Header()
	h.Set("Content-Type", "text/markdown; charset=utf-8")
	// The ports and fingerprints change with the configuration.
	h.Set("Cache-Control", "no-cache")
	// Meant for the agents of the users of this instance, not for search engines.
	h.Set("X-Robots-Tag", "noindex")
	c.w.WriteHeader(http.StatusOK)
	_, _ = c.w.Write(out.Bytes())
}

// sshPublicAddr is the host and port clients use for the SSH service: SSH_PUBLIC_ADDR
// when set, else the host of the public URL and the port the service listens on.
func (s *Server) sshPublicAddr(base string) (string, string) {
	if public := s.cfg.SSHPublicAddr; public != "" {
		if host, port, err := net.SplitHostPort(public); err == nil {
			return host, port
		}
		return public, "22"
	}
	host := ""
	if parsed, err := url.Parse(base); err == nil {
		host = parsed.Hostname()
	}
	_, port, err := net.SplitHostPort(s.cfg.SSHListenAddr)
	if err != nil || port == "" {
		port = "22"
	}
	if listenHost, _, err := net.SplitHostPort(s.cfg.SSHListenAddr); err == nil && host == "" {
		host = strings.Trim(listenHost, "[]")
	}
	return host, port
}
