package dns

// Provider creates the wildcard DNS CNAME record that routes *.domain
// to the cloudflared tunnel endpoint.
type Provider interface {
	// Name returns the human-readable provider name.
	Name() string

	// SetupWildcard creates (or updates) a CNAME record:
	//   *.domain → target
	// where target is the tunnel's public hostname, e.g.
	// <tunnel-id>.cfargotunnel.com.
	SetupWildcard(domain, target string) error
}
