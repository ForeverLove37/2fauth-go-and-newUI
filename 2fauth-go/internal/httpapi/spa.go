package httpapi

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"2fauth/internal/crypt"
)

// spaAssets holds the parsed Vite manifest and the document template.
type spaAssets struct {
	publicDir string
	entryJS   string
	entryCSS  []string
	preload   []string
	tmpl      *template.Template
}

type manifestChunk struct {
	File           string   `json:"file"`
	Name           string   `json:"name"`
	Src            string   `json:"src"`
	IsEntry        bool     `json:"isEntry"`
	CSS            []string `json:"css"`
	Imports        []string `json:"imports"`
	DynamicImports []string `json:"dynamicImports"`
}

// loadSPAFromDir reads the manifest and prepares the document template.
func loadSPAFromDir(publicDir string) (*spaAssets, error) {
	manifestPath := filepath.Join(publicDir, "build", "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read vite manifest %s: %w", manifestPath, err)
	}
	var manifest map[string]manifestChunk
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("invalid vite manifest: %w", err)
	}
	entry, ok := manifest["resources/js/app.js"]
	if !ok {
		return nil, fmt.Errorf("vite manifest has no resources/js/app.js entry")
	}
	assets := &spaAssets{
		publicDir: publicDir,
		entryJS:   "build/" + entry.File,
		entryCSS:  nil,
	}
	for _, css := range entry.CSS {
		assets.entryCSS = append(assets.entryCSS, "build/"+css)
	}
	seen := map[string]bool{}
	var walk func(key string)
	walk = func(key string) {
		chunk, ok := manifest[key]
		if !ok || seen[chunk.File] {
			return
		}
		seen[chunk.File] = true
		if chunk.File != entry.File {
			assets.preload = append(assets.preload, "build/"+chunk.File)
		}
		for _, imp := range chunk.Imports {
			walk(imp)
		}
	}
	for _, imp := range entry.Imports {
		walk(imp)
	}
	assets.tmpl = template.Must(template.New("landing").Parse(landingTemplate))
	return assets, nil
}

const landingTemplate = `<!DOCTYPE html>
<html data-theme="{{.Theme}}" lang="{{.Lang}}">
<head>
    <meta charset="utf-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="description" content="{{.Description}}" lang="{{.Lang}}">
    <meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1, user-scalable=0, shrink-to-fit=no, viewport-fit=cover">
    <meta name="csrf-token" content="{{.CSRF}}">
    <meta name="robots" content="noindex, nofollow">
    <meta name="mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
    <title>{{.AppName}}</title>

    <link rel="shortcut icon" href="{{.Base}}/favicon.ico" />
    <link rel="icon" type="image/png" href="{{.Base}}/favicon.png" />
    <link rel="apple-touch-icon" href="{{.Base}}/favicon_lg.png" />
    <link rel="apple-touch-icon-precomposed" href="{{.Base}}/favicon_lg.png" />
    <link rel="manifest" href="{{.Base}}/manifest.json">
{{range .CSS}}    <link rel="stylesheet" href="{{.}}" />
{{end}}</head>
<body>
    <div id="app">
        <app></app>
    </div>
    <script type="text/javascript" nonce="{{.Nonce}}">
        var appSettings = {{.AppSettings}};
        var appConfig = {{.AppConfig}};
        var urls = {{.Urls}};
        var defaultPreferences = {{.DefaultPreferences}};
        var lockedPreferences = {{.LockedPreferences}};
        var appVersion = '{{.AppVersion}}';
        var isDemoApp = {{.IsDemoApp}};
        var isTestingApp = {{.IsTestingApp}};
        var appLocales = {{.Locales}};
    </script>
    <script type="module" src="{{.EntryJS}}"></script>
</body>
</html>
`

