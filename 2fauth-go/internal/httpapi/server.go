package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"2fauth/internal/config"
	"2fauth/internal/crypt"
	"2fauth/internal/services"
	"2fauth/internal/store"
)

// Server serves the 2FAuth HTTP API, the SPA and static assets.
type Server struct {
	cfg      *config.Config
	st       *store.Store
	settings *services.SettingService
	enc      *crypt.Encrypter
	icons    *services.IconService

	spa        *spaAssets
	ui         *uiAssets
	routes     []route
	forceHTTPS bool

	// loginThrottle tracks failed login attempts by key.
	throttle *throttleStore
	// apiThrottle limits API requests.
	apiThrottle *throttleStore
}

// Options configures a Server.
type Options struct {
	Config   *config.Config
	Store    *store.Store
	Settings *services.SettingService
	Enc      *crypt.Encrypter
	HTTPS    bool
}

// New builds a Server and registers every route.
func New(opts Options) (*Server, error) {
	spa, err := loadSPAFromDir(opts.Config.AssetsPath)
	if err != nil {
		slog.Warn("front-end assets unavailable", "error", err, "assets_path", opts.Config.AssetsPath)
		spa = nil
	}
	s := &Server{
		cfg:         opts.Config,
		st:          opts.Store,
		settings:    opts.Settings,
		enc:         opts.Enc,
		icons:       services.NewIconService(opts.Config, opts.Store, opts.Settings),
		spa:         spa,
		ui:          loadUI(opts.Config.UIPath),
		forceHTTPS:  opts.HTTPS,
		throttle:    newThrottleStore(),
		apiThrottle: newThrottleStore(),
	}
	s.registerRoutes()
	return s, nil
}

// route is one entry of the routing table.
type route struct {
	method  string
	segs    []string
	auth    bool // a signed-in user is required
	admin   bool // an administrator is required
	csrf    bool // CSRF protection applies (defaults to true for unsafe methods)
	handler func(*ctx)
}

// ctx carries the request state through the handlers.
type ctx struct {
	s       *Server
	w       http.ResponseWriter
	r       *http.Request
	session *session
	user    *store.User

	params   map[string]string
	body     map[string]any
	bodyErr  error
	bodyRead bool
}

// add registers a route.
func (s *Server) add(method, pattern string, h func(*ctx)) {
	s.routes = append(s.routes, route{
		method:  method,
		segs:    splitPattern(pattern),
		handler: h,
	})
}

// addAuth registers an authenticated route.
func (s *Server) addAuth(method, pattern string, h func(*ctx)) {
	s.routes = append(s.routes, route{method: method, segs: splitPattern(pattern), auth: true, handler: h})
}

// addAdmin registers an administrator-only route.
func (s *Server) addAdmin(method, pattern string, h func(*ctx)) {
	s.routes = append(s.routes, route{method: method, segs: splitPattern(pattern), auth: true, admin: true, handler: h})
}

