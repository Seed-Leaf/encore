package dash

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"encr.dev/cli/daemon/dash/remoteui"
	"encr.dev/cli/daemon/engine/trace2"
	"encr.dev/cli/daemon/run"
	"encr.dev/cli/internal/jsonrpc2"
)

func TestServerHTTPAccess(t *testing.T) {
	var upstreamCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Add("Content-Security-Policy", "default-src 'self'")
		w.Header().Add("Content-Security-Policy", "object-src 'none'")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	s := &Server{access: testRemotePolicy(t), proxy: proxy, apiProxy: proxy}

	for _, tt := range []struct {
		method, path, origin string
		want                 int
	}{
		{http.MethodGet, "/", "https://dashboard.example.com", http.StatusOK},
		{http.MethodGet, "/apps/example/traces", "https://dashboard.example.com", http.StatusOK},
		{http.MethodHead, "/app.js", "https://dashboard.example.com", http.StatusOK},
		{http.MethodGet, "/app.js", "", http.StatusOK},
		{http.MethodGet, "/rpc.js", "", http.StatusOK},
		{http.MethodGet, "/styles.css", "", http.StatusOK},
		{http.MethodGet, "/old-upstream-asset.js", "", http.StatusNotFound},
		{http.MethodGet, "/__future/api", "", http.StatusNotFound},
		{http.MethodPost, "/asset.js", "https://dashboard.example.com", http.StatusMethodNotAllowed},
		{http.MethodDelete, "/asset.js", "https://dashboard.example.com", http.StatusMethodNotAllowed},
		{http.MethodGet, "/__graphql", "https://dashboard.example.com", http.StatusForbidden},
		{http.MethodPost, "/__graphql", "https://dashboard.example.com", http.StatusForbidden},
		{http.MethodGet, "/__encore/objects/content", "https://dashboard.example.com", http.StatusForbidden},
		{http.MethodGet, "/", "https://evil.test", http.StatusForbidden},
		{http.MethodGet, "/__graphql", "https://evil.test", http.StatusForbidden},
		{http.MethodGet, "/__encore/objects/content", "https://evil.test", http.StatusForbidden},
		{http.MethodGet, "/__encore", "https://evil.test", http.StatusForbidden},
	} {
		t.Run(tt.method+tt.path+tt.origin, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://localhost"+tt.path, nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			before := upstreamCalls.Load()
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("HTTP status = %d, want %d", rec.Code, tt.want)
			}
			wantPolicies := []string{remoteui.ContentSecurityPolicy}
			if got := rec.Header().Values("Content-Security-Policy"); !reflect.DeepEqual(got, wantPolicies) {
				t.Fatalf("response policies = %v, want %v", got, wantPolicies)
			}
			if upstreamCalls.Load() != before {
				t.Fatal("remote request reached the upstream")
			}
			if tt.want == http.StatusMethodNotAllowed && rec.Header().Get("Allow") != "GET, HEAD" {
				t.Fatal("method denial missing Allow header")
			}
		})
	}

	// Local mode retains non-GET asset requests and the Cloud proxy, while the
	// Server now applies the local origin guard even without outer middleware.
	s.access = AccessPolicy{}
	for _, path := range []string{"/", "/__graphql"} {
		req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, nil)
		req.Header.Set("Origin", "http://localhost:9400")
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("local POST %s = %d", path, rec.Code)
		}
		if got := rec.Header().Values("Content-Security-Policy"); !reflect.DeepEqual(got, []string{"default-src 'self'", "object-src 'none'"}) {
			t.Fatalf("local response policy changed: %v", got)
		}
		req.Header.Set("Origin", "https://evil.test")
		rec = httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("external local request %s = %d", path, rec.Code)
		}
	}
}

func TestServerWebSocketOrigins(t *testing.T) {
	for _, direct := range []bool{false, true} {
		for _, tt := range []struct {
			name    string
			remote  bool
			origins []string
			site    string
			mode    string
			dest    string
			allow   bool
		}{
			{name: "remote configured", remote: true, origins: []string{"https://dashboard.example.com"}, allow: true},
			{name: "remote absent", remote: true, allow: true},
			{name: "remote foreign", remote: true, origins: []string{"https://evil.test"}},
			{name: "remote localhost not implicit", remote: true, origins: []string{"http://localhost:9400"}},
			{name: "remote duplicates", remote: true, origins: []string{"https://dashboard.example.com", "https://dashboard.example.com"}},
			{name: "remote cross-site", remote: true, origins: []string{"https://dashboard.example.com"}, site: "cross-site"},
			{name: "remote cross-site navigation metadata cannot upgrade", remote: true, site: "cross-site", mode: "navigate", dest: "document"},
			{name: "local localhost", origins: []string{"http://localhost:9400"}, allow: true},
			{name: "local absent", allow: true},
			{name: "local foreign", origins: []string{"https://dashboard.example.com"}},
			{name: "local duplicates fail closed", origins: []string{"http://localhost:9400", "http://localhost:9400"}},
		} {
			name := "ServeHTTP/"
			if direct {
				name = "WebSocket/"
			}
			t.Run(name+tt.name, func(t *testing.T) {
				s := &Server{clients: make(map[chan<- *notification]struct{})}
				if tt.remote {
					s.access = testRemotePolicy(t)
				}
				var handler http.Handler = s
				if direct {
					handler = http.HandlerFunc(s.WebSocket)
				}
				srv := httptest.NewServer(handler)
				defer srv.Close()
				headers := make(http.Header)
				for _, origin := range tt.origins {
					headers.Add("Origin", origin)
				}
				headers.Set("Sec-Fetch-Site", tt.site)
				headers.Set("Sec-Fetch-Mode", tt.mode)
				headers.Set("Sec-Fetch-Dest", tt.dest)
				c, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/__encore", headers)
				if !tt.allow {
					if c != nil {
						c.Close()
					}
					if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
						t.Fatalf("denied handshake: response %v, err %v", resp, err)
					}
					resp.Body.Close()
					return
				}
				if err != nil {
					t.Fatalf("allowed handshake: %v", err)
				}
				wantPolicy := ""
				if tt.remote {
					wantPolicy = remoteui.ContentSecurityPolicy
				}
				if got := resp.Header.Get("Content-Security-Policy"); got != wantPolicy {
					t.Fatalf("handshake CSP = %q, want %q", got, wantPolicy)
				}
				defer c.Close()
				if err := c.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
					t.Fatal(err)
				}
				// A real allowed RPC verifies that the upgrade and dispatcher work.
				if err := c.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "version"}); err != nil {
					t.Fatal(err)
				}
				var response struct {
					Result map[string]json.RawMessage `json:"result"`
					Error  json.RawMessage            `json:"error"`
				}
				if err := c.ReadJSON(&response); err != nil {
					t.Fatal(err)
				}
				if len(response.Error) != 0 || response.Result["version"] == nil || response.Result["channel"] == nil {
					t.Fatalf("unexpected version response: %+v", response)
				}
				if tt.remote {
					if err := c.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "traces/clear", "params": map[string]any{}}); err != nil {
						t.Fatal(err)
					}
					var denied struct {
						Error struct {
							Code int `json:"code"`
						} `json:"error"`
					}
					if err := c.ReadJSON(&denied); err != nil || denied.Error.Code != -32601 {
						t.Fatalf("mutation response: %+v, %v", denied, err)
					}
				}
			})
		}
	}
}

