package httpapi

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

	"2fauth/internal/crypt"
)

// qrDataURI renders a QR code as a PNG data URI (mirrors chillerlan render()).
func qrDataURI(data string) (string, error) {
	code, err := qrcode.New(data, qrcode.Medium)
	if err != nil {
		return "", err
	}
	code.DisableBorder = false
	png, err := code.PNG(8)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

// maxUploadSize caps multipart bodies.
const maxUploadSize = 6 << 20

// uploadedFile extracts a file from a multipart or urlencoded request.
func uploadedFile(r *http.Request, field string) ([]byte, string, error) {
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		return nil, "", err
	}
	file, header, err := r.FormFile(field)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxUploadSize))
	if err != nil {
		return nil, "", err
	}
	return data, header.Filename, nil
}

// verifyPassword checks a password against a bcrypt hash.
func verifyPassword(hash, password string) bool {
	return crypt.CheckPassword(hash, password)
}

// iconIfExists returns the icon name when the file exists, nil otherwise.
func (s *Server) iconIfExists(name string) any {
	if name == "" || !s.icons.Exists(name) {
		return nil
	}
	return name
}

// iconMimeType returns the mime type of a stored icon.
func (s *Server) iconMimeType(name string) any {
	if name == "" || !s.icons.Exists(name) {
		return nil
	}
	return s.icons.MimeType(name)
}

// iconBase64 returns the base64 payload of a stored icon.
func (s *Server) iconBase64(name string) any {
	if name == "" || !s.icons.Exists(name) {
		return nil
	}
	content, err := s.icons.Get(name)
	if err != nil {
		return nil
	}
	return base64.StdEncoding.EncodeToString(content)
}

// serveIcon serves /storage/icons/{name}.
func (s *Server) serveIcon(c *ctx) {
	name := strings.TrimPrefix(c.r.URL.Path, s.baseURL()+"/storage/icons/")
	name = strings.TrimPrefix(name, "/")
	if name == "" || strings.Contains(name, "..") {
		http.NotFound(c.w, c.r)
		return
	}
	content, err := s.icons.Get(name)
	if err != nil {
		http.NotFound(c.w, c.r)
		return
	}
	c.w.Header().Set("Content-Type", s.icons.MimeType(name))
	c.w.Header().Set("Cache-Control", "public, max-age=604800")
	_, _ = c.w.Write(content)
}

// latestReleaseScan queries the GitHub API for the newest release.
func (s *Server) latestReleaseScan() any {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/Bubka/2FAuth/releases/latest")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return false
	}
	tag := ""
	for _, line := range strings.Split(string(body), ",") {
		if strings.Contains(line, `"tag_name"`) {
			parts := strings.Split(line, `"`)
			for i, p := range parts {
				if p == "tag_name" && i+2 < len(parts) {
					tag = parts[i+2]
				}
			}
			break
		}
	}
	if tag == "" {
		return false
	}
	tag = strings.TrimPrefix(tag, "v")
	if tag == s.cfg.Version {
		return false
	}
	return tag
}