func splitPattern(pattern string) []string {
	trimmed := strings.Trim(pattern, "/")
	if trimmed == "" {
		return []string{}
	}
	return strings.Split(trimmed, "/")
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("panic while handling request", "path", r.URL.Path, "panic", rec)
			jsonMessage(w, http.StatusInternalServerError, "server error")
		}
	}()

	path := r.URL.Path
	if s.cfg.Subdirectory != "" {
		prefix := "/" + s.cfg.Subdirectory
		if path == prefix {
			path = "/"
		} else if strings.HasPrefix(path, prefix+"/") {
			path = strings.TrimPrefix(path, prefix)
		}
	}

	// Buffer the response so that session cookies can still be written once the
	// handler has produced its output (including a login that rotates the session).
	bw := &bufferedWriter{header: http.Header{}, status: http.StatusOK}
	c := &ctx{s: s, w: bw, r: r, params: map[string]string{}}
	c.session = s.loadSession(r)

	// Resolve the authenticated user (session cookie or Bearer PAT).
	if c.session.UserID > 0 {
		if u, err := s.st.GetUser(c.session.UserID); err == nil {
			c.user = u
		} else {
			c.session.UserID = 0
		}
	}
	if c.user == nil {
		if tok := bearerToken(r); tok != "" {
			if u := s.userFromToken(tok); u != nil {
				c.user = u
			}
		}
	}

	// Laravel invalidates idle sessions; the SPA reacts to the 418 by logging out.
	if c.user != nil && s.sessionExpired(c.session) && strings.HasPrefix(path, "/api/") {
		c.session = newSession()
		c.session.UserID = 0
		c.user = nil
		bw := c.w.(*bufferedWriter)
		jsonMessage(bw, statusSessionExpired, "Session expired.")
		s.finish(w, bw, c.session)
		return
	}

	rt, params := s.match(r.Method, path)
	c.params = params
	if rt == nil {
		switch {
		case strings.HasPrefix(path, "/api/"):
			jsonMessage(bw, http.StatusNotFound, "unknowkn endpoint")
		case r.Method == http.MethodGet || r.Method == http.MethodHead:
			// SPA fallback for client side routes.
			s.serveFrontend(c, path)
		default:
			jsonMessage(bw, http.StatusNotFound, "unknowkn endpoint")
		}
		s.finish(w, bw, c.session)
		return
	}

	if rt.auth && c.user == nil {
		jsonMessage(bw, http.StatusUnauthorized, "Unauthenticated.")
		s.finish(w, bw, c.session)
		return
	}
	if rt.admin && (c.user == nil || !c.user.IsAdmin) {
		jsonMessage(bw, http.StatusForbidden, "Forbidden")
		s.finish(w, bw, c.session)
		return
	}

	if isUnsafeMethod(r.Method) && !s.csrfValid(r, c.session) {
		jsonResponse(bw, statusCSRFMismatch, map[string]any{
			"message": "CSRF token mismatch.",
		})
		s.finish(w, bw, c.session)
		return
	}

	c.session.LastActivity = time.Now().Unix()

	rt.handler(c)
	s.finish(w, bw, c.session)
}

// finish sets the session cookies then flushes the buffered response.
func (s *Server) finish(w http.ResponseWriter, bw *bufferedWriter, sess *session) {
	if sess != nil {
		s.persistSession(w, sess)
	}
	bw.flush(w)
}

// bufferedWriter records a handler response until the session cookies are set.
type bufferedWriter struct {
	header      http.Header
	status      int
	wroteHeader bool
	body        bytes.Buffer
}

func (b *bufferedWriter) Header() http.Header { return b.header }

func (b *bufferedWriter) WriteHeader(status int) {
	if b.wroteHeader {
		return
	}
	b.status = status
	b.wroteHeader = true
}

func (b *bufferedWriter) Write(p []byte) (int, error) {
	if !b.wroteHeader {
		b.WriteHeader(http.StatusOK)
	}
	return b.body.Write(p)
}

// flush copies the buffered response onto the real writer.
func (b *bufferedWriter) flush(w http.ResponseWriter) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	for k, values := range b.header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(b.status)
	if b.body.Len() > 0 {
		_, _ = w.Write(b.body.Bytes())
	}
}

// match finds the most specific route matching method and path.
func (s *Server) match(method, path string) (*route, map[string]string) {
	segs := splitPattern(path)
	var best *route
	var bestParams map[string]string
	bestScore := -1
	for i := range s.routes {
		rt := &s.routes[i]
		if rt.method != method && !(method == http.MethodHead && rt.method == http.MethodGet) {
			continue
		}
		params, score, ok := matchSegments(rt.segs, segs)
		if !ok {
			continue
		}
		if score > bestScore {
			best = rt
			bestParams = params
			bestScore = score
		}
	}
	return best, bestParams
}

