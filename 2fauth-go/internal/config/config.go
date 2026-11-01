// Package config loads 2FAuth configuration from a .env file plus process
// environment, mirroring the Laravel config/*.php defaults shipped by 2FAuth v9.
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is the resolved runtime configuration.
type Config struct {
	AppName      string
	Env          string
	Timezone     string
	Debug        bool
	Key          string // APP_KEY, either "base64:..." or a raw 32 char string
	URL          string
	Subdirectory string // without leading slash, "" when not set
	SiteOwner    string
	Version      string

	IsDemoApp    bool
	IsTestingApp bool

	ContentSecurityPolicy         bool
	BlockOtpauthImagelinkFetching bool

	// Database
	DBConnection string // sqlite | mysql | pgsql
	DBDatabase   string
	DBHost       string
	DBPort       string
	DBUsername   string
	DBPassword   string

	// Sessions
	SessionDriver   string
	SessionLifetime int // minutes
	SessionSecure   bool

	// Auth / security
	AuthenticationGuard    string // web-guard | reverse-proxy-guard
	LoginThrottle          int
	ThrottleAPI            int
	ThrottleAPIImport      int
	AuthLogRetentionTime   int
	OtpLogRetentionTime    int
	AuthProxyHeaderForUser string
	AuthProxyHeaderForMail string
	ProxyLogoutURL         string
	TrustedProxies         string

	// Webauthn
	WebAuthnName           string
	WebAuthnID             string
	WebAuthnUserValidation string

	// Mail
	MailMailer     string
	MailHost       string
	MailPort       int
	MailUsername   string
	MailPassword   string
	MailEncryption string
	MailFromName   string
	MailFromAddr   string

	// SSO (OpenID Connect)
	//
	// The OIDC_* values below are the *defaults* of the SSO configuration: the
	// matching runtime settings (openid.*, editable by an administrator through
	// the API) take precedence whenever they are present.
	OpenIDEnabled      bool
	OpenIDIssuer       string
	OpenIDClientID     string
	OpenIDClientSecret string
	OpenIDScopes       string
	OpenIDButtonLabel  string
	OpenIDAutoRegister bool
	OpenIDAutoRedirect bool
	GithubClientSecret string

	// Icon provider chain defaults (ICON_PROVIDERS / ICON_PROVIDER_MAX_TRIES).
	// They seed the iconProviders / iconProviderMaxTries settings, which an
	// administrator can override: the stored value always wins over this file.
	IconProviders        string
	IconProviderMaxTries int

	// Preferences defaults (USERPREF_DEFAULT__*)
	DefaultPreferences map[string]any
	// LockedPreferences (USERPREF_LOCKED__*)
	LockedPreferences []string
	// Default admin settings (config/2fauth.php settings section)
	Settings map[string]any

	Locales []string

	// Runtime paths
	BasePath     string // directory containing the .env and default data dirs
	AssetsPath   string // directory holding the public/ front-end assets
	UIPath       string // directory holding the compiled React panel (index.html + assets)
	DatabasePath string // resolved sqlite file path
}

// URL paths documented by the front end.
const (
	InstallDocURL     = "https://docs.2fauth.app/getting-started/installation/self-hosted-server/"
	UpgradeDocURL     = "https://docs.2fauth.app/getting-started/upgrade/"
	SsoDocURL         = "https://docs.2fauth.app/security/authentication/sso/"
	ExportSchemaURL   = "https://docs.2fauth.app/usage/migration/#export-schema"
	RepositoryURL     = "https://github.com/Bubka/2FAuth"
	LatestReleaseURL  = "https://api.github.com/repos/Bubka/2FAuth/releases/latest"
	AppVersionDefault = "9.0.0"
)

// Locales mirrors config('2fauth.locales').
var Locales = []string{
	"bg", "zh-CN", "zh-TW", "da", "nl", "en", "fr", "de", "hi", "it", "ja", "ko", "pt-BR", "ru", "es-ES", "tr",
}

// Defaults of the OpenID Connect single sign-on configuration.
const (
	DefaultOpenIDScopes = "openid profile email"
	DefaultOpenIDLabel  = "Sign in with SSO"
)

