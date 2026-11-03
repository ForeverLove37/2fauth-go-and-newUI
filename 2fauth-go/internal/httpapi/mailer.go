package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"2fauth/internal/mail"
	"2fauth/internal/store"
)

// Runtime settings of the mail transport (admin editable, they win over MAIL_*).
const (
	settingMailMailer     = "mail.mailer"
	settingMailHost       = "mail.host"
	settingMailPort       = "mail.port"
	settingMailUsername   = "mail.username"
	settingMailPassword   = "mail.password"
	settingMailEncryption = "mail.encryption"
	settingMailVerifyTLS  = "mail.verify_tls"
	settingMailFromName   = "mail.from_name"
	settingMailFromAddr   = "mail.from_address"
)

// failedAlertWindow is the shortest time between two "failed sign-in" e-mails.
const failedAlertWindow = 15 * time.Minute

// mailSettings resolves the effective transport: a runtime setting when it is
// present, the environment value otherwise.
func (s *Server) mailSettings() mail.Settings {
	eff := mail.Settings{
		Mailer:      s.cfg.MailMailer,
		Host:        s.cfg.MailHost,
		Port:        s.cfg.MailPort,
		Username:    s.cfg.MailUsername,
		Password:    s.cfg.MailPassword,
		Encryption:  s.cfg.MailEncryption,
		VerifyTLS:   s.cfg.MailVerifyTLS,
		FromName:    s.cfg.MailFromName,
		FromAddress: s.cfg.MailFromAddr,
	}
	if v, ok := s.settingString(settingMailMailer); ok {
		eff.Mailer = v
	}
	if v, ok := s.settingString(settingMailHost); ok {
		eff.Host = v
	}
	if v, ok := s.settingString(settingMailPort); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			eff.Port = n
		}
	}
	if v, ok := s.settingString(settingMailUsername); ok {
		eff.Username = v
	}
	if v, ok := s.settingString(settingMailPassword); ok {
		eff.Password = s.decryptStoredSecret(v)
	}
	if v, ok := s.settingString(settingMailEncryption); ok {
		eff.Encryption = v
	}
	if v, ok := s.settingBool(settingMailVerifyTLS); ok {
		eff.VerifyTLS = v
	}
	if v, ok := s.settingString(settingMailFromName); ok {
		eff.FromName = v
	}
	if v, ok := s.settingString(settingMailFromAddr); ok {
		eff.FromAddress = v
	}
	if eff.FromName == "" {
		eff.FromName = s.cfg.AppName
	}
	return eff
}

// publicURL returns the absolute URL of a path of the application.
func (s *Server) publicURL(path string) string {
	return s.publicBaseURL(nil) + path
}

func (s *Server) mailBrand() mail.Brand {
	return mail.Brand{AppName: s.cfg.AppName, AppURL: s.publicURL("/")}
}

// mailLang picks the language of an e-mail from the preferences of its recipient.
func (s *Server) mailLang(user *store.User) string {
	prefs := s.st.PreferencesWithDefaults(user)
	lang, _ := prefs["lang"].(string)
	return mail.Lang(lang, s.cfg.MailLocale)
}

// deliver renders and sends one e-mail, logging the outcome without its content:
// reset and verification links are credentials.
func (s *Server) deliver(kind string, user *store.User, vars map[string]string) {
	settings := s.mailSettings()
	if !settings.Enabled() {
		return
	}
	msg, err := mail.Render(kind, s.mailLang(user), s.mailBrand(), user.Email, vars)
	if err != nil {
		slog.Error("cannot render an e-mail", "kind", kind, "error", err)
		return
	}
	if err := mail.Send(context.Background(), settings, msg); err != nil {
		slog.Warn("cannot deliver an e-mail", "kind", kind, "user_id", user.ID, "error", err)
		return
	}
	slog.Info("e-mail delivered", "kind", kind, "user_id", user.ID)
}

// runMailJob runs a delivery in the background: a slow SMTP server must not delay
// the request, and its timing must not reveal anything to the caller.
func (s *Server) runMailJob(job func()) {
	s.mailJobs.Add(1)
	go func() {
		defer s.mailJobs.Done()
		s.mailSlots <- struct{}{}
		defer func() { <-s.mailSlots }()
		job()
	}()
}

