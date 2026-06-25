package dns

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── Namecheap ─────────────────────────────────────────────────────────────────

func TestParseNamecheapResponseOK(t *testing.T) {
	body := `<?xml version="1.0"?>
<ApiResponse Status="OK" xmlns="https://api.namecheap.com/xml.response">
  <Errors/>
  <RequestedCommand>namecheap.domains.dns.setHosts</RequestedCommand>
  <CommandResponse Type="namecheap.domains.dns.setHosts">
    <DomainDNSSetHostsResult Domain="example.com" IsSuccess="true"/>
  </CommandResponse>
</ApiResponse>`

	if err := parseNamecheapResponse(strings.NewReader(body)); err != nil {
		t.Errorf("expected no error for OK response, got: %v", err)
	}
}

func TestParseNamecheapResponseError(t *testing.T) {
	body := `<?xml version="1.0"?>
<ApiResponse Status="ERROR" xmlns="https://api.namecheap.com/xml.response">
  <Errors>
    <Error Number="2030166">Parameter ApiKey is invalid</Error>
  </Errors>
</ApiResponse>`

	err := parseNamecheapResponse(strings.NewReader(body))
	if err == nil {
		t.Fatal("expected error for ERROR response, got nil")
	}
	if !strings.Contains(err.Error(), "2030166") {
		t.Errorf("expected error to contain error number, got: %v", err)
	}
}

func TestSplitDomain(t *testing.T) {
	tests := []struct {
		input   string
		sld     string
		tld     string
		wantErr bool
	}{
		{"example.com", "example", "com", false},
		{"greennote.app", "greennote", "app", false},
		{"nodot", "", "", true},
		{"", "", "", true},
		{".com", "", "", true},
	}
	for _, tc := range tests {
		sld, tld, err := splitDomain(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("splitDomain(%q): wantErr=%v, got err=%v", tc.input, tc.wantErr, err)
			continue
		}
		if !tc.wantErr && (sld != tc.sld || tld != tc.tld) {
			t.Errorf("splitDomain(%q): want (%q, %q), got (%q, %q)", tc.input, tc.sld, tc.tld, sld, tld)
		}
	}
}

// ── GoDaddy ───────────────────────────────────────────────────────────────────

func TestGoDaddyPutCNAMERequest(t *testing.T) {
	var gotMethod, gotURL, gotAuth, gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotURL = r.URL.String()
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Swap the real GoDaddy base URL with the test server
	origURL := godaddyBaseURL
	godaddyBaseURL = srv.URL
	defer func() { godaddyBaseURL = origURL }()

	p := &GoDaddyProvider{}
	err := p.putCNAME("example.com", "abc123.cfargotunnel.com", "testkey", "testsecret")
	if err != nil {
		t.Fatalf("putCNAME: unexpected error: %v", err)
	}

	if gotMethod != http.MethodPut {
		t.Errorf("expected PUT, got %s", gotMethod)
	}
	if !strings.Contains(gotURL, "example.com") {
		t.Errorf("expected URL to contain domain, got %s", gotURL)
	}
	if gotAuth != "sso-key testkey:testsecret" {
		t.Errorf("unexpected auth header: %s", gotAuth)
	}
	if !strings.Contains(gotBody, "abc123.cfargotunnel.com") {
		t.Errorf("expected body to contain target, got %s", gotBody)
	}
}

// ── detectPublicIP ────────────────────────────────────────────────────────────

func TestDetectPublicIP(t *testing.T) {
	ip, err := detectPublicIP()
	if err != nil {
		t.Skipf("could not detect public IP (may be offline): %v", err)
	}
	if ip == "" {
		t.Error("expected non-empty IP")
	}
	// Should be a valid IP
	if net := strings.Count(ip, "."); net != 3 {
		// Could also be IPv6; just check it's non-empty
		if strings.Count(ip, ":") < 2 {
			t.Errorf("unexpected IP format: %q", ip)
		}
	}
}