// Defaults of the icon provider chain. The order of DefaultIconProviders is the
// order of the provider table: the most specific sets come first and the search
// based Iconify is the last resort.
const (
	DefaultIconProviders        = "selfhst,dashboard,walkx,simpleicons,lobehub,iconify"
	DefaultIconProviderMaxTries = 3
)

// Setting names of the icon provider chain (admin editable, they win over the
// ICON_PROVIDERS / ICON_PROVIDER_MAX_TRIES environment defaults).
const (
	SettingIconProviders        = "iconProviders"
	SettingIconProviderMaxTries = "iconProviderMaxTries"
)

type prefDefault struct {
	name     string
	envKey   string
	fallback any
}

// preferenceDefaults mirrors $preferences in config/2fauth.php, in declaration order.
var preferenceDefaults = []prefDefault{
	{"showOtpAsDot", "USERPREF_DEFAULT__SHOW_OTP_AS_DOT", false},
	{"showNextOtp", "USERPREF_DEFAULT__SHOW_NEXT_OTP", true},
	{"revealDottedOTP", "USERPREF_DEFAULT__REVEAL_DOTTED_OTP", false},
	{"closeOtpOnCopy", "USERPREF_DEFAULT__CLOSE_OTP_ON_COPY", false},
	{"copyOtpOnDisplay", "USERPREF_DEFAULT__COPY_OTP_ON_DISPLAY", false},
	{"clearSearchOnCopy", "USERPREF_DEFAULT__CLEAR_SEARCH_ON_COPY", false},
	{"useBasicQrcodeReader", "USERPREF_DEFAULT__USE_BASIC_QRCODE_READER", false},
	{"displayMode", "USERPREF_DEFAULT__DISPLAY_MODE", "list"},
	{"useDesktopTableLayout", "USERPREF_DEFAULT__USE_DESKTOP_TABLE_LAYOUT", false},
	{"showAccountsIcons", "USERPREF_DEFAULT__SHOW_ACCOUNTS_ICONS", true},
	{"iconSource", "USERPREF_DEFAULT__ICON_SOURCE", "logolib"},
	{"iconCollection", "USERPREF_DEFAULT__ICON_COLLECTION", "selfh"},
	{"iconPack", "USERPREF_DEFAULT__ICON_PACK", nil},
	{"iconVariant", "USERPREF_DEFAULT__ICON_VARIANT", "regular"},
	{"iconVariantStrictFetch", "USERPREF_DEFAULT__ICON_VARIANT_STRICT_FETCH", false},
	{"kickUserAfter", "USERPREF_DEFAULT__KICK_USER_AFTER", 15},
	{"activeGroup", "", 0},
	{"useGroupChips", "USERPREF_DEFAULT__USE_GROUP_CHIPS", true},
	{"showVirtualChips", "USERPREF_DEFAULT__SHOW_VIRTUAL_CHIPS", true},
	{"rememberActiveGroup", "USERPREF_DEFAULT__REMEMBER_ACTIVE_GROUP", true},
	{"viewDefaultGroupOnCopy", "USERPREF_DEFAULT__VIEW_DEFAULT_GROUP_ON_COPY", false},
	{"defaultGroup", "", 0},
	{"defaultCaptureMode", "USERPREF_DEFAULT__DEFAULT_CAPTURE_MODE", "livescan"},
	{"useDirectCapture", "USERPREF_DEFAULT__USE_DIRECT_CAPTURE", false},
	{"useWebauthnOnly", "USERPREF_DEFAULT__USE_WEBAUTHN_ONLY", false},
	{"getOfficialIcons", "USERPREF_DEFAULT__GET_OFFICIAL_ICONS", true},
	{"theme", "USERPREF_DEFAULT__THEME", "system"},
	{"formatPassword", "USERPREF_DEFAULT__FORMAT_PASSWORD", true},
	{"formatPasswordBy", "USERPREF_DEFAULT__FORMAT_PASSWORD_BY", 0.5},
	{"lang", "USERPREF_DEFAULT__LANG", "browser"},
	{"getOtpOnRequest", "USERPREF_DEFAULT__GET_OTP_ON_REQUEST", true},
	{"notifyOnNewAuthDevice", "USERPREF_DEFAULT__NOTIFY_ON_NEW_AUTH_DEVICE", false},
	{"notifyOnFailedLogin", "USERPREF_DEFAULT__NOTIFY_ON_FAILED_LOGIN", false},
	{"notifyOnOwnershipTransfer", "USERPREF_DEFAULT__NOTIFY_ON_OWNERSHIP_TRANSFER", true},
	{"notifyOnShare", "USERPREF_DEFAULT__NOTIFY_ON_SHARE", true},
	{"timezone", "USERPREF_DEFAULT__TIMEZONE", "UTC"},
	{"sortOrder", "USERPREF_DEFAULT__SORT_ORDER", "asc"},
	{"sortCaseSensitive", "USERPREF_DEFAULT__SORT_CASE_SENSITIVE", false},
	{"autoCloseTimeout", "USERPREF_DEFAULT__AUTO_CLOSE_TIMEOUT", 2},
	{"AutoSaveQrcodedAccount", "USERPREF_DEFAULT__AUTO_SAVE_QRCODED_ACCOUNT", false},
	{"showEmailInFooter", "USERPREF_DEFAULT__SHOW_EMAIL_IN_FOOTER", true},
	{"showQuickNavMenus", "USERPREF_DEFAULT__SHOW_QUICK_NAV_MENUS", true},
	{"enableFavorites", "USERPREF_DEFAULT__ENABLE_FAVORITES", true},
}

