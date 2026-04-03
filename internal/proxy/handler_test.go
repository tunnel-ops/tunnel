package proxy

import (
	"net/url"
	"testing"
)

func TestExtractPort(t *testing.T) {
	tests := []struct {
		host      string
		domain    string
		wantPort  int
		wantError bool
	}{
		{"9000.example.com", "example.com", 9000, false},
		{"3000.example.com", "example.com", 3000, false},
		{"65535.example.com", "example.com", 65535, false},
		{"9000.example.com:443", "example.com", 9000, false},
		// wrong domain
		{"9000.other.com", "example.com", 0, true},
		// non-numeric subdomain with no names store → error
		{"evil.example.com", "example.com", 0, true},
		// nested subdomain
		{"sub.9000.example.com", "example.com", 0, true},
		// privileged / out of range
		{"80.example.com", "example.com", 0, true},
		{"1023.example.com", "example.com", 0, true},
		{"65536.example.com", "example.com", 0, true},
		{"0.example.com", "example.com", 0, true},
	}

	for _, tc := range tests {
		port, errMsg := ExtractPort(tc.host, tc.domain)
		gotError := errMsg != ""

		if gotError != tc.wantError {
			t.Errorf("ExtractPort(%q, %q): wantError=%v, got errMsg=%q", tc.host, tc.domain, tc.wantError, errMsg)
			continue
		}
		if !tc.wantError && port != tc.wantPort {
			t.Errorf("ExtractPort(%q, %q): want port %d, got %d", tc.host, tc.domain, tc.wantPort, port)
		}
	}
}

func TestDefaultBlockedPorts(t *testing.T) {
	blocked := DefaultBlockedPorts()
	sensitive := []int{22, 25, 3306, 5432, 6379, 27017}
	for _, p := range sensitive {
		if _, ok := blocked[p]; !ok {
			t.Errorf("expected port %d to be blocked", p)
		}
	}
}

// TestRewriteQueryScheme covers the OAuth redirect_uri rewriting logic.
//
// Background: the proxy forwards requests to backends over plain HTTP while
// preserving the original Host header (e.g. "9090.greennote.app"). Backends
// such as Keycloak derive their own base URL from that Host header, so their
// valid redirect URI list becomes "http://9090.greennote.app/…". The browser
// constructs redirect_uri with the https scheme (matching the public URL),
// which the backend then rejects. The proxy must downgrade the scheme before
// forwarding.
//
// Three alternative rewrite strategies were evaluated during development:
//   - Scheme-only (current):  https://publicHost/p → http://publicHost/p
//     Works when the backend uses the Host header to compute its own URL
//     (confirmed for Keycloak 25+).
//   - Localhost rewrite:      https://publicHost/p → http://localhost:PORT/p
//     Works when the backend has http://localhost:PORT/* in its static whitelist
//     (e.g. Keycloak configured with KC_HOSTNAME_URL=http://localhost:PORT).
//   - 127.0.0.1 rewrite:      https://publicHost/p → http://127.0.0.1:PORT/p
//     Works when the backend whitelists the loopback address explicitly.
//
// The scheme-only strategy is the most general because it matches whatever the
// backend derives from the Host header. The localhost/127.0.0.1 strategies
// require the backend to be statically configured with those addresses. If a
// backend needs one of the other strategies, this function should be extended
// with an additional rewrite step.
func TestRewriteQueryScheme(t *testing.T) {
	const pub = "9090.example.com"

	tests := []struct {
		name       string
		rawQuery   string
		publicHost string
		wantQuery  string // expected decoded redirect_uri value; "" means unchanged
		wantOK     bool
	}{
		// ── Strategy: scheme-only (https → http, keep publicHost) ───────────────
		{
			name:       "keycloak_style: https public host → http public host",
			rawQuery:   "client_id=admin&redirect_uri=" + url.QueryEscape("https://"+pub+"/admin/console/"),
			publicHost: pub,
			wantQuery:  "http://" + pub + "/admin/console/",
			wantOK:     true,
		},
		{
			name:       "keycloak_style: preserves path and query in redirect_uri",
			rawQuery:   "redirect_uri=" + url.QueryEscape("https://"+pub+"/cb?foo=bar"),
			publicHost: pub,
			wantQuery:  "http://" + pub + "/cb?foo=bar",
			wantOK:     true,
		},
		{
			name:       "keycloak_style: multiple params, only redirect_uri changed",
			rawQuery:   "state=abc&redirect_uri=" + url.QueryEscape("https://"+pub+"/cb") + "&scope=openid",
			publicHost: pub,
			wantQuery:  "http://" + pub + "/cb",
			wantOK:     true,
		},

		// ── Cases that must pass through unchanged ───────────────────────────────
		{
			name:       "already http public host: no change needed",
			rawQuery:   "redirect_uri=" + url.QueryEscape("http://"+pub+"/cb"),
			publicHost: pub,
			wantOK:     false,
		},
		{
			name:       "localhost strategy: http://localhost:PORT unchanged (different host)",
			rawQuery:   "redirect_uri=" + url.QueryEscape("http://localhost:9090/cb"),
			publicHost: pub,
			wantOK:     false,
		},
		{
			name:       "127.0.0.1 strategy: http://127.0.0.1:PORT unchanged (different host)",
			rawQuery:   "redirect_uri=" + url.QueryEscape("http://127.0.0.1:9090/cb"),
			publicHost: pub,
			wantOK:     false,
		},
		{
			name:       "different host https: not rewritten",
			rawQuery:   "redirect_uri=" + url.QueryEscape("https://other.example.com/cb"),
			publicHost: pub,
			wantOK:     false,
		},
		{
			name:       "non-URL value: not rewritten",
			rawQuery:   "state=someopaquevalue",
			publicHost: pub,
			wantOK:     false,
		},
		{
			name:       "empty query: not rewritten",
			rawQuery:   "",
			publicHost: pub,
			wantOK:     false,
		},
		{
			name:       "empty publicHost: not rewritten",
			rawQuery:   "redirect_uri=" + url.QueryEscape("https://"+pub+"/cb"),
			publicHost: "",
			wantOK:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := rewriteQueryScheme(tc.rawQuery, tc.publicHost)
			if ok != tc.wantOK {
				t.Fatalf("rewriteQueryScheme ok=%v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				if got != tc.rawQuery {
					t.Errorf("query changed unexpectedly:\n got %q\nwant %q", got, tc.rawQuery)
				}
				return
			}
			// Decode and check the redirect_uri value specifically.
			parsed, err := url.ParseQuery(got)
			if err != nil {
				t.Fatalf("could not parse rewritten query %q: %v", got, err)
			}
			gotURI := parsed.Get("redirect_uri")
			if gotURI != tc.wantQuery {
				t.Errorf("redirect_uri:\n got %q\nwant %q", gotURI, tc.wantQuery)
			}
		})
	}
}