// serveSPA renders the single page application shell.
func (s *Server) serveSPA(c *ctx) {
	if s.spa == nil {
		http.Error(c.w, "front-end assets are missing: run `npm run build` or copy public/build", http.StatusInternalServerError)
		return
	}
	base := s.baseURL()

	settings := map[string]any{}
	for _, key := range s.settings.NullifiedKeys() {
		settings[key] = nil
	}
	for k, v := range s.settings.ForVisitor() {
		settings[k] = v
	}

	appConfig := map[string]any{
		"proxyAuth":      s.cfg.AuthenticationGuard == guardReverseProxy,
		"proxyLogoutUrl": falseIfEmpty(s.cfg.ProxyLogoutURL),
		"sso": map[string]any{
			"openid": s.cfg.OpenIDClientSecret != "",
			"github": s.cfg.GithubClientSecret != "",
		},
		"subdirectory": subdirValue(s.cfg.Subdirectory),
	}
	urls := map[string]any{
		"installDocUrl":   configURLs.InstallDoc,
		"ssoDocUrl":       configURLs.SsoDoc,
		"exportSchemaUrl": configURLs.ExportSchema,
	}

	prefs := s.cfg.DefaultPreferences
	theme, _ := prefs["theme"].(string)
	lang := "en"
	if l, ok := prefs["lang"].(string); ok && l != "browser" {
		lang = l
	}

	// The upstream shell has an inline script: only this one is allowed to run.
	nonce := crypt.RandomString(24)
	if s.cfg.ContentSecurityPolicy {
		c.w.Header().Set("Content-Security-Policy", strings.Replace(baseCSP, "script-src 'self'", "script-src 'self' 'nonce-"+nonce+"'", 1))
	}

	data := map[string]any{
		"Nonce":              nonce,
		"Theme":              theme,
		"Lang":               lang,
		"Description":        "A Web app to manage your Two-Factor Authentication (2FA) accounts and generate their security codes",
		"CSRF":               c.session.CSRFToken,
		"AppName":            s.cfg.AppName,
		"Base":               base,
		"CSS":                s.spa.assetURLs(base, s.spa.entryCSS),
		"EntryJS":            base + "/" + s.spa.entryJS,
		"AppSettings":        mustJSON(settings),
		"AppConfig":          mustJSON(appConfig),
		"Urls":               mustJSON(urls),
		"DefaultPreferences": mustJSON(prefs),
		"LockedPreferences":  mustJSON(s.cfg.LockedPreferences),
		"AppVersion":         s.cfg.Version,
		"IsDemoApp":          boolLiteral(s.cfg.IsDemoApp),
		"IsTestingApp":       boolLiteral(s.cfg.IsTestingApp),
		"Locales":            mustJSON(s.cfg.Locales),
	}

	c.w.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.w.Header().Set("Cache-Control", "no-store")
	if err := s.spa.tmpl.Execute(c.w, data); err != nil {
		http.Error(c.w, "template error", http.StatusInternalServerError)
	}
}

var configURLs = struct {
	InstallDoc   string
	SsoDoc       string
	ExportSchema string
}{
	InstallDoc:   "https://docs.2fauth.app/getting-started/installation/self-hosted-server/",
	SsoDoc:       "https://docs.2fauth.app/security/authentication/sso/",
	ExportSchema: "https://docs.2fauth.app/usage/migration/#export-schema",
}

func (a *spaAssets) assetURLs(base string, files []string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, base+"/"+f)
	}
	return out
}

func mustJSON(v any) template.JS {
	raw, err := json.Marshal(v)
	if err != nil {
		return template.JS("null")
	}
	return template.JS(raw) //nolint:gosec // values are marshalled from internal structures
}

func boolLiteral(b bool) template.JS {
	if b {
		return template.JS("true")
	}
	return template.JS("false")
}

func falseIfEmpty(v string) any {
	if v == "" {
		return false
	}
	return v
}

func subdirValue(v string) string {
	if v == "" {
		return ""
	}
	return "/" + strings.Trim(v, "/")
}

// baseURL returns the mount point of the app (subdirectory aware, no trailing slash).
func (s *Server) baseURL() string {
	if s.cfg.Subdirectory == "" {
		return ""
	}
	return "/" + strings.Trim(s.cfg.Subdirectory, "/")
}

// serveStatic serves the built front-end assets and the public files.
func (s *Server) serveStatic(c *ctx) {
	if s.spa == nil {
		http.NotFound(c.w, c.r)
		return
	}
	rel := strings.TrimPrefix(c.r.URL.Path, s.baseURL())
	rel = strings.TrimPrefix(rel, "/")
	if strings.Contains(rel, "..") {
		http.NotFound(c.w, c.r)
		return
	}
	full := filepath.Join(s.spa.publicDir, filepath.FromSlash(rel))
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.NotFound(c.w, c.r)
		return
	}
	if strings.HasPrefix(rel, "build/assets/") {
		c.w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.ServeFile(c.w, c.r, full)
}