// WaitForMail blocks until the background deliveries are finished.
func (s *Server) WaitForMail() { s.mailJobs.Wait() }

// sendAsync delivers one e-mail in the background.
func (s *Server) sendAsync(kind string, user *store.User, vars map[string]string) {
	if !s.mailSettings().Enabled() {
		return
	}
	s.runMailJob(func() { s.deliver(kind, user, vars) })
}

// mailTime formats a moment in the time zone of the recipient.
func (s *Server) mailTime(user *store.User, at time.Time) string {
	tz := s.userTimezone(user)
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc, tz = time.UTC, "UTC"
	}
	return at.In(loc).Format("2006-01-02 15:04") + " (" + tz + ")"
}

func (s *Server) prefEnabled(user *store.User, name string) bool {
	enabled, _ := s.st.PreferencesWithDefaults(user)[name].(bool)
	return enabled
}

// notifyNewDevice warns the user about a sign-in from an unknown address or browser.
// The very first sign-in has nothing to compare with and stays silent. It must run
// before the current sign-in is logged.
func (s *Server) notifyNewDevice(c *ctx, user *store.User, method string) {
	if !s.prefEnabled(user, "notifyOnNewAuthDevice") || s.st.CountSuccessfulLogins(user.ID) == 0 {
		return
	}
	ip, agent := s.clientIP(c.r), c.r.UserAgent()
	if s.st.SeenDevice(user.ID, ip, agent) {
		return
	}
	browser, platform, _ := parseUserAgent(agent)
	s.sendAsync(mail.KindNewDevice, user, map[string]string{
		"Email":       user.Email,
		"Time":        s.mailTime(user, time.Now()),
		"IP":          ip,
		"Browser":     browser,
		"Platform":    platform,
		"Method":      method,
		"SecurityURL": s.publicURL("/settings?tab=account"),
	})
}

// notifyFailedLogin warns about a wrong password, at most once per window: the
// alert is triggered by whoever knows an address, so it must not be a mail bomb.
func (s *Server) notifyFailedLogin(c *ctx, user *store.User) {
	if !s.prefEnabled(user, "notifyOnFailedLogin") {
		return
	}
	if ok, _ := s.failedAlerts.Allow(strconv.FormatInt(user.ID, 10), 1, failedAlertWindow); !ok {
		return
	}
	browser, platform, _ := parseUserAgent(c.r.UserAgent())
	s.sendAsync(mail.KindFailedLogin, user, map[string]string{
		"Email":       user.Email,
		"Time":        s.mailTime(user, time.Now()),
		"IP":          s.clientIP(c.r),
		"Browser":     browser,
		"Platform":    platform,
		"SecurityURL": s.publicURL("/settings?tab=account"),
	})
}

// notifyPasswordChanged confirms a password change to the owner of the account.
func (s *Server) notifyPasswordChanged(c *ctx, user *store.User) {
	s.sendAsync(mail.KindPasswordChanged, user, map[string]string{
		"Email":    user.Email,
		"Time":     s.mailTime(user, time.Now()),
		"IP":       s.clientIP(c.r),
		"ResetURL": s.publicURL("/password/lost"),
	})
}

// accountLabel is how an account is named in an e-mail.
func accountLabel(a *store.Account) (string, string) {
	if a.Service != "" {
		return a.Service, a.Account
	}
	return a.Account, ""
}

func (s *Server) notifyAccountEvent(kind, pref string, owner, recipient *store.User, a *store.Account) {
	if !s.prefEnabled(recipient, pref) {
		return
	}
	label, account := accountLabel(a)
	s.sendAsync(kind, recipient, map[string]string{
		"OwnerName": owner.Name,
		"Label":     label,
		"Account":   account,
		"AppURL":    s.publicURL("/"),
	})
}

// ---------------------------------------------------------------------------
// admin API
// ---------------------------------------------------------------------------

func (s *Server) mailSettingsPayload() map[string]any {
	eff := s.mailSettings()
	return map[string]any{
		"mailer":       eff.Mailer,
		"host":         eff.Host,
		"port":         eff.Port,
		"username":     eff.Username,
		"password_set": eff.Password != "",
		"encryption":   eff.Encryption,
		"verify_tls":   eff.VerifyTLS,
		"from_name":    eff.FromName,
		"from_address": eff.FromAddress,
		"enabled":      eff.Enabled(),
	}
}

