package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"2fauth/internal/config"
	"2fauth/internal/db"
	"2fauth/internal/store"
)

// validSVG is a minimal well formed SVG icon.
const validSVG = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M0 0h16v16H0z"/></svg>`

// iconTestService builds an IconService backed by a temporary SQLite database so
// the provider chain configuration is read and written through the real settings
// service.
func iconTestService(t *testing.T) (*IconService, *SettingService) {
	t.Helper()
	dir := t.TempDir()

	cfg := config.Load(dir)
	cfg.BasePath = dir
	cfg.Key = "base64:AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="

	conn, err := db.Open("sqlite", filepath.Join(dir, "icons.sqlite"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	st := store.New(conn, cfg)
	settings := NewSettingService(st, cfg)
	return NewIconService(cfg, st, settings), settings
}

// iconFiles lists the icons stored by the service.
func iconFiles(t *testing.T, svc *IconService) []string {
	t.Helper()
	entries, err := os.ReadDir(svc.Dir())
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", svc.Dir(), err)
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name())
	}
	return out
}

func TestServiceSlug(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Grafana Cloud", "grafana-cloud"},
		{"Proxmox VE", "proxmox-ve"},
		{"<script>", ""},
		{"", ""},
		{"   ", ""},
		{"GitHub", "github"},
		{"node.js", "node.js"},
		{"Tailscale_Funnel", "tailscale-funnel"},
		{"  Docker   Compose  ", "docker-compose"},
		{"***", ""},
		{"a/b", "a-b"},
		{"Nextcloud (Cloud)", "nextcloud-cloud"},
		{"alert(1)", "alert1"},
	}
	for _, tc := range cases {
		if got := ServiceSlug(tc.in); got != tc.want {
			t.Errorf("ServiceSlug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildIconSlugsVariants(t *testing.T) {
	slugs := buildIconSlugs("Grafana Cloud")
	if slugs.slug != "grafana-cloud" {
		t.Errorf("slug = %q", slugs.slug)
	}
	if slugs.raw != "Grafana-Cloud" {
		t.Errorf("raw = %q", slugs.raw)
	}
	if slugs.alnum != "grafanacloud" {
		t.Errorf("alnum = %q", slugs.alnum)
	}
	if slugs.service != "Grafana Cloud" {
		t.Errorf("service = %q", slugs.service)
	}

	// A markup-only name yields no slug and must therefore be rejected.
	bad := buildIconSlugs("<script>")
	if bad.slug != "" || bad.raw != "" || bad.alnum != "" {
		t.Fatalf("markup name produced slugs: %+v", bad)
	}
}

func TestIconChainConfigDefaults(t *testing.T) {
	svc, _ := iconTestService(t)

	chain := svc.Chain()
	if strings.Join(chain.Enabled, ",") != config.DefaultIconProviders {
		t.Errorf("default enabled chain = %v", chain.Enabled)
	}
	if chain.MaxTries != config.DefaultIconProviderMaxTries {
		t.Errorf("default max tries = %d", chain.MaxTries)
	}
	if len(chain.Attempts) != chain.MaxTries {
		t.Errorf("default attempts = %v", chain.Attempts)
	}
	for _, id := range chain.Attempts {
		if !IsKnownIconProvider(id) {
			t.Errorf("unknown provider in default chain: %q", id)
		}
	}
	if len(svc.Providers()) != ProviderCount {
		t.Errorf("provider count = %d", len(svc.Providers()))
	}
}

func TestIconChainIgnoresUnknownAndDuplicates(t *testing.T) {
	svc, settings := iconTestService(t)
	if err := settings.Set(config.SettingIconProviders, "walkx, nope ,WALKX,,simpleicons"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := settings.Set(config.SettingIconProviderMaxTries, 9); err != nil {
		t.Fatalf("Set: %v", err)
	}
	chain := svc.Chain()
	if strings.Join(chain.Enabled, ",") != "walkx,simpleicons" {
		t.Errorf("enabled = %v", chain.Enabled)
	}
	if chain.MaxTries != ProviderCount {
		t.Errorf("max tries = %d, want %d", chain.MaxTries, ProviderCount)
	}

	// A list with nothing valid in it falls back to the default chain.
	if err := settings.Set(config.SettingIconProviders, "unknown,unknown"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := strings.Join(svc.Chain().Enabled, ","); got != config.DefaultIconProviders {
		t.Errorf("fallback chain = %q", got)
	}
}

// fakeProvider answers like a real provider would, keyed by request path.
func fakeProvider(t *testing.T, routes map[string]fakeResponse) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if resp, ok := routes[r.URL.Path]; ok {
			if resp.contentType != "" {
				w.Header().Set("Content-Type", resp.contentType)
			}
			w.WriteHeader(resp.status)
			_, _ = w.Write([]byte(resp.body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

type fakeResponse struct {
	status      int
	body        string
	contentType string
}

func TestFetchIconFallsThroughTheChain(t *testing.T) {
	// selfhst misses (404) and dashboard answers a valid SVG.
	fake := fakeProvider(t, map[string]fakeResponse{
		"/gh/homarr-labs/dashboard-icons/svg/github.svg": {status: 200, body: validSVG, contentType: "image/svg+xml"},
	})
	svc, settings := iconTestService(t)
	svc.SetSources(ProviderSources{
		ProviderSelfHST:   fake.URL,
		ProviderDashboard: fake.URL,
		ProviderWalkx:     fake.URL,
	})
	if err := settings.Set(config.SettingIconProviders, "selfhst,dashboard"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := settings.Set(config.SettingIconProviderMaxTries, 3); err != nil {
		t.Fatalf("Set: %v", err)
	}

	res, err := svc.FetchIcon(context.Background(), "GitHub")
	if err != nil {
		t.Fatalf("FetchIcon: %v", err)
	}
	if res.Provider != ProviderDashboard {
		t.Errorf("provider = %q, want %q", res.Provider, ProviderDashboard)
	}
	if !strings.HasSuffix(res.Name, ".svg") {
		t.Errorf("stored name = %q, want a .svg file", res.Name)
	}
	content, err := svc.Get(res.Name)
	if err != nil {
		t.Fatalf("Get(%q): %v", res.Name, err)
	}
	if string(content) != validSVG {
		t.Errorf("stored content does not match the winning body")
	}
	if files := iconFiles(t, svc); len(files) != 1 {
		t.Errorf("stored files = %v, want exactly one", files)
	}
}

func TestFetchIconMaxTriesTruncatesTheChain(t *testing.T) {
	// The first provider of the chain misses, the second one would answer: with
	// max_tries=1 the second provider must not be tried at all.
	fake := fakeProvider(t, map[string]fakeResponse{
		"/gh/homarr-labs/dashboard-icons/svg/github.svg": {status: 200, body: validSVG, contentType: "image/svg+xml"},
	})
	svc, settings := iconTestService(t)
	svc.SetSources(ProviderSources{
		ProviderSelfHST:   fake.URL,
		ProviderDashboard: fake.URL,
	})
	if err := settings.Set(config.SettingIconProviders, "selfhst,dashboard"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := settings.Set(config.SettingIconProviderMaxTries, 1); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if _, err := svc.FetchIcon(context.Background(), "GitHub"); err == nil {
		t.Fatal("FetchIcon succeeded although the chain was truncated to a missing provider")
	}
	if files := iconFiles(t, svc); len(files) != 0 {
		t.Fatalf("stored files = %v, want none", files)
	}

	// Raising max_tries makes the second provider reachable again.
	if err := settings.Set(config.SettingIconProviderMaxTries, 2); err != nil {
		t.Fatalf("Set: %v", err)
	}
	res, err := svc.FetchIcon(context.Background(), "GitHub")
	if err != nil {
		t.Fatalf("FetchIcon: %v", err)
	}
	if res.Provider != ProviderDashboard {
		t.Errorf("provider = %q", res.Provider)
	}
}

func TestFetchIconOrderMatters(t *testing.T) {
	const otherSVG = `<svg xmlns="http://www.w3.org/2000/svg"><rect width="8" height="8"/></svg>`
	fake := fakeProvider(t, map[string]fakeResponse{
		"/gh/selfhst/icons/svg/github.svg":               {status: 200, body: validSVG, contentType: "image/svg+xml"},
		"/gh/homarr-labs/dashboard-icons/svg/github.svg": {status: 200, body: otherSVG, contentType: "image/svg+xml"},
	})
	svc, settings := iconTestService(t)
	svc.SetSources(ProviderSources{
		ProviderSelfHST:   fake.URL,
		ProviderDashboard: fake.URL,
	})

	if err := settings.Set(config.SettingIconProviders, "selfhst,dashboard"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	first, err := svc.FetchIcon(context.Background(), "GitHub")
	if err != nil {
		t.Fatalf("FetchIcon: %v", err)
	}
	if first.Provider != ProviderSelfHST {
		t.Errorf("provider = %q, want %q", first.Provider, ProviderSelfHST)
	}

	if err := settings.Set(config.SettingIconProviders, "dashboard,selfhst"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	second, err := svc.FetchIcon(context.Background(), "GitHub")
	if err != nil {
		t.Fatalf("FetchIcon: %v", err)
	}
	if second.Provider != ProviderDashboard {
		t.Errorf("provider = %q, want %q", second.Provider, ProviderDashboard)
	}
	if second.Name == first.Name {
		t.Errorf("both orders stored the same icon %q", second.Name)
	}
}

func TestFetchIconIconifySearch(t *testing.T) {
	fake := fakeProvider(t, map[string]fakeResponse{
		"/search":         {status: 200, body: `{"icons":["mdi:github"],"total":1}`, contentType: "application/json"},
		"/mdi/github.svg": {status: 200, body: validSVG, contentType: "image/svg+xml"},
	})
	svc, settings := iconTestService(t)
	svc.SetSources(ProviderSources{ProviderIconify: fake.URL})
	if err := settings.Set(config.SettingIconProviders, "iconify"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := settings.Set(config.SettingIconProviderMaxTries, 1); err != nil {
		t.Fatalf("Set: %v", err)
	}

	res, err := svc.FetchIcon(context.Background(), "GitHub")
	if err != nil {
		t.Fatalf("FetchIcon: %v", err)
	}
	if res.Provider != ProviderIconify {
		t.Errorf("provider = %q", res.Provider)
	}
	content, err := svc.Get(res.Name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(content) != validSVG {
		t.Error("stored icon is not the icon fetched from the search result")
	}
}

func TestFetchIconIconifyEmptySearch(t *testing.T) {
	fake := fakeProvider(t, map[string]fakeResponse{
		"/search": {status: 200, body: `{"icons":[],"total":0}`, contentType: "application/json"},
	})
	svc, settings := iconTestService(t)
	svc.SetSources(ProviderSources{ProviderIconify: fake.URL})
	if err := settings.Set(config.SettingIconProviders, "iconify"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := svc.FetchIcon(context.Background(), "Doesnotexist"); err != ErrIconNotFound {
		t.Fatalf("err = %v, want ErrIconNotFound", err)
	}
	if files := iconFiles(t, svc); len(files) != 0 {
		t.Errorf("stored files = %v, want none", files)
	}
}

func TestFetchIconRejectsUnusableBodies(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		status  int
		ct      string
		wantErr bool
	}{
		{"html error page", "<html><body>nope</body></html>", 200, "text/html", true},
		{"empty body", "", 200, "image/svg+xml", true},
		{"truncated svg", "<svg xmlns=\"http://www.w3.org/2000/svg\"><path d=\"M0", 200, "image/svg+xml", true},
		{"non 200", validSVG, 500, "image/svg+xml", true},
		{"plain text", "hello", 200, "text/plain", true},
		{"plain svg without xml header", `<svg xmlns="http://www.w3.org/2000/svg"><g/></svg>`, 200, "text/plain", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := fakeProvider(t, map[string]fakeResponse{
				"/gh/selfhst/icons/svg/github.svg": {status: tc.status, body: tc.body, contentType: tc.ct},
			})
			svc, settings := iconTestService(t)
			svc.SetSources(ProviderSources{ProviderSelfHST: fake.URL})
			if err := settings.Set(config.SettingIconProviders, "selfhst"); err != nil {
				t.Fatalf("Set: %v", err)
			}
			_, err := svc.FetchIcon(context.Background(), "GitHub")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("FetchIcon accepted an unusable body")
				}
				if files := iconFiles(t, svc); len(files) != 0 {
					t.Fatalf("stored files = %v, want none", files)
				}
				return
			}
			if err != nil {
				t.Fatalf("FetchIcon: %v", err)
			}
		})
	}
}

func TestFetchIconRejectsSluglessService(t *testing.T) {
	svc, settings := iconTestService(t)
	if err := settings.Set(config.SettingIconProviders, "selfhst"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	for _, service := range []string{"", "   ", "<script>", "***"} {
		if _, err := svc.FetchIcon(context.Background(), service); err != ErrIconNotFound {
			t.Errorf("FetchIcon(%q) err = %v, want ErrIconNotFound", service, err)
		}
	}
	if files := iconFiles(t, svc); len(files) != 0 {
		t.Errorf("stored files = %v, want none", files)
	}
}

func TestSniffIconContent(t *testing.T) {
	pngHeader := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}
	cases := []struct {
		name   string
		body   []byte
		want   string
		wantOK bool
	}{
		{"svg", []byte(validSVG), "image/svg+xml", true},
		{"png", pngHeader, "image/png", true},
		{"html", []byte("<html><body>x</body></html>"), "", false},
		{"empty", nil, "", false},
		{"json", []byte(`{"icons":["mdi:github"]}`), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := sniffIconContent(tc.body)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("sniffIconContent(%q) = %q, %v; want %q, %v", tc.body, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestLogoPacksListsEveryProvider(t *testing.T) {
	svc, _ := iconTestService(t)
	packs := svc.LogoPacks()
	if len(packs) != ProviderCount {
		t.Fatalf("packs = %d, want %d", len(packs), ProviderCount)
	}
	for _, pack := range packs {
		id, _ := pack["id"].(string)
		if !IsKnownIconProvider(id) {
			t.Errorf("unknown id in packs: %v", pack["id"])
		}
		for _, key := range []string{"name", "label", "url", "template", "variants"} {
			if _, ok := pack[key]; !ok {
				t.Errorf("pack %q has no %q", id, key)
			}
		}
		if !strings.HasPrefix(pack["template"].(string), "https://") {
			t.Errorf("pack %q template = %v", id, pack["template"])
		}
	}
}

func TestSetChainNormalises(t *testing.T) {
	svc, settings := iconTestService(t)
	if err := svc.SetChain([]string{" Walkx ", "walkx", "unknown", "Iconify"}, 2); err != nil {
		t.Fatalf("SetChain: %v", err)
	}
	if got := strings.Join(svc.Chain().Enabled, ","); got != "walkx,iconify" {
		t.Errorf("enabled = %q", got)
	}
	if got := svc.Chain().MaxTries; got != 2 {
		t.Errorf("max tries = %d", got)
	}
	if got, _ := settings.Get(config.SettingIconProviderMaxTries).(int); got != 2 {
		t.Errorf("persisted max tries = %v", settings.Get(config.SettingIconProviderMaxTries))
	}
}

func TestIconChainEnvDefaultsAndSettingsOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ICON_PROVIDERS", "iconify, selfhst")
	t.Setenv("ICON_PROVIDER_MAX_TRIES", "1")

	cfg := config.Load(dir)
	cfg.BasePath = dir
	if cfg.Settings[config.SettingIconProviders] != "iconify, selfhst" {
		t.Errorf("env default providers = %v", cfg.Settings[config.SettingIconProviders])
	}
	if v, ok := intValue(cfg.Settings[config.SettingIconProviderMaxTries]); !ok || v != 1 {
		t.Errorf("env default max tries = %v", cfg.Settings[config.SettingIconProviderMaxTries])
	}

	// The ICON_* variables are only defaults: a stored value wins over them.
	conn, err := db.Open("sqlite", filepath.Join(dir, "icons.sqlite"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	st := store.New(conn, cfg)
	settings := NewSettingService(st, cfg)
	svc := NewIconService(cfg, st, settings)

	chain := svc.Chain()
	if strings.Join(chain.Enabled, ",") != "iconify,selfhst" {
		t.Errorf("env chain = %v", chain.Enabled)
	}
	if chain.MaxTries != 1 || len(chain.Attempts) != 1 {
		t.Errorf("env max tries = %d (%v)", chain.MaxTries, chain.Attempts)
	}

	if err := settings.Set(config.SettingIconProviders, "walkx"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := settings.Set(config.SettingIconProviderMaxTries, 2); err != nil {
		t.Fatalf("Set: %v", err)
	}
	chain = svc.Chain()
	if strings.Join(chain.Enabled, ",") != "walkx" {
		t.Errorf("stored chain = %v", chain.Enabled)
	}
	if chain.MaxTries != 2 {
		t.Errorf("stored max tries = %d", chain.MaxTries)
	}
}

// pngIcon is a minimal PNG signature followed by a payload; http.DetectContentType
// identifies it as image/png from the signature alone.
var pngIcon = append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, []byte("payload")...)

func TestProviderSlugVariants(t *testing.T) {
	t.Run("dashboard tries the original casing", func(t *testing.T) {
		// Only the casing-preserving file exists, the lower-cased one 404s.
		fake := fakeProvider(t, map[string]fakeResponse{
			"/gh/homarr-labs/dashboard-icons/svg/Grafana-Cloud.svg": {status: 200, body: validSVG, contentType: "image/svg+xml"},
		})
		svc, settings := iconTestService(t)
		svc.SetSources(ProviderSources{ProviderDashboard: fake.URL})
		if err := settings.Set(config.SettingIconProviders, "dashboard"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if _, err := svc.FetchIcon(context.Background(), "Grafana Cloud"); err != nil {
			t.Fatalf("FetchIcon: %v", err)
		}
	})

	t.Run("lobehub retries with the -color suffix", func(t *testing.T) {
		fake := fakeProvider(t, map[string]fakeResponse{
			"/gh/lobehub/lobe-icons/packages/static-png/grafana-cloud-color.png": {status: 200, body: string(pngIcon), contentType: "image/png"},
		})
		svc, settings := iconTestService(t)
		svc.SetSources(ProviderSources{ProviderLobeHub: fake.URL})
		if err := settings.Set(config.SettingIconProviders, "lobehub"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		res, err := svc.FetchIcon(context.Background(), "Grafana Cloud")
		if err != nil {
			t.Fatalf("FetchIcon: %v", err)
		}
		if !strings.HasSuffix(res.Name, ".png") {
			t.Errorf("stored name = %q, want a .png file", res.Name)
		}
		if res.MimeType != "image/png" {
			t.Errorf("mime = %q", res.MimeType)
		}
	})

	t.Run("simpleicons uses alphanumerics only", func(t *testing.T) {
		fake := fakeProvider(t, map[string]fakeResponse{
			"/grafanacloud": {status: 200, body: validSVG, contentType: "image/svg+xml"},
		})
		svc, settings := iconTestService(t)
		svc.SetSources(ProviderSources{ProviderSimpleIcons: fake.URL})
		if err := settings.Set(config.SettingIconProviders, "simpleicons"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if _, err := svc.FetchIcon(context.Background(), "Grafana Cloud"); err != nil {
			t.Fatalf("FetchIcon: %v", err)
		}
	})
}
