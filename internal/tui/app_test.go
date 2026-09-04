package tui

import (
	"bytes"
	"testing"
	"time"

	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/library"
	"github.com/SuperCoolPencil/cue/internal/store"
	"github.com/SuperCoolPencil/cue/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

func TestMarkWatchedUpdatesCachedAndVisibleState(t *testing.T) {
	cache, err := store.NewLibraryStore("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	movie := &domain.MediaItem{ID: "movie-1", Title: "Movie", Type: domain.MediaTypeMovie}
	if err := cache.SaveMovies("movies", []*domain.MediaItem{movie}, 1); err != nil {
		t.Fatal(err)
	}

	column := components.NewListColumn(components.ColumnTypeMovies, "Movies")
	column.SetItems([]*domain.MediaItem{movie})
	stack := NewColumnStack()
	stack.Push(column, 0)
	m := Model{
		ColumnStack:    stack,
		Inspector:      components.NewInspector(),
		LibraryService: library.NewService(nil, cache, nil),
	}

	model, _ := m.Update(MarkWatchedMsg{ItemID: movie.ID, Title: movie.Title, LibraryID: "movies"})
	updated := model.(Model)
	if visible := updated.ColumnStack.Top().SelectedMediaItem(); visible == nil || !visible.IsPlayed {
		t.Fatal("visible movie was not marked watched")
	}
	cached, ok := cache.GetMovies("movies")
	if !ok || len(cached) != 1 || !cached[0].IsPlayed {
		t.Fatal("cached movie was not marked watched")
	}
}

func TestPlaybackStatusTextIncludesEpisodeShowAndElapsedTime(t *testing.T) {
	item := domain.MediaItem{Title: "Pilot", ShowTitle: "Example Show"}
	got := playbackStatusText(item, 65*time.Minute)
	if got != "Pilot - Example Show (01:05)" {
		t.Fatalf("playback status = %q", got)
	}
}

func TestModelPropagateWatchStatus(t *testing.T) {
	m := &Model{
		ColumnStack: NewColumnStack(),
	}

	// Setup stack: Shows -> Seasons -> Episodes
	showCol := components.NewListColumn(components.ColumnTypeShows, "Shows")
	show := &domain.Show{ID: "show1", UnwatchedCount: 10, EpisodeCount: 10}
	showCol.SetItems([]*domain.Show{show})
	m.ColumnStack.Push(showCol, 0)

	seasonCol := components.NewListColumn(components.ColumnTypeSeasons, "Seasons")
	season := &domain.Season{ID: "season1", UnwatchedCount: 5, EpisodeCount: 5}
	seasonCol.SetItems([]*domain.Season{season})
	m.ColumnStack.Push(seasonCol, 0)

	episodeCol := components.NewListColumn(components.ColumnTypeEpisodes, "Episodes")
	episode := &domain.MediaItem{
		ID:       "ep1",
		Type:     domain.MediaTypeEpisode,
		ShowID:   "show1",
		ParentID: "season1",
	}
	episodeCol.SetItems([]*domain.MediaItem{episode})
	m.ColumnStack.Push(episodeCol, 0)

	// Test: Mark episode as watched (watched=true, delta=-1)
	m.propagateWatchStatus(episode, true)

	if show.UnwatchedCount != 9 {
		t.Errorf("expected show unwatched 9, got %d", show.UnwatchedCount)
	}
	if season.UnwatchedCount != 4 {
		t.Errorf("expected season unwatched 4, got %d", season.UnwatchedCount)
	}

	// Test: Mark episode as unwatched (watched=false, delta=+1)
	m.propagateWatchStatus(episode, false)

	if show.UnwatchedCount != 10 {
		t.Errorf("expected show unwatched 10, got %d", show.UnwatchedCount)
	}
	if season.UnwatchedCount != 5 {
		t.Errorf("expected season unwatched 5, got %d", season.UnwatchedCount)
	}
}

func TestRefreshLibrariesPreservesValidNavigationStack(t *testing.T) {
	root := components.NewLibraryColumn([]domain.Library{{ID: "lib-1", Name: "Movies", Type: "movie"}})
	content := components.NewListColumn(components.ColumnTypeMovies, "Movies")
	stack := NewColumnStack()
	stack.Push(root, 0)
	stack.Push(content, 0)

	m := Model{ColumnStack: stack, currentLibID: "lib-1"}
	model, _ := m.Update(LibrariesLoadedMsg{
		Libraries: []domain.Library{{ID: "lib-1", Name: "Renamed", Type: "movie"}},
		Refresh:   true,
	})
	updated := model.(Model)
	if updated.ColumnStack.Len() != 2 || updated.ColumnStack.Get(1) != content {
		t.Fatal("refresh discarded valid deeper navigation")
	}
}

func TestRefreshLibrariesBatchesPosterRefresh(t *testing.T) {
	root := components.NewLibraryColumn([]domain.Library{{ID: "lib-1", Name: "Shows", Type: "show"}})
	content := components.NewListColumn(components.ColumnTypeShows, "Shows")
	content.SetItems([]*domain.Show{{ID: "show-1", ThumbURL: "https://media/poster"}})
	stack := NewColumnStack()
	stack.Push(root, 0)
	stack.Push(content, 0)

	m := Model{
		ColumnStack:  stack,
		Inspector:    components.NewInspector(),
		MediaClient:  &posterClientStub{},
		currentLibID: "lib-1",
		Width:        100,
		Height:       30,
	}
	_, cmd := m.Update(LibrariesLoadedMsg{
		Libraries: []domain.Library{{ID: "lib-1", Name: "Shows", Type: "show"}},
		Refresh:   true,
	})
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 3 {
		t.Fatalf("refresh command = %T with %d batched commands, want 3", batch, len(batch))
	}
}

func TestRefreshLibrariesResetsStackWhenCurrentLibraryDisappears(t *testing.T) {
	stack := NewColumnStack()
	stack.Push(components.NewLibraryColumn([]domain.Library{{ID: "lib-1", Name: "Movies", Type: "movie"}}), 0)
	stack.Push(components.NewListColumn(components.ColumnTypeMovies, "Movies"), 0)

	m := Model{ColumnStack: stack, currentLibID: "lib-1", currentShowID: "show-1"}
	model, _ := m.Update(LibrariesLoadedMsg{Refresh: true})
	updated := model.(Model)
	if updated.ColumnStack.Len() != 1 || updated.currentLibID != "" || updated.currentShowID != "" {
		t.Fatal("refresh retained navigation for a removed library")
	}
}

func TestLibrarySyncProgressDropsStaleGeneration(t *testing.T) {
	m := Model{
		SyncGen:      2,
		SyncingCount: 1,
		LibraryStates: map[string]components.LibrarySyncState{
			"lib-1": {Status: components.StatusSyncing},
		},
	}
	model, cmd := m.Update(LibrarySyncProgressMsg{
		LibraryID:  "lib-1",
		Generation: 1,
		Done:       true,
	})
	updated := model.(Model)
	if cmd != nil || updated.SyncingCount != 1 || updated.LibraryStates["lib-1"].Status != components.StatusSyncing {
		t.Fatal("stale sync progress mutated current generation")
	}
}

func TestCachedNavPlanKeepsPosterCommand(t *testing.T) {
	m := Model{
		ColumnStack: NewColumnStack(),
		Inspector:   components.NewInspector(),
		MediaClient: &posterClientStub{},
		Width:       100,
		Height:      30,
		navPlan: &NavPlan{
			Targets:   []NavTarget{{ID: "show-1"}},
			AwaitKind: AwaitShows,
			AwaitID:   "lib-1",
		},
	}
	result := m.pushAndLoadColumn(columnLoadSpec{
		colType:   components.ColumnTypeShows,
		name:      "Shows",
		awaitKind: AwaitShows,
		awaitID:   "lib-1",
		getCached: func() interface{} {
			return []*domain.Show{{ID: "show-1", ThumbURL: "https://media/poster"}}
		},
	}, 0)
	if result == nil || result.Cmd == nil {
		t.Fatal("cached nav-plan path discarded poster command")
	}
}

func TestUpdateInspectorReloadsPosterWhenRequestKeyChanges(t *testing.T) {
	col := components.NewListColumn(components.ColumnTypeShows, "Shows")
	col.SetItems([]*domain.Show{{ID: "show-1", ThumbURL: "https://media/poster-a"}})
	col.SetSize(50, 20)

	m := Model{
		ColumnStack: NewColumnStack(),
		Inspector:   components.NewInspector(),
		MediaClient: &posterClientStub{},
	}
	m.ColumnStack.Push(col, 0)

	if cmd := m.updateInspector(); cmd == nil {
		t.Fatal("expected a poster request for the initial selection")
	}
	firstRequestID := m.posterRequestID

	col.SetItems([]*domain.Show{{ID: "show-1", ThumbURL: "https://media/poster-b"}})
	m.posterContent = "old poster"
	if cmd := m.updateInspector(); cmd == nil {
		t.Fatal("expected a new poster request after the URL changed")
	}
	if m.posterRequestID == firstRequestID {
		t.Fatal("poster request generation did not advance")
	}
	if m.posterContent != "old poster" {
		t.Fatal("existing poster was cleared while its replacement was loading")
	}
}

func TestUpdateInspectorDoesNotReloadSidebarPosterWhenWidthChanges(t *testing.T) {
	col := components.NewListColumn(components.ColumnTypeShows, "Shows")
	col.SetItems([]*domain.Show{{ID: "show-1", ThumbURL: "https://media/poster"}})
	col.SetSize(40, 20)

	m := Model{
		ColumnStack: NewColumnStack(),
		Inspector:   components.NewInspector(),
		MediaClient: &posterClientStub{},
	}
	m.ColumnStack.Push(col, 0)

	if cmd := m.updateInspector(); cmd == nil {
		t.Fatal("expected a poster request for the initial selection")
	}
	firstRequestID := m.posterRequestID

	col.SetSize(70, 20)
	if cmd := m.updateInspector(); cmd != nil {
		t.Fatal("unexpected poster request after a width change")
	}
	if m.posterRequestID != firstRequestID {
		t.Fatal("poster request generation changed after resize")
	}
}

func TestUpdateInspectorDoesNotRequestPosterForEpisodePane(t *testing.T) {
	col := components.NewListColumn(components.ColumnTypeEpisodes, "Episodes")
	col.SetItems([]*domain.MediaItem{{ID: "episode-1", Type: domain.MediaTypeEpisode, ShowThumbURL: "https://media/poster"}})

	m := Model{
		ColumnStack:      NewColumnStack(),
		Inspector:        components.NewInspector(),
		MediaClient:      &posterClientStub{},
		posterItemID:     "show-1",
		posterContent:    "old poster",
		posterRequestKey: "show-1\x00old-url\x0020\x0010",
	}
	m.ColumnStack.Push(col, 0)

	if cmd := m.updateInspector(); cmd != nil {
		t.Fatal("unexpected poster request for episode pane")
	}
	if m.hasPosterState() {
		t.Fatal("episode pane retained a show poster")
	}
}

func TestUpdateInspectorRetainsShowPreviewInEpisodePane(t *testing.T) {
	showCol := components.NewListColumn(components.ColumnTypeShows, "Shows")
	showCol.SetItems([]*domain.Show{{ID: "show-1", ThumbURL: "https://media/show-poster"}})
	episodeCol := components.NewListColumn(components.ColumnTypeEpisodes, "Episodes")
	episodeCol.SetItems([]*domain.MediaItem{{ID: "episode-1", Type: domain.MediaTypeEpisode}})

	m := Model{
		ColumnStack: NewColumnStack(),
		Inspector:   components.NewInspector(),
		MediaClient: &posterClientStub{},
	}
	m.ColumnStack.Push(showCol, 0)
	if cmd := m.updateInspector(); cmd == nil {
		t.Fatal("expected initial show poster request")
	}
	m.posterContent = "show poster"
	m.ColumnStack.Push(episodeCol, 0)

	if cmd := m.updateInspector(); cmd != nil {
		t.Fatal("unexpected poster request after opening episode pane")
	}
	if m.posterItemID != "show-1" || m.posterContent != "show poster" {
		t.Fatal("episode pane did not retain the selected show's preview")
	}
}

func TestPosterLoadedIgnoresStaleRequest(t *testing.T) {
	m := Model{
		posterRequestID: 2,
		posterItemID:    "movie-1",
		posterContent:   "current poster",
	}

	model, _ := m.Update(PosterLoadedMsg{
		RequestID: 1,
		ItemID:    "movie-1",
		Content:   "stale poster",
	})
	updated := model.(Model)
	if updated.posterContent != "current poster" {
		t.Fatal("stale poster request replaced the current poster")
	}
}

func TestPosterFailureClearsRetainedPreviewAfterRequestCompletes(t *testing.T) {
	var output bytes.Buffer
	m := Model{
		posterRequestID:  3,
		posterItemID:     "movie-2",
		posterContent:    "previous poster",
		posterPlacement:  "previous placement",
		posterImageID:    42,
		posterOutput:     &output,
		posterRequestKey: "movie-2\x00url\x0030\x0020",
	}

	model, _ := m.Update(PosterLoadedMsg{RequestID: 3, ItemID: "movie-2"})
	updated := model.(Model)
	if updated.posterContent != "" || updated.posterPlacement != "" || updated.posterImageID != 0 {
		t.Fatal("failed replacement left the previous preview visible")
	}
	if output.Len() == 0 {
		t.Fatal("failed replacement did not delete the previous kitty image")
	}
}

func TestUpdateInspectorClearsPosterWithoutArtwork(t *testing.T) {
	col := components.NewListColumn(components.ColumnTypeShows, "Shows")
	col.SetItems([]*domain.Show{{ID: "show-1"}})

	m := Model{
		ColumnStack:      NewColumnStack(),
		Inspector:        components.NewInspector(),
		posterItemID:     "old-show",
		posterContent:    "old poster",
		posterRequestKey: "old-show\x00old-url\x0020\x000",
	}
	m.ColumnStack.Push(col, 0)

	if cmd := m.updateInspector(); cmd != nil {
		t.Fatal("unexpected poster request for an item without artwork")
	}
	if m.posterItemID != "" || m.posterContent != "" || m.posterRequestKey != "" {
		t.Fatal("old poster state was not cleared")
	}
}

func TestPosterMaxHeightFitsPreviewPane(t *testing.T) {
	m := Model{Height: 30}
	if got, want := m.posterMaxHeight(), 11; got != want {
		t.Fatalf("poster maxHeight = %d, want %d", got, want)
	}
}
