package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/viper"
)

// SourceType identifies the media server backend
type SourceType string

const (
	SourceTypePlex     SourceType = "plex"
	SourceTypeJellyfin SourceType = "jellyfin"
)

// Config holds all application configuration
type Config struct {
	Server         ServerConfig             `mapstructure:"server"`
	Player         PlayerConfig             `mapstructure:"player"`
	UI             UIConfig                 `mapstructure:"ui"`
	Logging        LoggingConfig            `mapstructure:"logging"`
	CurrentProfile string                   `mapstructure:"current_profile"`
	Profiles       map[string]ProfileConfig `mapstructure:"profiles"`
}

// ServerConfig holds media server configuration
type ServerConfig struct {
	Type     SourceType `mapstructure:"type"     yaml:"type"`       // "plex" or "jellyfin"
	URL      string     `mapstructure:"url"      yaml:"url"`        // Server URL
	Token    string     `mapstructure:"token"    yaml:"token"`      // Plex token OR Jellyfin API key
	UserID   string     `mapstructure:"user_id"  yaml:"user_id"`    // Jellyfin only
	Username string     `mapstructure:"username" yaml:"username"`   // Jellyfin only (display)
	DeviceID string     `mapstructure:"device_id" yaml:"device_id"` // Unique per-install device identifier
}

// PlayerConfig holds media player configuration
type PlayerConfig struct {
	Command   string   `mapstructure:"command"`
	Args      []string `mapstructure:"args"`
	StartFlag string   `mapstructure:"start_flag"` // e.g., "--start=%d" or "--start-time=%d"
}

