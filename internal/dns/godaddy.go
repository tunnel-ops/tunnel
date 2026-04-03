package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bellamy/requests/internal/keychain"
)

const (
	keychainAccountGoDaddyKey    = "godaddy-key"
	keychainAccountGoDaddySecret = "godaddy-secret"
)

// godaddyBaseURL is the GoDaddy API base. Overridden in tests.
var godaddyBaseURL = "https://api.godaddy.com"

// GoDaddyProvider creates a wildcard CNAME via the GoDaddy REST API.
// API keys can be obtained at developer.godaddy.com/keys.
type GoDaddyProvider struct{}

func (p *GoDaddyProvider) Name() string { return "GoDaddy" }

func (p *GoDaddyProvider) SetupWildcard(domain, target string) error {
	key, secret, err := p.loadCredentials()
	if err != nil {
		return err
	}
	defer zero([]byte(key))
	defer zero([]byte(secret))

	return p.putCNAME(domain, target, key, secret)
}

func (p *GoDaddyProvider) loadCredentials() (key, secret string, err error) {
	key, err = keychain.Load(keychainAccountGoDaddyKey)
	if err != nil {
		return "", "", fmt.Errorf("GoDaddy API key not found in keychain — run 'tunnel setup' first")
	}
	secret, err = keychain.Load(keychainAccountGoDaddySecret)
	if err != nil {
		return "", "", fmt.Errorf("GoDaddy API secret not found in keychain — run 'tunnel setup' first")
	}
	return key, secret, nil
}

func (p *GoDaddyProvider) putCNAME(domain, target, key, secret string) error {
	type record struct {
		Data string `json:"data"`
		TTL  int    `json:"ttl"`
	}

	body, err := json.Marshal([]record{{Data: target, TTL: 600}})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/v1/domains/%s/records/CNAME/%%2A", godaddyBaseURL, domain)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", key, secret))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GoDaddy API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GoDaddy API returned %d — check your API key and that the domain is in your account", resp.StatusCode)
	}
	return nil
}

// SaveCredentials stores the GoDaddy API key and secret in the OS keychain.
func SaveGoDaddyCredentials(key, secret string) error {
	if err := keychain.Save(keychainAccountGoDaddyKey, key); err != nil {
		return err
	}
	return keychain.Save(keychainAccountGoDaddySecret, secret)
}

// HasGoDaddyCredentials reports whether credentials are already stored.
func HasGoDaddyCredentials() bool {
	_, err1 := keychain.Load(keychainAccountGoDaddyKey)
	_, err2 := keychain.Load(keychainAccountGoDaddySecret)
	return err1 == nil && err2 == nil
}

// zero overwrites b with zeros to clear secrets from memory.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
