package httpapi

import (
	"encoding/json"
	"net/http"

	"2fauth/internal/crypt"
	"2fauth/internal/store"
)

// sessionCookieName mirrors config('session.cookie') for APP_NAME=2FAuth.
const sessionCookieName = "2fauth_session"

// xsrfCookieName is the CSRF cookie read by axios.
const xsrfCookieName = "XSRF-TOKEN"

// session is the state carried by the encrypted session cookie. Being signed in is
// decided by the user_sessions row SID points to, never by the cookie alone.
type session struct {
	UserID    int64  `json:"user_id"`
	SID       string `json:"sid,omitempty"`
	CSRFToken string `json:"csrf_token"`
}

// newSession builds a fresh anonymous session with a CSRF token.
func newSession() *session {
	return &session{CSRFToken: crypt.RandomString(40)}
}

// signIn opens a server-side session for the user, replacing any previous one of
// this browser, and rotates the CSRF token.
func (s *Server) signIn(c *ctx, user *store.User) error {
	if c.session != nil && c.session.SID != "" {
		_ = s.st.DeleteSession(c.session.SID)
	}
	sid, err := s.st.CreateSession(user.ID, s.clientIP(c.r), c.r.UserAgent())
	if err != nil {
		return err
	}
	c.session = &session{UserID: user.ID, SID: sid, CSRFToken: crypt.RandomString(40)}
	return nil
}

// loadSession reads the encrypted session cookie.
func (s *Server) loadSession(r *http.Request) *session {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return newSession()
	}
	plain, err := s.enc.DecryptString(c.Value)
	if err != nil {
		return newSession()
	}
	var sess session
	if err := json.Unmarshal([]byte(plain), &sess); err != nil {
		return newSession()
	}
	if sess.CSRFToken == "" {
		sess.CSRFToken = crypt.RandomString(40)
	}
	return &sess
}

// persistSession writes the encrypted session cookie and the XSRF cookie.
func (s *Server) persistSession(w http.ResponseWriter, sess *session) {
	raw, err := json.Marshal(sess)
	if err != nil {
		return
	}
	encrypted, err := s.enc.EncryptString(string(raw))
	if err != nil {
		return
	}
	secure := s.cfg.SessionSecure || s.isHTTPS()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    encrypted,
		Path:     s.cookiePath(),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   s.cfg.SessionMaxAge * 60,
	})
	// The XSRF cookie is readable by JavaScript on purpose (axios needs it).
	http.SetCookie(w, &http.Cookie{
		Name:     xsrfCookieName,
		Value:    sess.CSRFToken,
		Path:     s.cookiePath(),
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   s.cfg.SessionMaxAge * 60,
	})
}

// clearSession removes the session and XSRF cookies.
func (s *Server) clearSession(w http.ResponseWriter) {
	secure := s.cfg.SessionSecure || s.isHTTPS()
	for _, name := range []string{sessionCookieName, xsrfCookieName} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     s.cookiePath(),
			MaxAge:   -1,
			HttpOnly: name == sessionCookieName,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

// cookiePath returns the cookie path accounting for the app subdirectory.
func (s *Server) cookiePath() string {
	if s.cfg.Subdirectory != "" {
		return "/" + s.cfg.Subdirectory
	}
	return "/"
}

// isHTTPS reports whether the app is published over TLS.
func (s *Server) isHTTPS() bool {
	return s.forceHTTPS
}
