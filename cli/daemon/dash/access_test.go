package dash

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseAccessPolicy(t *testing.T) {
	for _, raw := range []string{
		"", `["https://dashboard.example.com"]`,
		`["https://dashboard.example.com:8443", "https://other.example.com"]`,
		`["https://127.0.0.1", "https://[::1]:8443"]`,
	} {
		t.Run("valid_"+raw, func(t *testing.T) {
			p, err := ParseAccessPolicy(raw)
			if err != nil || p.Remote() != (raw != "") {
				t.Fatalf("ParseAccessPolicy(%q) = remote %v, %v", raw, p.Remote(), err)
			}
		})
	}
	for _, raw := range []string{
		" ", `null`, `[]`, `{}`, `"https://dashboard.example.com"`,
		`[null]`, `[1]`, `[""]`, `["https://dashboard.example.com",]`,
		`["https://dashboard.example.com"] true`,
		`["http://dashboard.example.com"]`, `["//dashboard.example.com"]`,
		`["https:dashboard.example.com"]`, `["https://"]`,
		`["*"]`, `["https://*.example.com"]`,
		`["https://user:password@dashboard.example.com"]`,
		`["https://@dashboard.example.com"]`,
		`["https://dashboard.example.com/"]`, `["https://dashboard.example.com/path"]`,
		`["https://dashboard.example.com?x=1"]`, `["https://dashboard.example.com?"]`,
		`["https://dashboard.example.com#x"]`, `["https://dashboard.example.com#"]`,
		`[" https://dashboard.example.com"]`, `["https://dashboard.example.com "]`,
		`["https://dashboard.example.com:bad"]`, `["https://dashboard.example.com:"]`,
		`["https://dashboard.example.com:0"]`, `["https://dashboard.example.com:65536"]`,
		`["https://bad,host"]`, `["https://bad..host"]`, `["https://-bad.host"]`,
		`["https://[fe80::1%25eth0]"]`, `["https://::1"]`,
		`["https://[example.com]"]`, `["https://[127.0.0.1]"]`,
		`["https://dashboard.example.com", "https://dashboard.example.com"]`,
		`["https://DASHBOARD.example.com:443", "https://dashboard.example.com"]`,
	} {
		t.Run("invalid_"+raw, func(t *testing.T) {
			if _, err := ParseAccessPolicy(raw); err == nil {
				t.Fatalf("ParseAccessPolicy(%q) unexpectedly succeeded", raw)
			}
		})
	}
}

func TestAccessPolicyAllowRequest(t *testing.T) {
	remote := testRemotePolicy(t)
	for _, tt := range []struct {
		name    string
		origins []string
		site    string
		local   bool
		remote  bool
	}{
		{name: "navigation or nonbrowser", local: true, remote: true},
		{name: "configured origin", origins: []string{"https://dashboard.example.com"}, remote: true},
		{name: "equivalent origin", origins: []string{"https://DASHBOARD.example.com:443"}, remote: true},
		{name: "wrong port", origins: []string{"https://dashboard.example.com:8443"}},
		{name: "wrong scheme", origins: []string{"http://dashboard.example.com"}},
		{name: "subdomain", origins: []string{"https://sub.dashboard.example.com"}},
		{name: "suffix attack", origins: []string{"https://dashboard.example.com.evil.test"}},
		{name: "userinfo", origins: []string{"https://dashboard.example.com@evil.test"}},
		{name: "path", origins: []string{"https://dashboard.example.com/"}},
		{name: "null origin", origins: []string{"null"}},
		{name: "empty origin", origins: []string{""}, local: true},
		{name: "localhost", origins: []string{"http://localhost:9400"}, local: true},
		{name: "ipv4 loopback", origins: []string{"http://127.0.0.1:9400"}, local: true},
		{name: "ipv6 loopback", origins: []string{"http://[::1]:9400"}, local: true},
		{name: "cross-site no origin", site: "cross-site"},
		{name: "cross-site configured origin", origins: []string{"https://dashboard.example.com"}, site: "cross-site"},
		{name: "same-origin", origins: []string{"https://dashboard.example.com"}, site: "same-origin", remote: true},
		// Duplicate headers intentionally fail closed in local mode as well.
		{name: "duplicate localhost", origins: []string{"http://localhost", "http://localhost"}},
		{name: "duplicate configured", origins: []string{"https://dashboard.example.com", "https://dashboard.example.com"}},
		{name: "mixed origins", origins: []string{"https://dashboard.example.com", "https://evil.test"}},
		{name: "joined origins", origins: []string{"https://dashboard.example.com, https://evil.test"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
			for _, origin := range tt.origins {
				req.Header.Add("Origin", origin)
			}
			req.Header.Set("Sec-Fetch-Site", tt.site)
			if got := (AccessPolicy{}).AllowRequest(req); got != tt.local {
				t.Errorf("local AllowRequest = %v, want %v", got, tt.local)
			}
			if got := remote.AllowRequest(req); got != tt.remote {
				t.Errorf("remote AllowRequest = %v, want %v", got, tt.remote)
			}
		})
	}
}

