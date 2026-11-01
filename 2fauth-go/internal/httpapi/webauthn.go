package httpapi

import "net/http"

// WebAuthn support is not yet enabled in this build. The endpoints are kept so
// that the SPA can detect the feature as unavailable instead of failing with a
// routing error.

// webauthnLoginOptions handles POST /webauthn/login/options.
func (s *Server) webauthnLoginOptions(c *ctx) {
	s.webauthnUnavailable(c)
}

// webauthnLogin handles POST /webauthn/login.
func (s *Server) webauthnLogin(c *ctx) {
	s.webauthnUnavailable(c)
}

// webauthnLost handles POST /webauthn/lost.
func (s *Server) webauthnLost(c *ctx) {
	s.webauthnUnavailable(c)
}

// webauthnRecover handles POST /webauthn/recover.
func (s *Server) webauthnRecover(c *ctx) {
	s.webauthnUnavailable(c)
}

// webauthnRegisterOptions handles GET /webauthn/register/options.
func (s *Server) webauthnRegisterOptions(c *ctx) {
	s.webauthnUnavailable(c)
}

// webauthnRegister handles POST /webauthn/register.
func (s *Server) webauthnRegister(c *ctx) {
	s.webauthnUnavailable(c)
}

// webauthnCredentials handles GET /webauthn/credentials.
func (s *Server) webauthnCredentials(c *ctx) {
	credentials, err := s.st.ListWebAuthnCredentials(c.user.ID)
	if err != nil {
		jsonMessage(c.w, http.StatusInternalServerError, "server error")
		return
	}
	out := make([]map[string]any, 0, len(credentials))
	for _, cred := range credentials {
		out = append(out, map[string]any{
			"id":         cred.ID,
			"name":       cred.Name,
			"created_at": cred.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	jsonCollection(c.w, http.StatusOK, out)
}

// webauthnRenameCredential handles PATCH /webauthn/credentials/{id}.
func (s *Server) webauthnRenameCredential(c *ctx) {
	v := newValidator(c)
	name := v.required("name")
	if v.respond() {
		return
	}
	id := c.param("id")
	if id == "" || !s.st.RenameWebAuthnCredential(id, c.user.ID, name) {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return
	}
	jsonMessage(c.w, http.StatusOK, "credential renamed")
}

// webauthnDeleteCredential handles DELETE /webauthn/credentials/{id}.
func (s *Server) webauthnDeleteCredential(c *ctx) {
	id := c.param("id")
	if id == "" || !s.st.DeleteWebAuthnCredential(id, c.user.ID) {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return
	}
	jsonMessage(c.w, http.StatusOK, "credential deleted")
}

// webauthnUnavailable reports that WebAuthn is not part of this build.
func (s *Server) webauthnUnavailable(c *ctx) {
	jsonResponse(c.w, http.StatusNotImplemented, map[string]any{
		"message": "WebAuthn is not available in this build",
	})
}
