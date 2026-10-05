package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/SuperCoolPencil/cue/internal/config"
	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/viper"
)

func TestEveryConfigSettingHasTUIControl(t *testing.T) {
	model := Model{AppConfig: config.DefaultConfig()}
	entries := map[string]bool{}
	for _, entry := range model.configEntries() {
		if entries[entry.ID] {
			t.Errorf("duplicate config entry %q", entry.ID)
		}
		entries[entry.ID] = true
	}
	registered := map[string]bool{}
	for _, setting := range configSettings {
		if registered[setting.path] {
			t.Errorf("duplicate setting %q", setting.path)
		}
		registered[setting.path] = true
		if !entries[setting.id] {
			t.Errorf("%s has no rendered TUI control", setting.path)
		}
		if !configField(model.AppConfig, setting.path).IsValid() {
			t.Errorf("unknown config setting %s", setting.path)
		}
	}
	var walk func(reflect.Type, string)
	walk = func(typ reflect.Type, prefix string) {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			path := prefix + field.Tag.Get("mapstructure")
			if field.Type.Kind() == reflect.Struct {
				walk(field.Type, path+".")
			} else if !registered[path] {
				t.Errorf("config setting %s is missing from the TUI; add a control to configSettings", path)
			}
		}
	}
	walk(reflect.TypeOf(config.Config{}), "")
}

func TestConfigEntriesShowSkipSettingsAndMaskCredentials(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Player.Skip.Intro = "auto"
	cfg.Player.Skip.Key = "k"
	cfg.Server.Token = "private-server-token"
	cfg.Server.PlexAccountToken = "private-account-token"
	model := Model{AppConfig: cfg}
	names := map[string]string{}
	for _, entry := range model.configEntries() {
		names[entry.ID] = entry.Name
		if strings.Contains(entry.Name, "private-") {
			t.Fatal("exposed credential in Config menu")
		}
	}
	for id, want := range map[string]string{
		"__config_skip_intro__":    "Skip intros: auto",
		"__config_skip_key__":      "Skip key: k",
		"__config_skip_undo_key__": "Undo skip key: Alt+x",
		"__config_skip_analysis__": "Analyze intros at startup: on",
		"__config_skip_window__":   "Intro scan seconds (0 = default): 0",
		"__config_skip_chapters__": "Add missing skip chapters: on",
	} {
		if names[id] != want {
			t.Errorf("%s = %q, want %q", id, names[id], want)
		}
	}
}

func TestConfigInputValidationPreservesLiveSettings(t *testing.T) {
	cfg := config.DefaultConfig()
	for _, tc := range []struct{ path, text string }{
		{"player.skip.intro_window_seconds", "29"},
		{"player.skip.intro_window_seconds", "901"},
		{"player.skip.intro_window_seconds", "abc"},
		{"player.skip.key", "Alt+x"},
		{"player.skip.key", " "},
		{"player.args", "--fullscreen"},
		{"player.args", "null"},
		{"server.url", "not-a-url"},
		{"server.type", "unknown"},
		{"logging.level", "invalid"},
	} {
		if _, err := setConfigInput(cfg, tc.path, tc.text); err == nil {
			t.Errorf("accepted %s = %q", tc.path, tc.text)
		}
	}
	if !reflect.DeepEqual(cfg, config.DefaultConfig()) {
		t.Fatal("invalid input mutated configuration")
	}
	next, err := setConfigInput(cfg, "player.args", `["--fullscreen","--title=a b"]`)
	if err != nil || len(next.Player.Args) != 2 || next.Player.Args[1] != "--title=a b" {
		t.Fatalf("arguments: %#v, %v", next, err)
	}
}

func TestConfigEditorSavesAndCancelDoesNotCreatePlaylist(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	path := t.TempDir() + "/config.yaml"
	viper.SetConfigFile(path)
	// WriteConfigAs establishes the loaded file without touching the user's config.
	if err := viper.WriteConfigAs(path); err != nil {
		t.Fatal(err)
	}
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	model := Model{AppConfig: config.DefaultConfig(), InputModal: components.NewInputModal(), ColumnStack: NewColumnStack()}
	if !model.openConfigSetting("__config_skip_key__") || !model.InputModal.IsVisible() {
		t.Fatal("skip key has no editor")
	}
	model.InputModal.ShowValue("Skip key", "k", false)
	_, model, cmd := model.handleInputModalInput(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || model.AppConfig.Player.Skip.Key != "k" || model.InputModal.IsVisible() {
		t.Fatal("config edit did not save cleanly")
	}
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	if viper.GetString("player.skip.key") != "k" {
		t.Fatal("skip key was not persisted")
	}
	model.openConfigSetting("__config_skip_key__")
	model.InputModal.ShowValue("Skip key", "Alt+x", false)
	_, model, _ = model.handleInputModalInput(tea.KeyMsg{Type: tea.KeyEnter})
	if !model.InputModal.IsVisible() || model.AppConfig.Player.Skip.Key != "k" || !strings.Contains(model.InputModal.View(), "keys must be") {
		t.Fatal("invalid edit must preserve setting and show feedback in the editor")
	}
	model.openConfigSetting("__config_skip_key__")
	_, model, _ = model.handleInputModalInput(tea.KeyMsg{Type: tea.KeyEsc})
	if model.configInputPath != "" || model.AppConfig.Player.Skip.Key != "k" {
		t.Fatal("cancel changed setting or left editor active")
	}
	if !model.openConfigSetting("__config_skip_chapters__") || model.AppConfig.Player.Skip.ChaptersWhenMissing {
		t.Fatal("boolean control failed to toggle")
	}
}

func TestConfigInfoIsCompactAndListGetsRemainingHeight(t *testing.T) {
	for _, height := range []int{18, 43, 65} {
		model := Model{AppConfig: config.DefaultConfig(), ColumnStack: NewColumnStack(), Width: 120, Height: height + ChromeHeight}
		root := components.NewLibraryColumn([]domain.Library{{ID: configLibraryID, Name: "Config", Type: "cue"}})
		model.ColumnStack.Reset(root)
		col := components.NewListColumn(components.ColumnTypeLibraries, "Config")
		col.SetContentID(configLibraryID)
		col.SetItems(model.configEntries())
		model.ColumnStack.Push(col, 0)
		model.updateLayout()
		if col.Height() != height-configInfoHeight {
			t.Fatalf("height=%d list=%d", height, col.Height())
		}
		info := model.renderConfigInfo(col, 84)
		if lipgloss.Height(info) != configInfoHeight || lipgloss.Width(info) != 84 {
			t.Fatalf("info size = %dx%d", lipgloss.Width(info), lipgloss.Height(info))
		}
		if strings.Contains(info, "Type: Movies") || strings.Contains(info, "Press Enter to browse") {
			t.Fatal("config info uses media metadata")
		}
		view := model.renderSplitColumn(col, 84, height/3, height-height/3)
		if lipgloss.Height(view) != height {
			t.Fatalf("split height=%d want=%d", lipgloss.Height(view), height)
		}
	}
}
