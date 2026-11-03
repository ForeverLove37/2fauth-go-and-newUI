package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"2fauth/internal/config"
	"2fauth/internal/store"
)

// IconService ports App\Services\IconService/IconStoreService. Icons live in
// storage/icons and are exposed at /storage/icons/<name>.
//
// Remote icons are resolved through an ordered, configurable provider chain: the
// first provider that answers with a usable image wins and only its body is
// stored. The order and the number of attempts are admin editable settings
// (iconProviders / iconProviderMaxTries, seeded by the ICON_* environment
// variables).
type IconService struct {
	cfg *config.Config
	st  *store.Store
	dir string

	// settings resolves the iconProviders / iconProviderMaxTries settings. It is
	// optional: without it the configuration defaults are used.
	settings *SettingService

	mu      sync.RWMutex
	sources ProviderSources
	client  *http.Client
}

// Provider ids. They are the values accepted by the iconProviders setting and by
// PUT /api/v1/icons/providers.
const (
	ProviderSelfHST     = "selfhst"
	ProviderDashboard   = "dashboard"
	ProviderWalkx       = "walkx"
	ProviderSimpleIcons = "simpleicons"
	ProviderLobeHub     = "lobehub"
	ProviderIconify     = "iconify"
)

// Tuning of the provider chain. Each attempt gets its own deadline and the whole
// chain shares a single budget, so a dead provider cannot stall a request for
// longer than providerChainBudget.
const (
	providerAttemptTimeout = 6 * time.Second
	providerChainBudget    = 15 * time.Second
	// maxIconBytes caps a downloaded icon (and rejects anything bigger).
	maxIconBytes = 2 << 20 // 2 MiB
)

// ErrUnsupportedIcon reports that the bytes are not an acceptable image.
var ErrUnsupportedIcon = errors.New("unsupported icon")

// ErrIconNotFound reports that no provider of the chain could supply an icon.
var ErrIconNotFound = errors.New("icon not found for this service")

var (
	errNoIcon       = errors.New("no icon")
	errNotAnImage   = errors.New("the downloaded file is not an image")
	errIconTooLarge = errors.New("the downloaded icon is too large")
	errNoHTTPStatus = &iconError{"unable to fetch the icon"}
)

// Provider is the public description of one icon source.
type Provider struct {
	ID       string // stable id used by the settings and the admin API
	Label    string // human readable name
	Source   string // homepage / documentation of the icon set
	Template string // URL template, {slug} / {service} are substituted
}

// ProviderSources overrides the base URL of a provider. Keys are provider ids and
// values the base URL used instead of the public one; it exists so tests (or a
// mirror) can redirect the whole chain to a local server. A nil map means "use
// the public URLs".
type ProviderSources map[string]string

// IconChain is the effective provider chain resolved from the settings.
type IconChain struct {
	Enabled  []string // every enabled provider id, in configured order
	Attempts []string // Enabled truncated to MaxTries: what a lookup really tries
	MaxTries int
}

// IconResult is a successful chain lookup.
type IconResult struct {
	// Name is the file name of the stored icon in storage/icons.
	Name string
	// Provider is the id of the provider that answered.
	Provider string
	// MimeType is the sniffed content type of the stored body.
	MimeType string
}

// iconSlugs holds the normalisations of a service name used by the providers.
type iconSlugs struct {
	service string // trimmed original name, used by the search based provider
	slug    string // lower-cased, dash separated: "grafana-cloud"
	raw     string // casing preserved, separators normalised: "Grafana-Cloud"
	alnum   string // lower-cased alphanumerics only: "grafanaCloud" -> "grafanacloud"
}

// providerDef is the internal description of a provider: the public metadata, the
// base URL and the URL paths to try for a service name.
type providerDef struct {
	Provider
	base     string
	paths    func(iconSlugs) []string
	search   bool     // resolved through a search API (iconify)
	variants []string // variants advertised by GET /api/v1/icons/packs
}

