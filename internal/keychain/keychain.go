// Package keychain provides secure credential storage.
// On macOS it delegates to the system Keychain via the security(1) CLI.
// On Linux it falls back to a credentials file with 0600 permissions and
// prints a warning so the user knows it is less secure.
package keychain

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrNotFound is returned by Load when no credential exists for the given key.
var ErrNotFound = errors.New("credential not found")

const service = "requests-tunnel"

// Save stores secret under service/account in the OS keychain.
// On macOS the secret is passed to security(1) via its -w flag.
// On Linux it is written to ~/.config/requests/credentials with 0600 perms.
func Save(account, secret string) error {
	if runtime.GOOS == "darwin" {
		return saveMac(account, secret)
	}
	return saveFile(account, secret)
}

// Load retrieves the secret for account. Returns ErrNotFound if absent.
func Load(account string) (string, error) {
	if runtime.GOOS == "darwin" {
		return loadMac(account)
	}
	return loadFile(account)
}

// Delete removes the stored credential for account.
func Delete(account string) error {
	if runtime.GOOS == "darwin" {
		return deleteMac(account)
	}
	return deleteFile(account)
}

// ── macOS implementation ──────────────────────────────────────────────────────

func saveMac(account, secret string) error {
	cmd := exec.Command("security", "add-generic-password",
		"-U",          // update if already exists
		"-s", service,
		"-a", account,
		"-w", secret,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("keychain save failed: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func loadMac(account string) (string, error) {
	cmd := exec.Command("security", "find-generic-password",
		"-s", service,
		"-a", account,
		"-w", // print only the password
	)
	out, err := cmd.Output()
	if err != nil {
		// Exit code 44 = item not found
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 44 {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("keychain load failed: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func deleteMac(account string) error {
	cmd := exec.Command("security", "delete-generic-password",
		"-s", service,
		"-a", account,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 44 {
			return nil // already gone
		}
		return fmt.Errorf("keychain delete failed: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ── Linux file-based fallback ─────────────────────────────────────────────────

func credPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "requests", "credentials"), nil
}

func saveFile(account, secret string) error {
	fmt.Fprintln(os.Stderr, "warning: storing credentials in ~/.config/requests/credentials (no system keychain available)")

	path, err := credPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	existing, _ := loadAllFile()

	existing[account] = secret
	return writeAllFile(existing, path)
}

func loadFile(account string) (string, error) {
	m, err := loadAllFile()
	if err != nil {
		return "", ErrNotFound
	}
	v, ok := m[account]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func deleteFile(account string) error {
	m, _ := loadAllFile()
	delete(m, account)
	path, err := credPath()
	if err != nil {
		return err
	}
	return writeAllFile(m, path)
}

func loadAllFile() (map[string]string, error) {
	path, err := credPath()
	if err != nil {
		return map[string]string{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return map[string]string{}, err
	}

	m := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m, nil
}

func writeAllFile(m map[string]string, path string) error {
	var sb strings.Builder
	for k, v := range m {
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(v)
		sb.WriteByte('\n')
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(sb.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
