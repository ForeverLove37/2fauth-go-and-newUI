package httpapi

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/http/httptest"
	netmail "net/mail"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"2fauth/internal/config"
	"2fauth/internal/crypt"
	"2fauth/internal/oidc"
	otpgen "2fauth/internal/otp"
)

const testPassword = "password123"
const testSecret = "JBSWY3DPEHPK3PXP"

func newClient(t *testing.T, srv *Server) *client {
	t.Helper()
	c := &client{t: t, srv: srv}
	c.do(http.MethodGet, "/refresh-csrf", "", nil)
	return c
}

func registered(t *testing.T, srv *Server, name string) *client {
	t.Helper()
	c := newClient(t, srv)
	body := fmt.Sprintf(`{"name":%q,"email":%q,"password":%q,"password_confirmation":%q}`, name, name+"@example.org", testPassword, testPassword)
	if rec := c.do(http.MethodPost, "/user", body, nil); rec.Code != http.StatusCreated {
		t.Fatalf("register %s = %d (%s)", name, rec.Code, rec.Body.String())
	}
	return c
}

func signedIn(t *testing.T, srv *Server, name, password string) *client {
	t.Helper()
	c := newClient(t, srv)
	body := fmt.Sprintf(`{"email":%q,"password":%q}`, name+"@example.org", password)
	if rec := c.do(http.MethodPost, "/user/login", body, nil); rec.Code != http.StatusOK {
		t.Fatalf("login %s = %d (%s)", name, rec.Code, rec.Body.String())
	}
	return c
}

func createAccount(t *testing.T, c *client) int64 {
	t.Helper()
	body := fmt.Sprintf(`{"otp_type":"totp","service":"GitHub","account":"root","secret":%q,"digits":6,"algorithm":"sha1","period":30}`, testSecret)
	rec := c.do(http.MethodPost, "/api/v1/twofaccounts", body, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create account = %d (%s)", rec.Code, rec.Body.String())
	}
	data, _ := jsonBody(t, rec)["data"].(map[string]any)
	return int64(data["id"].(float64))
}

func userID(t *testing.T, srv *Server, name string) int64 {
	t.Helper()
	u, err := srv.st.GetUserByEmail(name + "@example.org")
	if err != nil {
		t.Fatalf("user %s: %v", name, err)
	}
	return u.ID
}

func setSetting(t *testing.T, admin *client, name string, value any) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"value": value})
	if rec := admin.do(http.MethodPut, "/api/v1/settings/"+name, string(raw), nil); rec.Code != http.StatusOK {
		t.Fatalf("set %s = %d (%s)", name, rec.Code, rec.Body.String())
	}
}

func upload(t *testing.T, c *client, field, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(content)
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/icons", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Cookie", c.cookie)
	req.Header.Set("X-XSRF-TOKEN", c.xsrf)
	rec := httptest.NewRecorder()
	c.srv.ServeHTTP(rec, req)
	return rec
}

var onePixelPNG, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==")

// --- uploads ----------------------------------------------------------------

func TestIconUploadRefusesAnythingButImages(t *testing.T) {
	srv := testServer(t)
	c := registered(t, srv, "alice")
	for name, content := range map[string]string{
		"page.svg":    `<!DOCTYPE html><html><script>alert(1)</script></html>`,
		"page.png":    `<html><body><script>alert(1)</script></body></html>`,
		"hidden.svg":  `<html><body><svg></svg><script>alert(1)</script></body></html>`,
		"text.jpg":    `just some text`,
		"broken.svg":  `<svg><script>`,
		"trailer.svg": `<svg xmlns="http://www.w3.org/2000/svg"></svg><script>alert(1)</script>`,
	} {
		if rec := upload(t, c, "icon", name, []byte(content)); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s = %d (%s)", name, rec.Code, rec.Body.String())
		}
	}
}

func TestIconUploadCleansSVG(t *testing.T) {
	srv := testServer(t)
	c := registered(t, srv, "alice")
	evil := `<?xml version="1.0"?><!DOCTYPE svg [<!ENTITY x "y">]><svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)" viewBox="0 0 1 1">
		<script>alert(2)</script><foreignObject><body xmlns="http://www.w3.org/1999/xhtml"><iframe src="javascript:alert(3)"/></body></foreignObject>
		<a href="javascript:alert(4)"><rect width="1" height="1" onclick="alert(5)" fill="url(https://evil.test/x)"/></a>
		<image href="https://evil.test/track.png"/><use href="https://evil.test/x.svg#a"/>
		<path d="M0 0" fill="url(#ok)" style="fill:red"/></svg>`
	rec := upload(t, c, "icon", "logo.svg", []byte(evil))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload = %d (%s)", rec.Code, rec.Body.String())
	}
	name, _ := jsonBody(t, rec)["icon"].(string)
	if !strings.HasSuffix(name, ".svg") {
		t.Fatalf("icon = %q", name)
	}
	anon := &client{t: t, srv: srv}
	served := anon.do(http.MethodGet, "/storage/icons/"+name, "", nil)
	body := served.Body.String()
	for _, bad := range []string{"script", "onload", "onclick", "foreignObject", "iframe", "javascript", "evil.test", "image", "ENTITY"} {
		if strings.Contains(body, bad) {
			t.Errorf("%q survived: %s", bad, body)
		}
	}
	if !strings.Contains(body, `d="M0 0"`) || !strings.Contains(body, `fill="url(#ok)"`) {
		t.Errorf("the harmless content was lost: %s", body)
	}
	if got := served.Header().Get("Content-Type"); got != "image/svg+xml" {
		t.Errorf("content type = %q", got)
	}
	csp := served.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "sandbox") || !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("csp = %q", csp)
	}
	if served.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("no nosniff header")
	}
}

func TestIconExtensionComesFromTheContent(t *testing.T) {
	srv := testServer(t)
	c := registered(t, srv, "alice")
	rec := upload(t, c, "icon", "logo.svg", onePixelPNG)
	name, _ := jsonBody(t, rec)["icon"].(string)
	if !strings.HasSuffix(name, ".png") {
		t.Fatalf("a PNG named .svg was stored as %q", name)
	}
}

