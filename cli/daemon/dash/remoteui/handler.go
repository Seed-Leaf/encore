// Package remoteui serves the independent, read-only dashboard frontend.
package remoteui

import (
	"bytes"
	"embed"
	"net/http"
	"path"
	"strings"
	"time"
)

// ContentSecurityPolicy limits the owned UI to its embedded assets and the
// same-origin dashboard connection. No Cloud login or third-party scripts are
// needed. The external authentication proxy remains mandatory.
const ContentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

//go:embed index.html app.js rpc.js styles.css
var assets embed.FS

// Handler serves only the embedded UI. Extensionless paths fall back to the
// entry document so bookmarked application views can load directly.
type Handler struct{}

func (Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var name, contentType string
	switch req.URL.Path {
	case "/", "/index.html":
		name, contentType = "index.html", "text/html; charset=utf-8"
	case "/app.js", "/rpc.js":
		name, contentType = strings.TrimPrefix(req.URL.Path, "/"), "text/javascript; charset=utf-8"
	case "/styles.css":
		name, contentType = "styles.css", "text/css; charset=utf-8"
	default:
		if strings.HasPrefix(req.URL.Path, "/__") || path.Ext(req.URL.Path) != "" || path.Clean(req.URL.Path) != strings.TrimSuffix(req.URL.Path, "/") {
			http.NotFound(w, req)
			return
		}
		name, contentType = "index.html", "text/html; charset=utf-8"
	}
	data, err := assets.ReadFile(name)
	if err != nil {
		http.Error(w, "dashboard asset unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, req, name, time.Time{}, bytes.NewReader(data))
}
