// Package mail renders the transactional e-mails of the application and delivers
// them over SMTP. See README.md in this directory for the variables of each template.
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// Encryption modes of the SMTP connection.
const (
	// EncryptionTLS upgrades the connection with STARTTLS and refuses to continue
	// when the server does not offer it (usually port 587).
	EncryptionTLS = "tls"
	// EncryptionSSL opens TLS from the first byte (usually port 465).
	EncryptionSSL = "ssl"
	// EncryptionNone sends in clear text; only meant for a local relay.
	EncryptionNone = "none"
)

// sendTimeout bounds a whole delivery.
const sendTimeout = 30 * time.Second

// ErrNotConfigured reports that no SMTP transport is configured.
var ErrNotConfigured = errors.New("mail: no SMTP transport is configured")

// Settings describes the transport and the sender.
type Settings struct {
	Mailer      string // smtp | log
	Host        string
	Port        int
	Username    string
	Password    string
	Encryption  string // tls | ssl | none
	VerifyTLS   bool
	FromName    string
	FromAddress string
}

// Enabled reports whether messages can really be delivered.
func (s Settings) Enabled() bool {
	return strings.EqualFold(s.Mailer, "smtp") && s.Host != "" && s.FromAddress != ""
}

// Message is a rendered e-mail.
type Message struct {
	To        string
	Subject   string
	Preheader string
	Text      string
	HTML      string
}

// Send delivers the message.
func Send(ctx context.Context, s Settings, m Message) error {
	if !s.Enabled() {
		return ErrNotConfigured
	}
	to, err := netmail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("mail: invalid recipient: %w", err)
	}
	from, err := netmail.ParseAddress(s.FromAddress)
	if err != nil {
		return fmt.Errorf("mail: invalid sender address: %w", err)
	}
	raw, err := Build(netmail.Address{Name: s.FromName, Address: from.Address}, *to, m, time.Now())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	client, err := dial(ctx, s)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("mail: sender refused: %w", err)
	}
	if err := client.Rcpt(to.Address); err != nil {
		return fmt.Errorf("mail: recipient refused: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA refused: %w", err)
	}
	if _, err := writer.Write(raw); err != nil {
		return fmt.Errorf("mail: cannot write the message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("mail: message refused: %w", err)
	}
	return client.Quit()
}

// Check connects and authenticates without sending anything.
func Check(ctx context.Context, s Settings) error {
	if !s.Enabled() {
		return ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	client, err := dial(ctx, s)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	return client.Quit()
}

// dial opens an authenticated SMTP session. TLS is never downgraded silently: with
// EncryptionTLS a server that does not offer STARTTLS is an error.
func dial(ctx context.Context, s Settings) (*smtp.Client, error) {
	port := s.Port
	if port == 0 {
		port = 587
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("mail: cannot reach %s: %w", addr, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	tlsConfig := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: !s.VerifyTLS} //nolint:gosec // an explicit administrator choice, on by default

	mode := strings.ToLower(strings.TrimSpace(s.Encryption))
	if mode == EncryptionSSL {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("mail: TLS handshake failed: %w", err)
		}
		conn = tlsConn
	}
	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("mail: the server did not greet: %w", err)
	}
	if err := client.Hello(localName(s.FromAddress)); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("mail: EHLO refused: %w", err)
	}
	if mode != EncryptionSSL && mode != EncryptionNone {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			_ = client.Close()
			return nil, errors.New("mail: the server does not offer STARTTLS (set the encryption to ssl or none if this is expected)")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("mail: STARTTLS failed: %w", err)
		}
	}
	if s.Username != "" {
		if err := authenticate(client, s); err != nil {
			_ = client.Close()
			return nil, err
		}
	}
	return client, nil
}

func authenticate(client *smtp.Client, s Settings) error {
	ok, mechanisms := client.Extension("AUTH")
	if !ok {
		return errors.New("mail: the server does not accept authentication on this connection")
	}
	var auth smtp.Auth
	if hasMechanism(mechanisms, "PLAIN") {
		// net/smtp refuses PLAIN over a clear connection, except to localhost.
		auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
	} else if hasMechanism(mechanisms, "LOGIN") {
		if _, isTLS := client.TLSConnectionState(); !isTLS {
			return errors.New("mail: refusing to send the password over a clear connection")
		}
		auth = &loginAuth{username: s.Username, password: s.Password}
	} else {
		return fmt.Errorf("mail: no supported authentication mechanism in %q", mechanisms)
	}
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("mail: authentication failed: %w", err)
	}
	return nil
}

func hasMechanism(list, name string) bool {
	for _, item := range strings.Fields(list) {
		if strings.EqualFold(item, name) {
			return true
		}
	}
	return false
}

// loginAuth implements the LOGIN mechanism still required by some providers.
type loginAuth struct {
	username, password string
}

func (a *loginAuth) Start(*smtp.ServerInfo) (string, []byte, error) { return "LOGIN", nil, nil }

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimRight(string(fromServer), ": ")) {
	case "username":
		return []byte(a.username), nil
	case "password":
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("mail: unexpected LOGIN challenge %q", fromServer)
}

func localName(from string) string {
	if at := strings.LastIndex(from, "@"); at >= 0 && at+1 < len(from) {
		return strings.Trim(from[at+1:], "> ")
	}
	return "localhost"
}

// Build renders the RFC 5322 multipart/alternative message.
func Build(from, to netmail.Address, m Message, at time.Time) ([]byte, error) {
	for _, value := range []string{m.Subject, from.Name, to.Name} {
		if strings.ContainsAny(value, "\r\n") {
			return nil, errors.New("mail: line break in a header value")
		}
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	header := func(key, value string) { fmt.Fprintf(&buf, "%s: %s\r\n", key, value) }
	header("From", from.String())
	header("To", to.String())
	header("Subject", mime.BEncoding.Encode("utf-8", m.Subject))
	header("Date", at.Format(time.RFC1123Z))
	header("Message-ID", fmt.Sprintf("<%s@%s>", hex.EncodeToString(id), localName(from.Address)))
	header("MIME-Version", "1.0")
	// Automatic replies must not answer these messages.
	header("Auto-Submitted", "auto-generated")
	header("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", writer.Boundary()))
	buf.WriteString("\r\n")

	for _, part := range []struct{ contentType, body string }{
		{"text/plain; charset=utf-8", m.Text},
		{"text/html; charset=utf-8", m.HTML},
	} {
		w, err := writer.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.contentType},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		qp := quotedprintable.NewWriter(w)
		if _, err := qp.Write([]byte(part.body)); err != nil {
			return nil, err
		}
		if err := qp.Close(); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