// nonLockablePreferences mirrors $nonLockablePreferences (never lockable via env).
var nonLockablePreferences = map[string]bool{
	"activeGroup":     true,
	"defaultGroup":    true,
	"useWebauthnOnly": true,
	"sortOrder":       true,
}

// defaultSettings mirrors config('2fauth.settings').
var defaultSettings = map[string]any{
	"useEncryption":              false,
	"checkForUpdate":             true,
	"lastRadarScan":              0,
	"latestRelease":              false,
	"disableRegistration":        false,
	"enableSso":                  true,
	"useSsoOnly":                 false,
	"allowPatWhileSsoOnly":       false,
	"restrictRegistration":       false,
	"restrictList":               "",
	"restrictRule":               "",
	"keepSsoRegistrationEnabled": false,
	"storeIconsInDatabase":       false,
	"enableSharing":              false,
	"enableAllUsersSharingScope": false,
}

// Load reads .env (if present) then applies process environment on top.
func Load(basePath string) *Config {
	env := map[string]string{}
	if basePath != "" {
		if f, err := os.Open(filepath.Join(basePath, ".env")); err == nil {
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				line := strings.TrimSpace(sc.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if strings.HasPrefix(line, "export ") {
					line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
				}
				idx := strings.Index(line, "=")
				if idx <= 0 {
					continue
				}
				k := strings.TrimSpace(line[:idx])
				v := strings.TrimSpace(line[idx+1:])
				v = unquote(v)
				env[k] = v
			}
			_ = f.Close()
		}
	}

	get := func(key string, def string) string {
		if key == "" {
			return def
		}
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
		if v, ok := env[key]; ok {
			return v
		}
		return def
	}

	c := &Config{BasePath: basePath}
	c.AppName = get("APP_NAME", "2FAuth")
	c.Env = get("APP_ENV", "production")
	c.Timezone = get("APP_TIMEZONE", "UTC")
	c.Debug = envBool(get("APP_DEBUG", "false"))
	c.Key = get("APP_KEY", "")
	c.URL = get("APP_URL", "http://localhost")
	c.Subdirectory = strings.Trim(strings.TrimSpace(get("APP_SUBDIRECTORY", "")), "/")
	c.SiteOwner = get("SITE_OWNER", "mail@example.com")
	c.Version = AppVersionDefault

	c.IsDemoApp = envBool(get("IS_DEMO_APP", "false"))
	c.IsTestingApp = envBool(get("IS_TESTING_APP", "false"))
	c.ContentSecurityPolicy = envBool(get("CONTENT_SECURITY_POLICY", "true"))
	c.BlockOtpauthImagelinkFetching = envBool(get("BLOCK_OPTAUTH_IMAGELINK_FETCHING", "true"))

	c.DBConnection = strings.TrimSpace(get("DB_CONNECTION", "mysql"))
	c.DBDatabase = get("DB_DATABASE", "")
	c.DBHost = get("DB_HOST", "")
	c.DBPort = get("DB_PORT", "")
	c.DBUsername = get("DB_USERNAME", "")
	c.DBPassword = get("DB_PASSWORD", "")

	c.SessionDriver = get("SESSION_DRIVER", "file")
	c.SessionLifetime = envInt(get("SESSION_LIFETIME", strconv.Itoa(60*24*90)), 60*24*90)
	c.SessionSecure = envBool(get("SESSION_SECURE_COOKIE", "false"))

	c.AuthenticationGuard = get("AUTHENTICATION_GUARD", "web-guard")
	c.LoginThrottle = envInt(get("LOGIN_THROTTLE", "5"), 5)
	c.ThrottleAPI = envInt(get("THROTTLE_API", "60"), 60)
	c.ThrottleAPIImport = envInt(get("THROTTLE_API_DURING_IMPORT", "1000"), 1000)
	c.AuthLogRetentionTime = envInt(get("AUTHENTICATION_LOG_RETENTION", "365"), 365)
	c.OtpLogRetentionTime = envInt(get("OTP_LOG_RETENTION", "365"), 365)
	c.AuthProxyHeaderForUser = nullToEmpty(get("AUTH_PROXY_HEADER_FOR_USER", ""))
	c.AuthProxyHeaderForMail = nullToEmpty(get("AUTH_PROXY_HEADER_FOR_EMAIL", ""))
	c.ProxyLogoutURL = nullToEmpty(get("PROXY_LOGOUT_URL", ""))
	c.TrustedProxies = nullToEmpty(get("TRUSTED_PROXIES", ""))

	c.WebAuthnName = get("WEBAUTHN_NAME", "2FAuth")
	c.WebAuthnID = nullToEmpty(get("WEBAUTHN_ID", ""))
	c.WebAuthnUserValidation = get("WEBAUTHN_USER_VERIFICATION", "preferred")

	c.MailMailer = get("MAIL_MAILER", "log")
	c.MailHost = get("MAIL_HOST", "")
	c.MailPort = envInt(get("MAIL_PORT", "2525"), 2525)
	c.MailUsername = nullToEmpty(get("MAIL_USERNAME", ""))
	c.MailPassword = nullToEmpty(get("MAIL_PASSWORD", ""))
	c.MailEncryption = nullToEmpty(get("MAIL_ENCRYPTION", ""))
	c.MailFromName = nullToEmpty(get("MAIL_FROM_NAME", ""))
	c.MailFromAddr = nullToEmpty(get("MAIL_FROM_ADDRESS", ""))

	c.OpenIDEnabled = envBool(get("OIDC_ENABLED", "false"))
	c.OpenIDIssuer = strings.TrimRight(strings.TrimSpace(nullToEmpty(get("OIDC_ISSUER", ""))), "/")
	c.OpenIDClientID = nullToEmpty(get("OIDC_CLIENT_ID", ""))
	// OIDC_CLIENT_SECRET is the current name; OPENID_CLIENT_SECRET is kept as a
	// fallback for .env files written for 2FAuth v9.
	c.OpenIDClientSecret = get("OIDC_CLIENT_SECRET", "")
	if c.OpenIDClientSecret == "" {
		c.OpenIDClientSecret = get("OPENID_CLIENT_SECRET", "")
	}
	c.OpenIDScopes = strings.TrimSpace(get("OIDC_SCOPES", DefaultOpenIDScopes))
	if c.OpenIDScopes == "" {
		c.OpenIDScopes = DefaultOpenIDScopes
	}
	c.OpenIDButtonLabel = strings.TrimSpace(get("OIDC_BUTTON_LABEL", DefaultOpenIDLabel))
	if c.OpenIDButtonLabel == "" {
		c.OpenIDButtonLabel = DefaultOpenIDLabel
	}
	c.OpenIDAutoRegister = envBool(get("OIDC_AUTO_REGISTER", "true"))
	c.OpenIDAutoRedirect = envBool(get("OIDC_AUTO_REDIRECT", "false"))
	c.GithubClientSecret = get("GITHUB_CLIENT_SECRET", "")

	c.Locales = Locales
	c.Settings = map[string]any{}
	for k, v := range defaultSettings {
		c.Settings[k] = v
	}

	c.IconProviders = strings.TrimSpace(nullToEmpty(get("ICON_PROVIDERS", DefaultIconProviders)))
	if c.IconProviders == "" {
		c.IconProviders = DefaultIconProviders
	}
	c.IconProviderMaxTries = envInt(get("ICON_PROVIDER_MAX_TRIES", strconv.Itoa(DefaultIconProviderMaxTries)), DefaultIconProviderMaxTries)
	if c.IconProviderMaxTries < 1 {
		c.IconProviderMaxTries = DefaultIconProviderMaxTries
	}
	// Environment first, database options afterwards: an administrator always
	// overrides this file through the settings service.
	c.Settings[SettingIconProviders] = c.IconProviders
	c.Settings[SettingIconProviderMaxTries] = c.IconProviderMaxTries

	c.DefaultPreferences = map[string]any{}
	for _, p := range preferenceDefaults {
		if p.envKey != "" {
			c.DefaultPreferences[p.name] = envAny(get(p.envKey, ""), p.fallback)
		} else {
			c.DefaultPreferences[p.name] = p.fallback
		}
	}

	for _, p := range preferenceDefaults {
		if nonLockablePreferences[p.name] {
			continue
		}
		envKey := "USERPREF_LOCKED__" + snakeUpper(p.name)
		if v, ok := os.LookupEnv(envKey); ok {
			if envBool(v) {
				c.LockedPreferences = append(c.LockedPreferences, p.name)
			}
			continue
		}
		if v, ok := env[envKey]; ok && v != "" && envBool(v) {
			c.LockedPreferences = append(c.LockedPreferences, p.name)
		}
	}

	c.AssetsPath = get("ASSETS_PATH", filepath.Join(basePath, "public"))
	c.UIPath = get("UI_ASSETS_PATH", filepath.Join(basePath, "ui"))

	if c.DBConnection == "sqlite" && c.DBDatabase == "" {
		c.DatabasePath = filepath.Join(basePath, "database", "database.sqlite")
	} else if c.DBConnection == "sqlite" {
		c.DatabasePath = c.DBDatabase
		if !filepath.IsAbs(c.DatabasePath) {
			c.DatabasePath = filepath.Join(basePath, c.DatabasePath)
		}
	}

	return c
}

