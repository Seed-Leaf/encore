package remoteui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedAssetsAndNavigation(t *testing.T) {
	h := Handler{}
	for _, tt := range []struct {
		path, contentType, marker string
	}{
		{"/", "text/html", `<meta name="encore-dashboard" content="remote-read-only">`},
		{"/index.html", "text/html", `<meta name="encore-dashboard" content="remote-read-only">`},
		{"/apps/example/traces?app=example", "text/html", `<meta name="encore-dashboard" content="remote-read-only">`},
		{"/apps/example/traces/", "text/html", `<meta name="encore-dashboard" content="remote-read-only">`},
		{"/app.js", "text/javascript", "encore-remote-read-only-ui"},
		{"/rpc.js", "text/javascript", "WebSocket"},
		{"/styles.css", "text/css", "{"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			get := httptest.NewRecorder()
			h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), tt.marker) {
				t.Fatalf("GET %s: HTTP %d, missing expected owned asset marker", tt.path, get.Code)
			}
			if !strings.HasPrefix(get.Header().Get("Content-Type"), tt.contentType) {
				t.Fatalf("unexpected Content-Type: %s", get.Header().Get("Content-Type"))
			}
			if get.Header().Get("Cache-Control") != "no-store" || get.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("asset missing cache or MIME protections")
			}
			head := httptest.NewRecorder()
			h.ServeHTTP(head, httptest.NewRequest(http.MethodHead, tt.path, nil))
			if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != get.Header().Get("Content-Length") {
				t.Fatalf("HEAD %s did not preserve asset metadata without body", tt.path)
			}
		})
	}
}

func TestAssetsDoNotFallbackForAPIOrMissingFiles(t *testing.T) {
	h := Handler{}
	for _, target := range []string{
		"/__encore", "/__graphql", "/__encore/objects/content", "/__future/api",
		"/missing.js", "/index.html.map", "/rpc.test.mjs", "/handler.go", "/assets/not-found.css",
		"/../app.js", "/a/../route", "/a//route",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, rec.Code)
		}
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, "/app.js", nil))
		if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
			t.Errorf("%s = %d, expected GET/HEAD-only route", method, rec.Code)
		}
	}
}