// providerCatalog is the provider table, in its default order.
var providerCatalog = []providerDef{
	{
		Provider: Provider{
			ID:       ProviderSelfHST,
			Label:    "selfh.st",
			Source:   "https://selfh.st/icons/",
			Template: "https://cdn.jsdelivr.net/gh/selfhst/icons/svg/{slug}.svg",
		},
		base: "https://cdn.jsdelivr.net",
		paths: func(s iconSlugs) []string {
			return []string{"/gh/selfhst/icons/svg/" + s.slug + ".svg"}
		},
		variants: []string{"regular", "dark", "light"},
	},
	{
		Provider: Provider{
			ID:       ProviderDashboard,
			Label:    "Homarr dashboard-icons",
			Source:   "https://github.com/homarr-labs/dashboard-icons",
			Template: "https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons/svg/{slug}.svg",
		},
		base: "https://cdn.jsdelivr.net",
		paths: func(s iconSlugs) []string {
			// The set mixes lower-cased and original-cased file names, so both
			// normalisations are tried.
			paths := []string{"/gh/homarr-labs/dashboard-icons/svg/" + s.slug + ".svg"}
			if s.raw != "" && s.raw != s.slug {
				paths = append(paths, "/gh/homarr-labs/dashboard-icons/svg/"+s.raw+".svg")
			}
			return paths
		},
		variants: []string{"regular"},
	},
	{
		Provider: Provider{
			ID:       ProviderWalkx,
			Label:    "walkxcode legacy",
			Source:   "https://github.com/walkxcode/dashboard-icons",
			Template: "https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/svg/{slug}.svg",
		},
		base: "https://cdn.jsdelivr.net",
		paths: func(s iconSlugs) []string {
			return []string{"/gh/walkxcode/dashboard-icons/svg/" + s.slug + ".svg"}
		},
		variants: []string{"regular"},
	},
	{
		Provider: Provider{
			ID:       ProviderSimpleIcons,
			Label:    "Simple Icons",
			Source:   "https://simpleicons.org/",
			Template: "https://cdn.simpleicons.org/{slug}",
		},
		base: "https://cdn.simpleicons.org",
		paths: func(s iconSlugs) []string {
			// Simple Icons uses alphanumerics only ("grafana", "proxmoxve").
			if s.alnum == "" {
				return nil
			}
			return []string{"/" + s.alnum}
		},
		variants: []string{"regular"},
	},
	{
		Provider: Provider{
			ID:       ProviderLobeHub,
			Label:    "LobeHub icons",
			Source:   "https://github.com/lobehub/lobe-icons",
			Template: "https://cdn.jsdelivr.net/gh/lobehub/lobe-icons/packages/static-png/{slug}.png",
		},
		base: "https://cdn.jsdelivr.net",
		paths: func(s iconSlugs) []string {
			// The PNG set ships a monochrome "-color" variant next to the plain
			// one; retry with it when the plain file is missing.
			base := "/gh/lobehub/lobe-icons/packages/static-png/"
			return []string{base + s.slug + ".png", base + s.slug + "-color.png"}
		},
		variants: []string{"regular", "color"},
	},
	{
		Provider: Provider{
			ID:       ProviderIconify,
			Label:    "Iconify search",
			Source:   "https://iconify.design/",
			Template: "https://api.iconify.design/search?query={service}&limit=1",
		},
		base:     "https://api.iconify.design",
		search:   true,
		variants: []string{"regular"},
	},
}

// ProviderCount is the size of the known provider set (the upper bound of the
// provider list and of max_tries).
const ProviderCount = 6

// ---------------------------------------------------------------------------
// construction
// ---------------------------------------------------------------------------

// NewIconService builds the icon store. The setting service may be nil, in which
// case only the configuration defaults of the provider chain are honoured.
func NewIconService(cfg *config.Config, st *store.Store, settings *SettingService) *IconService {
	dir := filepath.Join(cfg.BasePath, "storage", "icons")
	_ = os.MkdirAll(dir, 0o750)
	i := &IconService{cfg: cfg, st: st, dir: dir, settings: settings}
	i.client = i.newClient()
	return i
}

// newClient builds the shared HTTP client of the chain. It honours HTTP_PROXY,
// HTTPS_PROXY and NO_PROXY and never disables certificate verification.
func (i *IconService) newClient() *http.Client {
	return &http.Client{
		Timeout: providerAttemptTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			// Certificate verification stays enabled: InsecureSkipVerify is never set.
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        20,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}
}

// SetSources injects alternative provider base URLs (used by tests).
func (i *IconService) SetSources(sources ProviderSources) {
	i.mu.Lock()
	i.sources = sources
	i.mu.Unlock()
}

