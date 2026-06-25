package dns

import (
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tunnel-ops/tunnel/internal/keychain"
)

const (
	keychainAccountNamecheapUser   = "namecheap-user"
	keychainAccountNamecheapAPIKey = "namecheap-apikey"
)

// NamecheapProvider creates a wildcard CNAME via the Namecheap XML API.
// Requires API access enabled in your Namecheap account settings and your
// public IP whitelisted.
type NamecheapProvider struct{}

func (p *NamecheapProvider) Name() string { return "Namecheap" }

func (p *NamecheapProvider) SetupWildcard(domain, target string) error {
	username, apiKey, err := p.loadCredentials()
	if err != nil {
		return err
	}
	defer zero([]byte(username))
	defer zero([]byte(apiKey))

	publicIP, err := detectPublicIP()
	if err != nil {
		return fmt.Errorf("could not detect public IP (required by Namecheap API): %w", err)
	}

	sld, tld, err := splitDomain(domain)
	if err != nil {
		return err
	}

	return p.setHosts(username, apiKey, publicIP, sld, tld, target)
}

func (p *NamecheapProvider) loadCredentials() (username, apiKey string, err error) {
	username, err = keychain.Load(keychainAccountNamecheapUser)
	if err != nil {
		return "", "", fmt.Errorf("Namecheap username not found in keychain — run 'tunnel setup' first")
	}
	apiKey, err = keychain.Load(keychainAccountNamecheapAPIKey)
	if err != nil {
		return "", "", fmt.Errorf("Namecheap API key not found in keychain — run 'tunnel setup' first")
	}
	return username, apiKey, nil
}

func (p *NamecheapProvider) setHosts(username, apiKey, clientIP, sld, tld, target string) error {
	params := url.Values{
		"ApiUser":    {username},
		"ApiKey":     {apiKey},
		"UserName":   {username},
		"ClientIp":   {clientIP},
		"Command":    {"namecheap.domains.dns.setHosts"},
		"SLD":        {sld},
		"TLD":        {tld},
		"HostName1":  {"*"},
		"RecordType1": {"CNAME"},
		"Address1":   {target},
		"TTL1":       {"600"},
	}

	apiURL := "https://api.namecheap.com/xml.response"
	req, err := http.NewRequest(http.MethodPost, apiURL, strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Namecheap API request failed: %w", err)
	}
	defer resp.Body.Close()

	return parseNamecheapResponse(resp.Body)
}

type ncResponse struct {
	XMLName xml.Name `xml:"ApiResponse"`
	Status  string   `xml:"Status,attr"`
	Errors  []struct {
		Message string `xml:",chardata"`
		Number  string `xml:"Number,attr"`
	} `xml:"Errors>Error"`
}

func parseNamecheapResponse(r io.Reader) error {
	var result ncResponse
	if err := xml.NewDecoder(r).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse Namecheap response: %w", err)
	}
	if result.Status != "OK" {
		if len(result.Errors) > 0 {
			return fmt.Errorf("Namecheap API error %s: %s",
				result.Errors[0].Number,
				strings.TrimSpace(result.Errors[0].Message))
		}
		return fmt.Errorf("Namecheap API returned status: %s", result.Status)
	}
	return nil
}

// detectPublicIP returns the machine's outbound public IP by opening a UDP
// connection to a well-known address. This does not send any data — it only
// resolves the local interface used for outbound traffic.
func detectPublicIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "", fmt.Errorf("unexpected address type")
	}
	return addr.IP.String(), nil
}

// splitDomain splits "example.com" → ("example", "com").
// Only single-part TLDs are supported (sufficient for GoDaddy/Namecheap).
func splitDomain(domain string) (sld, tld string, err error) {
	parts := strings.SplitN(domain, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid domain %q — expected format: example.com", domain)
	}
	return parts[0], parts[1], nil
}

// SaveNamecheapCredentials stores username and API key in the OS keychain.
func SaveNamecheapCredentials(username, apiKey string) error {
	if err := keychain.Save(keychainAccountNamecheapUser, username); err != nil {
		return err
	}
	return keychain.Save(keychainAccountNamecheapAPIKey, apiKey)
}

// HasNamecheapCredentials reports whether credentials are already stored.
func HasNamecheapCredentials() bool {
	_, err1 := keychain.Load(keychainAccountNamecheapUser)
	_, err2 := keychain.Load(keychainAccountNamecheapAPIKey)
	return err1 == nil && err2 == nil
}
