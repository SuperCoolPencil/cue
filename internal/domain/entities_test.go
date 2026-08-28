package domain

import (
	"testing"
	"time"
)

func TestMediaItemFormattingAndStatus(t *testing.T) {
	item := MediaItem{Duration: 95 * time.Minute, Height: 2160, FileSize: 3 * 1024 * 1024 * 1024, AudioChannels: 6}
	if item.FormattedDuration() != "1h 35m" {
		t.Fatalf("duration = %q", item.FormattedDuration())
	}
	if item.Resolution() != "4K" {
		t.Fatalf("resolution = %q", item.Resolution())
	}
	if item.FormattedFileSize() != "3.0 GB" {
		t.Fatalf("file size = %q", item.FormattedFileSize())
	}
	if item.ChannelLayout() != "5.1" {
		t.Fatalf("channel layout = %q", item.ChannelLayout())
	}
	item.ViewOffset = time.Minute
	if item.WatchStatus() != WatchStatusInProgress || !item.ShouldResume() {
		t.Fatalf("watch status = %v resume=%v", item.WatchStatus(), item.ShouldResume())
	}
	item.IsPlayed = true
	if item.WatchStatus() != WatchStatusWatched || item.ShouldResume() {
		t.Fatalf("watch status = %v resume=%v", item.WatchStatus(), item.ShouldResume())
	}
}

func TestShowSeasonPlaylistDescriptions(t *testing.T) {
	show := Show{SeasonCount: 2, EpisodeCount: 10, UnwatchedCount: 5}
	if show.GetDescription() != "2 Seasons" || show.WatchStatus() != WatchStatusInProgress {
		t.Fatalf("show desc=%q status=%v", show.GetDescription(), show.WatchStatus())
	}
	season := Season{SeasonNum: 0, EpisodeCount: 1}
	if season.DisplayTitle() != "Specials" || season.GetDescription() != "1 Episode" {
		t.Fatalf("season title=%q desc=%q", season.DisplayTitle(), season.GetDescription())
	}
	playlist := Playlist{ItemCount: 2}
	if playlist.GetDescription() != "2 items" {
		t.Fatalf("playlist desc=%q", playlist.GetDescription())
	}
}

func TestMediaItemResolution(t *testing.T) {
	tests := []struct {
		width, height int
		expected      string
	}{
		{3840, 2160, "4K"},
		{3840, 1600, "4K"}, // Ultra-wide 4K
		{1920, 1080, "1080p"},
		{1920, 800, "1080p"}, // Ultra-wide 1080p
		{2560, 1440, "1440p"},
		{1280, 720, "720p"},
		{1280, 534, "720p"}, // Ultra-wide 720p
		{720, 480, "480p"},
		{640, 480, "480p"},
		{0, 480, "480p"},   // Only height
		{1920, 0, "1080p"}, // Only width
		{0, 1079, "720p"},  // Fallback to 720p class
		{0, 0, ""},         // No metadata
	}

	for _, tt := range tests {
		item := MediaItem{Width: tt.width, Height: tt.height}
		if got := item.Resolution(); got != tt.expected {
			t.Errorf("Resolution(%d, %d) = %q; want %q", tt.width, tt.height, got, tt.expected)
		}
	}
}

func TestNextUnplayedEpisodeAfterLastPlayed(t *testing.T) {
	tests := []struct {
		name     string
		episodes []*MediaItem
		wantID   string
	}{
		{
			name: "skips unplayed episodes before the latest played episode",
			episodes: []*MediaItem{
				{ID: "s2e2", SeasonNum: 2, EpisodeNum: 2, IsPlayed: false},
				{ID: "s1e1", SeasonNum: 1, EpisodeNum: 1, IsPlayed: true},
				{ID: "s1e2", SeasonNum: 1, EpisodeNum: 2, IsPlayed: false}, // intentionally skipped
				{ID: "s2e1", SeasonNum: 2, EpisodeNum: 1, IsPlayed: true},
			},
			wantID: "s2e2",
		},
		{
			name: "starts at the first unplayed episode when none have been played",
			episodes: []*MediaItem{
				{ID: "s1e2", SeasonNum: 1, EpisodeNum: 2},
				{ID: "s1e1", SeasonNum: 1, EpisodeNum: 1},
			},
			wantID: "s1e1",
		},
		{
			name: "returns nil when no unplayed episode follows the latest played episode",
			episodes: []*MediaItem{
				{ID: "s1e1", SeasonNum: 1, EpisodeNum: 1},
				{ID: "s1e2", SeasonNum: 1, EpisodeNum: 2, IsPlayed: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextUnplayedEpisodeAfterLastPlayed(tt.episodes)
			if tt.wantID == "" {
				if got != nil {
					t.Fatalf("NextUnplayedEpisodeAfterLastPlayed() = %q, want nil", got.ID)
				}
				return
			}
			if got == nil || got.ID != tt.wantID {
				t.Fatalf("NextUnplayedEpisodeAfterLastPlayed() = %#v, want %q", got, tt.wantID)
			}
		})
	}
}
