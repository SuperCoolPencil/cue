package player

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"time"

	"github.com/SuperCoolPencil/cue/internal/domain"
)

// PlaybackHandle provides channels for monitoring progress and final result.
type PlaybackHandle struct {
	ResultCh <-chan ScrobbleResult
	StatusCh <-chan string
}

// ScrobbleResult contains the final outcome of a monitored playback session.
type ScrobbleResult struct {
	Item       domain.MediaItem
	ItemID     string
	Title      string
	FinalPosMs int64
	Duration   time.Duration
	AutoMarked bool
	Err        error
}

// Scrobbler monitors a running player process and reports progress to the server.
type Scrobbler struct {
	client   domain.PlaybackClient
	logger   *slog.Logger
	interval time.Duration
}

type timelineUpdate struct {
	item  domain.MediaItem
	state string
	time  int64
}

// NewScrobbler creates a new scrobbler.
func NewScrobbler(client domain.PlaybackClient, logger *slog.Logger) *Scrobbler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scrobbler{
		client:   client,
		logger:   logger,
		interval: 10 * time.Second,
	}
}

// Monitor starts a background goroutine to track playback progress for one or more items.
// If multiple items are provided, it uses mpv IPC to detect which one is active.
// offsetMs is the starting playback position (e.g. a resume offset) so the
// server reports the correct starting point instead of 0.
func (s *Scrobbler) Monitor(ctx context.Context, cmd *exec.Cmd, ipcSocket string, playlistStart int, offsetMs int64, items ...domain.MediaItem) PlaybackHandle {

	resCh := make(chan ScrobbleResult, 1)
	statusCh := make(chan string, 10)

	go func() {
		defer close(resCh)
		defer removeMPVSocket(ipcSocket)
		var wg sync.WaitGroup
		defer func() {
			wg.Wait()
			close(statusCh)
		}()

		var mpv *mpvConn
		var err error
		var activeItem domain.MediaItem
		var lastPosMs int64
		curState := "playing"
		var mu sync.Mutex     // guards markedIDs
		var playMu sync.Mutex // guards activeItem/lastPosMs/curState (shared with mpv event handler)
		markedIDs := make(map[string]bool)

		// Keep timeline requests ordered. mpv emits time-pos changes frequently,
		// so event-driven updates are coalesced instead of creating an HTTP
		// request per event. State transitions and shutdown use the blocking
		// variant to preserve their ordering.
		timelineUpdates := make(chan timelineUpdate, 1)
		timelineDone := make(chan struct{})
		go func() {
			defer close(timelineDone)
			for update := range timelineUpdates {
				s.reportTimeline(update.item, update.state, update.time)
			}
		}()
		queueTimeline := func(item domain.MediaItem, state string, timeMs int64) {
			select {
			case timelineUpdates <- timelineUpdate{item: item, state: state, time: timeMs}:
			default:
			}
		}
		sendTimeline := func(item domain.MediaItem, state string, timeMs int64) {
			timelineUpdates <- timelineUpdate{item: item, state: state, time: timeMs}
		}

		if len(items) > 0 {
			startIdx := playlistStart
			if startIdx < 0 || startIdx >= len(items) {
				startIdx = 0
			}
			activeItem = items[startIdx]
		}

		// Try to connect to MPV IPC if available
		if ipcSocket != "" {
			mpv, err = dialMPV(ipcSocket)
			if err != nil {
				s.logger.Warn("mpv IPC connection failed, falling back to exit-only reporting", "error", err)
			}
		}

		// Subscribe to mpv events so Tautulli/Plex get immediate updates on
		// seek and pause instead of waiting for the next poll tick.
		if mpv != nil {
			mpv.SetEventHandler(func(event string, data json.RawMessage) {
				switch event {
				case "property-change":
					var pc struct {
						Name string          `json:"name"`
						Data json.RawMessage `json:"data"`
					}
					if err := json.Unmarshal(data, &pc); err != nil {
						return
					}
					switch pc.Name {
					case "time-pos":
						var pos float64
						if err := json.Unmarshal(pc.Data, &pos); err == nil {
							playMu.Lock()
							lastPosMs = int64(pos * 1000)
							item := activeItem
							state := curState
							playMu.Unlock()
							queueTimeline(item, state, lastPosMs)
						}
					case "pause":
						var paused bool
						if err := json.Unmarshal(pc.Data, &paused); err == nil {
							playMu.Lock()
							if paused {
								curState = "paused"
							} else {
								curState = "playing"
							}
							item := activeItem
							state := curState
							playMu.Unlock()
							queueTimeline(item, state, lastPosMs)
						}
					}
				case "seek":
					playMu.Lock()
					item := activeItem
					state := curState
					pos := lastPosMs
					playMu.Unlock()
					queueTimeline(item, state, pos)
				}
			})
			// Observation is best-effort; the poll loop still reports if events
			// aren't delivered for any reason.
			_ = mpv.Observe(1, "time-pos")
			_ = mpv.Observe(2, "pause")
		}

		// Announce the session to the server so it appears as "Now Playing"
		// (e.g. in Plex / Tautulli). Use the resume offset so the server sees
		// the correct starting position rather than 0.
		initialPosMs := offsetMs
		if mpv != nil {
			if p, perr := mpv.GetTimePos(); perr == nil && p > 0 {
				initialPosMs = int64(p * 1000)
			}
		}
		sendTimeline(activeItem, "playing", initialPosMs)

		// Polling loop
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		processDone := make(chan error, 1)
		go func() {
			processDone <- cmd.Wait()
		}()

	loop:
		for {
			select {
			case <-ctx.Done():
				break loop
			case err := <-processDone:
				if err != nil {
					s.logger.Warn("player process exited with error", "error", err)
				}
				break loop
			case <-ticker.C:
				if mpv != nil {
					// Detect if item changed (for playlists)
					if len(items) > 1 {
						// We assume for now the order in 'items' matches the playlist order.
						if pos, err := mpv.GetProperty("playlist-pos"); err == nil {
							if idx, ok := pos.(float64); ok && int(idx) < len(items) {
								newIdx := int(idx)
								newItem := items[newIdx]
								playMu.Lock()
								changed := newItem.ID != activeItem.ID
								playMu.Unlock()
								if changed {
									playMu.Lock()
									oldItem := activeItem
									oldPos := lastPosMs
									activeItem = newItem
									playMu.Unlock()
									s.logger.Info("playlist item changed", "from", oldItem.Title, "to", newItem.Title)
									// Close the session for the previous item and open one for the new item.
									sendTimeline(oldItem, "stopped", oldPos)
									sendTimeline(newItem, "playing", 0)
									// Mark all previous items in the playlist as watched
									s.markPreviousWatched(items, newIdx, markedIDs, &mu)
								}
							}
						}
					}

					if posSecs, err := mpv.GetTimePos(); err == nil {
						posMs := int64(posSecs * 1000)

						// Detect pause so we report the correct session state.
						paused := false
						if pv, perr := mpv.GetProperty("pause"); perr == nil {
							if b, ok := pv.(bool); ok {
								paused = b
							}
						}
						playMu.Lock()
						lastPosMs = posMs
						if paused {
							curState = "paused"
						} else {
							curState = "playing"
						}
						item := activeItem
						state := curState
						playMu.Unlock()

						s.logger.Debug("reporting progress", "item", item.Title, "pos", posMs)

						// Keep-alive timeline update; mpv events also report on change.
						queueTimeline(item, state, posMs)

						// Fire and forget progress update
						wg.Add(1)
						go func(item domain.MediaItem, pos int64) {
							defer wg.Done()
							updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
							defer cancel()
							if err := s.client.UpdateProgress(updateCtx, item.ID, pos); err == nil {
								// Format position as MM:SS for user display
								d := time.Duration(pos) * time.Millisecond
								select {
								case statusCh <- fmt.Sprintf("Saved %s %02d:%02d to server", item.Title, int(d.Minutes()), int(d.Seconds())%60):
								case <-ctx.Done():
								default:
								}
							} else {
								s.logger.Warn("failed to update progress", "error", err)
							}
						}(item, posMs)
					}
				}
			}
		}

		// Final position update on exit
		playMu.Lock()
		finalItem := activeItem
		finalPos := lastPosMs
		playMu.Unlock()
		if mpv != nil {
			if posSecs, perr := mpv.GetTimePos(); perr == nil {
				finalPos = int64(posSecs * 1000)
				playMu.Lock()
				lastPosMs = finalPos
				playMu.Unlock()
				s.logger.Debug("final progress update", "item", finalItem.Title, "pos", finalPos)
				updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := s.client.UpdateProgress(updateCtx, finalItem.ID, finalPos); err != nil {
					s.logger.Warn("failed to report final progress", "error", err)
				}
				cancel()
			}
		}

		// Stop the IPC reader before sending the terminal state so no later
		// event can enqueue a playing update after it. The reporter processes
		// this after all queued updates, making stopped the final request.
		if mpv != nil {
			_ = mpv.Close()
			<-mpv.readDone
		}
		sendTimeline(finalItem, "stopped", finalPos)
		close(timelineUpdates)
		<-timelineDone

		// Handle auto-scrobble on exit (90% threshold)
		autoMarked := false
		if activeItem.Duration > 0 && lastPosMs > 0 {
			progress := float64(lastPosMs) / float64(activeItem.Duration.Milliseconds())
			if progress >= 0.90 {
				s.logger.Info("auto-marking watched", "item", activeItem.Title, "progress", progress)
				markCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := s.client.MarkPlayed(markCtx, activeItem.ID); err == nil {
					autoMarked = true
					mu.Lock()
					markedIDs[activeItem.ID] = true
					mu.Unlock()
					// Find current index and mark all previous
					for i, it := range items {
						if it.ID == activeItem.ID {
							s.markPreviousWatched(items, i, markedIDs, &mu)
							break
						}
					}
				}
			}
		}

		resCh <- ScrobbleResult{
			Item:       activeItem,
			ItemID:     activeItem.ID,
			Title:      activeItem.Title,
			FinalPosMs: lastPosMs,
			Duration:   activeItem.Duration,
			AutoMarked: autoMarked,
		}
	}()

	return PlaybackHandle{
		ResultCh: resCh,
		StatusCh: statusCh,
	}
}

