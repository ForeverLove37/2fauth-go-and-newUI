package mail

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"io"
	"math/big"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"strings"
	"sync"
	"testing"
	"time"
)

func sampleVars() map[string]map[string]string {
	return map[string]map[string]string{
		KindResetPassword:   {"Email": "a@corp.test", "ResetURL": "https://app.test/password/reset?token=T&email=a", "ExpiresMinutes": "60"},
		KindVerifyEmail:     {"Email": "a@corp.test", "VerifyURL": "https://app.test/user/email/verify?token=T", "ExpiresHours": "24"},
		KindNewDevice:       {"Email": "a@corp.test", "Time": "2026-11-03 09:41 (UTC)", "IP": "10.0.0.1", "Browser": "Chrome", "Platform": "Linux", "Method": "password", "SecurityURL": "https://app.test/settings"},
		KindFailedLogin:     {"Email": "a@corp.test", "Time": "2026-11-03 09:41 (UTC)", "IP": "10.0.0.1", "Browser": "Chrome", "Platform": "Linux", "SecurityURL": "https://app.test/settings"},
		KindPasswordChanged: {"Email": "a@corp.test", "Time": "2026-11-03 09:41 (UTC)", "IP": "10.0.0.1", "ResetURL": "https://app.test/password/lost"},
		KindAccountShared:   {"OwnerName": "Zhou", "Label": "GitHub", "Account": "ops", "AppURL": "https://app.test/"},
		KindTransferred:     {"OwnerName": "Zhou", "Label": "GitHub", "Account": "", "AppURL": "https://app.test/"},
		KindTest:            {"Server": "smtp.test:587", "Encryption": "tls", "Time": "2026-11-03 09:41 (UTC)", "AdminName": "Zhou"},
	}
}

var brand = Brand{AppName: "2FAuth", AppURL: "https://app.test/"}

func TestEveryTemplateRendersInEveryLanguage(t *testing.T) {
	samples := sampleVars()
	if len(samples) != len(Kinds()) {
		t.Fatalf("samples = %d, kinds = %d", len(samples), len(Kinds()))
	}
	for kind, vars := range samples {
		for _, lang := range []string{"zh-CN", "en"} {
			msg, err := Render(kind, lang, brand, "a@corp.test", vars)
			if err != nil {
				t.Fatalf("%s/%s: %v", kind, lang, err)
			}
			if msg.Subject == "" || msg.Preheader == "" || !strings.Contains(msg.Text, "--") {
				t.Errorf("%s/%s: incomplete message %+v", kind, lang, msg)
			}
			if !strings.Contains(msg.HTML, `lang="`+lang+`"`) || !strings.Contains(msg.HTML, "<title>") {
				t.Errorf("%s/%s: missing document metadata", kind, lang)
			}
			if strings.Contains(msg.HTML, "<script") || strings.Contains(msg.HTML, "@@") {
				t.Errorf("%s/%s: unexpected markup", kind, lang)
			}
			// The action links and values must be in both alternatives.
			for name, value := range vars {
				// The sign-in method is translated rather than copied.
				if value == "" || name == "Method" {
					continue
				}
				if !strings.Contains(msg.Text, value) {
					t.Errorf("%s/%s: %s is missing from the text version", kind, lang, name)
				}
			}
		}
	}
}

func TestRenderEscapesUserContent(t *testing.T) {
	vars := map[string]string{
		"OwnerName": `<img src=x onerror=alert(1)>`,
		"Label":     `"><script>alert(2)</script>`,
		"Account":   `a&b`,
		"AppURL":    "https://app.test/",
	}
	msg, err := Render(KindAccountShared, "en", brand, "a@corp.test", vars)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"<img src=x", "<script>alert"} {
		if strings.Contains(msg.HTML, raw) {
			t.Errorf("%q reached the HTML", raw)
		}
	}
	if !strings.Contains(msg.HTML, "a&amp;b") {
		t.Error("the account was not escaped")
	}
}

func TestRenderRefusesBadInput(t *testing.T) {
	vars := sampleVars()[KindResetPassword]
	bad := map[string]string{}
	for k, v := range vars {
		bad[k] = v
	}
	bad["ResetURL"] = "javascript:alert(1)"
	if _, err := Render(KindResetPassword, "en", brand, "a@corp.test", bad); err == nil {
		t.Error("a javascript: link was accepted")
	}
	delete(bad, "ResetURL")
	if _, err := Render(KindResetPassword, "en", brand, "a@corp.test", bad); err == nil {
		t.Error("a missing variable was accepted")
	}
	if _, err := Render("nope", "en", brand, "a@corp.test", vars); err == nil {
		t.Error("an unknown template was accepted")
	}
}

