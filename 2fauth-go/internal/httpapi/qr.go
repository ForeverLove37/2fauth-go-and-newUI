package httpapi

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"

	"2fauth/internal/services"
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
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return nil, "", err
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	file, header, err := r.FormFile(field)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxUploadSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxUploadSize {
		return nil, "", errors.New("file too large")
	}
	return data, header.Filename, nil
}

// iconIfExists returns the icon name when the file exists, nil otherwise.
func (s *Server) iconIfExists(name string) any {
	if !services.ValidIconName(name) || !s.icons.Exists(name) {
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
	if !services.ValidIconName(name) {
		http.NotFound(c.w, c.r)
		return
	}
	content, err := s.icons.Get(name)
	if err != nil {
		http.NotFound(c.w, c.r)
		return
	}
	// Whatever was stored by an older build, only an image leaves this endpoint, and
	// even an SVG opened directly in a tab runs no script and reaches no resource.
	mimeType := s.icons.MimeType(name)
	if !strings.HasPrefix(mimeType, "image/") {
		http.NotFound(c.w, c.r)
		return
	}
	c.w.Header().Set("Content-Type", mimeType)
	c.w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox")
	c.w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
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
