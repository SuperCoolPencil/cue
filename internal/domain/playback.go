package domain

import (
	"context"
	"sort"
)

// NextUnplayedEpisodeAfterLastPlayed returns the first unplayed episode after
// the latest played one in episode order. This deliberately ignores older
// unplayed episodes: they may have been skipped intentionally.
func NextUnplayedEpisodeAfterLastPlayed(episodes []*MediaItem) *MediaItem {
	ordered := make([]*MediaItem, 0, len(episodes))
	for _, episode := range episodes {
		if episode != nil {
			ordered = append(ordered, episode)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].SeasonNum != ordered[j].SeasonNum {
			return ordered[i].SeasonNum < ordered[j].SeasonNum
		}
		return ordered[i].EpisodeNum < ordered[j].EpisodeNum
	})

	lastPlayed := -1
	for i, episode := range ordered {
		if episode.IsPlayed {
			lastPlayed = i
		}
	}
	for _, episode := range ordered[lastPlayed+1:] {
		if !episode.IsPlayed {
			return episode
		}
	}
	return nil
}

// Subtitle describes an external subtitle track that the player should side-load.
type Subtitle struct {
	URL      string // Direct URL the player can fetch
	Language string // ISO language code, e.g. "eng", "spa"
	Title    string // Human-readable label (may be empty)
	Codec    string // e.g. "srt", "ass", "vtt"
	Default  bool   // Server marked this track as default
	Forced   bool   // Forced subtitle track
}

// PlayableMedia describes everything the player needs to play an item:
// the main media URL plus any external (sidecar) subtitle tracks the server exposes.
type PlayableMedia struct {
	URL       string
	Subtitles []Subtitle
}

// PlaybackClient provides network operations for media playback.
type PlaybackClient interface {
	ResolvePlayable(ctx context.Context, itemID string) (PlayableMedia, error)
	MarkPlayed(ctx context.Context, itemID string) error
	MarkUnplayed(ctx context.Context, itemID string) error
	// UpdateProgress reports the current playback position to the server.
	// positionMs is the current position in milliseconds.
	UpdateProgress(ctx context.Context, itemID string, positionMs int64) error
	// ReportTimeline reports the current playback state to the server to
	// create and maintain a live session (e.g. Plex "Now Playing"). state is
	// one of "playing", "paused", "stopped", "buffering". ratingKey is the
	// item's rating key; timeMs/durationMs are in milliseconds.
	ReportTimeline(ctx context.Context, state, ratingKey string, timeMs, durationMs int64) error
}
