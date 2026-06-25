package proxy

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tunnel-ops/tunnel/internal/appconfig"
	"github.com/tunnel-ops/tunnel/internal/names"
)

// DefaultBlockedPorts returns the default set of ports that the proxy refuses
// to forward to, regardless of subdomain.
func DefaultBlockedPorts() map[int]struct{} {
	sensitive := []int{
		22,    // SSH
		25,    // SMTP
		110,   // POP3
		143,   // IMAP
		445,   // SMB
		1433,  // MSSQL
		3306,  // MySQL
		5432,  // PostgreSQL
		6379,  // Redis
		27017, // MongoDB
		27018,
		27019,
	}
	m := make(map[int]struct{}, len(sensitive))
	for _, p := range sensitive {
		m[p] = struct{}{}
	}
	return m
}

// LoadConfig builds a Config from environment variables, falling back to
// appconfig for DOMAIN and PROXY_PORT when env vars are not set.
func LoadConfig(store *names.Store) (Config, error) {
	domain := os.Getenv("DOMAIN")
	proxyPortStr := os.Getenv("PROXY_PORT")

	if domain == "" || proxyPortStr == "" {
		if cfg, err := appconfig.Load(); err == nil {
			if domain == "" && cfg.Domain != "" {
				domain = cfg.Domain
			}
			if proxyPortStr == "" && cfg.ProxyPort != 0 {
				proxyPortStr = strconv.Itoa(cfg.ProxyPort)
			}
		}
	}

	if domain == "" {
		return Config{}, &MissingEnvError{"DOMAIN"}
	}

	if proxyPortStr == "" {
		proxyPortStr = "7999"
	}
	proxyPort := proxyPortStr

	blocked := DefaultBlockedPorts()
	if raw := os.Getenv("BLOCKED_PORTS"); raw != "" {
		blocked = parseBlockedPorts(raw)
	}

	maxBodyMB := int64(10)
	if raw := os.Getenv("MAX_BODY_MB"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			maxBodyMB = n
		}
	}

	userBlocked, _ := names.NewBlocked("")

	return Config{
		Domain:       domain,
		ProxyPort:    proxyPort,
		HealthPort:   envOr("HEALTH_PORT", "7998"),
		BlockedPorts: blocked,
		MaxBodyBytes: maxBodyMB * 1024 * 1024,
		ReadTimeout:  parseDuration(os.Getenv("READ_TIMEOUT"), 30*time.Second),
		WriteTimeout: parseDuration(os.Getenv("WRITE_TIMEOUT"), 30*time.Second),
		IdleTimeout:  parseDuration(os.Getenv("IDLE_TIMEOUT"), 120*time.Second),
		Names:        store,
		Blocked:      userBlocked,
	}, nil
}

// MissingEnvError is returned when a required env var is absent.
type MissingEnvError struct{ Name string }

func (e *MissingEnvError) Error() string {
	return "required environment variable " + e.Name + " is not set"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseBlockedPorts(raw string) map[int]struct{} {
	m := make(map[int]struct{})
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if n, err := strconv.Atoi(s); err == nil {
			m[n] = struct{}{}
		}
	}
	return m
}

func parseDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}
