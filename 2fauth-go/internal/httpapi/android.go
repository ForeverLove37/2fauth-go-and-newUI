package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// androidMaxAPK bounds an uploaded package.
const androidMaxAPK = 200 << 20

// androidRelease is one published build of the phone app.
type androidRelease struct {
	VersionCode int64  `json:"version_code"`
	VersionName string `json:"version_name"`
	Notes       string `json:"notes"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	PublishedAt int64  `json:"published_at"`
}

var androidMu sync.Mutex

func (s *Server) androidDir() string {
	return filepath.Join(s.cfg.BasePath, "storage", "releases", "android")
}

func (s *Server) androidFile(code int64) string {
	return filepath.Join(s.androidDir(), "2fauth-"+strconv.FormatInt(code, 10)+".apk")
}

// androidReleases reads the index, newest first.
func (s *Server) androidReleases() ([]androidRelease, error) {
	raw, err := os.ReadFile(filepath.Join(s.androidDir(), "releases.json"))
	if errors.Is(err, os.ErrNotExist) {
		return []androidRelease{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list []androidRelease
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].VersionCode > list[j].VersionCode })
	return list, nil
}

func (s *Server) saveAndroidReleases(list []androidRelease) error {
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.androidDir(), "releases.json.tmp")
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.androidDir(), "releases.json"))
}

func (s *Server) androidPayload(r androidRelease) map[string]any {
	return map[string]any{
		"version_code": r.VersionCode,
		"version_name": r.VersionName,
		"notes":        r.Notes,
		"size":         r.Size,
		"sha256":       r.SHA256,
		"published_at": r.PublishedAt,
		"url":          s.baseURL() + "/download/android/2fauth-" + strconv.FormatInt(r.VersionCode, 10) + ".apk",
	}
}

// androidLatest handles GET /api/v1/android/latest. It is public: the app asks before
// anyone signs in, and the landing page links to it.
func (s *Server) androidLatest(c *ctx) {
	list, err := s.androidReleases()
	if err != nil {
		s.srvError(c, err)
		return
	}
	c.w.Header().Set("Cache-Control", "no-cache")
	if len(list) == 0 {
		jsonMessage(c.w, http.StatusNotFound, "No Android release has been published")
		return
	}
	jsonResponse(c.w, http.StatusOK, s.androidPayload(list[0]))
}

// androidList handles GET /api/v1/android/releases (administrators).
func (s *Server) androidList(c *ctx) {
	list, err := s.androidReleases()
	if err != nil {
		s.srvError(c, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, r := range list {
		out = append(out, s.androidPayload(r))
	}
	jsonResponse(c.w, http.StatusOK, out)
}

// androidPublish handles POST /api/v1/android/releases: a signed APK, its version code
// and name, and the release notes shown on the phones.
func (s *Server) androidPublish(c *ctx) {
	if err := c.r.ParseMultipartForm(8 << 20); err != nil {
		jsonMessage(c.w, http.StatusUnprocessableEntity, "Send the package as multipart/form-data")
		return
	}
	defer func() { _ = c.r.MultipartForm.RemoveAll() }()

	code, err := strconv.ParseInt(strings.TrimSpace(c.r.FormValue("version_code")), 10, 64)
	name := strings.TrimSpace(c.r.FormValue("version_name"))
	notes := strings.TrimSpace(c.r.FormValue("notes"))
	errs := map[string][]string{}
	if err != nil || code <= 0 {
		errs["version_code"] = []string{"The version code must be a positive whole number."}
	}
	if name == "" || len(name) > 64 {
		errs["version_name"] = []string{"The version name is required (64 characters at most)."}
	}
	if len(notes) > 4000 {
		errs["notes"] = []string{"The notes may not be longer than 4000 characters."}
	}
	file, _, ferr := c.r.FormFile("apk")
	if ferr != nil {
		errs["apk"] = []string{"The package is required."}
	}
	if len(errs) > 0 {
		jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{"message": "The given data was invalid.", "errors": errs})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, androidMaxAPK+1))
	if err != nil {
		s.srvError(c, err)
		return
	}
	// An APK is a zip archive holding a manifest.
	if len(data) > androidMaxAPK || !bytes.HasPrefix(data, []byte("PK\x03\x04")) || !bytes.Contains(data, []byte("AndroidManifest.xml")) {
		jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
			"message": "This is not an Android package.",
			"errors":  map[string][]string{"apk": {"This is not an Android package."}},
		})
		return
	}

	androidMu.Lock()
	defer androidMu.Unlock()
	list, err := s.androidReleases()
	if err != nil {
		s.srvError(c, err)
		return
	}
	for _, r := range list {
		if r.VersionCode >= code {
			jsonResponse(c.w, http.StatusUnprocessableEntity, map[string]any{
				"message": "The version code must be higher than the published ones.",
				"errors":  map[string][]string{"version_code": {"The version code must be higher than " + strconv.FormatInt(r.VersionCode, 10) + "."}},
			})
			return
		}
	}
	if err := os.MkdirAll(s.androidDir(), 0o755); err != nil {
		s.srvError(c, err)
		return
	}
	tmp := s.androidFile(code) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		s.srvError(c, err)
		return
	}
	if err := os.Rename(tmp, s.androidFile(code)); err != nil {
		s.srvError(c, err)
		return
	}
	sum := sha256.Sum256(data)
	release := androidRelease{
		VersionCode: code, VersionName: name, Notes: notes,
		Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), PublishedAt: time.Now().Unix(),
	}
	list = append([]androidRelease{release}, list...)
	if err := s.saveAndroidReleases(list); err != nil {
		s.srvError(c, err)
		return
	}
	jsonResponse(c.w, http.StatusCreated, s.androidPayload(release))
}

// androidDelete handles DELETE /api/v1/android/releases/{code}.
func (s *Server) androidDelete(c *ctx) {
	code, err := strconv.ParseInt(c.param("code"), 10, 64)
	if err != nil {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return
	}
	androidMu.Lock()
	defer androidMu.Unlock()
	list, err := s.androidReleases()
	if err != nil {
		s.srvError(c, err)
		return
	}
	kept := list[:0]
	found := false
	for _, r := range list {
		if r.VersionCode == code {
			found = true
			continue
		}
		kept = append(kept, r)
	}
	if !found {
		jsonMessage(c.w, http.StatusNotFound, "not found")
		return
	}
	if err := s.saveAndroidReleases(kept); err != nil {
		s.srvError(c, err)
		return
	}
	_ = os.Remove(s.androidFile(code))
	jsonNoContent(c.w)
}

// androidDownload handles GET /download/android/{file}: "latest.apk" or a versioned
// file name.
func (s *Server) androidDownload(c *ctx) {
	name := c.param("file")
	list, err := s.androidReleases()
	if err != nil || len(list) == 0 {
		http.NotFound(c.w, c.r)
		return
	}
	release := list[0]
	if name != "latest.apk" {
		raw := strings.TrimSuffix(strings.TrimPrefix(name, "2fauth-"), ".apk")
		code, err := strconv.ParseInt(raw, 10, 64)
		found := false
		for _, r := range list {
			if err == nil && r.VersionCode == code && name == "2fauth-"+raw+".apk" {
				release, found = r, true
			}
		}
		if !found {
			http.NotFound(c.w, c.r)
			return
		}
	}
	f, err := os.Open(s.androidFile(release.VersionCode))
	if err != nil {
		http.NotFound(c.w, c.r)
		return
	}
	defer f.Close()
	h := c.w.Header()
	h.Set("Content-Type", "application/vnd.android.package-archive")
	h.Set("Content-Disposition", `attachment; filename="2FAuth-`+release.VersionName+`.apk"`)
	h.Set("X-Content-SHA256", release.SHA256)
	if name == "latest.apk" {
		h.Set("Cache-Control", "no-cache")
	} else {
		h.Set("Cache-Control", "public, max-age=86400, immutable")
	}
	http.ServeContent(c.w, c.r, "", time.Unix(release.PublishedAt, 0), f)
}
