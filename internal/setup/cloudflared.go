package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var tunnelIDRe = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// IsCloudflaredInstalled reports whether the cloudflared binary is on PATH.
func IsCloudflaredInstalled() bool {
	_, err := exec.LookPath("cloudflared")
	return err == nil
}

// IsAuthenticated reports whether a Cloudflare cert already exists locally.
func IsAuthenticated() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".cloudflared", "cert.pem"))
	return err == nil
}

// Login runs `cloudflared tunnel login`, inheriting the terminal so the user
// can authenticate in their browser.
func Login() error {
	cmd := exec.Command("cloudflared", "tunnel", "login")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CreateTunnel creates a named cloudflared tunnel and returns its UUID.
// It parses the UUID from the output line:
//
//	Created tunnel <name> with id <uuid>
func CreateTunnel(name string) (string, error) {
	out, err := exec.Command("cloudflared", "tunnel", "create", name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cloudflared tunnel create failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	id := tunnelIDRe.FindString(string(out))
	if id == "" {
		return "", fmt.Errorf("could not parse tunnel ID from output:\n%s", string(out))
	}
	return id, nil
}

// ListTunnelNames returns the names of all active (non-deleted) cloudflare tunnels.
func ListTunnelNames() (map[string]bool, error) {
	out, err := exec.Command("cloudflared", "tunnel", "list", "--output", "json").Output()
	if err != nil {
		return nil, fmt.Errorf("cloudflared tunnel list failed: %w", err)
	}
	var entries []struct {
		Name      string  `json:"name"`
		DeletedAt *string `json:"deleted_at"`
	}
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("could not parse tunnel list: %w", err)
	}
	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.DeletedAt == nil {
			names[e.Name] = true
		}
	}
	return names, nil
}

// TunnelExists reports whether a tunnel with the given name already exists.
func TunnelExists(name string) (id string, exists bool) {
	out, err := exec.Command("cloudflared", "tunnel", "list", "--output", "json").Output()
	if err != nil {
		return "", false
	}
	// Quick scan: look for the name in the JSON blob
	if !strings.Contains(string(out), `"`+name+`"`) {
		return "", false
	}
	// Extract tunnel ID for the given name by scanning lines
	id = tunnelIDRe.FindString(string(out))
	return id, id != ""
}

// IsCloudflaredRunning reports whether the cloudflared process is currently running.
func IsCloudflaredRunning() bool {
	out, err := exec.Command("pgrep", "cloudflared").Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

// StartCloudflaredService starts the cloudflared user LaunchAgent.
func StartCloudflaredService() error {
	return exec.Command("launchctl", "start", "com.cloudflare.cloudflared").Run()
}

// StopCloudflaredService stops the cloudflared launchd service.
func StopCloudflaredService() error {
	return exec.Command("launchctl", "stop", "com.cloudflare.cloudflared").Run()
}

// WriteCloudflaredConfig writes the cloudflared config file to
// ~/.cloudflared/config.yml with the supplied values.
func WriteCloudflaredConfig(tunnelID, tunnelName, domain string, proxyPort int) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dir := filepath.Join(home, ".cloudflared")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	content := fmt.Sprintf(`tunnel: %s
credentials-file: %s/%s.json

# QUIC is lower-latency than the default HTTP/2 transport.
protocol: quic

ingress:
  - hostname: "*.%s"
    service: http://localhost:%d
    originRequest:
      connectTimeout: 10s
      noTLSVerify: false
  - service: http_status:404
`,
		tunnelID,
		dir, tunnelID,
		domain,
		proxyPort,
	)

	path := filepath.Join(dir, "config.yml")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
