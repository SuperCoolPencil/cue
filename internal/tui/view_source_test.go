package tui

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/playlist"
	"github.com/SuperCoolPencil/cue/internal/store"
	"github.com/SuperCoolPencil/cue/internal/tui/components"
)

func TestLocalViewsRefreshWithoutServerLibraryRequests(t *testing.T) {
	for _, id := range []string{recentLibraryID, queueLibraryID, filterUnwatchedID, filterInProgressID, filter4KID, filterHighlyRatedID} {
		for _, manual := range []bool{false, true} {
			t.Run(id+"/"+map[bool]string{true: "manual", false: "playback"}[manual], func(t *testing.T) {
				st, _ := store.NewLibraryStore("", "", "")
				movie := &domain.MediaItem{ID: "movie", Type: domain.MediaTypeMovie, ViewOffset: time.Minute, Width: 3840, Height: 2160, Rating: 9}
				if id == filterUnwatchedID {
					movie.ViewOffset = 0
				}
				if err := st.SaveLibraries([]domain.Library{{ID: "movies", Type: "movie"}}); err != nil {
					t.Fatal(err)
				}
				if err := st.SaveMovies("movies", []*domain.MediaItem{movie}, 1); err != nil {
					t.Fatal(err)
				}
				if err := st.SaveQueueItems([]*domain.MediaItem{movie}); err != nil {
					t.Fatal(err)
				}
				m := newVirtualTestModel(st) // No client: a server call would fail the test.
				m.Inspector = components.NewInspector()
				m.ColumnStack.Reset(components.NewLibraryColumn(m.allLibraryEntries()))
				col := components.NewListColumn(components.ColumnTypeMixed, "Local view")
				col.SetContentID(id)
				m.ColumnStack.Push(col, 0)
				if manual {
					m, _ = m.refreshLibraryContent(col)
				} else {
					m.refreshCurrentView()
				}
				if col.ItemCount() != 1 || col.SelectedItemID() != movie.ID || col.IsRefreshing() {
					t.Fatalf("local projection not refreshed: count=%d refreshing=%v", col.ItemCount(), col.IsRefreshing())
				}
				if _, ok := st.GetMovies("movies"); !ok {
					t.Fatal("refresh erased local projection input")
				}
			})
		}
	}
}

type viewPlaylistClient struct {
	domain.PlaylistClient
	requested string
}

func (c *viewPlaylistClient) GetPlaylistItems(_ context.Context, id string) ([]*domain.MediaItem, error) {
	c.requested = id
	return []*domain.MediaItem{{ID: "movie", Type: domain.MediaTypeMovie}}, nil
}

func TestPlaylistPlaybackRefreshKeepsPlaylistView(t *testing.T) {
	st, _ := store.NewLibraryStore("", "", "")
	m := newVirtualTestModel(st)
	client := &viewPlaylistClient{}
	m.PlaylistService = playlist.NewService(client, st, slog.Default())
	m.ColumnStack.Reset(components.NewLibraryColumn(m.allLibraryEntries()))
	col := components.NewListColumn(components.ColumnTypePlaylistItems, "Playlist")
	col.SetContentID("playlist-1")
	m.ColumnStack.Push(col, 0)
	cmd := m.refreshCurrentView()
	if cmd == nil {
		t.Fatal("no refresh")
	}
	result := cmd().(viewResultMsg)
	if _, ok := result.message.(PlaylistItemsLoadedMsg); !ok || client.requested != "playlist-1" {
		t.Fatalf("wrong playlist refresh: %q", client.requested)
	}
	if m.ColumnStack.Top() != col {
		t.Fatal("playlist view replaced during refresh")
	}
}

func TestPlaybackCompletionUpdatesLocalProjectionBeforeRefresh(t *testing.T) {
	for _, id := range []string{queueLibraryID, filterInProgressID} {
		for _, watched := range []bool{false, true} {
			t.Run(id+"/"+map[bool]string{false: "partial", true: "watched"}[watched], func(t *testing.T) {
				st, _ := store.NewLibraryStore("", "", "")
				item := &domain.MediaItem{ID: "movie", Type: domain.MediaTypeMovie, Duration: time.Minute}
				if err := st.SaveLibraries([]domain.Library{{ID: "lib", Type: "movie"}}); err != nil {
					t.Fatal(err)
				}
				if err := st.SaveMovies("lib", []*domain.MediaItem{item}, 1); err != nil {
					t.Fatal(err)
				}
				if err := st.SaveQueueItems([]*domain.MediaItem{item}); err != nil {
					t.Fatal(err)
				}
				m := newVirtualTestModel(st)
				m.Inspector = components.NewInspector()
				m.ColumnStack.Reset(components.NewLibraryColumn(m.allLibraryEntries()))
				col := components.NewListColumn(components.ColumnTypeMixed, "View")
				col.SetContentID(id)
				col.SetItems([]*domain.MediaItem{item})
				m.ColumnStack.Push(col, 0)
				updated, _ := m.Update(PlaybackFinishedMsg{Item: *item, FinalPosMs: 30000, PositionKnown: true, AutoMarked: watched})
				m = updated.(Model)
				wantCount := 1
				if watched && id == filterInProgressID {
					wantCount = 0
				}
				if col.ItemCount() != wantCount || m.Loading {
					t.Fatalf("projection did not reflect playback: count=%d", col.ItemCount())
				}
				queue, _ := st.GetQueueItems()
				wantOffset := 30 * time.Second
				if watched {
					wantOffset = 0
				}
				if queue[0].ViewOffset != wantOffset || queue[0].IsPlayed != watched || queue[0].LastPlayedAt == 0 {
					t.Fatalf("stale queue snapshot: %#v", queue[0])
				}
			})
		}
	}
}
