package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"2fauth/internal/config"
	"2fauth/internal/crypt"
	"2fauth/internal/db"
	"2fauth/internal/services"
	"2fauth/internal/store"
)

// testServer builds a Server backed by a temporary SQLite database.
func testServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()

	cfg := config.Load(dir)
	cfg.Key = "base64:AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	cfg.Version = config.AppVersionDefault
	cfg.SessionLifetime = 120
	// Minimal front-end bundle so that the landing page can be rendered.
	assets := filepath.Join(dir, "public")
	if err := os.MkdirAll(filepath.Join(assets, "build", "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	manifest := `{"resources/js/app.js":{"file":"assets/app.js","src":"resources/js/app.js","isEntry":true}}`
	if err := os.WriteFile(filepath.Join(assets, "build", "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assets, "build", "assets", "app.js"), []byte("/* app */"), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	cfg.AssetsPath = assets

	conn, err := db.Open("sqlite", filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	st := store.New(conn, cfg)
	enc, err := crypt.New(cfg.Key)
	if err != nil {
		t.Fatalf("crypt.New: %v", err)
	}
	srv, err := New(Options{
		Config:   cfg,
		Store:    st,
		Settings: services.NewSettingService(st, cfg),
		Enc:      enc,
	})
	if err != nil {
		t.Fatalf("httpapi.New: %v", err)
	}
	return srv
}

// client drives the server through httptest, keeping cookies between calls.
type client struct {
	t      *testing.T
	srv    *Server
	cookie string
	xsrf   string
}

func (c *client) do(method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	c.t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}
	if c.xsrf != "" {
		req.Header.Set("X-XSRF-TOKEN", c.xsrf)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	c.srv.ServeHTTP(rec, req)

	var cookies []string
	for _, sc := range rec.Result().Cookies() {
		if sc.Name == xsrfCookieName {
			c.xsrf = sc.Value
		}
		if sc.Value != "" {
			cookies = append(cookies, sc.Name+"="+sc.Value)
		}
	}
	if len(cookies) > 0 {
		c.cookie = strings.Join(cookies, "; ")
	}
	return rec
}

func jsonBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON response %q: %v", rec.Body.String(), err)
	}
	return out
}

func TestRegisterLoginAndOtpFlow(t *testing.T) {
	srv := testServer(t)
	c := &client{t: t, srv: srv}

	// The landing page issues the session and the CSRF cookie.
	if rec := c.do(http.MethodGet, "/", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d", rec.Code)
	}
	if c.xsrf == "" {
		t.Fatal("no XSRF cookie issued by the landing page")
	}

	// Register the first user: it must become the administrator.
	rec := c.do(http.MethodPost, "/user",
		`{"name":"alice","email":"alice@example.com","password":"password123","password_confirmation":"password123"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register = %d (%s)", rec.Code, rec.Body.String())
	}
	payload := jsonBody(t, rec)
	if payload["is_admin"] != true {
		t.Errorf("first user is_admin = %v", payload["is_admin"])
	}

	// A duplicate name must be rejected with a 422 validation payload.
	rec = c.do(http.MethodPost, "/user",
		`{"name":"alice","email":"other@example.com","password":"password123","password_confirmation":"password123"}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate register = %d", rec.Code)
	}
	errors, _ := jsonBody(t, rec)["errors"].(map[string]any)
	if _, ok := errors["name"]; !ok {
		t.Errorf("validation errors = %v", errors)
	}

	// Logout then login again.
	if rec := c.do(http.MethodGet, "/user/logout", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("logout = %d", rec.Code)
	}
	if rec := c.do(http.MethodGet, "/api/v1/user", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("user after logout = %d", rec.Code)
	}
	rec = c.do(http.MethodPost, "/user/login",
		`{"email":"alice@example.com","password":"password123"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d (%s)", rec.Code, rec.Body.String())
	}
	if message := jsonBody(t, rec)["message"]; message != "authenticated" {
		t.Errorf("login message = %v", message)
	}

	// Create a TOTP account.
	rec = c.do(http.MethodPost, "/api/v1/twofaccounts",
		`{"otp_type":"totp","account":"alice@example.com","service":"GitHub","secret":"JBSWY3DPEHPK3PXP"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create account = %d (%s)", rec.Code, rec.Body.String())
	}
	data, _ := jsonBody(t, rec)["data"].(map[string]any)
	id, _ := data["id"].(float64)
	if id != 1 {
		t.Fatalf("created account id = %v", data["id"])
	}
	if data["secret"] != "JBSWY3DPEHPK3PXP" {
		t.Errorf("secret = %v", data["secret"])
	}

	// The OTP endpoint returns a fresh 6 digit code.
	rec = c.do(http.MethodGet, "/api/v1/twofaccounts/1/otp", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("otp = %d (%s)", rec.Code, rec.Body.String())
	}
	otp := jsonBody(t, rec)
	password, _ := otp["password"].(string)
	if len(password) != 6 || strings.Trim(password, "0123456789") != "" {
		t.Errorf("otp password = %v", otp["password"])
	}
	if otp["otp_type"] != "totp" {
		t.Errorf("otp type = %v", otp["otp_type"])
	}
	if _, ok := otp["next_password"]; !ok {
		t.Error("TOTP payload has no next_password")
	}

	// Reading the collection with withOtp honours the `at` parameter. The value
	// below is the TOTP of JBSWY3DPEHPK3PXP at that timestamp (independently computed).
	rec = c.do(http.MethodGet, "/api/v1/twofaccounts?withOtp=1&at=1111111109", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("withOtp list = %d", rec.Code)
	}
	items, _ := jsonBody(t, rec)["data"].([]any)
	if len(items) != 1 {
		t.Fatalf("withOtp list length = %d", len(items))
	}
	embedded, _ := items[0].(map[string]any)["otp"].(map[string]any)
	if embedded["password"] != "071271" {
		t.Errorf("otp at 1111111109 = %v, want 071271", embedded["password"])
	}
	if _, leaked := items[0].(map[string]any)["secret"]; leaked {
		t.Error("the collection leaked the secret without withSecret")
	}
}

func TestCsrfAndAuthGuards(t *testing.T) {
	srv := testServer(t)
	c := &client{t: t, srv: srv}

	if rec := c.do(http.MethodGet, "/", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d", rec.Code)
	}

	// A POST without the CSRF header is rejected with 419.
	xsrf := c.xsrf
	c.xsrf = ""
	if rec := c.do(http.MethodPost, "/user/login", `{"email":"a@b.c","password":"x"}`, nil); rec.Code != statusCSRFMismatch {
		t.Fatalf("login without CSRF = %d", rec.Code)
	}
	c.xsrf = xsrf

	// Protected API routes answer 401 when unauthenticated.
	if rec := c.do(http.MethodGet, "/api/v1/twofaccounts", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated accounts = %d", rec.Code)
	}

	// Unknown API routes keep the legacy typo of the PHP implementation.
	rec := c.do(http.MethodGet, "/api/v1/nope", "", nil)
	if rec.Code != http.StatusNotFound || jsonBody(t, rec)["message"] != "unknowkn endpoint" {
		t.Fatalf("unknown endpoint = %d %s", rec.Code, rec.Body.String())
	}
}

func TestPersonalAccessTokenAuthentication(t *testing.T) {
	srv := testServer(t)
	c := &client{t: t, srv: srv}

	c.do(http.MethodGet, "/", "", nil)
	if rec := c.do(http.MethodPost, "/user",
		`{"name":"alice","email":"alice@example.com","password":"password123","password_confirmation":"password123"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("register = %d", rec.Code)
	}

	rec := c.do(http.MethodPost, "/oauth/personal-access-tokens", `{"name":"cli"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("create PAT = %d (%s)", rec.Code, rec.Body.String())
	}
	token, _ := jsonBody(t, rec)["accessToken"].(string)
	if token == "" {
		t.Fatal("no access token returned")
	}

	// Bearer tokens work without cookies nor CSRF protection.
	bare := &client{t: t, srv: srv}
	rec = bare.do(http.MethodGet, "/api/v1/user", "", map[string]string{"Authorization": "Bearer " + token})
	if rec.Code != http.StatusOK {
		t.Fatalf("bearer auth = %d (%s)", rec.Code, rec.Body.String())
	}
	user, _ := jsonBody(t, rec)["data"].(map[string]any)
	if user["email"] != "alice@example.com" {
		t.Errorf("bearer user = %v", user["email"])
	}
}