// SetHTTPClient replaces the HTTP client of the chain (used by tests).
func (i *IconService) SetHTTPClient(client *http.Client) {
	if client == nil {
		client = i.newClient()
	}
	i.mu.Lock()
	i.client = client
	i.mu.Unlock()
}

func (i *IconService) httpClient() *http.Client {
	i.mu.RLock()
	client := i.client
	i.mu.RUnlock()
	if client == nil {
		return i.newClient()
	}
	return client
}

// baseFor returns the base URL of a provider, honouring the injected sources.
func (i *IconService) baseFor(def providerDef) string {
	i.mu.RLock()
	override := i.sources[def.ID]
	i.mu.RUnlock()
	if strings.TrimSpace(override) != "" {
		return strings.TrimRight(strings.TrimSpace(override), "/")
	}
	return def.base
}

// userAgent is the User-Agent sent to the providers.
func (i *IconService) userAgent() string {
	if i.cfg != nil && i.cfg.Version != "" {
		return "2FAuth/" + i.cfg.Version
	}
	return "2FAuth"
}

// ---------------------------------------------------------------------------
// provider catalogue
// ---------------------------------------------------------------------------

// Providers lists every known provider, in table order.
func (i *IconService) Providers() []Provider {
	out := make([]Provider, 0, len(providerCatalog))
	for _, def := range providerCatalog {
		out = append(out, def.Provider)
	}
	return out
}

// IsKnownIconProvider reports whether id is a provider of the table.
func IsKnownIconProvider(id string) bool {
	_, ok := providerByID(strings.ToLower(strings.TrimSpace(id)))
	return ok
}

// AllProviderIDs returns every provider id in table order.
func AllProviderIDs() []string {
	ids := make([]string, 0, len(providerCatalog))
	for _, def := range providerCatalog {
		ids = append(ids, def.ID)
	}
	return ids
}

func providerByID(id string) (providerDef, bool) {
	for _, def := range providerCatalog {
		if def.ID == id {
			return def, true
		}
	}
	return providerDef{}, false
}