func testRemotePolicy(t *testing.T) AccessPolicy {
	t.Helper()
	p, err := ParseAccessPolicy(`["https://dashboard.example.com"]`)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRemoteCrossSiteNavigation(t *testing.T) {
	policy := testRemotePolicy(t)
	for _, tt := range []struct {
		name, method, path string
		change             func(http.Header)
		allow              bool
	}{
		{name: "SSO redirect", method: http.MethodGet, path: "/", allow: true},
		{name: "external deep link", method: http.MethodGet, path: "/app/traces", allow: true},
		{name: "HEAD navigation", method: http.MethodHead, path: "/", allow: true},
		{name: "POST", method: http.MethodPost, path: "/"},
		{name: "origin present", change: func(h http.Header) { h.Set("Origin", "https://dashboard.example.com") }},
		{name: "empty origin present", change: func(h http.Header) { h.Set("Origin", "") }},
		{name: "missing mode", change: func(h http.Header) { h.Del("Sec-Fetch-Mode") }},
		{name: "missing destination", change: func(h http.Header) { h.Del("Sec-Fetch-Dest") }},
		{name: "subresource", change: func(h http.Header) { h.Set("Sec-Fetch-Mode", "no-cors"); h.Set("Sec-Fetch-Dest", "image") }},
		{name: "iframe", change: func(h http.Header) { h.Set("Sec-Fetch-Dest", "iframe") }},
		{name: "websocket mode", change: func(h http.Header) { h.Set("Sec-Fetch-Mode", "websocket") }},
		{name: "upgrade", change: func(h http.Header) { h.Set("Upgrade", "websocket") }},
		{name: "multiple sites", change: func(h http.Header) { h.Add("Sec-Fetch-Site", "same-origin") }},
		{name: "multiple modes", change: func(h http.Header) { h.Add("Sec-Fetch-Mode", "navigate") }},
		{name: "multiple destinations", change: func(h http.Header) { h.Add("Sec-Fetch-Dest", "document") }},
		{name: "RPC path", path: "/__encore"},
		{name: "GraphQL path", path: "/__graphql"},
		{name: "object path", path: "/__encore/objects/content"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			method, path := tt.method, tt.path
			if method == "" {
				method = http.MethodGet
			}
			if path == "" {
				path = "/"
			}
			req := httptest.NewRequest(method, "https://dashboard.example.com"+path, nil)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			req.Header.Set("Sec-Fetch-Mode", "navigate")
			req.Header.Set("Sec-Fetch-Dest", "document")
			if tt.change != nil {
				tt.change(req.Header)
			}
			if got := policy.AllowRequest(req); got != tt.allow {
				t.Errorf("AllowRequest = %v, want %v", got, tt.allow)
			}
			if (AccessPolicy{}).AllowRequest(req) {
				t.Error("local policy unexpectedly permits cross-site request")
			}
		})
	}
}
