package segments

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"github.com/SuperCoolPencil/cue/internal/domain"
)

type AnalysisClient interface {
	domain.LibraryClient
	domain.PlaybackClient
}
type Analyzer struct {
	Client                    AnalysisClient
	Cache                     Cache
	Server, User              string
	WindowSeconds, AudioTrack int
	IntroWindowSeconds        int
	Logger                    *slog.Logger
	// Extractor can be injected for deterministic integration tests.
	Extractor func(context.Context, string, int64, int, int) ([]uint32, error)
}

func (a Analyzer) analyze(ctx context.Context, episodes []*domain.MediaItem) (int, error) {
	if len(episodes) < 3 {
		return 0, nil
	}
	var key progressKey
	progress := AnalysisProgress{Active: true}
	for _, episode := range episodes {
		if episode != nil {
			key = progressKey{a.Server, a.User, episode.ShowID, episode.ParentID}
			progress.Total++
		}
	}
	setAnalysisProgress(key, progress)
	defer setAnalysisProgress(key, AnalysisProgress{})
	window := a.WindowSeconds
	if window == 0 {
		window = 300
	}
	extract := a.Extractor
	if extract == nil {
		extract = Extract
	}
	var entries []Cached
	var durations, offsets []int64
	var heads, tails [][]uint32
	changed := false
	for _, episode := range episodes {
		if episode == nil {
			continue
		}
		requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		media, err := a.Client.ResolvePlayable(requestCtx, episode.ID)
		cancel()
		if err != nil || media.SourceID == "" || media.DurationMs <= 0 {
			return 0, fmt.Errorf("could not resolve episode source")
		}
		id := Identity{Server: a.Server, User: a.User, Show: episode.ShowID, Item: episode.ID, Source: media.SourceID, Revision: media.Revision, DurationMs: media.DurationMs}
		entry, ok := a.Cache.Load(id)
		introWindow := introWindowSeconds(media.DurationMs, a.IntroWindowSeconds)
		headChanged := !ok || entry.IntroWindowSeconds != introWindow || entry.AudioTrack != a.AudioTrack || len(entry.Head) == 0
		tailChanged := !ok || entry.WindowSeconds != window || entry.AudioTrack != a.AudioTrack || len(entry.Tail) == 0
		if headChanged || tailChanged {
			changed = true
			entry.Identity, entry.SeasonID, entry.Version = id, episode.ParentID, Version
			entry.WindowSeconds, entry.IntroWindowSeconds, entry.AudioTrack = window, introWindow, a.AudioTrack
			entry.Membership, entry.Segments = "", nil
			if headChanged {
				entry.Head, err = extract(ctx, media.URL, 0, introWindow, a.AudioTrack)
				if err != nil {
					return 0, err
				}
			}
			if tailChanged {
				entry.Tail, err = extract(ctx, media.URL, max(int64(0), media.DurationMs-int64(window)*1000), window, a.AudioTrack)
				if err != nil {
					return 0, err
				}
			}
			if err := a.Cache.Save(entry); err != nil {
				return 0, err
			}
		}
		if entry.SeasonID != episode.ParentID {
			entry.SeasonID = episode.ParentID
			changed = true
		}
		entries = append(entries, entry)
		durations = append(durations, media.DurationMs)
		offsets = append(offsets, max(int64(0), media.DurationMs-int64(window)*1000))
		heads = append(heads, entry.Head)
		tails = append(tails, entry.Tail)
		progress.Completed++
		setAnalysisProgress(key, progress)
	}
	if len(entries) < 3 {
		return 0, nil
	}
	// Membership is part of the matching cache, so newly added/removed episodes
	// trigger matching while unchanged seasons need only lightweight metadata.
	membership := "matcher-v4;"
	for _, entry := range entries {
		membership += entry.Identity.Item + ":" + entry.Identity.Revision + ";"
	}
	membership = hashID(membership)
	for _, entry := range entries {
		if entry.Membership != membership {
			changed = true
		}
	}
	if !changed {
		count := 0
		for _, entry := range entries {
			count += len(entry.Segments)
		}
		return count, nil
	}
	matchCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	progress.Matching = true
	setAnalysisProgress(key, progress)
	intros, err := Detect(matchCtx, heads, durations, make([]int64, len(entries)), "intro")
	if err != nil {
		return 0, err
	}
	outros, err := Detect(matchCtx, tails, durations, offsets, "outro")
	if err != nil {
		return 0, err
	}
	count := 0
	for i, entry := range entries {
		entry.Membership = membership
		entry.Segments = domain.ValidSkipSegments(append(intros[i], outros[i]...), entry.Identity.DurationMs)
		count += len(entry.Segments)
		if err := a.Cache.Save(entry); err != nil {
			return 0, err
		}
	}
	return count, nil
}

