package components

import (
	"strings"
	"testing"

	"github.com/SuperCoolPencil/cue/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

func TestListColumnSortSelectionAndFilter(t *testing.T) {
	col := NewListColumn(ColumnTypeMovies, "Movies")
	col.SetFocused(true)
	col.SetItems([]*domain.MediaItem{
		{ID: "old", Title: "Old", AddedAt: 10},
		{ID: "new", Title: "New", AddedAt: 30},
		{ID: "middle", Title: "Middle", AddedAt: 20},
	})

	col.ApplySort(SortDateAdded, SortDesc)
	if item := col.SelectedMediaItem(); item == nil || item.ID != "new" {
		t.Fatalf("selected after sort = %#v", item)
	}

	col.ToggleFilter()
	col, _ = col.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("mid")})
	if col.ItemCount() != 1 {
		t.Fatalf("filtered count = %d", col.ItemCount())
	}
	if item := col.SelectedMediaItem(); item == nil || item.ID != "middle" {
		t.Fatalf("selected after filter = %#v", item)
	}
	col.ClearFilter()
	if col.ItemCount() != 3 {
		t.Fatalf("count after clear = %d", col.ItemCount())
	}
}

func TestListColumnSetSelectedByIDHonorsSort(t *testing.T) {
	col := NewListColumn(ColumnTypeMovies, "Movies")
	col.SetItems([]*domain.MediaItem{
		{ID: "b", Title: "B"},
		{ID: "a", Title: "A"},
	})
	col.ApplySort(SortTitle, SortAsc)
	if !col.SetSelectedByID("b") {
		t.Fatal("expected to select existing item")
	}
	if item := col.SelectedMediaItem(); item == nil || item.ID != "b" {
		t.Fatalf("selected = %#v", item)
	}
	if col.SetSelectedByID("missing") {
		t.Fatal("missing id should not select")
	}
}

func TestListColumnHideWatched(t *testing.T) {
	col := NewListColumn(ColumnTypeMovies, "Movies")
	col.SetItems([]*domain.MediaItem{
		{ID: "watched", Title: "Watched", IsPlayed: true},
		{ID: "unwatched", Title: "Unwatched", IsPlayed: false},
	})

	if col.ItemCount() != 2 {
		t.Fatalf("initial count = %d", col.ItemCount())
	}

	col.SetHideWatched(true)
	if col.ItemCount() != 1 {
		t.Fatalf("count after hide watched = %d", col.ItemCount())
	}
	if item := col.SelectedMediaItem(); item == nil || item.ID != "unwatched" {
		t.Fatalf("selected after hide watched = %#v", item)
	}

	col.SetHideWatched(false)
	if col.ItemCount() != 2 {
		t.Fatalf("count after show watched = %d", col.ItemCount())
	}
}

func TestGetSeasonPlaylist(t *testing.T) {
	col := NewListColumn(ColumnTypeEpisodes, "Episodes")
	episodes := []*domain.MediaItem{
		{ID: "ep1", Title: "Ep 1", Type: domain.MediaTypeEpisode, IsPlayed: true},
		{ID: "ep2", Title: "Ep 2", Type: domain.MediaTypeEpisode, IsPlayed: false},
		{ID: "ep3", Title: "Ep 3", Type: domain.MediaTypeEpisode, IsPlayed: false},
	}
	col.SetItems(episodes)

	// Hide watched (Ep 1)
	col.SetHideWatched(true)

	// Select Ep 2 - should be at cursor 0 now
	if !col.SetSelectedByID("ep2") {
		t.Fatal("failed to select ep2 after hide watched")
	}

	if col.ItemCount() != 2 {
		t.Fatalf("visible count = %d, expected 2", col.ItemCount())
	}

	// Get playlist - should include ALL 3 episodes
	playlist, idx := col.GetSeasonPlaylist()
	if len(playlist) != 3 {
		t.Fatalf("playlist length = %d, expected 3", len(playlist))
	}
	if idx != 1 {
		t.Fatalf("selected index = %d, expected 1 (Ep 2), got %d", idx, idx)
	}
	if playlist[0].ID != "ep1" {
		t.Errorf("expected ep1 at index 0, got %s", playlist[0].ID)
	}
}

func TestListColumnSetItemsPreservesSelection(t *testing.T) {
	col := NewListColumn(ColumnTypeMovies, "Movies")
	col.SetItems([]*domain.MediaItem{
		{ID: "1", Title: "Movie 1"},
		{ID: "2", Title: "Movie 2"},
		{ID: "3", Title: "Movie 3"},
	})

	// Select second item
	col.SetSelectedIndex(1)
	if col.SelectedMediaItem().ID != "2" {
		t.Fatalf("expected ID 2, got %s", col.SelectedMediaItem().ID)
	}

	// Update items with same IDs but different titles/status
	col.SetItems([]*domain.MediaItem{
		{ID: "1", Title: "Movie 1 (Updated)"},
		{ID: "2", Title: "Movie 2 (Updated)"},
		{ID: "3", Title: "Movie 3 (Updated)"},
	})

	// Cursor should still be at 1, pointing to ID "2"
	if col.SelectedIndex() != 1 {
		t.Fatalf("expected cursor 1, got %d", col.SelectedIndex())
	}
	if col.SelectedMediaItem().ID != "2" {
		t.Fatalf("expected ID 2, got %s", col.SelectedMediaItem().ID)
	}

	// Update with different order
	col.SetItems([]*domain.MediaItem{
		{ID: "3", Title: "Movie 3"},
		{ID: "1", Title: "Movie 1"},
		{ID: "2", Title: "Movie 2"},
	})

	// ID "2" ("Movie 2") should be at display index 1 due to Title Asc sorting
	// (ID 1: "Movie 1", ID 2: "Movie 2", ID 3: "Movie 3")
	if col.SelectedIndex() != 1 {
		t.Fatalf("expected cursor 1, got %d", col.SelectedIndex())
	}
	if col.SelectedMediaItem().ID != "2" {
		t.Fatalf("expected ID 2, got %s", col.SelectedMediaItem().ID)
	}
}

