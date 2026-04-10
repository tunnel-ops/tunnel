package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/tunnel-ops/tunnel/internal/appconfig"
)

const githubRepo = "tunnel-ops/tunnel"

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var r [3]int
	for i, p := range parts {
		if i < 3 {
			r[i], _ = strconv.Atoi(p)
		}
	}
	return r
}

// isNewerVersion returns true only when latest is strictly newer than current
// and current is not a dev build.
func isNewerVersion(current, latest string) bool {
	if current == "dev" || latest == "" || latest == current {
		return false
	}
	a, b := parseSemver(latest), parseSemver(current)
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/" + githubRepo + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	return rel.TagName, nil
}

// applyUpdate downloads version and atomically replaces the running binary.
// Returns an error if the install directory is not writable.
func applyUpdate(version string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not resolve binary path: %w", err)
	}

	goos := runtime.GOOS
	goarch := runtime.GOARCH
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/tunnel_%s_%s",
		githubRepo, version, goos, goarch)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d for %s", resp.StatusCode, url)
	}

	tmp, err := os.CreateTemp(filepath.Dir(exe), ".tunnel-update-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s — try with sudo if it is a system directory: %w", filepath.Dir(exe), err)
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write failed: %w", err)
	}
	tmp.Close()

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, exe); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace failed in %s — try with sudo if it is a system directory: %w", filepath.Dir(exe), err)
	}
	return nil
}

func cmdUpdate() {
	hr := dimStyle.Render(strings.Repeat("─", 55))
	fmt.Println()
	fmt.Printf("  %s\n", gradientTunnel()+boldStyle.Render("  update"))
	fmt.Printf("  %s\n", hr)
	fmt.Printf("  current: %s\n", dimStyle.Render(Version))

	latest, err := fetchLatestVersion()
	if err != nil {
		fmt.Printf("  %s\n", hr)
		fmt.Println()
		fmt.Fprintf(os.Stderr, "  error: could not check for updates: %v\n\n", err)
		os.Exit(1)
	}

	if !isNewerVersion(Version, latest) {
		fmt.Printf("  %s  already up to date\n", doneStyle.Render("✓"))
		fmt.Printf("  %s\n", hr)
		fmt.Println()
		return
	}

	fmt.Printf("  latest:  %s\n", liveStyle.Render(latest))
	fmt.Printf("  %s\n", hr)
	fmt.Println()
	fmt.Printf("  downloading %s...\n\n", latest)

	if err := applyUpdate(latest); err != nil {
		fmt.Fprintf(os.Stderr, "  error: %v\n\n", err)
		os.Exit(1)
	}

	fmt.Printf("  %s  updated to %s — restart to use new version\n\n",
		doneStyle.Render("✓"), liveStyle.Render(latest))
}

func cmdUpdateToggle(enable bool) {
	cfg, err := appconfig.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	cfg.AutoUpdate = enable
	if err := appconfig.Save(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	hr := dimStyle.Render(strings.Repeat("─", 55))
	fmt.Println()
	fmt.Printf("  %s\n", gradientTunnel()+boldStyle.Render("  update"))
	fmt.Printf("  %s\n", hr)
	if enable {
		fmt.Printf("  %s  auto-updates enabled\n", doneStyle.Render("✓"))
	} else {
		fmt.Printf("  %s  auto-updates disabled\n", doneStyle.Render("✓"))
	}
	fmt.Printf("  %s\n", hr)
	fmt.Println()
}

// maybeAutoUpdate checks for a newer release at most once per day and, when
// AutoUpdate is enabled, downloads and replaces the running binary silently.
func maybeAutoUpdate() {
	cfg, err := appconfig.Load()
	if err != nil || cfg == nil || !cfg.AutoUpdate {
		return
	}

	if cfg.LastUpdateCheck != "" {
		last, err := time.Parse(time.RFC3339, cfg.LastUpdateCheck)
		if err == nil && time.Since(last) < 24*time.Hour {
			return
		}
	}

	latest, err := fetchLatestVersion()
	cfg.LastUpdateCheck = time.Now().UTC().Format(time.RFC3339)
	_ = appconfig.Save(cfg)

	if err != nil || !isNewerVersion(Version, latest) {
		return
	}

	fmt.Printf("  %s  update available: %s → %s  downloading...\n",
		warnStyle.Render("↑"), dimStyle.Render(Version), liveStyle.Render(latest))

	if err := applyUpdate(latest); err != nil {
		return // silent on failure — user can run 'tunnel update' manually
	}

	fmt.Printf("  %s  updated to %s — changes take effect next run\n\n",
		doneStyle.Render("✓"), liveStyle.Render(latest))
}