// UIConfig holds UI configuration
type UIConfig struct {
	ShowWatchStatus   bool `mapstructure:"show_watch_status"`   // Show watched/unwatched/in-progress indicators
	ShowLibraryCounts bool `mapstructure:"show_library_counts"` // Keep library item counts visible after sync
	HideWatched       bool `mapstructure:"hide_watched"`        // Hide items that are already watched
	Autoplay          bool `mapstructure:"autoplay"`            // Automatically play the next episode
	PlayNextOnSelect  bool `mapstructure:"play_next_on_select"` // Play the next episode when selecting a show
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	File  string `mapstructure:"file"`
	Level string `mapstructure:"level"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		CurrentProfile: "default",
		Profiles:       make(map[string]ProfileConfig),
		UI: UIConfig{
			ShowWatchStatus:   true,
			ShowLibraryCounts: false,
			HideWatched:       false,
			Autoplay:          true,
			PlayNextOnSelect:  true,
		},
		Logging: LoggingConfig{
			File:  defaultLogPath(),
			Level: "INFO",
		},
	}
}

// defaultLogPath returns the default log file path for the current OS
func defaultLogPath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "cue", "cue.log")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "cue", "cue.log")
	}
}

// defaultConfigPath returns the default config file path for the current OS
func defaultConfigPath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "cue")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "cue")
	}
}

// LoadConfig loads configuration from file and environment
func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(defaultConfigPath())
	viper.AddConfigPath(".")
	// The config file contains the server token: never world-readable
	viper.SetConfigPermissions(0o600)

	// Environment variable overrides. The replacer maps nested keys such as
	// server.token to CUE_SERVER_TOKEN, while explicit bindings ensure
	// env-only values are visible to Unmarshal.
	viper.SetEnvPrefix("CUE")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	for _, key := range []string{
		"server.type", "server.url", "server.token", "server.user_id",
		"server.username", "server.device_id",
		"player.command", "player.args", "player.start_flag",
		"ui.show_watch_status", "ui.show_library_counts", "ui.hide_watched", "ui.autoplay",
		"logging.file", "logging.level", "current_profile",
	} {
		_ = viper.BindEnv(key)
	}

	// Read config file if it exists
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found is OK, use defaults
	}

	// Tighten pre-existing config files created with the old 0644 default
	if configFile := viper.ConfigFileUsed(); configFile != "" {
		_ = os.Chmod(configFile, 0o600)
	}

	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error parsing config: %w", err)
	}
	cfg.applyCurrentProfile()

	// Ensure this install has a stable, unique device ID. Media servers
	// (Jellyfin in particular) revoke tokens when another login reuses the
	// same device ID, so a shared/static ID causes intermittent auth failures.
	if cfg.Server.DeviceID == "" {
		cfg.Server.DeviceID = generateDeviceID()
		// Persist immediately for already-configured installs so the ID
		// stays stable across runs. Fresh installs save during setup.
		if configFile := viper.ConfigFileUsed(); configFile != "" {
			viper.Set("server.device_id", cfg.Server.DeviceID)
			if err := viper.WriteConfigAs(configFile); err != nil {
				return nil, fmt.Errorf("failed to save device ID: %w", err)
			}
		}
	}

	return cfg, nil
}

// generateDeviceID returns a random unique identifier for this install
func generateDeviceID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		// Fall back to the legacy static ID; auth still works, just without
		// per-install uniqueness
		return "cue-tui-client"
	}
	return "cue-" + hex.EncodeToString(buf)
}

// SaveConfig writes back to the loaded file, or the default path for a fresh install.
func SaveConfig(cfg *Config) error {
	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		configPath := defaultConfigPath()
		if err := os.MkdirAll(configPath, 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
		configFile = filepath.Join(configPath, "config.yaml")
	}

	viper.SetConfigPermissions(0o600)

	// Set server fields individually to ensure correct key names (snake_case)
	viper.Set("server.type", cfg.Server.Type)
	viper.Set("server.url", cfg.Server.URL)
	viper.Set("server.token", cfg.Server.Token)
	viper.Set("server.user_id", cfg.Server.UserID)
	viper.Set("server.username", cfg.Server.Username)
	viper.Set("server.device_id", cfg.Server.DeviceID)

	// Set player fields
	viper.Set("player.command", cfg.Player.Command)
	viper.Set("player.args", cfg.Player.Args)
	viper.Set("player.start_flag", cfg.Player.StartFlag)

	// Set UI fields
	viper.Set("ui.show_watch_status", cfg.UI.ShowWatchStatus)
	viper.Set("ui.show_library_counts", cfg.UI.ShowLibraryCounts)
	viper.Set("ui.hide_watched", cfg.UI.HideWatched)
	viper.Set("ui.autoplay", cfg.UI.Autoplay)
	viper.Set("ui.play_next_on_select", cfg.UI.PlayNextOnSelect)

	// Set logging fields
	viper.Set("logging.file", cfg.Logging.File)
	viper.Set("logging.level", cfg.Logging.Level)

	cfg.rememberCurrentProfile()
	viper.Set("current_profile", cfg.CurrentProfile)
	viper.Set("profiles", cfg.Profiles)

	if err := viper.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func (c *Config) applyCurrentProfile() {
	if c.Profiles == nil {
		c.Profiles = make(map[string]ProfileConfig)
	}
	if c.CurrentProfile == "" {
		c.CurrentProfile = "default"
	}
	profile, ok := c.Profiles[c.CurrentProfile]
	if ok && profile.Server.URL != "" {
		c.Server = profile.Server
	}
}

func (c *Config) rememberCurrentProfile() {
	if c.Profiles == nil {
		c.Profiles = make(map[string]ProfileConfig)
	}
	if c.CurrentProfile == "" {
		c.CurrentProfile = "default"
	}
	if c.Server.URL != "" || c.Server.Token != "" {
		c.Profiles[c.CurrentProfile] = ProfileConfig{Server: c.Server}
	}
}

// IsConfigured returns true if the server URL and token are set
func (c *Config) IsConfigured() bool {
	return c.Server.URL != "" && c.Server.Token != ""
}

// DefaultCachePath returns the default cache directory path for the current OS
func DefaultCachePath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "cue", "cache")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "cue", "cache")
	}
}

// ClearServerConfig removes all server-related configuration (type, URL, credentials)
// while preserving other settings (player, UI, logging)
func ClearServerConfig() error {
	// Remove the active profile's credential copy as well, otherwise the next
	// load would immediately restore the token we just cleared.
	var current Config
	if err := viper.Unmarshal(&current); err == nil && current.Profiles != nil {
		delete(current.Profiles, current.CurrentProfile)
		viper.Set("profiles", current.Profiles)
	}

	// Clear server fields in viper
	viper.Set("server.type", "")
	viper.Set("server.url", "")
	viper.Set("server.token", "")
	viper.Set("server.user_id", "")
	viper.Set("server.username", "")

	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		configPath := defaultConfigPath()
		if err := os.MkdirAll(configPath, 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
		configFile = filepath.Join(configPath, "config.yaml")
	}

	viper.SetConfigPermissions(0o600)
	if err := viper.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ClearCache removes all cached data
func ClearCache() error {
	cachePath := DefaultCachePath()
	if err := os.RemoveAll(cachePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear cache: %w", err)
	}
	return nil
}
