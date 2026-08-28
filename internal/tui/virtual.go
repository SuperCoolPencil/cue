package tui

import (
	"fmt"
	"runtime"

	"github.com/SuperCoolPencil/cue/internal/domain"
)

const (
	filterUnwatchedID   = "__filter_unwatched__"
	filterInProgressID  = "__filter_in_progress__"
	filter4KID          = "__filter_4k__"
	filterHighlyRatedID = "__filter_highly_rated__"
)

func smartFilterEntries() []domain.Library {
	return []domain.Library{
		{ID: filterUnwatchedID, Name: "Unwatched", Type: "filter"},
		{ID: filterInProgressID, Name: "In Progress", Type: "filter"},
		{ID: filter4KID, Name: "4K", Type: "filter"},
		{ID: filterHighlyRatedID, Name: "Highly Rated", Type: "filter"},
	}
}

func (m Model) profileEntries() []domain.Library {
	entries := []domain.Library{{ID: "__profile_current__", Name: "Current: " + m.currentProfileName(), Type: "profile"}}
	if m.AppConfig != nil {
		for _, name := range m.AppConfig.ProfileNames() {
			entries = append(entries, domain.Library{ID: "__profile_" + name, Name: name, Type: "profile"})
		}
	}
	return entries
}

func (m Model) configEntries() []domain.Library {
	showWatch := "off"
	showCounts := "off"
	hideWatched := "off"
	autoplay := "off"
	playNextOnSelect := "off"
	if m.UIConfig.ShowWatchStatus {
		showWatch = "on"
	}
	if m.UIConfig.ShowLibraryCounts {
		showCounts = "on"
	}
	if m.UIConfig.HideWatched {
		hideWatched = "on"
	}
	if m.UIConfig.Autoplay {
		autoplay = "on"
	}
	if m.UIConfig.PlayNextOnSelect {
		playNextOnSelect = "on"
	}
	return []domain.Library{
		{ID: "__config_player__", Name: "Player: " + m.playerName(), Type: "config"},
		{ID: "__config_watch__", Name: "Watch indicators: " + showWatch, Type: "config"},
		{ID: "__config_counts__", Name: "Library counts: " + showCounts, Type: "config"},
		{ID: "__config_hide_watched__", Name: "Hide watched: " + hideWatched, Type: "config"},
		{ID: "__config_autoplay__", Name: "Autoplay: " + autoplay, Type: "config"},
		{ID: "__config_play_next_on_select__", Name: "Play next episode on select: " + playNextOnSelect, Type: "config"},
		{ID: "__config_os__", Name: "Platform: " + runtime.GOOS, Type: "config"},
	}
}

func (m Model) cacheEntries() []domain.Library {
	libs := len(m.Libraries)
	queue := len(m.PlaylistService.QueueItems())
	return []domain.Library{
		{ID: "__cache_libraries__", Name: fmt.Sprintf("Libraries cached: %d", libs), Type: "cache"},
		{ID: "__cache_queue__", Name: fmt.Sprintf("Queue items: %d", queue), Type: "cache"},
		{ID: "__cache_clear__", Name: "Clear all cache", Type: "cache"},
		{ID: "__cache_refresh__", Name: "Press R to rebuild all cached data", Type: "cache"},
	}
}

func (m Model) currentProfileName() string {
	if m.AppConfig == nil || m.AppConfig.CurrentProfile == "" {
		return "default"
	}
	return m.AppConfig.CurrentProfile
}

func (m Model) playerName() string {
	if m.AppConfig == nil || m.AppConfig.Player.Command == "" {
		return "auto"
	}
	return m.AppConfig.Player.Command
}

func filterKey(id string) string {
	switch id {
	case filterUnwatchedID:
		return "unwatched"
	case filterInProgressID:
		return "in_progress"
	case filter4KID:
		return "4k"
	case filterHighlyRatedID:
		return "highly_rated"
	default:
		return ""
	}
}
