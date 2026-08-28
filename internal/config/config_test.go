package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestEnvVarOverrides(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("CUE_SERVER_TOKEN", "env-token")
	t.Setenv("CUE_UI_AUTOPLAY", "false")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Token != "env-token" || cfg.UI.Autoplay {
		t.Fatalf("nested environment overrides not applied: token=%q autoplay=%v", cfg.Server.Token, cfg.UI.Autoplay)
	}
}

func TestSaveAndClearUseLoadedConfigFile(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	work := t.TempDir()
	t.Chdir(work)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)
	localConfig := filepath.Join(work, "config.yaml")
	if err := os.WriteFile(localConfig, []byte("server:\n  url: http://example\n  token: old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Server.Token = "new-token"
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(localConfig)
	if !strings.Contains(string(data), "new-token") {
		t.Fatalf("loaded config was not updated: %s", data)
	}
	if err := ClearServerConfig(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(localConfig)
	if strings.Contains(string(data), "new-token") {
		t.Fatalf("credentials remained in loaded config: %s", data)
	}
}

// TestLoadConfigGeneratesDeviceID verifies that an already-configured install
// without a device_id (pre-existing config.yaml) gets a unique ID generated
// and persisted, so the ID stays stable across runs.
func TestLoadConfigGeneratesDeviceID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home) // windows

	configDir := filepath.Join(home, ".config", "cue")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	existing := `server:
  type: "jellyfin"
  url: "http://localhost:8096"
  token: "abc123"
  user_id: "user1"
`
	configFile := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Server.DeviceID == "" {
		t.Fatal("expected device ID to be generated")
	}
	if cfg.Server.DeviceID == "cue-tui-client" {
		t.Fatal("device ID should be unique, not the legacy shared ID")
	}
	if !strings.HasPrefix(cfg.Server.DeviceID, "cue-") {
		t.Fatalf("unexpected device ID format: %q", cfg.Server.DeviceID)
	}

	// Must be persisted so the same ID is used on the next run
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), cfg.Server.DeviceID) {
		t.Fatalf("device ID %q not persisted to config file:\n%s", cfg.Server.DeviceID, data)
	}

	// Existing credentials must survive the rewrite
	if !strings.Contains(string(data), "abc123") {
		t.Fatalf("token lost when persisting device ID:\n%s", data)
	}

	// The config file contains the token: it must not be world-readable,
	// including pre-existing files created with the old 0644 default
	info, err := os.Stat(configFile)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config file mode = %o, want 600", perm)
	}
}

func TestDefaultConfigEnablesPlayNextOnSelect(t *testing.T) {
	if !DefaultConfig().UI.PlayNextOnSelect {
		t.Fatal("PlayNextOnSelect should be enabled by default")
	}
}
