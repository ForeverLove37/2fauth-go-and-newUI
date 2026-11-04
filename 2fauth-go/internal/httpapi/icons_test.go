package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"2fauth/internal/services"
)

// selfhstSVG and dashboardSVG are distinguishable icon bodies served by the fake
// provider used by the tests below.
const (
	selfhstSVG   = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect width="16" height="16"/></svg>`
	dashboardSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><circle cx="4" cy="4" r="4"/></svg>`
)

// fakeIconSite mimics the CDN of the icon providers. Tests never reach the
// internet: every provider base URL is pointed at this server.
func fakeIconSite(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/gh/selfhst/icons/svg/"):
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(selfhstSVG))
		case strings.HasPrefix(r.URL.Path, "/gh/homarr-labs/dashboard-icons/svg/"):
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(dashboardSVG))
		case r.URL.Path == "/search":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"icons":[],"total":0}`))
		default:
			// walkx, simpleicons and lobehub always miss.
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found"))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// pointIconsAt redirects every provider of the server at the fake icon site.
func pointIconsAt(srv *Server, site *httptest.Server) {
	srv.icons.SetSources(services.ProviderSources{
		services.ProviderSelfHST:     site.URL,
		services.ProviderDashboard:   site.URL,
		services.ProviderWalkx:       site.URL,
		services.ProviderSimpleIcons: site.URL,
		services.ProviderLobeHub:     site.URL,
		services.ProviderIconify:     site.URL,
	})
}

// iconAdminClient starts a server and signs in the administrator account.
func iconAdminClient(t *testing.T) (*Server, *client) {
	t.Helper()
	srv := testServer(t)
	c := &client{t: t, srv: srv}
	if rec := c.do(http.MethodGet, "/", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d", rec.Code)
	}
	rec := c.do(http.MethodPost, "/user",
		`{"name":"alice","email":"alice@example.com","password":"password123","password_confirmation":"password123"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register = %d (%s)", rec.Code, rec.Body.String())
	}
	return srv, c
}

// countIcons returns how many files are stored in the icon directory.
func countIcons(t *testing.T, srv *Server) int {
	t.Helper()
	entries, err := os.ReadDir(srv.icons.Dir())
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", srv.icons.Dir(), err)
	}
	return len(entries)
}

// setChain configures the provider chain through the admin endpoint.
func setChain(t *testing.T, c *client, providers string, maxTries int) map[string]any {
	t.Helper()
	body := `{"providers":[` + providers + `],"max_tries":` + itoaTest(maxTries) + `}`
	rec := c.do(http.MethodPut, "/api/v1/icons/providers", body, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT providers %s = %d (%s)", body, rec.Code, rec.Body.String())
	}
	return jsonBody(t, rec)
}

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func TestIconProvidersEndpoint(t *testing.T) {
	srv, c := iconAdminClient(t)

	rec := c.do(http.MethodGet, "/api/v1/icons/providers", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET providers = %d (%s)", rec.Code, rec.Body.String())
	}
	payload := jsonBody(t, rec)

	if payload["max_tries"] != float64(services.ProviderCount) {
		t.Errorf("max_tries = %v, want %d", payload["max_tries"], services.ProviderCount)
	}
	providers, _ := payload["providers"].([]any)
	if len(providers) != services.ProviderCount {
		t.Fatalf("providers = %d, want %d", len(providers), services.ProviderCount)
	}
	all, _ := payload["all"].([]any)
	if len(all) != services.ProviderCount {
		t.Fatalf("all = %d, want %d", len(all), services.ProviderCount)
	}

	wantOrder := map[string]float64{
		"selfhst": 1, "dashboard": 2, "simpleicons": 3,
		"walkx": 4, "lobehub": 5, "iconify": 6,
	}
	wantStatus := map[string]string{
		"selfhst": "available", "dashboard": "available", "walkx": "available",
		"simpleicons": "available", "lobehub": "available", "iconify": "available",
	}
	for _, raw := range providers {
		p, _ := raw.(map[string]any)
		id, _ := p["id"].(string)
		for _, key := range []string{"id", "label", "source", "template", "enabled", "order", "status"} {
			if _, ok := p[key]; !ok {
				t.Errorf("provider %q has no %q", id, key)
			}
		}
		if p["enabled"] != true {
			t.Errorf("provider %q is not enabled by default", id)
		}
		if p["order"] != wantOrder[id] {
			t.Errorf("provider %q order = %v, want %v", id, p["order"], wantOrder[id])
		}
		if p["status"] != wantStatus[id] {
			t.Errorf("provider %q status = %v, want %v", id, p["status"], wantStatus[id])
		}
		if !strings.HasPrefix(p["template"].(string), "https://") {
			t.Errorf("provider %q template = %v", id, p["template"])
		}
	}

	// The legacy packs endpoint keeps working and lists every provider.
	rec = c.do(http.MethodGet, "/api/v1/icons/packs", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET packs = %d", rec.Code)
	}
	packs, _ := jsonBody(t, rec)["packs"].([]any)
	if len(packs) != services.ProviderCount {
		t.Fatalf("packs = %d, want %d", len(packs), services.ProviderCount)
	}
	for _, raw := range packs {
		pack, _ := raw.(map[string]any)
		if pack["id"] == nil || pack["name"] == nil || pack["label"] == nil ||
			pack["url"] == nil || pack["template"] == nil {
			t.Errorf("incomplete pack entry: %v", pack)
		}
	}

	// No outbound request was needed to answer either endpoint.
	if n := countIcons(t, srv); n != 0 {
		t.Errorf("icons stored = %d, want 0", n)
	}
}