func TestLegacyIconFilesAreNotServedAsPages(t *testing.T) {
	srv := testServer(t)
	// A file stored by an earlier build, before uploads were validated.
	if err := os.WriteFile(filepath.Join(srv.icons.Dir(), "old.svg"), []byte(`<!DOCTYPE html><script>alert(1)</script>`), 0o644); err != nil {
		t.Fatal(err)
	}
	anon := &client{t: t, srv: srv}
	if rec := anon.do(http.MethodGet, "/storage/icons/old.svg", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("legacy page = %d, content type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestPublicAssetsCarryNoCookie(t *testing.T) {
	srv := testServer(t)
	c := registered(t, srv, "alice")
	name, _ := jsonBody(t, upload(t, c, "icon", "a.png", onePixelPNG))["icon"].(string)
	rec := c.do(http.MethodGet, "/storage/icons/"+name, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("icon = %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "public") {
		t.Errorf("cache-control = %q", rec.Header().Get("Cache-Control"))
	}
	if cookies := rec.Header().Values("Set-Cookie"); len(cookies) != 0 {
		t.Errorf("a public response sets cookies: %v", cookies)
	}
}

func TestIconDeletionRespectsOwnership(t *testing.T) {
	srv := testServer(t)
	alice := registered(t, srv, "alice")
	bob := registered(t, srv, "bob")
	name, _ := jsonBody(t, upload(t, alice, "icon", "a.png", onePixelPNG))["icon"].(string)

	if rec := bob.do(http.MethodDelete, "/api/v1/icons/"+name, "", nil); rec.Code != http.StatusForbidden {
		t.Errorf("foreign delete = %d", rec.Code)
	}
	if !srv.icons.Exists(name) {
		t.Fatal("bob deleted alice's icon")
	}
	// Bob uploads the same bytes: the file is shared and survives alice's delete.
	upload(t, bob, "icon", "b.png", onePixelPNG)
	if rec := alice.do(http.MethodDelete, "/api/v1/icons/"+name, "", nil); rec.Code != http.StatusOK {
		t.Errorf("own delete = %d", rec.Code)
	}
	if !srv.icons.Exists(name) {
		t.Fatal("alice removed a file bob still uses")
	}
	if rec := bob.do(http.MethodDelete, "/api/v1/icons/"+name, "", nil); rec.Code != http.StatusOK {
		t.Errorf("last delete = %d", rec.Code)
	}
	if srv.icons.Exists(name) {
		t.Error("the file was not removed by its last user")
	}
}

func TestIconNamesCannotEscape(t *testing.T) {
	srv := testServer(t)
	admin := registered(t, srv, "alice")
	for _, name := range []string{".", "..", "%2e", "..%2f..%2fx"} {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/icons/placeholder", nil)
		req.URL.Path = "/api/v1/icons/" + strings.ReplaceAll(strings.ReplaceAll(name, "%2e", "."), "%2f", "/")
		req.Header.Set("Cookie", admin.cookie)
		req.Header.Set("X-XSRF-TOKEN", admin.xsrf)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK || rec.Code >= 500 {
			t.Errorf("DELETE %q = %d", name, rec.Code)
		}
	}
	if info, err := os.Stat(srv.icons.Dir()); err != nil || !info.IsDir() {
		t.Fatalf("the icons directory is gone: %v", err)
	}
	if rec := upload(t, admin, "icon", "a.png", onePixelPNG); rec.Code != http.StatusOK {
		t.Errorf("upload afterwards = %d", rec.Code)
	}
	name := `../../user/logout#/x.png`
	raw, _ := json.Marshal(map[string]any{"otp_type": "totp", "account": "a", "secret": testSecret, "icon": name})
	if rec := admin.do(http.MethodPost, "/api/v1/twofaccounts", string(raw), nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("crafted icon name = %d", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	srv := testServer(t)
	rec := (&client{t: t, srv: srv}).do(http.MethodGet, "/api/v1/frontend-config", "", nil)
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
		"Cache-Control":          "no-store",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q", header, got)
		}
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe-eval") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("csp = %q", csp)
	}
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS on a clear-text deployment")
	}
}

func TestLegacyShellUsesANonce(t *testing.T) {
	srv := testServer(t)
	rec := (&client{t: t, srv: srv}).do(http.MethodGet, "/legacy/", "", nil)
	match := regexp.MustCompile(`'nonce-([A-Za-z0-9]+)'`).FindStringSubmatch(rec.Header().Get("Content-Security-Policy"))
	if match == nil || !strings.Contains(rec.Body.String(), `nonce="`+match[1]+`"`) {
		t.Fatalf("csp = %q", rec.Header().Get("Content-Security-Policy"))
	}
}

// --- sessions ---------------------------------------------------------------

func replay(t *testing.T, srv *Server, cookie string) int {
	t.Helper()
	c := &client{t: t, srv: srv, cookie: cookie}
	return c.do(http.MethodGet, "/api/v1/user", "", nil).Code
}

func TestLogoutEndsTheSessionOnTheServer(t *testing.T) {
	srv := testServer(t)
	registered(t, srv, "alice")
	c := signedIn(t, srv, "alice", testPassword)
	stolen := c.cookie
	if code := replay(t, srv, stolen); code != http.StatusOK {
		t.Fatalf("before logout = %d", code)
	}
	if rec := c.do(http.MethodPost, "/user/logout", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("logout = %d", rec.Code)
	}
	if code := replay(t, srv, stolen); code != http.StatusUnauthorized {
		t.Errorf("replayed cookie = %d", code)
	}
}

func TestGetLogoutNeedsTheCSRFToken(t *testing.T) {
	srv := testServer(t)
	registered(t, srv, "alice")
	c := signedIn(t, srv, "alice", testPassword)
	forged := &client{t: t, srv: srv, cookie: c.cookie}
	if rec := forged.do(http.MethodGet, "/user/logout", "", nil); rec.Code != statusCSRFMismatch {
		t.Errorf("forged logout = %d", rec.Code)
	}
	if rec := c.do(http.MethodGet, "/user/logout", "", nil); rec.Code != http.StatusOK {
		t.Errorf("logout = %d", rec.Code)
	}
}

func TestPasswordChangeRevokesEverythingElse(t *testing.T) {
	srv := testServer(t)
	current := registered(t, srv, "alice")
	other := signedIn(t, srv, "alice", testPassword)
	rec := current.do(http.MethodPost, "/oauth/personal-access-tokens", `{"name":"t"}`, nil)
	token, _ := jsonBody(t, rec)["accessToken"].(string)
	if token == "" {
		t.Fatalf("no token: %s", rec.Body.String())
	}
	bearer := map[string]string{"Authorization": "Bearer " + token}
	if rec := (&client{t: t, srv: srv}).do(http.MethodGet, "/api/v1/user", "", bearer); rec.Code != http.StatusOK {
		t.Fatalf("token before = %d", rec.Code)
	}
	body := `{"currentPassword":"password123","password":"another-password","password_confirmation":"another-password"}`
	if rec := current.do(http.MethodPatch, "/user/password", body, nil); rec.Code != http.StatusOK {
		t.Fatalf("change = %d (%s)", rec.Code, rec.Body.String())
	}
	if code := replay(t, srv, other.cookie); code != http.StatusUnauthorized {
		t.Errorf("other device = %d", code)
	}
	if rec := (&client{t: t, srv: srv}).do(http.MethodGet, "/api/v1/user", "", bearer); rec.Code != http.StatusUnauthorized {
		t.Errorf("token after = %d", rec.Code)
	}
	if code := current.do(http.MethodGet, "/api/v1/user", "", nil).Code; code != http.StatusOK {
		t.Errorf("current device = %d", code)
	}
}

func TestAdminPasswordResetEndsTheSessions(t *testing.T) {
	srv := testServer(t)
	admin := registered(t, srv, "admin")
	bob := registered(t, srv, "bob")
	id := userID(t, srv, "bob")
	if rec := admin.do(http.MethodPatch, "/api/v1/users/"+strconv.FormatInt(id, 10)+"/password/reset", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("reset = %d", rec.Code)
	}
	if code := replay(t, srv, bob.cookie); code != http.StatusUnauthorized {
		t.Errorf("session after an admin reset = %d", code)
	}
}

func expireSession(t *testing.T, srv *Server) {
	t.Helper()
	old := time.Now().Add(-3 * time.Hour).Unix()
	if _, err := srv.st.DB.Exec(`UPDATE user_sessions SET last_activity = ?`, old); err != nil {
		t.Fatal(err)
	}
}

func TestIdleSessionsExpireOnEveryRoute(t *testing.T) {
	srv := testServer(t)
	c := registered(t, srv, "alice")
	expireSession(t, srv)
	probe := &client{t: t, srv: srv, cookie: c.cookie}
	if rec := probe.do(http.MethodGet, "/api/v1/user", "", nil); rec.Code != statusSessionExpired {
		t.Errorf("API route = %d", rec.Code)
	}

	d := registered(t, srv, "bob")
	expireSession(t, srv)
	// The non-API routes used to authenticate and even extend an idle session.
	probe = &client{t: t, srv: srv, cookie: d.cookie}
	if rec := probe.do(http.MethodGet, "/oauth/personal-access-tokens", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("web route = %d", rec.Code)
	}
	again := &client{t: t, srv: srv, cookie: d.cookie}
	if rec := again.do(http.MethodGet, "/api/v1/user", "", nil); rec.Code == http.StatusOK {
		t.Error("the idle session came back to life")
	}
}

func TestSessionsHaveAnAbsoluteLifetime(t *testing.T) {
	srv := testServer(t)
	c := registered(t, srv, "alice")
	old := time.Now().Add(-time.Duration(srv.cfg.SessionMaxAge+1) * time.Minute).Unix()
	if _, err := srv.st.DB.Exec(`UPDATE user_sessions SET created_at = ?`, old); err != nil {
		t.Fatal(err)
	}
	if rec := c.do(http.MethodGet, "/api/v1/user", "", nil); rec.Code != statusSessionExpired {
		t.Errorf("expired session = %d", rec.Code)
	}
}

// --- client address and throttling ------------------------------------------

func TestLoginThrottleIgnoresForgedForwardedFor(t *testing.T) {
	srv := testServer(t)
	registered(t, srv, "alice")
	c := newClient(t, srv)
	limited := false
	for i := 0; i < 8; i++ {
		rec := c.do(http.MethodPost, "/user/login", `{"email":"alice@example.org","password":"wrong-password"}`,
			map[string]string{"X-Forwarded-For": fmt.Sprintf("10.1.%d.1", i)})
		if rec.Code == http.StatusTooManyRequests {
			limited = true
		}
	}
	if !limited {
		t.Fatal("rotating X-Forwarded-For bypassed the throttle")
	}
	logs, err := srv.st.ListAuthLogs(userID(t, srv, "alice"), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range logs {
		if strings.HasPrefix(l.IPAddress, "10.1.") {
			t.Fatalf("a forged address reached the audit log: %q", l.IPAddress)
		}
	}
}

func TestClientIPTrustsOnlyConfiguredProxies(t *testing.T) {
	srv := testServerWith(t, func(cfg *config.Config) { cfg.TrustedProxies = "192.0.2.0/24, 10.0.0.5" })
	request := func(peer, forwarded string) string {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = peer + ":1234"
		if forwarded != "" {
			r.Header.Set("X-Forwarded-For", forwarded)
		}
		return srv.clientIP(r)
	}
	cases := []struct{ peer, forwarded, want string }{
		{"203.0.113.9", "1.2.3.4", "203.0.113.9"},              // untrusted peer: header ignored
		{"192.0.2.1", "1.2.3.4", "1.2.3.4"},                    // trusted proxy
		{"192.0.2.1", "6.6.6.6, 1.2.3.4", "1.2.3.4"},           // client-supplied hop on the left
		{"192.0.2.1", "6.6.6.6, 1.2.3.4, 10.0.0.5", "1.2.3.4"}, // chain of trusted proxies
		{"192.0.2.1", "not-an-ip", "192.0.2.1"},
		{"192.0.2.1", "", "192.0.2.1"},
	}
	for _, tc := range cases {
		if got := request(tc.peer, tc.forwarded); got != tc.want {
			t.Errorf("peer %s, XFF %q = %q, want %q", tc.peer, tc.forwarded, got, tc.want)
		}
	}
}

func TestLoginTimingDoesNotRevealTheAddress(t *testing.T) {
	srv := testServer(t)
	registered(t, srv, "alice")
	c := newClient(t, srv)
	start := time.Now()
	c.do(http.MethodPost, "/user/login", `{"email":"nobody@example.org","password":"wrong-password"}`, nil)
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("an unknown address answered in %s, without any hashing", elapsed)
	}
}

func TestAPIThrottle(t *testing.T) {
	srv := testServerWith(t, func(cfg *config.Config) { cfg.ThrottleAPI = 5 })
	c := registered(t, srv, "alice")
	var last *httptest.ResponseRecorder
	for i := 0; i < 8; i++ {
		last = c.do(http.MethodGet, "/api/v1/twofaccounts/count", "", nil)
	}
	if last.Code != http.StatusTooManyRequests || last.Header().Get("Retry-After") == "" {
		t.Fatalf("after the limit = %d (Retry-After %q)", last.Code, last.Header().Get("Retry-After"))
	}
}

func TestRequestBodiesAreLimited(t *testing.T) {
	srv := testServer(t)
	c := newClient(t, srv)
	body := `{"email":"a@example.org","password":"` + strings.Repeat("A", 2<<20) + `"}`
	if rec := c.do(http.MethodPost, "/user/login", body, nil); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("declared length = %d", rec.Code)
	}
	// Without a declared length the reader itself stops, and the input is refused.
	req := httptest.NewRequest(http.MethodPost, "/user/login", strings.NewReader(body))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", c.cookie)
	req.Header.Set("X-XSRF-TOKEN", c.xsrf)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK || rec.Code >= 500 {
		t.Errorf("chunked body = %d", rec.Code)
	}
}

func TestAllowListener(t *testing.T) {
	allowed, err := ParseIPSet("127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refused := make(chan string, 1)
	listener := &AllowListener{Listener: inner, Allowed: allowed, OnRefused: func(a string) { refused <- a }}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	// 127.0.0.1 is not in the set: dropped before anything is read.
	conn, err := net.Dial("tcp", inner.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	select {
	case <-refused:
	case <-time.After(2 * time.Second):
		t.Fatal("the connection was not refused")
	}
	// 127.0.0.2 is, the same listener lets it through.
	dialer := net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.2")}}
	good, err := dialer.Dial("tcp", inner.Addr().String())
	if err != nil {
		t.Skipf("cannot bind 127.0.0.2: %v", err)
	}
	defer good.Close()
	select {
	case c := <-accepted:
		c.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("the allowed address was refused")
	}
}

func TestAllowedClientsInTheHandler(t *testing.T) {
	srv := testServerWith(t, func(cfg *config.Config) { cfg.AllowedClientIPs = "192.168.1.118" })
	request := func(peer string, headers map[string]string) int {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/frontend-config", nil)
		r.RemoteAddr = peer
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, r)
		return rec.Code
	}
	if code := request("192.168.1.118:5000", nil); code != http.StatusOK {
		t.Errorf("allowed client = %d", code)
	}
	if code := request("192.168.1.119:5000", nil); code != http.StatusForbidden {
		t.Errorf("other client = %d", code)
	}
	if code := request("192.168.1.119:5000", map[string]string{"X-Forwarded-For": "192.168.1.118"}); code != http.StatusForbidden {
		t.Errorf("forged header = %d", code)
	}
	if code := request("[2001:db8::1]:5000", nil); code != http.StatusForbidden {
		t.Errorf("IPv6 client = %d", code)
	}
}

func TestParseIPSetRefusesTypos(t *testing.T) {
	for _, raw := range []string{"192.168.1.256", "10.0.0.0/33", "host.example.org"} {
		if _, err := ParseIPSet(raw); err == nil {
			t.Errorf("%q was accepted", raw)
		}
	}
	if _, err := New(Options{Config: &config.Config{AllowedClientIPs: "nope"}}); err == nil {
		t.Error("the server started with an unusable allow list")
	}
}

func TestReverseProxyGuard(t *testing.T) {
	if _, err := New(Options{Config: &config.Config{AuthenticationGuard: guardReverseProxy, AuthProxyHeaderForUser: "Remote-User"}}); err == nil {
		t.Fatal("reverse-proxy-guard started without TRUSTED_PROXIES")
	}
	srv := testServerWith(t, func(cfg *config.Config) {
		cfg.AuthenticationGuard = guardReverseProxy
		cfg.TrustedProxies = "10.0.0.5"
		cfg.AuthProxyHeaderForUser = "Remote-User"
		cfg.AuthProxyHeaderForMail = "Remote-Email"
	})
	request := func(peer, method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.RemoteAddr = peer
		r.Header.Set("Remote-User", "proxied")
		r.Header.Set("Remote-Email", "proxied@example.org")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, r)
		return rec
	}
	if rec := request("10.0.0.9:1", http.MethodGet, "/api/v1/user"); rec.Code != http.StatusUnauthorized {
		t.Errorf("forged headers from another peer = %d", rec.Code)
	}
	rec := request("10.0.0.5:1", http.MethodGet, "/api/v1/user")
	if rec.Code != http.StatusOK {
		t.Fatalf("trusted proxy = %d (%s)", rec.Code, rec.Body.String())
	}
	data, _ := jsonBody(t, rec)["data"].(map[string]any)
	if data["email"] != "proxied@example.org" || data["authenticated_by_proxy"] != true {
		t.Errorf("user = %v", data)
	}
	// The proxy's login is ambient: unsafe requests still need the CSRF token.
	if rec := request("10.0.0.5:1", http.MethodDelete, "/api/v1/twofaccounts?ids=1"); rec.Code != statusCSRFMismatch {
		t.Errorf("unsafe request without CSRF = %d", rec.Code)
	}
	c := newClient(t, srv)
	if rec := c.do(http.MethodPost, "/user/login", `{"email":"proxied@example.org","password":"whatever-it-is"}`, nil); rec.Code != http.StatusForbidden {
		t.Errorf("password login = %d", rec.Code)
	}
}

// --- policies ---------------------------------------------------------------

func TestSSOOnlyPolicy(t *testing.T) {
	srv := testServer(t)
	admin := registered(t, srv, "admin")
	bob := registered(t, srv, "bob")
	rec := bob.do(http.MethodPost, "/oauth/personal-access-tokens", `{"name":"t"}`, nil)
	token, _ := jsonBody(t, rec)["accessToken"].(string)
	setSetting(t, admin, "useSsoOnly", true)

	c := newClient(t, srv)
	if rec := c.do(http.MethodPost, "/user/login", `{"email":"bob@example.org","password":"password123"}`, nil); rec.Code != http.StatusForbidden {
		t.Errorf("password login under SSO-only = %d", rec.Code)
	}
	if rec := c.do(http.MethodPost, "/user/login", `{"email":"admin@example.org","password":"password123"}`, nil); rec.Code != http.StatusOK {
		t.Errorf("administrators keep a way in: %d", rec.Code)
	}
	body := `{"name":"new","email":"new@example.org","password":"password123","password_confirmation":"password123"}`
	if rec := newClient(t, srv).do(http.MethodPost, "/user", body, nil); rec.Code != http.StatusForbidden {
		t.Errorf("registration under SSO-only = %d", rec.Code)
	}
	bearer := map[string]string{"Authorization": "Bearer " + token}
	if rec := (&client{t: t, srv: srv}).do(http.MethodGet, "/api/v1/user", "", bearer); rec.Code != http.StatusUnauthorized {
		t.Errorf("token under SSO-only = %d", rec.Code)
	}
	setSetting(t, admin, "allowPatWhileSsoOnly", true)
	if rec := (&client{t: t, srv: srv}).do(http.MethodGet, "/api/v1/user", "", bearer); rec.Code != http.StatusOK {
		t.Errorf("token with allowPatWhileSsoOnly = %d", rec.Code)
	}
}

func TestRestrictRegistrationFailsClosed(t *testing.T) {
	srv := testServer(t)
	admin := registered(t, srv, "admin")
	try := func(email string) int {
		body := fmt.Sprintf(`{"name":%q,"email":%q,"password":"password123","password_confirmation":"password123"}`, email, email)
		return newClient(t, srv).do(http.MethodPost, "/user", body, nil).Code
	}
	setSetting(t, admin, "restrictRegistration", true)
	if code := try("a@evil.test"); code != http.StatusUnprocessableEntity {
		t.Errorf("nothing allowed = %d", code)
	}
	setSetting(t, admin, "restrictList", "only@corp.test|@team.test")
	if code := try("b@evil.test"); code != http.StatusUnprocessableEntity {
		t.Errorf("not listed = %d", code)
	}
	if code := try("only@corp.test"); code != http.StatusCreated {
		t.Errorf("listed = %d", code)
	}
	if code := try("x@team.test"); code != http.StatusCreated {
		t.Errorf("listed domain = %d", code)
	}
	setSetting(t, admin, "restrictRule", `^.*@regex\.test$`)
	if code := try("c@evil.test"); code != http.StatusUnprocessableEntity {
		t.Errorf("regex rule, no match = %d", code)
	}
	if code := try("d@regex.test"); code != http.StatusCreated {
		t.Errorf("regex rule, match = %d", code)
	}
	setSetting(t, admin, "restrictRule", `([`)
	if code := try("e@regex.test"); code != http.StatusUnprocessableEntity {
		t.Errorf("broken regex = %d", code)
	}
	setSetting(t, admin, "restrictRule", "deny")
	if code := try("only@corp.test"); code != http.StatusUnprocessableEntity {
		t.Errorf("deny list = %d", code)
	}
	if code := try("free@evil.test"); code != http.StatusCreated {
		t.Errorf("not in the deny list = %d", code)
	}
}

func TestFirstAdministratorIsUnique(t *testing.T) {
	srv := testServer(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		c := newClient(t, srv)
		wg.Add(1)
		go func(i int, c *client) {
			defer wg.Done()
			body := fmt.Sprintf(`{"name":"u%d","email":"u%d@example.org","password":"password123","password_confirmation":"password123"}`, i, i)
			c.do(http.MethodPost, "/user", body, nil)
		}(i, c)
	}
	wg.Wait()
	admins, err := srv.st.CountAdmins()
	if err != nil {
		t.Fatal(err)
	}
	if admins != 1 {
		t.Fatalf("administrators = %d", admins)
	}
}

// --- secrets and sharing ----------------------------------------------------

func TestSecretsAreEncryptedAtRest(t *testing.T) {
	srv := testServer(t)
	c := registered(t, srv, "alice")
	id := createAccount(t, c)
	var stored string
	if err := srv.st.DB.QueryRow(`SELECT secret FROM twofaccounts WHERE id = ?`, id).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, testSecret) || !srv.enc.IsEncrypted(stored) {
		t.Fatalf("stored secret = %q", stored)
	}
	rec := c.do(http.MethodGet, fmt.Sprintf("/api/v1/twofaccounts/%d", id), "", nil)
	data, _ := jsonBody(t, rec)["data"].(map[string]any)
	if !strings.HasPrefix(data["secret"].(string), testSecret) {
		t.Fatalf("secret read back = %v", data["secret"])
	}

	// Rows written before this protection are sealed at start-up, once.
	if _, err := srv.st.DB.Exec(`UPDATE twofaccounts SET secret = ? WHERE id = ?`, testSecret, id); err != nil {
		t.Fatal(err)
	}
	if n, err := srv.st.EncryptExistingSecrets(); err != nil || n != 1 {
		t.Fatalf("EncryptExistingSecrets = %d, %v", n, err)
	}
	if n, err := srv.st.EncryptExistingSecrets(); err != nil || n != 0 {
		t.Fatalf("second run = %d, %v", n, err)
	}
	a, err := srv.st.GetAccount(id)
	if err != nil || a.Secret != testSecret {
		t.Fatalf("after migration = %q, %v", a.Secret, err)
	}
	// useEncryption cannot be turned off.
	admin := c
	setSetting(t, admin, "useEncryption", false)
	if srv.settings.Bool("useEncryption") != true {
		t.Error("useEncryption was turned off")
	}
}