// ParseProviderList normalises a comma separated provider list: entries are
// trimmed and lower-cased, unknown ids and duplicates are dropped. The result is
// empty when nothing usable is left.
func ParseProviderList(raw string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(providerCatalog))
	for _, part := range strings.Split(raw, ",") {
		id := strings.ToLower(strings.TrimSpace(part))
		if id == "" || seen[id] {
			continue
		}
		if _, ok := providerByID(id); !ok {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// ---------------------------------------------------------------------------
// configuration
// ---------------------------------------------------------------------------

// Chain returns the effective provider chain. A missing or unusable setting falls
// back to the defaults so a bad configuration never disables icons entirely.
func (i *IconService) Chain() IconChain {
	rawList := config.DefaultIconProviders
	maxTries := config.DefaultIconProviderMaxTries

	if i.settings != nil {
		if v := strings.TrimSpace(i.settings.Str(config.SettingIconProviders)); v != "" {
			rawList = v
		}
		if v, ok := intValue(i.settings.Get(config.SettingIconProviderMaxTries)); ok {
			maxTries = v
		}
	} else if i.cfg != nil {
		if v, ok := i.cfg.Settings[config.SettingIconProviders].(string); ok && strings.TrimSpace(v) != "" {
			rawList = v
		}
		if v, ok := intValue(i.cfg.Settings[config.SettingIconProviderMaxTries]); ok {
			maxTries = v
		}
	}

	enabled := ParseProviderList(rawList)
	if len(enabled) == 0 {
		enabled = AllProviderIDs()
	}
	if maxTries < 1 {
		maxTries = 1
	}
	if maxTries > ProviderCount {
		maxTries = ProviderCount
	}
	attempts := enabled
	if len(attempts) > maxTries {
		attempts = attempts[:maxTries]
	}
	return IconChain{Enabled: enabled, Attempts: attempts, MaxTries: maxTries}
}

// SetChain persists a new provider chain. The ids are validated against the
// known set (unknown ids are dropped) and an empty list restores the default, so
// the stored configuration is always usable.
func (i *IconService) SetChain(providers []string, maxTries int) error {
	if i.settings == nil {
		return errors.New("icon: no setting service")
	}
	clean := make([]string, 0, len(providers))
	seen := map[string]bool{}
	for _, p := range providers {
		id := strings.ToLower(strings.TrimSpace(p))
		if id == "" || seen[id] || !IsKnownIconProvider(id) {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		clean = AllProviderIDs()
	}
	if maxTries < 1 {
		maxTries = 1
	}
	if maxTries > ProviderCount {
		maxTries = ProviderCount
	}
	if err := i.settings.Set(config.SettingIconProviders, strings.Join(clean, ",")); err != nil {
		return err
	}
	return i.settings.Set(config.SettingIconProviderMaxTries, maxTries)
}

func intValue(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
			return n, true
		}
	}
	return 0, false
}

// ---------------------------------------------------------------------------
// lookup
// ---------------------------------------------------------------------------

// FetchIcon runs the configured provider chain and stores the first usable image.
// It returns ErrIconNotFound when every attempted provider missed, so a total
// miss is never reported as a server error.
func (i *IconService) FetchIcon(ctx context.Context, service string) (*IconResult, error) {
	slugs := buildIconSlugs(service)
	if slugs.slug == "" {
		return nil, ErrIconNotFound
	}
	chain := i.Chain()
	if len(chain.Attempts) == 0 {
		return nil, ErrIconNotFound
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, providerChainBudget)
	defer cancel()

	for _, id := range chain.Attempts {
		if ctx.Err() != nil {
			break
		}
		def, ok := providerByID(id)
		if !ok {
			continue
		}
		body, mime, err := i.fetchProvider(ctx, def, slugs)
		if err != nil {
			continue
		}
		// Only the winning body is written, and only once it has been fully
		// received and identified as an image. Third-party SVGs are cleaned too.
		if mime == svgMime {
			if body, err = SanitizeSVG(body); err != nil {
				continue
			}
		}
		name, err := i.StoreWithExtension(extensionForContentType(mime), body)
		if err != nil {
			return nil, err
		}
		return &IconResult{Name: name, Provider: def.ID, MimeType: mime}, nil
	}
	return nil, ErrIconNotFound
}

// fetchProvider tries every URL of a provider and returns the first usable body.
func (i *IconService) fetchProvider(ctx context.Context, def providerDef, slugs iconSlugs) ([]byte, string, error) {
	base := i.baseFor(def)
	if def.search {
		return i.fetchBySearch(ctx, base, slugs)
	}
	var lastErr error = errNoIcon
	for _, path := range def.paths(slugs) {
		if path == "" {
			continue
		}
		body, err := i.getBody(ctx, base+path)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, "", err
			}
			continue
		}
		mime, ok := sniffIconContent(body)
		if !ok {
			lastErr = errNotAnImage
			continue
		}
		return body, mime, nil
	}
	return nil, "", lastErr
}

