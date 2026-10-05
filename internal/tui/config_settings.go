package tui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/SuperCoolPencil/cue/internal/config"
	"github.com/SuperCoolPencil/cue/internal/domain"
)

// Explicit registration lets the schema coverage test catch missing controls.
// Existing controls retain their immediate UI updates and mode cycling.
type configSetting struct {
	path, id, label  string
	secret, existing bool
}

var configSettings = []configSetting{
	{"player.command", "__config_player__", "Player", false, false},
	{"ui.theme", "__config_theme__", "Theme", false, true},
	{"ui.show_watch_status", "__config_watch__", "Watch indicators", false, true},
	{"ui.show_library_counts", "__config_counts__", "Library counts", false, true},
	{"ui.hide_watched", "__config_hide_watched__", "Hide watched", false, true},
	{"ui.autoplay", "__config_autoplay__", "Autoplay", false, true},
	{"player.skip.intro", "__config_skip_intro__", "Skip intros", false, true},
	{"player.skip.outro", "__config_skip_outro__", "Skip outros", false, true},
	{"player.skip.key", "__config_skip_key__", "Skip key", false, false},
	{"player.skip.undo_key", "__config_skip_undo_key__", "Undo skip key", false, false},
	{"player.skip.analysis_at_startup", "__config_skip_analysis__", "Analyze intros at startup", false, false},
	{"player.skip.intro_window_seconds", "__config_skip_window__", "Intro scan seconds (0 = default)", false, false},
	{"player.skip.chapters_when_missing", "__config_skip_chapters__", "Add missing skip chapters", false, false},
	{"player.args", "__config_player_args__", "Player arguments (JSON array)", false, false},
	{"player.start_flag", "__config_player_start_flag__", "Player resume flag", false, false},
	{"server.type", "__config_server_type__", "Server type", false, false},
	{"server.url", "__config_server_url__", "Server URL", false, false},
	{"server.token", "__config_server_token__", "Server token", true, false},
	{"server.plex_account_token", "__config_account_token__", "Plex account token", true, false},
	{"server.user_id", "__config_server_user__", "Server user ID", false, false},
	{"server.username", "__config_server_username__", "Server username", false, false},
	{"server.device_id", "__config_server_device__", "Device ID", false, false},
	{"logging.file", "__config_log_file__", "Log file", false, false},
	{"logging.level", "__config_log_level__", "Log level", false, false},
	{"current_profile", "__config_current_profile__", "Current profile", false, false},
	{"profiles", profilesLibraryID, "Profiles", false, true},
}

func configField(cfg *config.Config, path string) reflect.Value {
	v := reflect.ValueOf(cfg).Elem()
	for _, part := range strings.Split(path, ".") {
		found := false
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).Tag.Get("mapstructure") == part {
				v = v.Field(i)
				found = true
				break
			}
		}
		if !found {
			return reflect.Value{}
		}
	}
	return v
}

func configValueText(v reflect.Value) string {
	if v.Kind() == reflect.Slice {
		if v.IsNil() {
			return "[]"
		}
		b, _ := json.Marshal(v.Interface())
		return string(b)
	}
	if v.Kind() == reflect.Bool {
		if v.Bool() {
			return "on"
		}
		return "off"
	}
	return fmt.Sprint(v.Interface())
}

func (m Model) settingsEntries() []domain.Library {
	cfg := m.AppConfig
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	var entries []domain.Library
	for _, setting := range configSettings {
		v := configField(cfg, setting.path)
		text := configValueText(v)
		if setting.secret {
			text = "not set"
			if v.String() != "" {
				text = "configured"
			}
		} else if setting.path == "profiles" {
			text = strconv.Itoa(v.Len())
		} else if text == "" {
			text = "not set"
			if setting.path == "player.command" {
				text = "auto"
			}
		}
		entries = append(entries, domain.Library{ID: setting.id, Name: setting.label + ": " + text, Type: "config"})
	}
	return entries
}

func (m *Model) openConfigSetting(id string) bool {
	for _, setting := range configSettings {
		if setting.id != id || setting.existing {
			continue
		}
		if m.AppConfig == nil {
			return true
		}
		v := configField(m.AppConfig, setting.path)
		if v.Kind() == reflect.Bool {
			if err := m.saveConfigInput(setting.path, strconv.FormatBool(!v.Bool())); err != nil {
				m.StatusMsg, m.StatusIsErr = err.Error(), true
			}
		} else {
			m.configInputPath = setting.path
			m.InputModal.ShowValue(setting.label, configValueText(v), setting.secret)
		}
		return true
	}
	return false
}

// setConfigInput returns a validated copy, leaving live configuration unchanged
// if parsing, validation, or the subsequent save fails.
func setConfigInput(cfg *config.Config, path, text string) (*config.Config, error) {
	next := *cfg
	next.Profiles = make(map[string]config.ProfileConfig, len(cfg.Profiles))
	for name, profile := range cfg.Profiles {
		next.Profiles[name] = profile
	}
	v := configField(&next, path)
	if !v.IsValid() {
		return nil, fmt.Errorf("Unknown setting: %s", path)
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(text)
	case reflect.Int:
		n, err := strconv.Atoi(text)
		if err != nil {
			return nil, fmt.Errorf("Enter a whole number")
		}
		v.SetInt(int64(n))
	case reflect.Bool:
		b, err := strconv.ParseBool(text)
		if err != nil {
			return nil, fmt.Errorf("Enter true or false")
		}
		v.SetBool(b)
	case reflect.Slice:
		var args []string
		if err := json.Unmarshal([]byte(text), &args); err != nil || strings.TrimSpace(text) == "null" {
			return nil, fmt.Errorf("Enter arguments as a JSON array of strings")
		}
		v.Set(reflect.ValueOf(args))
	default:
		return nil, fmt.Errorf("Use Profiles to manage server profiles")
	}
	if err := next.Player.Skip.Validate(); err != nil {
		return nil, err
	}
	switch path {
	case "server.type":
		if next.Server.Type != config.SourceTypePlex && next.Server.Type != config.SourceTypeJellyfin {
			return nil, fmt.Errorf("Server type must be plex or jellyfin")
		}
	case "server.url":
		u, err := url.Parse(text)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("Enter an HTTP or HTTPS server URL")
		}
	case "logging.level":
		next.Logging.Level = strings.ToUpper(text)
		switch next.Logging.Level {
		case "DEBUG", "INFO", "WARN", "WARNING", "ERROR":
		default:
			return nil, fmt.Errorf("Log level must be DEBUG, INFO, WARN, or ERROR")
		}
	case "current_profile":
		if text == "" {
			return nil, fmt.Errorf("Profile name cannot be empty")
		}
		next.ApplyProfileForUI(text)
	}
	return &next, nil
}

func (m *Model) saveConfigInput(path, text string) error {
	next, err := setConfigInput(m.AppConfig, path, text)
	if err != nil {
		return err
	}
	if err := config.SaveConfig(next); err != nil {
		return fmt.Errorf("Failed to save config: %w", err)
	}
	*m.AppConfig = *next
	if m.PlaybackSvc != nil {
		m.PlaybackSvc.SetSkipConfig(&next.Player.Skip)
	}
	if top := m.ColumnStack.Top(); top != nil {
		top.SetItems(m.configEntries())
	}
	m.StatusIsErr = false
	m.StatusMsg = "Saved setting (restart Cue to apply)"
	if strings.HasPrefix(path, "player.skip.") && path != "player.skip.analysis_at_startup" && path != "player.skip.intro_window_seconds" {
		m.StatusMsg = "Saved skip setting (applies to next playback)"
	}
	return nil
}