func TestBorrowerCannotReadTheSecret(t *testing.T) {
	srv := testServer(t)
	owner := registered(t, srv, "owner")
	bob := registered(t, srv, "bob")
	setSetting(t, owner, "enableSharing", true)
	id := createAccount(t, owner)
	share := fmt.Sprintf(`{"user_ids":[%d]}`, userID(t, srv, "bob"))
	if rec := owner.do(http.MethodPost, fmt.Sprintf("/api/v1/twofaccounts/%d/shares", id), share, nil); rec.Code != http.StatusCreated {
		t.Fatalf("share = %d (%s)", rec.Code, rec.Body.String())
	}
	base := fmt.Sprintf("/api/v1/twofaccounts/%d", id)
	if rec := bob.do(http.MethodGet, base+"/otp", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("a recipient generates codes: %d", rec.Code)
	}
	for _, path := range []string{
		fmt.Sprintf("/api/v1/twofaccounts/export?ids=%d", id),
		fmt.Sprintf("/api/v1/twofaccounts/export?ids=%d&otpauth=1", id),
	} {
		rec := bob.do(http.MethodGet, path, "", nil)
		if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), testSecret) {
			t.Errorf("%s = %d (%s)", path, rec.Code, rec.Body.String())
		}
	}
	body := fmt.Sprintf(`{"ids":"%d","passphrase":"correct horse battery"}`, id)
	if rec := bob.do(http.MethodPost, "/api/v1/twofaccounts/export", body, nil); rec.Code != http.StatusForbidden {
		t.Errorf("encrypted export = %d", rec.Code)
	}
	for _, path := range []string{base, base + "?withSecret=1"} {
		if rec := bob.do(http.MethodGet, path, "", nil); strings.Contains(rec.Body.String(), testSecret) {
			t.Errorf("%s leaks the secret", path)
		}
	}
	rec := bob.do(http.MethodGet, "/api/v1/twofaccounts?withSecret=1", "", nil)
	if strings.Contains(rec.Body.String(), testSecret) {
		t.Error("the listing leaks the secret")
	}
	// The all-users scope must also be reported as borrowed.
	list, _ := jsonBody(t, rec)["data"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["is_borrowed"] != true {
		t.Errorf("listing = %v", list)
	}

	setSetting(t, owner, "enableSharing", false)
	if rec := bob.do(http.MethodGet, base, "", nil); rec.Code != http.StatusForbidden {
		t.Errorf("show after disabling sharing = %d", rec.Code)
	}
	if rec := bob.do(http.MethodGet, base+"/otp", "", nil); rec.Code != http.StatusForbidden {
		t.Errorf("otp after disabling sharing = %d", rec.Code)
	}
}