// snakeUpper mimics Laravel Str::snake()->upper() with 2FAuth's revealDottedOtp special case.
func snakeUpper(name string) string {
	if name == "revealDottedOTP" {
		name = "revealDottedOtp"
	}
	var b strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		if r >= 'A' && r <= 'Z' {
			if i > 0 && (runes[i-1] >= 'a' && runes[i-1] <= 'z') {
				b.WriteByte('_')
			}
			b.WriteRune(r)
		} else {
			b.WriteRune(r)
		}
	}
	return strings.ToUpper(b.String())
}

func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

func nullToEmpty(v string) string {
	if strings.EqualFold(v, "null") {
		return ""
	}
	return v
}

func envBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(nullToEmpty(v))) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

func envInt(v string, def int) int {
	if v == "" || strings.EqualFold(v, "null") {
		return def
	}
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return n
	}
	return def
}

// envAny parses a raw env value into a JSON-friendly value, keeping the fallback type
// when the variable is unset/empty (mirrors envUnlessEmpty()).
func envAny(v string, fallback any) any {
	if v == "" || strings.EqualFold(v, "null") {
		return fallback
	}
	switch fallback.(type) {
	case bool:
		return envBool(v)
	case int:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	case float64:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return v
}

// PreferenceOrder returns the preference names in declaration order.
func (c *Config) PreferenceOrder() []string {
	names := make([]string, 0, len(preferenceDefaults))
	for _, p := range preferenceDefaults {
		names = append(names, p.name)
	}
	return names
}

// GetAssetsPath returns the directory holding the built front-end assets.
func (c *Config) GetAssetsPath() string { return c.AssetsPath }