func TestOptionalAccountLeavesNoEmptyLine(t *testing.T) {
	msg, err := Render(KindTransferred, "zh-CN", brand, "a@corp.test", sampleVars()[KindTransferred])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(msg.Text, "（）") || strings.Contains(msg.HTML, `line-height:26px;"></div>`) {
		t.Errorf("empty optional value: %s", msg.Text)
	}
}

func TestLang(t *testing.T) {
	for _, tc := range []struct{ pref, fallback, want string }{
		{"zh-TW", "en", "zh-CN"}, {"en", "zh-CN", "en"}, {"browser", "zh-CN", "zh-CN"}, {"fr", "", "en"}, {"", "en", "en"},
	} {
		if got := Lang(tc.pref, tc.fallback); got != tc.want {
			t.Errorf("Lang(%q, %q) = %q", tc.pref, tc.fallback, got)
		}
	}
}

func TestBuildProducesAReadableMessage(t *testing.T) {
	msg, err := Render(KindResetPassword, "zh-CN", brand, "a@corp.test", sampleVars()[KindResetPassword])
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Build(netmail.Address{Name: "二步验证", Address: "no-reply@corp.test"}, netmail.Address{Address: "a@corp.test"}, msg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	checkMessage(t, raw, msg)
}

func TestBuildRefusesHeaderInjection(t *testing.T) {
	_, err := Build(netmail.Address{Address: "a@corp.test"}, netmail.Address{Address: "b@corp.test"},
		Message{Subject: "hello\r\nBcc: evil@corp.test"}, time.Now())
	if err == nil {
		t.Fatal("a line break in the subject was accepted")
	}
}

func checkMessage(t *testing.T, raw []byte, msg Message) {
	t.Helper()
	parsed, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
	if err != nil || subject != msg.Subject {
		t.Errorf("subject = %q, %v", subject, err)
	}
	if parsed.Header.Get("Message-ID") == "" || parsed.Header.Get("Date") == "" {
		t.Error("missing Message-ID or Date")
	}
	_, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(parsed.Body, params["boundary"])
	found := map[string]string{}
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(quotedprintable.NewReader(part))
		if err != nil {
			t.Fatal(err)
		}
		// Line ends are CRLF on the wire.
		found[strings.Split(part.Header.Get("Content-Type"), ";")[0]] = strings.ReplaceAll(string(body), "\r\n", "\n")
	}
	if found["text/plain"] != msg.Text || found["text/html"] != msg.HTML {
		t.Errorf("the parts do not round-trip: %v", found)
	}
}

// fakeSMTP is a small SMTP server covering what Send needs.
type fakeSMTP struct {
	t         *testing.T
	listener  net.Listener
	tlsConfig *tls.Config
	implicit  bool
	startTLS  bool
	authMech  string
	password  string

	mu       sync.Mutex
	messages [][]byte
	authed   []string
	sawTLS   bool
}

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func startSMTP(t *testing.T, f *fakeSMTP) (string, int) {
	t.Helper()
	f.t = t
	f.tlsConfig = &tls.Config{Certificates: []tls.Certificate{selfSigned(t)}}
	var err error
	if f.implicit {
		f.listener, err = tls.Listen("tcp", "127.0.0.1:0", f.tlsConfig)
	} else {
		f.listener, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.listener.Close() })
	go func() {
		for {
			conn, err := f.listener.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()
	addr := f.listener.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port
}

func (f *fakeSMTP) serve(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	write := func(line string) { fmt.Fprintf(conn, "%s\r\n", line) }
	write("220 fake ESMTP")
	isTLS := f.implicit
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			write("250-fake")
			if f.startTLS && !isTLS {
				write("250-STARTTLS")
			}
			if f.authMech != "" {
				write("250-AUTH " + f.authMech)
			}
			write("250 8BITMIME")
		case upper == "STARTTLS":
			write("220 go")
			tlsConn := tls.Server(conn, f.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			reader = bufio.NewReader(conn)
			isTLS = true
			f.mu.Lock()
			f.sawTLS = true
			f.mu.Unlock()
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			decoded, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(line[len("AUTH PLAIN"):]))
			parts := strings.Split(string(decoded), "\x00")
			if len(parts) == 3 && parts[2] == f.password {
				f.mu.Lock()
				f.authed = append(f.authed, parts[1])
				f.mu.Unlock()
				write("235 ok")
			} else {
				write("535 bad credentials")
			}
		case strings.HasPrefix(upper, "AUTH LOGIN"):
			write("334 " + base64.StdEncoding.EncodeToString([]byte("Username:")))
			user, _ := reader.ReadString('\n')
			write("334 " + base64.StdEncoding.EncodeToString([]byte("Password:")))
			pass, _ := reader.ReadString('\n')
			u, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(user))
			p, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(pass))
			if string(p) == f.password {
				f.mu.Lock()
				f.authed = append(f.authed, string(u))
				f.mu.Unlock()
				write("235 ok")
			} else {
				write("535 bad credentials")
			}
		case strings.HasPrefix(upper, "MAIL FROM"), strings.HasPrefix(upper, "RCPT TO"):
			write("250 ok")
		case upper == "DATA":
			write("354 go")
			var body bytes.Buffer
			for {
				l, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				body.WriteString(strings.TrimPrefix(l, "."))
			}
			f.mu.Lock()
			f.messages = append(f.messages, body.Bytes())
			f.mu.Unlock()
			write("250 queued")
		case upper == "QUIT":
			write("221 bye")
			return
		default:
			write("500 what")
		}
	}
}