func TestBorrowerCannotComputeFutureCodes(t *testing.T) {
	srv := testServer(t)
	owner := registered(t, srv, "owner")
	id := createAccount(t, owner)
	future := time.Now().Add(90 * 24 * time.Hour).Unix()
	rec := owner.do(http.MethodGet, fmt.Sprintf("/api/v1/twofaccounts/%d?withOtp=1&at=%d", id, future), "", nil)
	data, _ := jsonBody(t, rec)["data"].(map[string]any)
	code := data["otp"].(map[string]any)["password"]
	forFuture, err := otpgen.Code(otpgen.Account{Type: otpgen.TypeTOTP, Secret: testSecret, Digits: 6}, future)
	if err != nil {
		t.Fatal(err)
	}
	if code == forFuture {
		t.Fatal("a code 90 days ahead was computed")
	}
	logs, err := srv.st.ListOtpLogs(id, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Errorf("show?withOtp logged %d generations, want 1", len(logs))
	}
}

func TestRecipientsNeedSharing(t *testing.T) {
	srv := testServer(t)
	owner := registered(t, srv, "owner")
	registered(t, srv, "bob")
	id := createAccount(t, owner)
	path := fmt.Sprintf("/api/v1/twofaccounts/%d/recipients", id)
	if rec := owner.do(http.MethodGet, path, "", nil); rec.Code != http.StatusForbidden {
		t.Errorf("recipients without sharing = %d", rec.Code)
	}
	setSetting(t, owner, "enableSharing", true)
	if rec := owner.do(http.MethodGet, path, "", nil); rec.Code != http.StatusOK {
		t.Errorf("recipients with sharing = %d", rec.Code)
	}
}

