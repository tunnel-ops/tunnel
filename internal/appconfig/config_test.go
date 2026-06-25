package appconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tunnel-ops/tunnel/internal/appconfig"
)

func TestAutoUpdateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &appconfig.Config{
		Domain:          "example.com",
		AutoUpdate:      true,
		LastUpdateCheck: "2026-04-09T00:00:00Z",
	}

	data, err := appconfig.MarshalConfig(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	loaded, err := appconfig.LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !loaded.AutoUpdate {
		t.Error("expected AutoUpdate=true")
	}
	if loaded.LastUpdateCheck != "2026-04-09T00:00:00Z" {
		t.Errorf("unexpected LastUpdateCheck: %q", loaded.LastUpdateCheck)
	}
}