// matchSegments compares pattern and path segments; literal matches score higher.
func matchSegments(pattern, path []string) (map[string]string, int, bool) {
	params := map[string]string{}
	score := 0
	pi := 0
	for pi < len(pattern) {
		seg := pattern[pi]
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(seg, "{"), "}")
			catchAll := strings.HasSuffix(name, "...")
			name = strings.TrimSuffix(name, "...")
			if catchAll {
				if pi != len(pattern)-1 {
					return nil, 0, false
				}
				rest := strings.Join(path[pi:], "/")
				params[name] = rest
				return params, score, true
			}
			if pi >= len(path) {
				return nil, 0, false
			}
			params[name] = path[pi]
			pi++
			continue
		}
		if pi >= len(path) || path[pi] != seg {
			return nil, 0, false
		}
		score++
		pi++
	}
	if len(path) != len(pattern) {
		return nil, 0, false
	}
	return params, score, true
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// csrfValid validates the CSRF token of an unsafe request.
func (s *Server) csrfValid(r *http.Request, sess *session) bool {
	if sess == nil {
		return false
	}
	token := r.Header.Get("X-XSRF-TOKEN")
	if token == "" {
		token = r.Header.Get("X-CSRF-TOKEN")
	}
	if token == "" {
		_ = r.ParseForm()
		token = r.PostFormValue("_token")
	}
	if token == "" {
		// A Bearer authenticated API client is not subject to CSRF.
		if bearerToken(r) != "" {
			return true
		}
		return false
	}
	return token == sess.CSRFToken
}

// bearerToken extracts an Authorization: Bearer token.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

// userFromToken resolves a personal access token to its owner.
func (s *Server) userFromToken(token string) *store.User {
	u, err := s.st.UserFromToken(token)
	if err != nil {
		return nil
	}
	return u
}

// body decodes and caches the JSON (or form) body.
func (c *ctx) json() map[string]any {
	if c.bodyRead {
		return c.body
	}
	c.bodyRead = true
	c.body = map[string]any{}
	ct := c.r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		if err := decodeJSON(c.r, &c.body); err != nil {
			c.bodyErr = err
		}
		return c.body
	}
	if err := c.r.ParseForm(); err == nil {
		for k, v := range c.r.PostForm {
			if len(v) > 0 {
				c.body[k] = v[0]
			}
		}
	}
	return c.body
}

// input returns a string body/query value.
func (c *ctx) input(name string) string {
	if v, ok := c.json()[name]; ok {
		return toString(v)
	}
	return c.r.URL.Query().Get(name)
}

// hasInput reports whether the request carries the field (body or query).
func (c *ctx) hasInput(name string) bool {
	if _, ok := c.json()[name]; ok {
		return true
	}
	return c.r.URL.Query().Has(name)
}

// query returns a query string parameter.
func (c *ctx) query(name string) string { return c.r.URL.Query().Get(name) }

// param returns a path parameter.
func (c *ctx) param(name string) string { return c.params[name] }

// requireAuth aborts with 401 when no user is signed in.
func (c *ctx) requireAuth() bool {
	if c.user == nil {
		jsonMessage(c.w, http.StatusUnauthorized, "Unauthenticated.")
		return false
	}
	return true
}

func toString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "1"
		}
		return ""
	case json.Number:
		return t.String()
	case float64:
		return fmt.Sprintf("%v", t)
	case int:
		return fmt.Sprintf("%d", t)
	case int64:
		return fmt.Sprintf("%d", t)
	}
	return fmt.Sprint(v)
}

// statusCSRFMismatch is Laravel's 419 answer to a bad CSRF token.
const statusCSRFMismatch = 419

// statusSessionExpired is the 418 answer used when the session went idle.
const statusSessionExpired = 418

// sessionExpired reports whether the session idled past its lifetime.
func (s *Server) sessionExpired(sess *session) bool {
	if sess == nil || sess.UserID == 0 || sess.LastActivity == 0 {
		return false
	}
	lifetime := s.cfg.SessionLifetime
	if lifetime <= 0 {
		lifetime = 120
	}
	return time.Now().Unix()-sess.LastActivity > int64(lifetime)*60
}