func TestRemoteServerDoesNotCreateUpstreamProxies(t *testing.T) {
	s := NewServer(nil, &run.Manager{}, nil, &closedTraceStore{}, 9400, testRemotePolicy(t))
	if s.proxy != nil || s.apiProxy != nil {
		t.Fatal("remote server constructed an asset or Cloud proxy")
	}
	// Exercise normal navigation after an SSO redirect, with no proxy available
	// and an extra policy already installed by outer middleware.
	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	rec.Header().Add("Content-Security-Policy", "object-src 'none'")
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<meta name="encore-dashboard" content="remote-read-only">`) {
		t.Fatalf("remote UI without upstream: HTTP %d, missing owned HTML marker", rec.Code)
	}
	want := []string{"object-src 'none'", remoteui.ContentSecurityPolicy}
	if got := rec.Header().Values("Content-Security-Policy"); !reflect.DeepEqual(got, want) {
		t.Fatalf("response policies = %v, want %v", got, want)
	}
}

type closedTraceStore struct{ trace2.Store }

func (*closedTraceStore) Listen(ch chan<- trace2.NewSpanEvent) {
	close(ch)
}

func TestRemoteRPCDeniesBeforeParamsOrBackend(t *testing.T) {
	// Deliberately absent backends make accidental dispatch of valid requests
	// fail loudly; malformed params must receive the same policy denial first.
	h := &handler{readOnly: true}
	for _, method := range []string{
		"db/query", "db/transaction", "db-migration-status", "api-call",
		"objects/list", "objects/search", "objects/delete", "objects/create-folder",
		"objects/download-url", "objects/open", "objects/reveal",
		"onboarding/set", "telemetry", "traces/clear", "editors/list", "editors/open", "future/method",
	} {
		for _, params := range []any{nil, map[string]any{}, "invalid parameter shape"} {
			t.Run(method, func(t *testing.T) {
				r, err := jsonrpc2.NewCall(jsonrpc2.NewIntID(1), method, params)
				if err != nil {
					t.Fatal(err)
				}
				called := 0
				err = h.Handle(context.Background(), func(ctx context.Context, result any, err error) error {
					called++
					if result != nil || !errors.Is(err, jsonrpc2.ErrMethodNotFound) {
						t.Fatalf("result %v, err %v; want policy denial", result, err)
					}
					return nil
				}, r)
				if err != nil || called != 1 {
					t.Fatalf("Handle: %v, reply calls %d", err, called)
				}
			})
		}
	}
	// Local mode must still enter the original handler: missing db/query params
	// give the original parameter error, rather than the remote denial.
	h.readOnly = false
	var r jsonrpc2.Call
	if err := json.Unmarshal([]byte(`{"jsonrpc":"2.0","id":1,"method":"db/query"}`), &r); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(context.Background(), func(ctx context.Context, result any, err error) error {
		if err == nil || err.Error() != "missing params" {
			t.Fatalf("local handler changed: %v", err)
		}
		return nil
	}, &r); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteNotificationsSuppressProcessOutput(t *testing.T) {
	for _, remote := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan *notification, 3)
		for _, method := range []string{"process/output", "process/start", "trace/new"} {
			ch <- &notification{Method: method}
		}
		var got []string
		rpc := &notificationRecorder{notify: func(method string) {
			got = append(got, method)
			if method == "trace/new" {
				cancel()
			}
		}}
		h := &handler{rpc: rpc, readOnly: remote}
		h.listenNotify(ctx, ch)
		cancel()
		want := []string{"process/output", "process/start", "trace/new"}
		if remote {
			want = want[1:]
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("remote %v: notifications %v, want %v", remote, got, want)
		}
	}
}

type notificationRecorder struct {
	jsonrpc2.Conn
	notify func(string)
}

func (r *notificationRecorder) Notify(ctx context.Context, method string, params any) error {
	r.notify(method)
	return nil
}
