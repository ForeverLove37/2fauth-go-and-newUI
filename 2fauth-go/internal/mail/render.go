package mail

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	"io/fs"
	"net/url"
	"sort"
	"strings"
	texttemplate "text/template"
	"time"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// Template names.
const (
	KindResetPassword   = "reset_password"
	KindVerifyEmail     = "verify_email"
	KindNewDevice       = "new_device"
	KindFailedLogin     = "failed_login"
	KindPasswordChanged = "password_changed"
	KindAccountShared   = "account_shared"
	KindTransferred     = "account_transferred"
	KindTest            = "test_email"
)

// required lists the variables each template needs. Variables whose name ends in
// URL must be absolute http(s) links. Every variable is plain text: the template
// engines escape it for its context.
var required = map[string][]string{
	KindResetPassword:   {"Email", "ResetURL", "ExpiresMinutes"},
	KindVerifyEmail:     {"Email", "VerifyURL", "ExpiresHours"},
	KindNewDevice:       {"Email", "Time", "IP", "Browser", "Platform", "Method", "SecurityURL"},
	KindFailedLogin:     {"Email", "Time", "IP", "Browser", "Platform", "SecurityURL"},
	KindPasswordChanged: {"Email", "Time", "IP", "ResetURL"},
	KindAccountShared:   {"OwnerName", "Label", "Account", "AppURL"},
	KindTransferred:     {"OwnerName", "Label", "Account", "AppURL"},
	KindTest:            {"Server", "Encryption", "Time", "AdminName"},
}

// Kinds returns every template name, sorted.
func Kinds() []string {
	out := make([]string, 0, len(required))
	for kind := range required {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}

// Brand is the identity shown in every e-mail.
type Brand struct {
	AppName string
	AppURL  string
}

type button struct{ URL, Label string }

type fact struct{ Label, Value string }

type pageData struct {
	App   string
	Home  string
	Lang  string
	ZH    bool
	Year  int
	Title string
	Pre   string
	V     map[string]string
	// MSO holds the Outlook width wrappers: html/template strips HTML comments.
	MSOOpen, MSOClose htmltemplate.HTML
}

var funcs = map[string]any{
	"btn":  func(u, label string) button { return button{u, label} },
	"fact": func(label, value string) fact { return fact{label, value} },
}

// Lang normalises a preference to one of the languages of the templates.
func Lang(preference, fallback string) string {
	for _, value := range []string{preference, fallback} {
		lower := strings.ToLower(strings.TrimSpace(value))
		switch {
		case strings.HasPrefix(lower, "zh"):
			return "zh-CN"
		case strings.HasPrefix(lower, "en"):
			return "en"
		}
	}
	return "en"
}

// Render produces the subject, summary, plain text and HTML of an e-mail.
func Render(kind, lang string, brand Brand, to string, vars map[string]string) (Message, error) {
	names, ok := required[kind]
	if !ok {
		return Message{}, fmt.Errorf("mail: unknown template %q", kind)
	}
	for _, name := range names {
		value, present := vars[name]
		if !present {
			return Message{}, fmt.Errorf("mail: template %s: missing variable %s", kind, name)
		}
		if strings.HasSuffix(name, "URL") {
			parsed, err := url.Parse(value)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return Message{}, fmt.Errorf("mail: template %s: %s is not an absolute http(s) URL", kind, name)
			}
		}
	}
	lang = Lang(lang, "en")
	data := pageData{
		App: brand.AppName, Home: brand.AppURL, Lang: lang, ZH: lang == "zh-CN", Year: time.Now().Year(), V: vars,
		MSOOpen:  `<!--[if mso]><table role="presentation" width="640" align="center" cellpadding="0" cellspacing="0" border="0"><tr><td><![endif]-->`,
		MSOClose: `<!--[if mso]></td></tr></table><![endif]-->`,
	}

	textSource, err := fs.ReadFile(templateFS, "templates/"+kind+".txt.tmpl")
	if err != nil {
		return Message{}, err
	}
	texts, err := texttemplate.New(kind).Option("missingkey=error").Funcs(funcs).Parse(string(textSource))
	if err != nil {
		return Message{}, fmt.Errorf("mail: template %s: %w", kind, err)
	}
	plain := func(block string) (string, error) {
		var buf bytes.Buffer
		if err := texts.ExecuteTemplate(&buf, block, data); err != nil {
			return "", fmt.Errorf("mail: template %s/%s: %w", kind, block, err)
		}
		return strings.TrimSpace(buf.String()), nil
	}
	msg := Message{To: to}
	if msg.Subject, err = plain("subject"); err != nil {
		return Message{}, err
	}
	if msg.Preheader, err = plain("preheader"); err != nil {
		return Message{}, err
	}
	if msg.Text, err = plain("text"); err != nil {
		return Message{}, err
	}
	msg.Text += "\n"
	if data.ZH {
		msg.Text += "\n--\n这封邮件由 " + brand.AppName + " 自动发送，请勿直接回复。\n"
	} else {
		msg.Text += "\n--\nSent automatically by " + brand.AppName + ". Replies are not read.\n"
	}

	data.Title, data.Pre = msg.Subject, msg.Preheader
	pages, err := htmltemplate.New("layout.html.tmpl").Option("missingkey=error").Funcs(funcs).
		ParseFS(templateFS, "templates/layout.html.tmpl", "templates/"+kind+".html.tmpl")
	if err != nil {
		return Message{}, fmt.Errorf("mail: template %s: %w", kind, err)
	}
	var page bytes.Buffer
	if err := pages.ExecuteTemplate(&page, "layout.html.tmpl", data); err != nil {
		return Message{}, fmt.Errorf("mail: template %s: %w", kind, err)
	}
	msg.HTML = page.String()
	return msg, nil
}