func TestIconFetchChainFallsThroughAndFollowsOrder(t *testing.T) {
	srv, c := iconAdminClient(t)
	site := fakeIconSite(t)
	pointIconsAt(srv, site)

	// selfhst answers, dashboard would answer too: the first wins.
	setChain(t, c, `"selfhst","dashboard"`, 2)

	rec := c.do(http.MethodPost, "/api/v1/icons/default", `{"service":"GitHub"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("icon fetch = %d (%s)", rec.Code, rec.Body.String())
	}
	first := jsonBody(t, rec)
	if first["provider"] != "selfhst" {
		t.Errorf("provider = %v, want selfhst", first["provider"])
	}
	firstIcon, _ := first["icon"].(string)
	if firstIcon == "" || !strings.HasSuffix(firstIcon, ".svg") {
		t.Fatalf("icon = %v", first["icon"])
	}
	if first["url"] != "/storage/icons/"+firstIcon {
		t.Errorf("url = %v", first["url"])
	}
	if _, err := os.Stat(srv.icons.Dir() + "/" + firstIcon); err != nil {
		t.Errorf("stored icon missing: %v", err)
	}
	if n := countIcons(t, srv); n != 1 {
		t.Errorf("icons stored = %d, want 1", n)
	}

	// Reordering the chain through the API changes which provider wins.
	setChain(t, c, `"dashboard","selfhst"`, 2)
	rec = c.do(http.MethodPost, "/api/v1/icons/default", `{"service":"GitHub"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("icon fetch after reorder = %d (%s)", rec.Code, rec.Body.String())
	}
	second := jsonBody(t, rec)
	if second["provider"] != "dashboard" {
		t.Errorf("provider after reorder = %v, want dashboard", second["provider"])
	}
	if second["icon"] == first["icon"] {
		t.Errorf("both orders stored the same icon %v", second["icon"])
	}
	if n := countIcons(t, srv); n != 2 {
		t.Errorf("icons stored = %d, want 2", n)
	}
}

func TestIconFetchHonoursMaxTries(t *testing.T) {
	srv, c := iconAdminClient(t)
	site := fakeIconSite(t)
	pointIconsAt(srv, site)

	// walkx always misses, dashboard would answer: max_tries=1 truncates the
	// chain before dashboard is reached.
	payload := setChain(t, c, `"walkx","dashboard"`, 1)
	if payload["max_tries"] != float64(1) {
		t.Errorf("max_tries = %v", payload["max_tries"])
	}
	for _, raw := range payload["providers"].([]any) {
		p, _ := raw.(map[string]any)
		if p["id"] == "dashboard" {
			if p["enabled"] != true {
				t.Error("dashboard should stay enabled")
			}
			if p["status"] != "skipped" || p["order"] != float64(2) {
				t.Errorf("dashboard = %v (order %v), want skipped/2", p["status"], p["order"])
			}
		}
	}

	rec := c.do(http.MethodPost, "/api/v1/icons/default", `{"service":"GitHub"}`, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("truncated chain = %d (%s), want 404", rec.Code, rec.Body.String())
	}
	if msg := jsonBody(t, rec)["message"]; msg != "Icon not found for this service" {
		t.Errorf("message = %v", msg)
	}
	if n := countIcons(t, srv); n != 0 {
		t.Fatalf("icons stored = %d, want 0", n)
	}

	// Raising max_tries makes the second provider reachable.
	setChain(t, c, `"walkx","dashboard"`, 2)
	rec = c.do(http.MethodPost, "/api/v1/icons/default", `{"service":"GitHub"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("with max_tries=2 = %d (%s)", rec.Code, rec.Body.String())
	}
	if provider := jsonBody(t, rec)["provider"]; provider != "dashboard" {
		t.Errorf("provider = %v, want dashboard", provider)
	}
}

func TestIconFetchMissIsA4xx(t *testing.T) {
	srv, c := iconAdminClient(t)
	site := fakeIconSite(t)
	pointIconsAt(srv, site)

	// An unreachable provider (transport error) must not turn into a 500 either.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	setChain(t, c, `"walkx","simpleicons","lobehub","iconify"`, 4)
	rec := c.do(http.MethodPost, "/api/v1/icons/default", `{"service":"GitHub"}`, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("all providers missing = %d (%s), want 404", rec.Code, rec.Body.String())
	}
	if n := countIcons(t, srv); n != 0 {
		t.Fatalf("icons stored = %d, want 0", n)
	}

	// Same with a transport error on the only provider of the chain.
	srv.icons.SetSources(services.ProviderSources{
		services.ProviderWalkx:       deadURL,
		services.ProviderSimpleIcons: deadURL,
		services.ProviderLobeHub:     deadURL,
		services.ProviderIconify:     deadURL,
	})
	rec = c.do(http.MethodPost, "/api/v1/icons/default", `{"service":"GitHub"}`, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("transport error = %d (%s), want 404", rec.Code, rec.Body.String())
	}

	// A service name that cannot be slugified is a miss as well.
	rec = c.do(http.MethodPost, "/api/v1/icons/default", `{"service":"<script>"}`, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("markup service = %d (%s), want 404", rec.Code, rec.Body.String())
	}

	// A missing service field stays a validation error.
	rec = c.do(http.MethodPost, "/api/v1/icons/default", `{}`, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing service = %d, want 422", rec.Code)
	}
}

func TestUpdateIconProvidersValidation(t *testing.T) {
	_, c := iconAdminClient(t)

	cases := []struct {
		name  string
		body  string
		field string
	}{
		{"unknown id", `{"providers":["selfhst","nope"],"max_tries":2}`, "providers.1"},
		{"empty list", `{"providers":[],"max_tries":2}`, "providers"},
		{"duplicate", `{"providers":["selfhst","selfhst"],"max_tries":2}`, "providers.1"},
		{"missing list", `{"max_tries":2}`, "providers"},
		{"max_tries zero", `{"providers":["selfhst"],"max_tries":0}`, "max_tries"},
		{"max_tries too big", `{"providers":["selfhst"],"max_tries":7}`, "max_tries"},
		{"max_tries not a number", `{"providers":["selfhst"],"max_tries":"many"}`, "max_tries"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := c.do(http.MethodPut, "/api/v1/icons/providers", tc.body, nil)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d (%s), want 422", rec.Code, rec.Body.String())
			}
			payload := jsonBody(t, rec)
			if _, ok := payload["message"]; !ok {
				t.Errorf("payload has no message: %v", payload)
			}
			errors, _ := payload["errors"].(map[string]any)
			if _, ok := errors[tc.field]; !ok {
				t.Errorf("errors = %v, want a %q entry", errors, tc.field)
			}
		})
	}

	// A rejected update must not have changed the persisted configuration.
	rec := c.do(http.MethodGet, "/api/v1/icons/providers", "", nil)
	enabled, _ := jsonBody(t, rec)["providers"].([]any)
	for _, raw := range enabled {
		p, _ := raw.(map[string]any)
		if p["enabled"] != true {
			t.Errorf("provider %v was disabled by a rejected update", p["id"])
		}
	}
}

func TestUpdateIconProvidersRequiresAdmin(t *testing.T) {
	_, admin := iconAdminClient(t)

	// Unauthenticated: 401.
	anon := &client{t: t, srv: admin.srv}
	if rec := anon.do(http.MethodPut, "/api/v1/icons/providers", `{"providers":["iconify"],"max_tries":1}`, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous PUT = %d, want 401", rec.Code)
	}

	// A regular user: 403.
	if rec := admin.do(http.MethodGet, "/user/logout", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("logout = %d", rec.Code)
	}
	rec := admin.do(http.MethodPost, "/user",
		`{"name":"bob","email":"bob@example.com","password":"password123","password_confirmation":"password123"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register bob = %d (%s)", rec.Code, rec.Body.String())
	}
	if isAdmin, _ := jsonBody(t, rec)["is_admin"].(bool); isAdmin {
		t.Fatal("bob unexpectedly became an administrator")
	}
	rec = admin.do(http.MethodPut, "/api/v1/icons/providers", `{"providers":["iconify"],"max_tries":1}`, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("regular user PUT = %d (%s), want 403", rec.Code, rec.Body.String())
	}

	// Reading the configuration only requires an authenticated user.
	if rec := admin.do(http.MethodGet, "/api/v1/icons/providers", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("regular user GET = %d, want 200", rec.Code)
	}
}

func TestUpdateIconProvidersKeepsMaxTriesWhenOmitted(t *testing.T) {
	_, c := iconAdminClient(t)

	setChain(t, c, `"walkx","dashboard"`, 2)

	rec := c.do(http.MethodPut, "/api/v1/icons/providers", `{"providers":["dashboard"]}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT without max_tries = %d (%s)", rec.Code, rec.Body.String())
	}
	payload := jsonBody(t, rec)
	if payload["max_tries"] != float64(2) {
		t.Errorf("max_tries = %v, want the previous value 2", payload["max_tries"])
	}
	providers, _ := payload["providers"].([]any)
	for _, raw := range providers {
		p, _ := raw.(map[string]any)
		if p["id"] == "dashboard" && p["order"] != float64(1) {
			t.Errorf("dashboard order = %v, want 1", p["order"])
		}
	}
}