func TestSendModes(t *testing.T) {
	msg, err := Render(KindTest, "en", brand, "admin@corp.test", sampleVars()[KindTest])
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		server     fakeSMTP
		encryption string
	}{
		{"starttls plain auth", fakeSMTP{startTLS: true, authMech: "PLAIN", password: "pw"}, EncryptionTLS},
		{"starttls login auth", fakeSMTP{startTLS: true, authMech: "LOGIN", password: "pw"}, EncryptionTLS},
		{"implicit tls", fakeSMTP{implicit: true, authMech: "PLAIN", password: "pw"}, EncryptionSSL},
	}
	for i := range cases {
		tc := &cases[i]
		t.Run(tc.name, func(t *testing.T) {
			host, port := startSMTP(t, &tc.server)
			err := Send(context.Background(), Settings{
				Mailer: "smtp", Host: host, Port: port, Username: "user", Password: "pw",
				Encryption: tc.encryption, VerifyTLS: false, FromName: "2FAuth", FromAddress: "no-reply@corp.test",
			}, msg)
			if err != nil {
				t.Fatalf("Send: %v", err)
			}
			tc.server.mu.Lock()
			defer tc.server.mu.Unlock()
			if len(tc.server.messages) != 1 || len(tc.server.authed) != 1 || tc.server.authed[0] != "user" {
				t.Fatalf("messages = %d, authed = %v", len(tc.server.messages), tc.server.authed)
			}
			checkMessage(t, tc.server.messages[0], msg)
		})
	}
}

func TestSendRefusesToLowerTheSecurity(t *testing.T) {
	msg := Message{To: "a@corp.test", Subject: "s", Text: "t", HTML: "h"}

	// STARTTLS expected, not offered: nothing may be sent in clear text.
	plain := &fakeSMTP{authMech: "PLAIN", password: "pw"}
	host, port := startSMTP(t, plain)
	base := Settings{Mailer: "smtp", Host: host, Port: port, FromAddress: "n@corp.test", Encryption: EncryptionTLS}
	if err := Send(context.Background(), base, msg); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("no STARTTLS = %v", err)
	}

	// A self-signed certificate is refused unless explicitly accepted.
	strict := &fakeSMTP{startTLS: true}
	host, port = startSMTP(t, strict)
	base = Settings{Mailer: "smtp", Host: host, Port: port, FromAddress: "n@corp.test", Encryption: EncryptionTLS, VerifyTLS: true}
	if err := Send(context.Background(), base, msg); err == nil {
		t.Error("an untrusted certificate was accepted")
	}

	// A wrong password is reported.
	auth := &fakeSMTP{startTLS: true, authMech: "PLAIN", password: "right"}
	host, port = startSMTP(t, auth)
	base = Settings{Mailer: "smtp", Host: host, Port: port, FromAddress: "n@corp.test", Encryption: EncryptionTLS, Username: "u", Password: "wrong"}
	if err := Send(context.Background(), base, msg); err == nil || !strings.Contains(err.Error(), "authentication") {
		t.Errorf("wrong password = %v", err)
	}

	// Clear text with a password toward a remote host is refused by net/smtp and here.
	login := &fakeSMTP{authMech: "LOGIN", password: "pw"}
	host, port = startSMTP(t, login)
	base = Settings{Mailer: "smtp", Host: host, Port: port, FromAddress: "n@corp.test", Encryption: EncryptionNone, Username: "u", Password: "pw"}
	if err := Send(context.Background(), base, msg); err == nil {
		t.Error("a password was sent over a clear connection")
	}
}

func TestDisabledTransport(t *testing.T) {
	if err := Send(context.Background(), Settings{Mailer: "log"}, Message{To: "a@corp.test"}); err != ErrNotConfigured {
		t.Fatalf("Send = %v", err)
	}
	if (Settings{Mailer: "smtp", Host: "h"}).Enabled() {
		t.Error("a transport without a sender is enabled")
	}
}
