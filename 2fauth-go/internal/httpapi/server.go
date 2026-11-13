package httpapi

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
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

	// trustedProxies may set X-Forwarded-For; allowedClients may connect at all.
	trustedProxies *IPSet
	allowedClients *IPSet

	// mailJobs tracks the e-mails being delivered in the background.
	mailJobs sync.WaitGroup
	// mailSlots bounds the concurrent SMTP deliveries.
	mailSlots chan struct{}
	// failedAlerts limits the "failed sign-in" e-mails per account.
	failedAlerts *throttleStore

	iconFill *iconFiller

	sshFailures        *throttleStore
	sshMu              sync.RWMutex
	sshHostFingerprint string
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
	trusted, err := ParseIPSet(opts.Config.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("TRUSTED_PROXIES: %w", err)
	}
	allowed, err := ParseIPSet(opts.Config.AllowedClientIPs)
	if err != nil {
		return nil, fmt.Errorf("ALLOWED_CLIENT_IPS: %w", err)
	}
	if opts.Config.AuthenticationGuard == guardReverseProxy {
		// Without a trusted proxy anyone could forge the identity headers.
		if trusted.Empty() {
			return nil, errors.New("AUTHENTICATION_GUARD=reverse-proxy-guard requires TRUSTED_PROXIES")
		}
		if opts.Config.AuthProxyHeaderForUser == "" {
			return nil, errors.New("AUTHENTICATION_GUARD=reverse-proxy-guard requires AUTH_PROXY_HEADER_FOR_USER")
		}
	}
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

		trustedProxies: trusted,
		allowedClients: allowed,
		mailSlots:      make(chan struct{}, 4),
		failedAlerts:   newThrottleStore(),
		iconFill:       newIconFiller(),
		sshFailures:    newThrottleStore(),
	}
	s.registerRoutes()
	return s, nil
}

// route is one entry of the routing table.
type route struct {
	method string
	segs   []string
	// key is "METHOD pattern", what the allow-list of device tokens names.
	key     string
	auth    bool // a signed-in user is required
	admin   bool // an administrator is required
	static  bool // cacheable asset: no session is read and no cookie is written
	maxBody int64
	handler func(*ctx)
}

// ctx carries the request state through the handlers.
type ctx struct {
	s       *Server
	w       http.ResponseWriter
	r       *http.Request
	session *session
	user    *store.User
	// viaToken is set when the request is authenticated by a personal access token,
	// which a browser never attaches on its own: CSRF does not apply.
	viaToken bool
	// device is set when that token is the token of a phone app (DeviceScope).
	device  bool
	tokenID string

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
		key:     method + " " + pattern,
		handler: h,
	})
}

// addAuth registers an authenticated route.
func (s *Server) addAuth(method, pattern string, h func(*ctx)) {
	s.routes = append(s.routes, route{method: method, segs: splitPattern(pattern), key: method + " " + pattern, auth: true, handler: h})
}

// addAdmin registers an administrator-only route.
func (s *Server) addAdmin(method, pattern string, h func(*ctx)) {
	s.routes = append(s.routes, route{method: method, segs: splitPattern(pattern), key: method + " " + pattern, auth: true, admin: true, handler: h})
}

// addStatic registers a public asset route.
func (s *Server) addStatic(method, pattern string, h func(*ctx)) {
	s.routes = append(s.routes, route{method: method, segs: splitPattern(pattern), key: method + " " + pattern, static: true, handler: h})
}

// limitBody sets the largest request body of the last registered route.
func (s *Server) limitBody(max int64) {
	s.routes[len(s.routes)-1].maxBody = max
}

func splitPattern(pattern string) []string {
	trimmed := strings.Trim(pattern, "/")
	if trimmed == "" {
		return []string{}
	}
	return strings.Split(trimmed, "/")
}

// Request body limits. Most endpoints take a small JSON document, the larger limits
// are set per route (see registerRoutes).
const (
	defaultMaxBody = 1 << 20
	uploadMaxBody  = 8 << 20
	importMaxBody  = 32 << 20
)

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("panic while handling request", "path", r.URL.Path, "panic", rec)
			s.setSecurityHeaders(w.Header())
			jsonMessage(w, http.StatusInternalServerError, "server error")
		}
	}()

	// The listener already refuses these addresses; this also covers an embedder
	// that serves the handler on its own listener.
	if !s.allowedClients.Empty() && !s.allowedClients.Contains(peerAddr(r.RemoteAddr)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	path := r.URL.Path
	if s.cfg.Subdirectory != "" {
		prefix := "/" + s.cfg.Subdirectory
		switch {
		case path == prefix:
			path = "/"
		case strings.HasPrefix(path, prefix+"/"):
			path = strings.TrimPrefix(path, prefix)
		default:
			s.setSecurityHeaders(w.Header())
			http.NotFound(w, r)
			return
		}
	}

	// Buffer the response so that session cookies can still be written once the
	// handler has produced its output (including a login that rotates the session).
	bw := &bufferedWriter{header: http.Header{}, status: http.StatusOK}
	s.setSecurityHeaders(bw.header)
	c := &ctx{s: s, w: bw, r: r, params: map[string]string{}}

	rt, params := s.match(r.Method, path)
	c.params = params

	// Cacheable assets never touch the session: a shared cache must not be able to
	// store a Set-Cookie header together with a public response.
	if rt != nil && rt.static {
		rt.handler(c)
		bw.flush(w)
		return
	}

	maxBody := int64(defaultMaxBody)
	if rt != nil && rt.maxBody > 0 {
		maxBody = rt.maxBody
	}
	if r.ContentLength > maxBody {
		jsonMessage(bw, http.StatusRequestEntityTooLarge, "The request body is too large.")
		bw.flush(w)
		return
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	}

	c.session = s.loadSession(r)
	if s.resolveUser(c) && strings.HasPrefix(path, "/api/") {
		// The SPA reacts to this 418 by logging out.
		jsonMessage(bw, statusSessionExpired, "Session expired.")
		s.finish(w, bw, c.session)
		return
	}

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

	// The token of a phone app reaches only what a phone needs (see devices.go).
	if c.device && !deviceRoutes[rt.key] {
		jsonMessage(bw, http.StatusForbidden, "This endpoint is not available to the token of a device")
		s.finish(w, bw, c.session)
		return
	}

	if isUnsafeMethod(r.Method) && !c.viaToken && !s.csrfValid(r, c.session) {
		jsonResponse(bw, statusCSRFMismatch, map[string]any{
			"message": "CSRF token mismatch.",
		})
		s.finish(w, bw, c.session)
		return
	}

	if retry, limited := s.apiLimited(c, path); limited {
		bw.Header().Set("Retry-After", strconv.Itoa(retry))
		jsonMessage(bw, http.StatusTooManyRequests, "Too many requests.")
		s.finish(w, bw, c.session)
		return
	}

	rt.handler(c)
	s.finish(w, bw, c.session)
}

