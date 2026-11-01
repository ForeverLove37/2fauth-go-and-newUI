package httpapi

import (
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// uiAssets is the compiled React panel (2fauth-ui). It is served at the site
// root while the original Vue application stays reachable under /legacy.
type uiAssets struct {
	dir string
}

// loadUI returns nil when no compiled panel is available.
func loadUI(dir string) *uiAssets {
	if dir == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		return nil
	}
	return &uiAssets{dir: dir}
}

// serveUI serves the panel entry point.
func (s *Server) serveUI(c *ctx) {
	if s.ui == nil {
		http.NotFound(c.w, c.r)
		return
	}
	body, err := os.ReadFile(filepath.Join(s.ui.dir, "index.html"))
	if err != nil {
		http.Error(c.w, "front-end assets are missing: run `npm run build` in 2fauth-ui", http.StatusInternalServerError)
		return
	}
	c.w.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.w.Header().Set("Cache-Control", "no-cache")
	_, _ = c.w.Write(body)
}

// serveUIAsset serves a hashed bundle of the panel.
func (s *Server) serveUIAsset(c *ctx) {
	if s.ui == nil {
		http.NotFound(c.w, c.r)
		return
	}
	rel := strings.TrimPrefix(c.r.URL.Path, s.baseURL()+"/assets/")
	rel = strings.TrimPrefix(rel, "/")
	clean := strings.TrimPrefix(path.Clean("/"+rel), "/")
	if clean == "" || strings.Contains(clean, "..") {
		http.NotFound(c.w, c.r)
		return
	}
	file := filepath.Join(s.ui.dir, "assets", filepath.FromSlash(clean))
	if !strings.HasPrefix(file, s.ui.dir) {
		http.NotFound(c.w, c.r)
		return
	}
	if info, err := os.Stat(file); err != nil || info.IsDir() {
		http.NotFound(c.w, c.r)
		return
	}
	if ct := mime.TypeByExtension(filepath.Ext(file)); ct != "" {
		c.w.Header().Set("Content-Type", ct)
	}
	c.w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(c.w, c.r, file)
}

// serveUIFavicon serves the panel favicon when present.
func (s *Server) serveUIFavicon(c *ctx) {
	if s.ui == nil {
		http.NotFound(c.w, c.r)
		return
	}
	file := filepath.Join(s.ui.dir, "favicon.svg")
	if _, err := os.Stat(file); err != nil {
		http.NotFound(c.w, c.r)
		return
	}
	c.w.Header().Set("Content-Type", "image/svg+xml")
	http.ServeFile(c.w, c.r, file)
}

// serveFrontend dispatches between the React panel and the legacy Vue app.
func (s *Server) serveFrontend(c *ctx, reqPath string) {
	if strings.HasPrefix(reqPath, "/legacy") {
		s.serveSPA(c)
		return
	}
	if s.ui != nil {
		s.serveUI(c)
		return
	}
	s.serveSPA(c)
}

// frontendConfig exposes the public configuration the React panel needs before
// a user signs in. It is an addition to the upstream API (the Vue app receives
// the same values through the landing page globals).
func (s *Server) frontendConfig(c *ctx) {
	jsonResponse(c.w, http.StatusOK, map[string]any{
		"appName":      s.cfg.AppName,
		"appVersion":   s.cfg.Version,
		"subdirectory": s.cfg.Subdirectory,
		"isDemoApp":    s.cfg.IsDemoApp,
		"sso": map[string]any{
			"openid": s.cfg.OpenIDClientSecret != "",
			"github": s.cfg.GithubClientSecret != "",
		},
		"appSettings": s.settings.ForVisitor(),
		"features":    s.featureStates(),
	})
}

// featureStates returns the flag map used by the panel.
func (s *Server) featureStates() map[string]string {
	out := map[string]string{}
	for _, name := range featureNames {
		out[name] = s.featureState(name)
	}
	return out
}
