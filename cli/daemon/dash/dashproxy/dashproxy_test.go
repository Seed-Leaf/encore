package dashproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"sync/atomic"
	"testing"

	"encr.dev/internal/conf"
	"encr.dev/internal/httpcache"
	"encr.dev/internal/httpcache/diskcache"
)

func TestRemoteProxyStripsCredentials(t *testing.T) {
	oldCache := conf.CacheDevDash
	conf.CacheDevDash = false
	t.Cleanup(func() { conf.CacheDevDash = oldCache })

	requests := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Header.Clone()
		w.Header().Add("Set-Cookie", "upstream=must-not-reach-browser; Secure")
		w.Header().Set("Content-Type", "application/javascript")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	for _, remote := range []bool{true, false} {
		newProxy := New
		if remote {
			newProxy = NewRemote
		}
		proxy, err := newProxy(upstream.URL)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "https://dashboard.example.com/asset.js", nil)
		for _, name := range []string{
			"Cookie", "Authorization", "Proxy-Authorization", "X-Auth-Request-User", "X-Auth-Request-Email",
			"X-Forwarded-For", "X-Forwarded-User", "X-Forwarded-Access-Token", "X-Forwarded-Host", "X-Forwarded-Proto",
			"Forwarded", "X-Remote-User", "X-Custom-SSO-Token", "Origin", "Referer",
		} {
			req.Header.Set(name, "sensitive")
		}
		preserved := map[string]string{
			"Accept": "application/javascript", "Accept-Encoding": "gzip", "Accept-Language": "ko",
			"Range": "bytes=0-100", "If-None-Match": "etag", "If-Modified-Since": "Wed, 21 Oct 2015 07:28:00 GMT",
			"Cache-Control": "no-cache", "If-Range": "range-etag",
		}
		for name, value := range preserved {
			req.Header.Set(name, value)
		}
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("remote %v: status %d", remote, rec.Code)
		}
		received := <-requests
		if received.Get("Cookie") != "" {
			t.Fatal("Cookie leaked upstream")
		}
		if remote {
			for name := range req.Header {
				if _, allowed := preserved[name]; !allowed && received.Get(name) != "" {
					t.Errorf("credential or identity header %s leaked upstream", name)
				}
			}
			for name, value := range preserved {
				if got := received.Get(name); got != value {
					t.Errorf("asset header %s = %q, want %q", name, got, value)
				}
			}
			if rec.Header().Get("Set-Cookie") != "" {
				t.Fatal("upstream Set-Cookie reached browser")
			}
		} else {
			if received.Get("Authorization") != "sensitive" || rec.Header().Get("Set-Cookie") == "" {
				t.Fatal("local proxy behavior changed")
			}
			if received.Get("Cache-Control") != "stale-if-error" {
				t.Fatal("local cache policy changed")
			}
		}
	}
}

func TestRemoteProxyStripsCookiesOnDiskCacheHit(t *testing.T) {
	// Attach the production cache transport with an explicitly isolated disk
	// cache. XDG_CACHE_HOME does not redirect os.UserCacheDir on macOS.
	oldCache := conf.CacheDevDash
	conf.CacheDevDash = false
	t.Cleanup(func() { conf.CacheDevDash = oldCache })
	var upstreamCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Cache-Control", "public,max-age=3600")
		w.Header().Set("Set-Cookie", "upstream=secret; Secure")
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = io.WriteString(w, "console.log('asset');")
	}))
	defer upstream.Close()
	proxy, err := NewRemote(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	cache := diskcache.New(t.TempDir())
	transport := httpcache.NewTransport(cache)
	transport.Transport = proxy.Transport
	proxy.Transport = transport

	for i := 0; i < 2; i++ {
		if i == 1 {
			// A cached asset can predate remote mode. Seed a cookie into the
			// existing entry to prove ModifyResponse also protects cache hits.
			cacheURL := upstream.URL + "/asset.js"
			cached, err := httpcache.CachedResponse(cache, httptest.NewRequest(http.MethodGet, cacheURL, nil))
			if err != nil || cached == nil {
				t.Fatalf("first response was not cached: %v", err)
			}
			cached.Header.Set("Set-Cookie", "previous-local-cache=secret; Secure")
			data, err := httputil.DumpResponse(cached, true)
			cached.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			cache.Set(cacheURL, data)
		}
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "https://dashboard.example.com/asset.js", nil))
		if rec.Code != http.StatusOK || rec.Body.String() != "console.log('asset');" {
			t.Fatalf("request %d: status %d, body %q", i+1, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Set-Cookie") != "" {
			t.Fatalf("request %d leaked Set-Cookie", i+1)
		}
		if i == 1 && rec.Header().Get(httpcache.XFromCache) != "1" {
			t.Fatal("second response did not use the disk cache")
		}
	}
	if got := upstreamCalls.Load(); got != 1 {
		t.Fatalf("upstream requests = %d, want 1", got)
	}
}