// resolveUser sets c.user from the session, the personal access token or the
// reverse proxy headers. It reports whether a session just expired.
func (s *Server) resolveUser(c *ctx) bool {
	expired := false
	if c.session.UserID > 0 {
		user, isExpired := s.userFromSession(c.session)
		expired = isExpired
		if user != nil {
			c.user = user
		} else {
			// A revoked, expired or foreign session: drop it from this browser too.
			c.session = newSession()
		}
	}
	if s.cfg.AuthenticationGuard == guardReverseProxy {
		// In this mode the proxy is the only authority: a session or a token cannot
		// outlive what it says about the current request.
		// The proxy's own login is ambient like a cookie, so CSRF still applies.
		c.user = s.userFromProxy(c.r)
		return false
	}
	if c.user == nil {
		if tok := bearerToken(c.r); tok != "" && s.patPolicyAllows() {
			if token, u, err := s.st.TokenFromPlain(tok); err == nil {
				c.user = u
				c.viaToken = true
				c.tokenID = token.ID
				c.device = token.HasScope(store.DeviceScope)
			}
		}
	}
	return expired && c.user == nil
}

// userFromSession validates a session against its server-side record and records
// the activity. It reports whether the session has just expired.
func (s *Server) userFromSession(sess *session) (*store.User, bool) {
	if sess.SID == "" {
		return nil, false
	}
	row, err := s.st.GetSession(sess.SID)
	if err != nil || row.UserID != sess.UserID {
		return nil, false
	}
	now := time.Now().Unix()
	lifetime := int64(s.cfg.SessionLifetime)
	if lifetime <= 0 {
		lifetime = 120
	}
	maxAge := int64(s.cfg.SessionMaxAge)
	if maxAge <= 0 {
		maxAge = 60 * 24 * 30
	}
	if now-row.LastActivity > lifetime*60 || now-row.CreatedAt > maxAge*60 {
		_ = s.st.DeleteSession(row.ID)
		return nil, true
	}
	user, err := s.st.GetUser(row.UserID)
	if err != nil {
		_ = s.st.DeleteSession(row.ID)
		return nil, false
	}
	// One write per minute is precise enough for an idle timeout.
	if now-row.LastActivity >= 60 {
		s.st.TouchSession(row.ID, now)
	}
	return user, false
}

// apiLimited applies THROTTLE_API to the JSON API, per user or per client address.
func (s *Server) apiLimited(c *ctx, path string) (int, bool) {
	if s.cfg.ThrottleAPI <= 0 || !strings.HasPrefix(path, "/api/") {
		return 0, false
	}
	key := "ip:" + s.clientIP(c.r)
	if c.user != nil {
		key = "user:" + strconv.FormatInt(c.user.ID, 10)
	}
	ok, retry := s.apiThrottle.Allow(key, s.cfg.ThrottleAPI, time.Minute)
	return retry, !ok
}

// baseCSP is the policy of the React panel. Styles stay inline-capable because the
// theme engine and the 3D layouts set style attributes.
const baseCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; " +
	"font-src 'self' data:; connect-src 'self'; worker-src 'self' blob:; object-src 'none'; base-uri 'self'; " +
	"form-action 'self'; frame-ancestors 'none'"

// setSecurityHeaders sets the headers shared by every response. Handlers override
// Cache-Control for assets; dynamic answers may carry secrets and are never stored.
func (s *Server) setSecurityHeaders(h http.Header) {
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Permissions-Policy", "camera=(self), microphone=(), geolocation=(), payment=()")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("Cache-Control", "no-store")
	if s.cfg.ContentSecurityPolicy {
		h.Set("Content-Security-Policy", baseCSP)
	}
	if s.isHTTPS() {
		h.Set("Strict-Transport-Security", "max-age=15552000")
	}
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
	if sess == nil || sess.CSRFToken == "" {
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
	return subtle.ConstantTimeCompare([]byte(token), []byte(sess.CSRFToken)) == 1
}

// bearerToken extracts an Authorization: Bearer token.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
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
