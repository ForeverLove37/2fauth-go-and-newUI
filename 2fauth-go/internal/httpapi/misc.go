package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strconv"
	"time"
)

func timeNowRFC1123() string { return time.Now().Format(time.RFC1123Z) }

func timeNowFormatted() string { return time.Now().UTC().Format("2006-01-02 15:04:05") }

func timeNowUnix() int64 { return time.Now().Unix() }

func runtimeOS() string { return runtime.GOOS + " " + runtime.GOARCH }

func boolExport(b bool) string {
	if b {
		return "'1'"
	}
	return "''"
}

func orNone(v string) string {
	if v == "" {
		return "none"
	}
	return v
}

// serveBuildAsset serves a file from public/build.
func (s *Server) serveBuildAsset(c *ctx) {
	s.serveStatic(c)
}

// servePublicFile serves a file from the public directory.
func (s *Server) servePublicFile(c *ctx) {
	s.serveStatic(c)
}

// serveIconRoute serves an icon file.
func (s *Server) serveIconRoute(c *ctx) {
	s.serveIcon(c)
}

var _ = fmt.Sprint
var _ = strconv.Itoa
var _ = http.StatusOK

// srvError logs a server side error and returns the generic error response.
func (s *Server) srvError(c *ctx, err error) {
	slog.Error("request failed", "method", c.r.Method, "path", c.r.URL.Path, "error", err)
	jsonMessage(c.w, http.StatusInternalServerError, "server error")
}