// reportTimeline sends a best-effort playback timeline update to create and
// maintain a live server session (Plex "Now Playing"). It is a no-op for
// backends that don't support it (Jellyfin). Failures are logged and never
// block or break local playback.
func (s *Scrobbler) reportTimeline(item domain.MediaItem, state string, timeMs int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.client.ReportTimeline(ctx, state, item.ID, timeMs, item.Duration.Milliseconds()); err != nil {
		s.logger.Warn("failed to report playback timeline", "error", err)
	}
}

func (s *Scrobbler) markPreviousWatched(items []domain.MediaItem, currentIdx int, markedIDs map[string]bool, mu *sync.Mutex) {
	// Mark items sequentially in a single background goroutine to avoid flooding the server
	go func() {
		for i := 0; i < currentIdx; i++ {
			item := items[i]
			mu.Lock()
			if item.IsPlayed || markedIDs[item.ID] {
				mu.Unlock()
				continue
			}
			markedIDs[item.ID] = true
			mu.Unlock()

			s.logger.Info("bulk-marking previous item watched", "item", item.Title)

			func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := s.client.MarkPlayed(ctx, item.ID); err != nil {
					s.logger.Warn("failed to mark previous item watched", "item", item.Title, "error", err)
				}
			}()
		}
	}()
}