// fetchBySearch resolves an icon through a search API (iconify): the search
// answers {"icons":["prefix:name"]} and the icon is then fetched from
// <base>/<prefix>/<name>.svg.
func (i *IconService) fetchBySearch(ctx context.Context, base string, slugs iconSlugs) ([]byte, string, error) {
	if slugs.service == "" {
		return nil, "", errNoIcon
	}
	query := url.Values{}
	query.Set("query", slugs.service)
	query.Set("limit", "1")

	raw, err := i.getBody(ctx, base+"/search?"+query.Encode())
	if err != nil {
		return nil, "", err
	}
	var payload struct {
		Icons []string `json:"icons"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, "", fmt.Errorf("icon search answered an invalid payload: %w", err)
	}
	if len(payload.Icons) == 0 {
		return nil, "", errNoIcon
	}
	prefix, name, ok := strings.Cut(strings.TrimSpace(payload.Icons[0]), ":")
	if !ok || !validSearchIdentifier(prefix) || !validSearchIdentifier(name) {
		return nil, "", errNoIcon
	}

	body, err := i.getBody(ctx, base+"/"+prefix+"/"+name+".svg")
	if err != nil {
		return nil, "", err
	}
	mime, ok := sniffIconContent(body)
	if !ok {
		return nil, "", errNotAnImage
	}
	return body, mime, nil
}

// validSearchIdentifier guards the values coming from a search response so they
// can never escape the provider path.
func validSearchIdentifier(v string) bool {
	if v == "" || len(v) > 64 {
		return false
	}
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

// getBody performs one GET request bounded by the per-attempt timeout.
func (i *IconService) getBody(ctx context.Context, rawURL string) ([]byte, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, providerAttemptTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", i.userAgent())
	req.Header.Set("Accept", "image/svg+xml,image/png,image/*;q=0.8,*/*;q=0.1")

	resp, err := i.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("icon provider answered %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxIconBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxIconBytes {
		return nil, errIconTooLarge
	}
	return body, nil
}

// sniffIconContent reports whether body is a usable image and returns its mime
// type. Empty bodies, HTML error pages and truncated downloads are rejected.
func sniffIconContent(body []byte) (string, bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", false
	}
	detected := http.DetectContentType(trimmed)
	switch detected {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return detected, true
	}
	if strings.Contains(detected, "svg") {
		return "image/svg+xml", true
	}
	// http.DetectContentType has no SVG signature and reports SVG documents as
	// XML or plain text, so the markup itself is inspected as well.
	if strings.HasPrefix(detected, "text/") || strings.HasPrefix(detected, "application/xml") {
		if looksLikeSVG(trimmed) {
			return "image/svg+xml", true
		}
	}
	return "", false
}

// looksLikeSVG requires both the opening element and the closing tag so that a
// truncated download is never mistaken for a valid icon.
func looksLikeSVG(body []byte) bool {
	lower := bytes.ToLower(body)
	return bytes.Contains(lower, []byte("<svg")) && bytes.Contains(lower, []byte("</svg>"))
}

// imageType identifies an image from its content alone. Unlike sniffIconContent it
// is strict about SVG: the root element must really be <svg>, so an HTML document
// that merely contains one is not an image.
func imageType(body []byte) (string, bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", false
	}
	switch detected := http.DetectContentType(trimmed); detected {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return detected, true
	}
	if _, err := SanitizeSVG(trimmed); err == nil {
		return svgMime, true
	}
	return "", false
}

// ---------------------------------------------------------------------------
// slugs
// ---------------------------------------------------------------------------

// ServiceSlug normalises a service name into the slug used by most provider URL
// templates: lower-cased, spaces and underscores turned into dashes, everything
// outside [a-z0-9.-] removed. It returns "" when no usable slug is left, and
// rejects names carrying markup or control characters instead of silently
// turning "<script>" into the slug "script".
func ServiceSlug(service string) string {
	slug, _ := normaliseServiceName(service, true)
	return slug
}

func buildIconSlugs(service string) iconSlugs {
	slug, ok := normaliseServiceName(service, true)
	if !ok {
		return iconSlugs{}
	}
	raw, _ := normaliseServiceName(service, false)
	return iconSlugs{
		service: strings.TrimSpace(service),
		slug:    slug,
		raw:     raw,
		alnum:   alnumSlug(slug),
	}
}

// normaliseServiceName lower-cases (when asked) and slugifies a service name.
func normaliseServiceName(service string, lower bool) (string, bool) {
	s := strings.TrimSpace(service)
	if s == "" {
		return "", false
	}
	for _, r := range s {
		// Control characters and markup never belong to a service name.
		if r < 0x20 || r == 0x7f || strings.ContainsRune("<>\"'`\\", r) {
			return "", false
		}
	}
	if lower {
		s = strings.ToLower(s)
	}
	s = strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\t", "-").Replace(s)

	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			b.WriteRune(r)
		}
	}
	out := collapseIconSeparators(strings.Trim(b.String(), "-."))
	return out, out != ""
}