// showMailSettings handles GET /api/v1/mail/settings. The password never leaves.
func (s *Server) showMailSettings(c *ctx) {
	jsonResponse(c.w, http.StatusOK, s.mailSettingsPayload())
}

// updateMailSettings handles PUT /api/v1/mail/settings.
func (s *Server) updateMailSettings(c *ctx) {
	v := newValidator(c)
	mailer := strings.ToLower(strings.TrimSpace(v.optional("mailer")))
	v.in("mailer", mailer, "smtp", "log")
	encryption := strings.ToLower(strings.TrimSpace(v.optional("encryption")))
	v.in("encryption", encryption, mail.EncryptionTLS, mail.EncryptionSSL, mail.EncryptionNone)
	if v.has("port") {
		if port, ok := v.integer("port"); ok {
			v.between("port", port, 1, 65535)
		}
	}
	host := strings.TrimSpace(v.optional("host"))
	if strings.ContainsAny(host, " /\\\r\n") {
		v.fail("host", "The host field must be a host name or an address.")
	}
	from := strings.TrimSpace(v.optional("from_address"))
	v.email("from_address", from)
	if strings.ContainsAny(v.optional("from_name"), "\r\n") {
		v.fail("from_name", "The from name must fit on one line.")
	}
	if v.respond() {
		return
	}

	values := map[string]any{}
	if v.has("mailer") {
		values[settingMailMailer] = mailer
	}
	if v.has("host") {
		values[settingMailHost] = host
	}
	if v.has("port") {
		values[settingMailPort] = strings.TrimSpace(v.optional("port"))
	}
	if v.has("username") {
		values[settingMailUsername] = strings.TrimSpace(v.optional("username"))
	}
	if v.has("encryption") {
		values[settingMailEncryption] = encryption
	}
	if verify, present := v.boolean("verify_tls"); present {
		values[settingMailVerifyTLS] = verify
	}
	if v.has("from_name") {
		values[settingMailFromName] = strings.TrimSpace(v.optional("from_name"))
	}
	if v.has("from_address") {
		values[settingMailFromAddr] = from
	}
	for name, value := range values {
		if err := s.settings.Set(name, value); err != nil {
			s.srvError(c, err)
			return
		}
	}

	clearPassword, _ := v.boolean("clear_password")
	password := v.optional("password")
	switch {
	case clearPassword:
		// An empty value wins over MAIL_PASSWORD; deleting would fall back to it.
		if err := s.settings.Set(settingMailPassword, ""); err != nil {
			s.srvError(c, err)
			return
		}
	case password != "":
		encrypted, err := s.encryptStoredSecret(password)
		if err != nil {
			s.srvError(c, err)
			return
		}
		if err := s.settings.Set(settingMailPassword, encrypted); err != nil {
			s.srvError(c, err)
			return
		}
	}
	jsonResponse(c.w, http.StatusOK, s.mailSettingsPayload())
}

// systemTestEmail handles POST /system/test-email: it really sends a message to the
// administrator and reports the transport error, which only they can read.
func (s *Server) systemTestEmail(c *ctx) {
	settings := s.mailSettings()
	fail := func(detail string) {
		jsonResponse(c.w, http.StatusFailedDependency, map[string]any{
			"message": "Failed to send the test email",
			"details": detail,
		})
	}
	if !settings.Enabled() {
		fail("no mail transport configured: choose smtp and set a host and a sender address")
		return
	}
	port := settings.Port
	if port == 0 {
		port = 587
	}
	encryption := settings.Encryption
	if encryption == "" {
		encryption = mail.EncryptionTLS
	}
	msg, err := mail.Render(mail.KindTest, s.mailLang(c.user), s.mailBrand(), c.user.Email, map[string]string{
		"Server":     settings.Host + ":" + strconv.Itoa(port),
		"Encryption": encryption,
		"Time":       s.mailTime(c.user, time.Now()),
		"AdminName":  c.user.Name,
	})
	if err != nil {
		fail(err.Error())
		return
	}
	if err := mail.Send(c.r.Context(), settings, msg); err != nil {
		fail(err.Error())
		return
	}
	jsonMessage(c.w, http.StatusOK, "Ok")
}