func TestAllUsersShareDoesNotComeBack(t *testing.T) {
	srv := testServer(t)
	owner := registered(t, srv, "owner")
	bob := registered(t, srv, "bob")
	id := createAccount(t, owner)
	setSetting(t, owner, "enableSharing", true)
	base := fmt.Sprintf("/api/v1/twofaccounts/%d", id)
	if rec := owner.do(http.MethodPost, base+"/shares/all", "", nil); rec.Code != http.StatusForbidden {
		t.Errorf("share with all while the scope is off = %d", rec.Code)
	}
	setSetting(t, owner, "enableAllUsersSharingScope", true)
	if rec := owner.do(http.MethodPost, base+"/shares/all", "", nil); rec.Code != http.StatusCreated {
		t.Fatalf("share with all = %d", rec.Code)
	}
	if rec := bob.do(http.MethodGet, base, "", nil); rec.Code != http.StatusOK {
		t.Fatalf("all-users recipient = %d", rec.Code)
	}
	setSetting(t, owner, "enableAllUsersSharingScope", false)
	owner.do(http.MethodDelete, base+"/shares", "", nil)
	setSetting(t, owner, "enableAllUsersSharingScope", true)
	if rec := bob.do(http.MethodGet, base, "", nil); rec.Code != http.StatusForbidden {
		t.Errorf("a removed share came back: %d", rec.Code)
	}
}

// --- OTP --------------------------------------------------------------------

func TestUnusableURIsAreRefused(t *testing.T) {
	srv := testServer(t)
	c := registered(t, srv, "alice")
	for _, uri := range []string{
		"otpauth://totp/Evil:x?secret=" + testSecret + "&digits=32",
		"otpauth://totp/Evil:x?secret=!!!notbase32",
		"otpauth://totp/Evil:x?secret=" + testSecret + "&period=0",
	} {
		raw, _ := json.Marshal(map[string]string{"uri": uri})
		if rec := c.do(http.MethodPost, "/api/v1/twofaccounts", string(raw), nil); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s = %d", uri, rec.Code)
		}
	}
}

func TestMD5AccountsNeverBreakTheListing(t *testing.T) {
	srv := testServer(t)
	c := registered(t, srv, "alice")
	rec := c.do(http.MethodPost, "/api/v1/twofaccounts", `{"otp_type":"hotp","account":"m","secret":"`+testSecret+`","algorithm":"md5"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d", rec.Code)
	}
	data, _ := jsonBody(t, rec)["data"].(map[string]any)
	path := fmt.Sprintf("/api/v1/twofaccounts/%d/otp", int64(data["id"].(float64)))
	for i := 0; i < 64; i++ {
		if rec := c.do(http.MethodGet, path, "", nil); rec.Code != http.StatusOK {
			t.Fatalf("code %d = %d", i, rec.Code)
		}
	}
}

func TestHOTPCodesAreNeverHandedOutTwice(t *testing.T) {
	srv := testServer(t)
	c := registered(t, srv, "alice")
	rec := c.do(http.MethodPost, "/api/v1/twofaccounts", `{"otp_type":"hotp","account":"h","secret":"`+testSecret+`"}`, nil)
	data, _ := jsonBody(t, rec)["data"].(map[string]any)
	path := fmt.Sprintf("/api/v1/twofaccounts/%d/otp", int64(data["id"].(float64)))
	const workers = 8
	codes := make(chan string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker := &client{t: t, srv: srv, cookie: c.cookie, xsrf: c.xsrf}
			rec := worker.do(http.MethodGet, path, "", nil)
			var out map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &out)
			password, _ := out["password"].(string)
			codes <- password
		}()
	}
	wg.Wait()
	close(codes)
	seen := map[string]bool{}
	for code := range codes {
		if code == "" || seen[code] {
			t.Fatalf("code %q was handed out twice or is empty", code)
		}
		seen[code] = true
	}
}

// --- administration ---------------------------------------------------------

func TestNumericSettingsSurvive(t *testing.T) {
	srv := testServer(t)
	admin := registered(t, srv, "admin")
	setSetting(t, admin, "lastRadarScan", 12345)
	if got := srv.settings.Get("lastRadarScan"); got != 12345 {
		t.Fatalf("lastRadarScan = %v", got)
	}
}

func TestProtectedSettingsNamespaces(t *testing.T) {
	srv := testServer(t)
	admin := registered(t, srv, "admin")
	for _, name := range []string{"openid.client_secret", "mail.password"} {
		if rec := admin.do(http.MethodPut, "/api/v1/settings/"+name, `{"value":"x"}`, nil); rec.Code != http.StatusNotFound {
			t.Errorf("PUT %s = %d", name, rec.Code)
		}
		if rec := admin.do(http.MethodGet, "/api/v1/settings/"+name, "", nil); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d", name, rec.Code)
		}
	}
}

func TestWebAuthnCredentialRoutes(t *testing.T) {
	srv := testServer(t)
	c := registered(t, srv, "alice")
	if _, err := srv.st.DB.Exec(`INSERT INTO webauthn_credentials (id, user_id, alias, counter, rp_id, origin, public_key) VALUES ('cred1', ?, 'k', 0, 'r', 'o', 'p')`,
		strconv.FormatInt(userID(t, srv, "alice"), 10)); err != nil {
		t.Fatal(err)
	}
	if rec := c.do(http.MethodPatch, "/webauthn/credentials/cred1/name", `{"name":"key"}`, nil); rec.Code != http.StatusOK {
		t.Errorf("rename = %d (%s)", rec.Code, rec.Body.String())
	}
	if rec := c.do(http.MethodDelete, "/webauthn/credentials/cred1", "", nil); rec.Code != http.StatusOK {
		t.Errorf("delete = %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestMissingPublicFilesAre404(t *testing.T) {
	srv := testServerWith(t, func(cfg *config.Config) { cfg.AssetsPath = filepath.Join(cfg.BasePath, "absent") })
	if rec := (&client{t: t, srv: srv}).do(http.MethodGet, "/favicon.ico", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("favicon = %d", rec.Code)
	}
}

func TestReactPanelMounting(t *testing.T) {
	dist := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dist, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	page := `<!doctype html><html><head><link rel="icon" href="./favicon.svg" /><script src="./assets/a.js"></script></head></html>`
	_ = os.WriteFile(filepath.Join(dist, "index.html"), []byte(page), 0o644)
	_ = os.WriteFile(filepath.Join(dist, "assets", "a.js"), []byte("1"), 0o644)

	// Relative paths work.
	wd, _ := os.Getwd()
	if err := os.Chdir(filepath.Dir(dist)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()
	relative := "./" + filepath.Base(dist)
	srv := testServerWith(t, func(cfg *config.Config) { cfg.UIPath = relative; cfg.URL = "https://app.test" })
	anon := &client{t: t, srv: srv}
	if rec := anon.do(http.MethodGet, "/assets/a.js", "", nil); rec.Code != http.StatusOK {
		t.Errorf("asset with a relative UI path = %d", rec.Code)
	}
	// A deep client-side route still reaches the root-relative bundle.
	if rec := anon.do(http.MethodGet, "/settings/deep", "", nil); !strings.Contains(rec.Body.String(), `src="/assets/a.js"`) {
		t.Errorf("index = %s", rec.Body.String())
	}

	// Under a sub-directory every URL is prefixed and the panel is told its base.
	sub := testServerWith(t, func(cfg *config.Config) {
		cfg.UIPath = dist
		cfg.Subdirectory = "2fauth"
		cfg.URL = "https://app.test/2fauth"
	})
	anon = &client{t: t, srv: sub}
	rec := anon.do(http.MethodGet, "/2fauth/login", "", nil)
	for _, want := range []string{`src="/2fauth/assets/a.js"`, `href="/2fauth/favicon.svg"`, `<meta name="2fauth-base" content="/2fauth"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("missing %q in %s", want, rec.Body.String())
		}
	}
	if rec := anon.do(http.MethodGet, "/2fauth/assets/a.js", "", nil); rec.Code != http.StatusOK {
		t.Errorf("asset under the sub-directory = %d", rec.Code)
	}
	if rec := anon.do(http.MethodGet, "/outside", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("outside the sub-directory = %d", rec.Code)
	}
}

