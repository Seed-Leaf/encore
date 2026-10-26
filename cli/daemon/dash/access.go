package dash

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"encr.dev/pkg/httpx"
)

// AccessPolicy controls browser origins and the dashboard's remote read-only mode.
// Its zero value retains the local developer dashboard's access policy.
// It does not authenticate users: a remote listener must be completely protected
// by an external authentication proxy, with no direct ingress to the listener.
type AccessPolicy struct {
	origins map[string]struct{}
}

// ParseAccessPolicy accepts an empty string for local mode, or a JSON array of
// exact HTTPS origins for remote mode. Hostname case and the default HTTPS port
// are normalized; duplicate origins are rejected. Localhost is not implicitly
// allowed in remote mode.
func ParseAccessPolicy(raw string) (AccessPolicy, error) {
	if raw == "" {
		return AccessPolicy{}, nil
	}
	var origins []string
	if err := json.Unmarshal([]byte(raw), &origins); err != nil {
		return AccessPolicy{}, fmt.Errorf("dashboard origins must be a JSON array of HTTPS origins: %w", err)
	}
	if len(origins) == 0 {
		return AccessPolicy{}, fmt.Errorf("dashboard origins must contain at least one HTTPS origin")
	}
	policy := AccessPolicy{origins: make(map[string]struct{}, len(origins))}
	for i, origin := range origins {
		canonical, err := canonicalHTTPSOrigin(origin)
		if err != nil {
			return AccessPolicy{}, fmt.Errorf("dashboard origin %d: %w", i+1, err)
		}
		if _, exists := policy.origins[canonical]; exists {
			return AccessPolicy{}, fmt.Errorf("dashboard origin %d duplicates another origin", i+1)
		}
		policy.origins[canonical] = struct{}{}
	}
	return policy, nil
}

// Remote reports whether the dashboard must run in remote read-only mode.
func (p AccessPolicy) Remote() bool { return p.origins != nil }

// AllowRequest checks browser origins, not user identity. Requests without an
// Origin are allowed for normal navigation and authenticated proxy/nonbrowser
// clients. Multiple Origin headers always fail closed, including in local mode.
func (p AccessPolicy) AllowRequest(req *http.Request) bool {
	var origins []string
	for name, values := range req.Header {
		if strings.EqualFold(name, "Origin") {
			origins = append(origins, values...)
		}
	}
	if len(origins) > 1 {
		return false
	}
	if !p.Remote() {
		return httpx.IsNotExternalWebsite(req)
	}
	for _, value := range req.Header.Values("Sec-Fetch-Site") {
		for _, site := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(site), "cross-site") {
				return false
			}
		}
	}
	if len(origins) == 0 {
		return true
	}
	origin, err := canonicalHTTPSOrigin(origins[0])
	if err != nil {
		return false
	}
	_, allowed := p.origins[origin]
	return allowed
}

func canonicalHTTPSOrigin(origin string) (string, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Opaque != "" || u.Host == "" || u.User != nil ||
		u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery ||
		u.Fragment != "" || strings.ContainsAny(origin, "#*") {
		return "", fmt.Errorf("expected an HTTPS origin without credentials, path, query, fragment, or wildcard")
	}
	host := strings.ToLower(u.Hostname())
	addr, ipErr := netip.ParseAddr(host)
	if strings.HasPrefix(u.Host, "[") && (ipErr != nil || !addr.Is6()) {
		return "", fmt.Errorf("bracketed origin host must be a valid IPv6 address")
	}
	if ipErr == nil {
		if addr.Zone() != "" {
			return "", fmt.Errorf("origin must not contain an IPv6 zone")
		}
		host = addr.String()
		if addr.Is6() {
			if !strings.HasPrefix(u.Host, "[") {
				return "", fmt.Errorf("IPv6 origin must use brackets")
			}
			host = "[" + host + "]"
		}
	} else {
		// Browser origins use ASCII hostnames (IDNs must be provided as punycode).
		domain := strings.TrimSuffix(host, ".")
		if domain == "" || len(domain) > 253 {
			return "", fmt.Errorf("invalid origin hostname")
		}
		for _, label := range strings.Split(domain, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return "", fmt.Errorf("invalid origin hostname")
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
					return "", fmt.Errorf("invalid origin hostname")
				}
			}
		}
	}
	if strings.HasSuffix(u.Host, ":") {
		return "", fmt.Errorf("origin port must not be empty")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("invalid origin port")
		}
		if n != 443 {
			host += ":" + strconv.Itoa(n)
		}
	}
	return "https://" + host, nil
}

// remoteReadMethod deliberately defaults to false so new RPC operations cannot
// become remotely callable without a separate review.
func remoteReadMethod(method string) bool {
	switch method {
	case "version", "list-apps", "status", "onboarding/get",
		"traces/list", "traces/get", "traces/spans/summaries/list", "traces/spans/events/list":
		return true
	default:
		return false
	}
}