// Intro scans stay within the first quarter to avoid matching music in story
// content. The configurable cap defaults to ten minutes.
func introWindowSeconds(durationMs int64, capSeconds int) int {
	if capSeconds <= 0 {
		capSeconds = 600
	}
	return max(1, min(capSeconds, int(durationMs/4000)))
}

func (a Analyzer) AnalyzeSeason(ctx context.Context, seasonID string) (int, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	episodes, err := a.Client.GetEpisodes(requestCtx, seasonID)
	cancel()
	if err != nil {
		return 0, fmt.Errorf("could not load episodes")
	}
	return a.analyze(ctx, episodes)
}

// Startup inventories the full visible show list before removing obsolete show
// directories. One sequential worker bounds CPU, network and server load.
func (a Analyzer) Startup(ctx context.Context) error {
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	libraries, err := a.Client.GetLibraries(requestCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("show inventory unavailable; cleanup skipped")
	}
	var shows []*domain.Show
	seen := map[string]bool{}
	for _, library := range libraries {
		if library.Type != "show" && library.Type != "mixed" {
			continue
		}
		for offset := 0; ; {
			requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			page, total, err := a.Client.GetShows(requestCtx, library.ID, offset, 200)
			cancel()
			if err != nil {
				return fmt.Errorf("show inventory incomplete; cleanup skipped")
			}
			if len(page) == 0 {
				if offset < total {
					return fmt.Errorf("empty show page; cleanup skipped")
				}
				break
			}
			for _, show := range page {
				if show != nil && !seen[show.ID] {
					seen[show.ID] = true
					shows = append(shows, show)
				}
			}
			offset += len(page)
			if offset >= total {
				break
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ids := make([]string, 0, len(shows))
	for _, show := range shows {
		ids = append(ids, show.ID)
	}
	if err := a.Cache.PruneShows(a.Server, a.User, ids); err != nil {
		return err
	}
	// Cleanup still runs when the analyzer dependency is unavailable.
	if a.Extractor == nil {
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		output, err := exec.CommandContext(probeCtx, "ffmpeg", "-hide_banner", "-h", "muxer=chromaprint").CombinedOutput()
		cancel()
		if err != nil || !bytes.Contains(output, []byte("Muxer chromaprint")) {
			return fmt.Errorf("startup analysis unavailable: FFmpeg with Chromaprint required")
		}
	}
	for _, show := range shows {
		if err := ctx.Err(); err != nil {
			return err
		}
		requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		seasons, err := a.Client.GetSeasons(requestCtx, show.ID)
		cancel()
		if err != nil {
			if a.Logger != nil {
				a.Logger.Warn("skip analysis: seasons unavailable", "showID", show.ID)
			}
			continue
		}
		episodeIDs := []string{}
		completeEpisodes := true
		for _, season := range seasons {
			if season == nil {
				continue
			}
			requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			episodes, inventoryErr := a.Client.GetEpisodes(requestCtx, season.ID)
			cancel()
			if inventoryErr != nil {
				completeEpisodes = false
				continue
			}
			for _, episode := range episodes {
				if episode != nil {
					episodeIDs = append(episodeIDs, episode.ID)
					if episode.ShowID == "" {
						episode.ShowID = show.ID
					}
				}
			}
			count, err := a.analyze(ctx, episodes)
			if err != nil {
				if a.Logger != nil {
					a.Logger.Warn("skip analysis failed", "showID", show.ID, "seasonID", season.ID)
				}
				continue
			}
			if a.Logger != nil {
				a.Logger.Info("skip analysis complete", "showID", show.ID, "seasonID", season.ID, "segments", count)
			}
		}
		if completeEpisodes && ctx.Err() == nil {
			if err := a.Cache.PruneEpisodes(a.Server, a.User, show.ID, episodeIDs); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}