func replacementMovies(titles ...string) []*domain.MediaItem {
	items := make([]*domain.MediaItem, len(titles))
	for i, title := range titles {
		items[i] = &domain.MediaItem{ID: "id-" + title, Title: title, Type: domain.MediaTypeMovie}
	}
	return items
}

func TestReplaceItemsPreservesViewState(t *testing.T) {
	col := NewListColumn(ColumnTypeMovies, "Movies")
	col.SetSize(40, 20)
	col.SetItems(replacementMovies("Alpha", "Bravo", "Charlie"))
	col.ApplySort(SortTitle, SortDesc)
	if !col.SetSelectedByID("id-Bravo") {
		t.Fatal("failed to select Bravo")
	}
	col.SetRefreshing(true)

	col.ReplaceItems(replacementMovies("Alpha", "Anchor", "Bravo", "Charlie"))

	if got := col.SelectedMediaItem(); got == nil || got.ID != "id-Bravo" {
		t.Fatalf("selection after replacement = %#v", got)
	}
	field, direction := col.SortState()
	if field != SortTitle || direction != SortDesc {
		t.Fatalf("sort after replacement = %v, %v", field, direction)
	}
	if col.IsRefreshing() {
		t.Fatal("refresh flag was not cleared")
	}
}

func TestReplaceItemsClampsMissingSelection(t *testing.T) {
	col := NewListColumn(ColumnTypeMovies, "Movies")
	col.SetItems(replacementMovies("Alpha", "Bravo", "Charlie"))
	col.SetSelectedIndex(2)

	col.ReplaceItems(replacementMovies("Alpha", "Bravo"))

	if got := col.SelectedIndex(); got != 1 {
		t.Fatalf("selected index = %d, want 1", got)
	}
}

func TestListColumnEpisodeShowTitle(t *testing.T) {
	ep := &domain.MediaItem{
		ID:         "ep1",
		Title:      "Pilot",
		Type:       domain.MediaTypeEpisode,
		ShowTitle:  "Breaking Bad",
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	col := NewListColumn(ColumnTypeEpisodes, "Episodes")
	col.SetItems([]*domain.MediaItem{ep})

	// Without the flag, only the episode title is shown (with episode code)
	got := col.renderEpisodeItem(*ep, false, 80)
	if !strings.Contains(got, "Pilot") || strings.Contains(got, "Breaking Bad") {
		t.Errorf("expected episode title only without flag, got %q", got)
	}

	// With the flag, the series name prefixes the episode info
	col.SetShowShowTitle(true)
	got = col.renderEpisodeItem(*ep, false, 80)
	want := "Breaking Bad - S01E01 Pilot"
	if !strings.Contains(got, want) {
		t.Errorf("expected rendered line to contain %q, got %q", want, got)
	}
}

func TestListColumnContinueWatchingMixedItemsShowEpisodeSeries(t *testing.T) {
	items := []*domain.MediaItem{
		{ID: "movie1", Title: "A Movie", Type: domain.MediaTypeMovie},
		{
			ID:         "ep1",
			Title:      "Pilot",
			Type:       domain.MediaTypeEpisode,
			ShowTitle:  "Breaking Bad",
			SeasonNum:  1,
			EpisodeNum: 1,
		},
	}
	col := NewListColumn(ColumnTypeMixed, "Continue Watching")
	col.SetShowShowTitle(true)
	col.SetItems(items)

	if col.ColumnType() != ColumnTypeMixed {
		t.Fatalf("expected mixed column for movie-and-episode list, got %v", col.ColumnType())
	}

	got := col.renderItem(1, false, 80)
	want := "Breaking Bad - S01E01 Pilot"
	if !strings.Contains(got, want) {
		t.Errorf("expected rendered line to contain %q, got %q", want, got)
	}
}

// TestListColumnContinueWatchingShowTitle reproduces the real runtime path:
// the Continue Watching column is created as Mixed, SetShowShowTitle(true) is set,
// then SetItems coerces the column type to Episodes (episode-only list). The
// dispatched renderer must still show the series name.
func TestListColumnContinueWatchingShowTitle(t *testing.T) {
	eps := []*domain.MediaItem{
		{
			ID:         "ep1",
			Title:      "Pilot",
			Type:       domain.MediaTypeEpisode,
			ShowTitle:  "Breaking Bad",
			SeasonNum:  1,
			EpisodeNum: 1,
		},
	}
	col := NewListColumn(ColumnTypeMixed, "Continue Watching")
	col.SetShowShowTitle(true)
	col.SetItems(eps)

	if col.ColumnType() != ColumnTypeEpisodes {
		t.Fatalf("expected ColumnTypeEpisodes after SetItems (real path), got %v", col.ColumnType())
	}

	got := col.renderItem(0, false, 80)
	want := "Breaking Bad - S01E01 Pilot"
	if !strings.Contains(got, want) {
		t.Errorf("expected drilled Continue Watching row to contain %q, got %q", want, got)
	}
}