// --- OpenID Connect ---------------------------------------------------------

func ssoLogin(t *testing.T, c *client) string {
	t.Helper()
	state, _ := startLogin(t, c)
	rec := c.do(http.MethodGet, "/socialite/callback/openid?code=test&state="+state, "", nil)
	return rec.Header().Get("Location")
}

func ssoServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	oidc.ResetCache()
	srv := testServer(t)
	idp := fakeIdP(t)
	enableOpenID(srv, idp.URL, "s3cr3t-keep-private")
	return srv, idp
}

func TestUnverifiedEmailDoesNotTakeOverAnAccount(t *testing.T) {
	srv, _ := ssoServer(t)
	registered(t, srv, "admin")
	testUserClaims = map[string]any{"sub": "attacker", "email": "ADMIN@example.org", "email_verified": false, "name": "x"}
	c := newClient(t, srv)
	if location := ssoLogin(t, c); location != "/login?sso_error=email_unverified" {
		t.Fatalf("location = %q", location)
	}
	if rec := c.do(http.MethodGet, "/api/v1/user", "", nil); rec.Code == http.StatusOK {
		t.Fatal("the attacker is signed in")
	}
	delete(testUserClaims, "email_verified")
	if location := ssoLogin(t, newClient(t, srv)); location != "/login?sso_error=email_unverified" {
		t.Fatalf("no claim at all: location = %q", location)
	}
	// Providers that never send the claim need an explicit decision of the administrator.
	srv.cfg.OpenIDTrustUnverifiedEmail = true
	if location := ssoLogin(t, newClient(t, srv)); location != "/" {
		t.Fatalf("with the opt-in: location = %q", location)
	}
}

func TestPreRegistrationCannotHijackAnSSOAccount(t *testing.T) {
	srv, _ := ssoServer(t)
	registered(t, srv, "admin")
	// The attacker registers the address of the future SSO user.
	attacker := registered(t, srv, "victim")
	token := jsonBody(t, attacker.do(http.MethodPost, "/oauth/personal-access-tokens", `{"name":"t"}`, nil))["accessToken"].(string)

	testUserClaims = map[string]any{"sub": "victim-sub", "email": "victim@example.org", "email_verified": true}
	victim := newClient(t, srv)
	if location := ssoLogin(t, victim); location != "/" {
		t.Fatalf("location = %q", location)
	}
	// None of what the attacker held still works.
	if rec := newClient(t, srv).do(http.MethodPost, "/user/login", `{"email":"victim@example.org","password":"password123"}`, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("attacker password = %d", rec.Code)
	}
	if code := replay(t, srv, attacker.cookie); code != http.StatusUnauthorized {
		t.Errorf("attacker session = %d", code)
	}
	if rec := (&client{t: t, srv: srv}).do(http.MethodGet, "/api/v1/user", "", map[string]string{"Authorization": "Bearer " + token}); rec.Code != http.StatusUnauthorized {
		t.Errorf("attacker token = %d", rec.Code)
	}
	if rec := victim.do(http.MethodGet, "/api/v1/user", "", nil); rec.Code != http.StatusOK {
		t.Errorf("victim = %d", rec.Code)
	}
}

func TestVerifiedLocalAccountKeepsItsPasswordWhenLinked(t *testing.T) {
	srv, _ := ssoServer(t)
	registered(t, srv, "admin")
	registered(t, srv, "real")
	if err := srv.st.MarkEmailVerified(userID(t, srv, "real")); err != nil {
		t.Fatal(err)
	}
	testUserClaims = map[string]any{"sub": "real-sub", "email": "real@example.org", "email_verified": true}
	if location := ssoLogin(t, newClient(t, srv)); location != "/" {
		t.Fatalf("location = %q", location)
	}
	signedIn(t, srv, "real", testPassword)
}