// alnumSlug keeps the alphanumerics of a slug only ("grafana-cloud" -> "grafanacloud").
func alnumSlug(slug string) string {
	var b strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// collapseIconSeparators turns runs of dashes into a single one.
func collapseIconSeparators(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevDash := false
	for _, r := range s {
		if r == '-' {
			if prevDash {
				continue
			}
			prevDash = true
		} else {
			prevDash = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// storage
// ---------------------------------------------------------------------------

// Dir returns the storage directory.
func (i *IconService) Dir() string { return i.dir }

// InDatabase reports whether icons are persisted in the database.
func (i *IconService) InDatabase() bool {
	return false
}

// Exists reports whether the icon is available.
func (i *IconService) Exists(name string) bool {
	if !ValidIconName(name) {
		return false
	}
	info, err := os.Stat(filepath.Join(i.dir, name))
	return err == nil && info.Mode().IsRegular()
}

// Get reads the raw icon bytes.
func (i *IconService) Get(name string) ([]byte, error) {
	if !ValidIconName(name) {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(filepath.Join(i.dir, name))
}

// MimeType returns the type a stored icon is served with: the one of its content,
// or "" when the file is not an image.
func (i *IconService) MimeType(name string) string {
	content, err := i.Get(name)
	if err != nil {
		return ""
	}
	mime, ok := imageType(content)
	if !ok {
		return ""
	}
	return mime
}

// Store writes an icon, returning its file name.
func (i *IconService) Store(name string, content []byte) error {
	if name == "" {
		name = RandomIconName(content) + ".png"
	}
	if !ValidIconName(name) {
		return ErrUnsupportedIcon
	}
	if err := os.MkdirAll(i.dir, 0o750); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(i.dir, name), content, 0o640)
}

// StoreWithExtension writes an icon with the given extension.
func (i *IconService) StoreWithExtension(extension string, content []byte) (string, error) {
	name := RandomIconName(content)
	if ext := strings.TrimPrefix(extension, "."); ext != "" {
		name += "." + ext
	}
	if err := i.Store(name, content); err != nil {
		return "", err
	}
	return name, nil
}

// StoreUpload validates untrusted bytes and writes them. The extension is decided
// by the content, and an SVG is rewritten from a list of harmless elements.
func (i *IconService) StoreUpload(content []byte) (string, error) {
	if len(content) > maxIconBytes {
		return "", ErrUnsupportedIcon
	}
	mime, ok := imageType(content)
	if !ok {
		return "", ErrUnsupportedIcon
	}
	if mime == svgMime {
		cleaned, err := SanitizeSVG(content)
		if err != nil {
			return "", ErrUnsupportedIcon
		}
		content = cleaned
	}
	return i.StoreWithExtension(extensionForContentType(mime), content)
}

// Delete removes an icon file.
func (i *IconService) Delete(name string) error {
	if !ValidIconName(name) {
		return os.ErrNotExist
	}
	err := os.Remove(filepath.Join(i.dir, name))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// RandomIconName builds a unique icon name (without extension) from the content hash.
func RandomIconName(content []byte) string {
	sum := sha256Hex(content)
	return sum[:20]
}

// FetchRemote downloads an image from a URL and stores it. It is the single-shot
// counterpart of the provider chain (used for explicitly supplied image links).
func (i *IconService) FetchRemote(rawURL string) (string, error) {
	body, err := i.getBody(context.Background(), rawURL)
	if err != nil {
		return "", err
	}
	mime, ok := sniffIconContent(body)
	if !ok {
		return "", errNoHTTPStatus
	}
	return i.StoreWithExtension(extensionForContentType(mime), body)
}

func extensionForContentType(ct string) string {
	switch {
	case strings.Contains(ct, "svg"):
		return "svg"
	case strings.Contains(ct, "jpeg"), strings.Contains(ct, "jpg"):
		return "jpg"
	case strings.Contains(ct, "webp"):
		return "webp"
	case strings.Contains(ct, "gif"):
		return "gif"
	default:
		return "png"
	}
}

type iconError struct{ msg string }

func (e *iconError) Error() string { return e.msg }

// LogoPacks lists the icon packs the front end can choose from: every provider of
// the chain, with its id, label, source and URL template.
func (i *IconService) LogoPacks() []map[string]any {
	out := make([]map[string]any, 0, len(providerCatalog))
	for _, def := range providerCatalog {
		out = append(out, map[string]any{
			// "name" is kept for the front-ends that only know the legacy shape.
			"name":     def.ID,
			"id":       def.ID,
			"label":    def.Label,
			"url":      def.Source,
			"template": def.Template,
			"variants": def.variants,
		})
	}
	return out
}

// BuildFromOfficialLogo tries to resolve an official icon for a service name
// through the provider chain. It returns "" when no provider could supply one.
func (i *IconService) BuildFromOfficialLogo(service string) string {
	res, err := i.FetchIcon(context.Background(), service)
	if err != nil || res == nil {
		return ""
	}
	return res.Name
}
