package services

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"2fauth/internal/config"
	"2fauth/internal/store"
)

// IconService ports App\Services\IconService/IconStoreService. Icons live in
// storage/icons and are exposed at /storage/icons/<name>.
type IconService struct {
	cfg *config.Config
	st  *store.Store
	dir string
}

// NewIconService builds the icon store.
func NewIconService(cfg *config.Config, st *store.Store) *IconService {
	dir := filepath.Join(cfg.BasePath, "storage", "icons")
	_ = os.MkdirAll(dir, 0o755)
	return &IconService{cfg: cfg, st: st, dir: dir}
}

// Dir returns the storage directory.
func (i *IconService) Dir() string { return i.dir }

// InDatabase reports whether icons are persisted in the database.
func (i *IconService) InDatabase() bool {
	return false
}

// Exists reports whether the icon is available.
func (i *IconService) Exists(name string) bool {
	if name == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(i.dir, filepath.Base(name)))
	return err == nil
}

// Get reads the raw icon bytes.
func (i *IconService) Get(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(i.dir, filepath.Base(name)))
}

// MimeType returns the mime type of a stored icon.
func (i *IconService) MimeType(name string) string {
	content, err := i.Get(name)
	if err != nil {
		return ""
	}
	return http.DetectContentType(content)
}

// Store writes an icon, returning its file name.
func (i *IconService) Store(name string, content []byte) error {
	if name == "" {
		name = RandomIconName(content) + ".png"
	}
	return os.WriteFile(filepath.Join(i.dir, filepath.Base(name)), content, 0o644)
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

// Delete removes an icon file.
func (i *IconService) Delete(name string) error {
	err := os.Remove(filepath.Join(i.dir, filepath.Base(name)))
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

// FetchRemote downloads an icon from a URL and stores it.
func (i *IconService) FetchRemote(url string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errNoHTTPStatus
	}
	buf := make([]byte, 0, 32*1024)
	chunk := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
		}
		if err != nil {
			break
		}
		if len(buf) > 2*1024*1024 {
			break
		}
	}
	if len(buf) == 0 {
		return "", errNoHTTPStatus
	}
	ext := extensionForContentType(resp.Header.Get("Content-Type"))
	name, err := i.StoreWithExtension(ext, buf)
	if err != nil {
		return "", err
	}
	return name, nil
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

var errNoHTTPStatus = &iconError{"unable to fetch the icon"}

type iconError struct{ msg string }

func (e *iconError) Error() string { return e.msg }

// LogoPacks lists the icon packs the front end can choose from.
func (i *IconService) LogoPacks() []map[string]any {
	return []map[string]any{
		{"name": "selfh", "label": "selfh.st", "url": "https://selfh.st/icons/", "variants": []string{"regular", "dark", "light"}},
		{"name": "dashboardicons", "label": "Dashboard Icons", "url": "https://github.com/homarr-labs/dashboard-icons", "variants": []string{"regular"}},
	}
}

// BuildFromOfficialLogo tries to resolve an official icon for a service name.
func (i *IconService) BuildFromOfficialLogo(service string) string {
	if service == "" {
		return ""
	}
	slug := strings.ToLower(strings.TrimSpace(service))
	slug = strings.NewReplacer(" ", "-", "_", "-", "/", "-", "&", "and").Replace(slug)
	url := "https://cdn.jsdelivr.net/gh/selfhst/icons/png/" + slug + ".png"
	name, err := i.FetchRemote(url)
	if err != nil {
		return ""
	}
	return name
}