func TestBadIDTokensAreRefused(t *testing.T) {
	srv, idp := ssoServer(t)
	registered(t, srv, "admin")
	cases := map[string]func(map[string]any){
		"wrong nonce":    func(c map[string]any) { c["nonce"] = "other" },
		"no nonce":       func(c map[string]any) { delete(c, "nonce") },
		"wrong audience": func(c map[string]any) { c["aud"] = "other-client" },
		"wrong issuer":   func(c map[string]any) { c["iss"] = "https://evil.test" },
		"expired":        func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() },
	}
	for name, change := range cases {
		testIDTokenPatch = change
		c := newClient(t, srv)
		if location := ssoLogin(t, c); location != "/login?sso_error=token" {
			t.Errorf("%s: location = %q", name, location)
		}
		if rec := c.do(http.MethodGet, "/api/v1/user", "", nil); rec.Code == http.StatusOK {
			t.Errorf("%s: signed in", name)
		}
	}
	testIDTokenPatch = nil

	// alg:none and a signature from another key.
	unsigned := func(claims map[string]any) string {
		raw, _ := json.Marshal(claims)
		return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + base64.RawURLEncoding.EncodeToString(raw) + "."
	}
	testIDTokenSigner = unsigned
	if location := ssoLogin(t, newClient(t, srv)); location != "/login?sso_error=token" {
		t.Errorf("alg none: location = %q", location)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	_ = other
	testIDTokenSigner = func(claims map[string]any) string {
		raw, _ := json.Marshal(claims)
		return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"ES256","kid":"test-key"}`)) + "." +
			base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(make([]byte, 64))
	}
	if location := ssoLogin(t, newClient(t, srv)); location != "/login?sso_error=token" {
		t.Errorf("forged signature: location = %q", location)
	}
	testIDTokenSigner = nil

	// Userinfo answering for somebody else.
	testUserClaims = map[string]any{"sub": "idp-user-1", "email": "x@example.org", "email_verified": true}
	testIDTokenPatch = func(c map[string]any) { c["sub"] = "somebody-else" }
	if location := ssoLogin(t, newClient(t, srv)); location != "/login?sso_error=userinfo" {
		t.Errorf("subject mismatch: location = %q", location)
	}
	_ = idp
}

func TestSSORegistrationFollowsThePolicy(t *testing.T) {
	srv, _ := ssoServer(t)
	admin := registered(t, srv, "admin")
	setSetting(t, admin, "restrictRegistration", true)
	setSetting(t, admin, "restrictRule", "allow")
	setSetting(t, admin, "restrictList", "only@corp.test")

	testUserClaims = map[string]any{"sub": "out", "email": "outsider@evil.test", "email_verified": true}
	if location := ssoLogin(t, newClient(t, srv)); location != "/login?sso_error=register_disabled" {
		t.Errorf("restricted: location = %q", location)
	}
	testUserClaims = map[string]any{"sub": "in", "email": "only@corp.test", "email_verified": true}
	if location := ssoLogin(t, newClient(t, srv)); location != "/" {
		t.Errorf("allowed: location = %q", location)
	}
	setSetting(t, admin, "restrictRegistration", false)
	setSetting(t, admin, "disableRegistration", true)
	testUserClaims = map[string]any{"sub": "new", "email": "new@corp.test", "email_verified": true}
	if location := ssoLogin(t, newClient(t, srv)); location != "/login?sso_error=register_disabled" {
		t.Errorf("registration closed: location = %q", location)
	}
	setSetting(t, admin, "keepSsoRegistrationEnabled", true)
	if location := ssoLogin(t, newClient(t, srv)); location != "/" {
		t.Errorf("keepSsoRegistrationEnabled: location = %q", location)
	}
	user, err := srv.st.GetUserByEmail("new@corp.test")
	if err != nil {
		t.Fatal(err)
	}
	if user.OauthProvider != "openid" || user.EmailVerifiedAt == nil {
		t.Errorf("SSO user = %+v", user)
	}
}

func TestSSOUserConfirmsWithARecentSignIn(t *testing.T) {
	srv, _ := ssoServer(t)
	registered(t, srv, "admin")
	testUserClaims = map[string]any{"sub": "sso", "email": "sso@corp.test", "email_verified": true}
	c := newClient(t, srv)
	if location := ssoLogin(t, c); location != "/" {
		t.Fatalf("location = %q", location)
	}
	if _, err := srv.st.DB.Exec(`UPDATE user_sessions SET created_at = ?`, time.Now().Add(-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if rec := c.do(http.MethodDelete, "/user", `{}`, nil); rec.Code != http.StatusForbidden {
		t.Errorf("stale session = %d", rec.Code)
	}
	if _, err := srv.st.DB.Exec(`UPDATE user_sessions SET created_at = ?`, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if rec := c.do(http.MethodDelete, "/user", `{}`, nil); rec.Code != http.StatusNoContent {
		t.Errorf("fresh session = %d (%s)", rec.Code, rec.Body.String())
	}
}

// --- e-mail -----------------------------------------------------------------

// mailbox is a minimal SMTP server recording what it receives.
type mailbox struct {
	mu       sync.Mutex
	messages []string
	listener net.Listener
}

func startMailbox(t *testing.T) *mailbox {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	box := &mailbox{listener: listener}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go box.serve(conn)
		}
	}()
	return box
}

func (b *mailbox) serve(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	fmt.Fprint(conn, "220 mailbox\r\n")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		switch command := strings.ToUpper(strings.TrimSpace(line)); {
		case command == "DATA":
			fmt.Fprint(conn, "354 go\r\n")
			var body strings.Builder
			for {
				l, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				body.WriteString(strings.TrimPrefix(l, "."))
			}
			b.mu.Lock()
			b.messages = append(b.messages, body.String())
			b.mu.Unlock()
			fmt.Fprint(conn, "250 ok\r\n")
		case command == "QUIT":
			fmt.Fprint(conn, "221 bye\r\n")
			return
		default:
			fmt.Fprint(conn, "250 ok\r\n")
		}
	}
}

func (b *mailbox) all() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.messages...)
}

type receivedMail struct {
	Subject, To, Text string
}

func readMail(t *testing.T, raw string) receivedMail {
	t.Helper()
	msg, err := netmail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parse mail: %v", err)
	}
	subject, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(msg.Body, params["boundary"])
	part, err := reader.NextRawPart()
	if err != nil {
		t.Fatal(err)
	}
	var text bytes.Buffer
	if _, err := text.ReadFrom(quotedprintable.NewReader(part)); err != nil {
		t.Fatal(err)
	}
	return receivedMail{Subject: subject, To: msg.Header.Get("To"), Text: text.String()}
}

func mailServer(t *testing.T) (*Server, *mailbox) {
	t.Helper()
	box := startMailbox(t)
	addr := box.listener.Addr().(*net.TCPAddr)
	srv := testServerWith(t, func(cfg *config.Config) {
		cfg.MailMailer = "smtp"
		cfg.MailHost = "127.0.0.1"
		cfg.MailPort = addr.Port
		cfg.MailEncryption = "none"
		cfg.MailFromAddr = "no-reply@example.org"
		cfg.MailFromName = "2FAuth"
		cfg.MailLocale = "en"
		cfg.URL = "https://app.example.org"
	})
	return srv, box
}

var linkPattern = regexp.MustCompile(`https://app\.example\.org/[^\s]+`)

func TestPasswordResetFlow(t *testing.T) {
	srv, box := mailServer(t)
	c := registered(t, srv, "alice")
	other := signedIn(t, srv, "alice", testPassword)
	srv.WaitForMail()
	start := len(box.all())

	// An unknown address gets the same answer, and no mail.
	anon := newClient(t, srv)
	if rec := anon.do(http.MethodPost, "/user/password/lost", `{"email":"nobody@example.org"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("unknown address = %d", rec.Code)
	}
	srv.WaitForMail()
	if got := len(box.all()); got != start {
		t.Fatalf("mails after an unknown address = %d", got-start)
	}

	if rec := anon.do(http.MethodPost, "/user/password/lost", `{"email":"alice@example.org"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("lost = %d", rec.Code)
	}
	srv.WaitForMail()
	mails := box.all()
	if len(mails) != start+1 {
		t.Fatalf("mails = %d", len(mails)-start)
	}
	mail := readMail(t, mails[len(mails)-1])
	if !strings.Contains(mail.Subject, "password") || !strings.Contains(mail.To, "alice@example.org") {
		t.Fatalf("mail = %+v", mail)
	}
	link, err := url.Parse(linkPattern.FindString(mail.Text))
	if err != nil {
		t.Fatal(err)
	}
	token := link.Query().Get("token")
	if token == "" || link.Path != "/password/reset" {
		t.Fatalf("link = %v", link)
	}
	var stored string
	if err := srv.st.DB.QueryRow(`SELECT token FROM password_resets WHERE email = ?`, "alice@example.org").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token {
		t.Fatal("the reset token is stored in clear text")
	}

	reset := func(token string) int {
		raw, _ := json.Marshal(map[string]string{"email": "alice@example.org", "token": token, "password": "brand-new-password", "password_confirmation": "brand-new-password"})
		return newClient(t, srv).do(http.MethodPost, "/user/password/reset", string(raw), nil).Code
	}
	if code := reset("wrong-token"); code != http.StatusUnprocessableEntity {
		t.Errorf("wrong token = %d", code)
	}
	if code := reset(token); code != http.StatusOK {
		t.Fatalf("reset = %d", code)
	}
	if code := reset(token); code != http.StatusUnprocessableEntity {
		t.Errorf("token used twice = %d", code)
	}
	signedIn(t, srv, "alice", "brand-new-password")
	for _, session := range []*client{c, other} {
		if code := replay(t, srv, session.cookie); code != http.StatusUnauthorized {
			t.Errorf("session after the reset = %d", code)
		}
	}
	u, _ := srv.st.GetUserByEmail("alice@example.org")
	if u.EmailVerifiedAt == nil {
		t.Error("following the link did not verify the address")
	}
	srv.WaitForMail()
	last := readMail(t, box.all()[len(box.all())-1])
	if !strings.Contains(last.Subject, "changed") {
		t.Errorf("no confirmation of the change: %+v", last)
	}
}

func TestExpiredResetTokenIsRefused(t *testing.T) {
	srv, _ := mailServer(t)
	registered(t, srv, "alice")
	if err := srv.st.SavePasswordReset("alice@example.org", crypt.SHA256Hex("tok")); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-2 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := srv.st.DB.Exec(`UPDATE password_resets SET created_at = ?`, old); err != nil {
		t.Fatal(err)
	}
	body := `{"email":"alice@example.org","token":"tok","password":"brand-new-password","password_confirmation":"brand-new-password"}`
	if rec := newClient(t, srv).do(http.MethodPost, "/user/password/reset", body, nil); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expired token = %d", rec.Code)
	}
}

func TestEmailVerificationFlow(t *testing.T) {
	srv, box := mailServer(t)
	c := registered(t, srv, "alice")
	srv.WaitForMail()
	mails := box.all()
	if len(mails) == 0 {
		t.Fatal("no verification mail at registration")
	}
	text := readMail(t, mails[0])
	link, err := url.Parse(linkPattern.FindString(text.Text))
	if err != nil {
		t.Fatal(err)
	}
	if link.Path != "/user/email/verify" {
		t.Fatalf("link = %v", link)
	}
	rec := (&client{t: t, srv: srv}).do(http.MethodGet, link.RequestURI(), "", nil)
	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "email_verified=ok") {
		t.Fatalf("verify = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	u, _ := srv.st.GetUserByEmail("alice@example.org")
	if u.EmailVerifiedAt == nil {
		t.Fatal("not verified")
	}
	// A new address loses the mark, and the old link does not apply to it.
	body := `{"name":"alice","email":"new@example.org","password":"password123"}`
	if rec := c.do(http.MethodPut, "/user", body, nil); rec.Code != http.StatusOK {
		t.Fatalf("update = %d (%s)", rec.Code, rec.Body.String())
	}
	u, _ = srv.st.GetUserByEmail("new@example.org")
	if u.EmailVerifiedAt != nil {
		t.Fatal("the new address is considered verified")
	}
	rec = (&client{t: t, srv: srv}).do(http.MethodGet, link.RequestURI(), "", nil)
	if !strings.Contains(rec.Header().Get("Location"), "email_verified=invalid") {
		t.Errorf("stale link = %q", rec.Header().Get("Location"))
	}
	// A session cookie or anything else encrypted is not a verification token.
	rec = (&client{t: t, srv: srv}).do(http.MethodGet, "/user/email/verify?token=garbage", "", nil)
	if !strings.Contains(rec.Header().Get("Location"), "email_verified=invalid") {
		t.Errorf("garbage token = %q", rec.Header().Get("Location"))
	}
}

func TestSecurityNotifications(t *testing.T) {
	srv, box := mailServer(t)
	owner := registered(t, srv, "owner")
	bob := registered(t, srv, "bob")
	srv.WaitForMail()

	subjects := func() string {
		srv.WaitForMail()
		var out []string
		for _, raw := range box.all() {
			out = append(out, readMail(t, raw).Subject)
		}
		return strings.Join(out, "\n")
	}

	// Sharing and transfers tell the recipient.
	setSetting(t, owner, "enableSharing", true)
	id := createAccount(t, owner)
	share := fmt.Sprintf(`{"user_ids":[%d]}`, userID(t, srv, "bob"))
	owner.do(http.MethodPost, fmt.Sprintf("/api/v1/twofaccounts/%d/shares", id), share, nil)
	if got := subjects(); !strings.Contains(got, "owner shared GitHub") {
		t.Errorf("no share mail in:\n%s", got)
	}
	transfer := fmt.Sprintf(`{"new_owner_id":%d,"confirm_password":"password123"}`, userID(t, srv, "bob"))
	if rec := owner.do(http.MethodPatch, fmt.Sprintf("/api/v1/twofaccounts/%d/owner", id), transfer, nil); rec.Code != http.StatusOK {
		t.Fatalf("transfer = %d (%s)", rec.Code, rec.Body.String())
	}
	if got := subjects(); !strings.Contains(got, "owner transferred GitHub") {
		t.Errorf("no transfer mail in:\n%s", got)
	}

	// Sign-in alerts follow the preferences.
	if rec := bob.do(http.MethodPut, "/api/v1/user/preferences/notifyOnNewAuthDevice", `{"value":true}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("preference = %d", rec.Code)
	}
	if rec := bob.do(http.MethodPut, "/api/v1/user/preferences/notifyOnFailedLogin", `{"value":true}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("preference = %d", rec.Code)
	}
	browser := map[string]string{"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) Firefox/140.0"}
	c := newClient(t, srv)
	if rec := c.do(http.MethodPost, "/user/login", `{"email":"bob@example.org","password":"password123"}`, browser); rec.Code != http.StatusOK {
		t.Fatalf("login = %d", rec.Code)
	}
	if got := subjects(); !strings.Contains(got, "New sign-in") {
		t.Errorf("no new device mail in:\n%s", got)
	}
	before := strings.Count(subjects(), "New sign-in")
	newClient(t, srv).do(http.MethodPost, "/user/login", `{"email":"bob@example.org","password":"password123"}`, browser)
	if after := strings.Count(subjects(), "New sign-in"); after != before {
		t.Errorf("a known device triggered a new mail: %d -> %d", before, after)
	}

	// A failed sign-in alerts once, however many attempts follow.
	for i := 0; i < 3; i++ {
		newClient(t, srv).do(http.MethodPost, "/user/login", `{"email":"bob@example.org","password":"wrong-password"}`, nil)
	}
	if got := strings.Count(subjects(), "Failed sign-in"); got != 1 {
		t.Errorf("failed sign-in mails = %d", got)
	}
}

func TestLostPasswordCannotFloodAMailbox(t *testing.T) {
	srv, box := mailServer(t)
	registered(t, srv, "alice")
	srv.WaitForMail()
	start := len(box.all())
	for i := 0; i < 8; i++ {
		newClient(t, srv).do(http.MethodPost, "/user/password/lost", `{"email":"alice@example.org"}`, nil)
	}
	srv.WaitForMail()
	if sent := len(box.all()) - start; sent > 3 {
		t.Errorf("mails sent = %d", sent)
	}
}

func TestMailAdminEndpoints(t *testing.T) {
	box := startMailbox(t)
	srv := testServerWith(t, func(cfg *config.Config) {
		cfg.MailMailer = "log"
		cfg.MailHost = ""
		cfg.MailPassword = "env-password"
		cfg.URL = "https://app.example.org"
	})
	admin := registered(t, srv, "admin")
	bob := registered(t, srv, "bob")
	if rec := bob.do(http.MethodGet, "/api/v1/mail/settings", "", nil); rec.Code != http.StatusForbidden {
		t.Errorf("non-admin = %d", rec.Code)
	}
	rec := admin.do(http.MethodPost, "/system/test-email", "", nil)
	if rec.Code != http.StatusFailedDependency {
		t.Errorf("no transport = %d", rec.Code)
	}
	if rec := admin.do(http.MethodPut, "/api/v1/mail/settings", `{"mailer":"carrier-pigeon"}`, nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad mailer = %d", rec.Code)
	}
	addr := box.listener.Addr().(*net.TCPAddr)
	body := fmt.Sprintf(`{"mailer":"smtp","host":"127.0.0.1","port":%d,"encryption":"none","from_address":"no-reply@example.org","from_name":"Vault","password":"s3cr3t-smtp","username":""}`, addr.Port)
	rec = admin.do(http.MethodPut, "/api/v1/mail/settings", body, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("save = %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "s3cr3t-smtp") || strings.Contains(rec.Body.String(), "env-password") {
		t.Fatalf("the password was returned: %s", rec.Body.String())
	}
	payload := jsonBody(t, rec)
	if payload["password_set"] != true || payload["enabled"] != true {
		t.Errorf("payload = %v", payload)
	}
	var stored string
	if err := srv.st.DB.QueryRow(`SELECT value FROM options WHERE "key" = ?`, "mail.password").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "s3cr3t-smtp") {
		t.Fatalf("the SMTP password is stored in clear text: %q", stored)
	}
	if srv.mailSettings().Password != "s3cr3t-smtp" {
		t.Error("the stored password cannot be read back")
	}
	rec = admin.do(http.MethodPost, "/system/test-email", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("test mail = %d (%s)", rec.Code, rec.Body.String())
	}
	mails := box.all()
	received := readMail(t, mails[len(mails)-1])
	if !strings.Contains(received.To, "admin@example.org") || !strings.Contains(received.Subject, "SMTP") {
		t.Errorf("mail = %+v", received)
	}
	if rec := admin.do(http.MethodPut, "/api/v1/mail/settings", `{"clear_password":true}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("clear = %d", rec.Code)
	}
	if srv.mailSettings().Password != "" {
		t.Error("clearing the password fell back to the environment")
	}
	cfg := jsonBody(t, (&client{t: t, srv: srv}).do(http.MethodGet, "/api/v1/frontend-config", "", nil))
	if cfg["mailEnabled"] != true {
		t.Errorf("frontend-config = %v", cfg)
	}
}
