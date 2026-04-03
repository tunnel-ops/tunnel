package dns

import (
	"fmt"
	"os/exec"
	"strings"
)

// CloudflareProvider uses the cloudflared CLI to register the wildcard DNS
// route. It requires the user to have already authenticated via
// `cloudflared tunnel login`.
type CloudflareProvider struct {
	// TunnelName is the named tunnel (e.g. "dev") used in
	// `cloudflared tunnel route dns <name> *.domain`.
	TunnelName string
}

func (p *CloudflareProvider) Name() string { return "Cloudflare" }

func (p *CloudflareProvider) SetupWildcard(domain, _ string) error {
	hostname := "*." + domain
	cmd := exec.Command("cloudflared", "tunnel", "route", "dns", p.TunnelName, hostname)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cloudflared route dns failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
