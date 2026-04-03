package appconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config holds persistent app settings. It never contains API secrets.
type Config struct {
	Domain     string `json:"domain"`
	Provider   string `json:"provider"`   // "cloudflare", "godaddy", "namecheap", "manual"
	TunnelID   string `json:"tunnelId"`
	TunnelName string `json:"tunnelName"`
	ProxyPort  int    `json:"proxyPort"`
}

func defaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "requests", "config.json"), nil
}

// Load reads the config file. Returns a zero-value Config (not an error) if the
// file does not exist yet.
func Load() (*Config, error) {
	path, err := defaultPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes cfg atomically with owner-only permissions (0600).
func Save(cfg *Config) error {
	path, err := defaultPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
