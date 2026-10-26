// Package dashproxy proxies requests to the dash server,
// caching them locally for offline access.
package dashproxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/peterbourgon/diskv"

	"encr.dev/internal/conf"
	"encr.dev/internal/httpcache"
	"encr.dev/internal/httpcache/diskcache"
	"encr.dev/internal/version"
)

func New(targetURL string) (*httputil.ReverseProxy, error) {
	return newProxy(targetURL, false)
}

// NewRemote creates a proxy for the remote read-only dashboard. Authentication
// credentials and proxy identity headers must never reach the asset host.
func NewRemote(targetURL string) (*httputil.ReverseProxy, error) {
	return newProxy(targetURL, true)
}

func newProxy(targetURL string, remote bool) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, errors.Wrap(err, "parse target url")
	}

	var transport http.RoundTripper = &versionAddingTransport{version: version.Version}
	if conf.CacheDevDash {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return nil, errors.Wrap(err, "get user cache dir")
		}

		cache := diskcache.NewWithDiskv(diskv.New(diskv.Options{
			BasePath:     filepath.Join(cacheDir, "encore", "dashcache"),
			CacheSizeMax: 1024 * 1024 * 1024, // 1GiB
			Compression:  diskv.NewGzipCompression(),
		}))

		// Wrap the transport with a caching transport.
		cachingTransport := httpcache.NewTransport(cache)
		cachingTransport.Transport = transport
		transport = cachingTransport
	}

	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			if remote {
				r.Out.Header = assetRequestHeaders(r.Out.Header)
			}

			// Configure cache headers so the cache behaves the way we want it to.
			r.Out.Header.Del("Cookie")
			if !remote || r.Out.Header.Get("Cache-Control") == "" {
				r.Out.Header.Set("Cache-Control", "stale-if-error")
			}
			r.Out.Header.Del("Vary")
		},
		ModifyResponse: func(resp *http.Response) error {
			if remote {
				resp.Header.Del("Set-Cookie")
			}
			if resp.StatusCode < 300 {
				resp.Header.Del("Vary")
				resp.Header.Set("Cache-Control", "max-age=60,stale-if-error=86400")
			}
			return nil
		},
	}

	return proxy, nil
}

func assetRequestHeaders(in http.Header) http.Header {
	out := make(http.Header)
	for _, name := range []string{
		"Accept", "Accept-Encoding", "Accept-Language", "Cache-Control",
		"If-Match", "If-Modified-Since", "If-None-Match", "If-Range", "If-Unmodified-Since", "Range",
	} {
		if values := in.Values(name); len(values) > 0 {
			out[name] = append([]string(nil), values...)
		}
	}
	return out
}

type versionAddingTransport struct {
	version string
}

func (t *versionAddingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.version != "" {
		vals := req.URL.Query()
		vals.Set("cli_version", t.version)
		req.URL.RawQuery = vals.Encode()
	}
	return http.DefaultTransport.RoundTrip(req)
}
