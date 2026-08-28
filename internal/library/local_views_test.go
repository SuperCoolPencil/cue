package library

import (
	"log/slog"
	"testing"
	"time"

	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/store"
)

func TestLocalViews(t *testing.T) {
	st, err := store.NewLibraryStore("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	libs := []domain.Library{{ID: "movies", Name: "Movies", Type: "movie"}}
	if err := st.SaveLibraries(libs); err != nil {
		t.Fatal(err)
	}
	items := []*domain.MediaItem{
		{ID: "old", Title: "Old", AddedAt: 10, UpdatedAt: 10, IsPlayed: true},
		{ID: "recent", Title: "Recent", AddedAt: 30, UpdatedAt: 30, Rating: 8.5, Height: 2160},
		{ID: "progress", Title: "Progress", AddedAt: 20, UpdatedAt: 40, ViewOffset: 5 * time.Minute},
	}
	if err := st.SaveMovies("movies", items, 1); err != nil {
		t.Fatal(err)
	}

	svc := NewService(nil, st, slog.Default())

	continueItems := svc.ContinueWatching(0)
	if len(continueItems) != 1 || continueItems[0].ID != "progress" {
		t.Fatalf("continue watching = %#v", continueItems)
	}

	recent := svc.RecentlyAdded(2)
	if len(recent) != 2 || recent[0].GetID() != "recent" || recent[1].GetID() != "progress" {
		t.Fatalf("recent = %#v", recent)
	}

	filtered := svc.SmartFiltered("4k", 0)
	if len(filtered) != 1 || filtered[0].ID != "recent" {
		t.Fatalf("4k filter = %#v", filtered)
	}
}

func TestSmartFilteredShows(t *testing.T) {
	st, err := store.NewLibraryStore("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	libs := []domain.Library{{ID: "shows", Name: "Shows", Type: "show"}}
	if err := st.SaveLibraries(libs); err != nil {
		t.Fatal(err)
	}

	shows := []*domain.Show{{ID: "show1", Title: "Show 1", LibraryID: "shows"}}
	if err := st.SaveShows("shows", shows, 1); err != nil {
		t.Fatal(err)
	}

	seasons := []*domain.Season{{ID: "season1", ShowID: "show1", SeasonNum: 1}}
	if err := st.SaveSeasons("shows", "show1", seasons); err != nil {
		t.Fatal(err)
	}

	episodes := []*domain.MediaItem{
		{ID: "ep1", Title: "Episode 1", Type: domain.MediaTypeEpisode, Height: 2160, ShowID: "show1"},
	}
	if err := st.SaveEpisodes("shows", "show1", "season1", episodes); err != nil {
		t.Fatal(err)
	}

	svc := NewService(nil, st, slog.Default())

	filtered := svc.SmartFiltered("4k", 0)
	if len(filtered) != 1 || filtered[0].GetID() != "ep1" {
		t.Fatalf("4k filter with shows = %#v", filtered)
	}
}

func TestRecentlyAddedMixed(t *testing.T) {
	st, err := store.NewLibraryStore("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	libs := []domain.Library{
		{ID: "movies", Name: "Movies", Type: "movie"},
		{ID: "shows", Name: "Shows", Type: "show"},
	}
	if err := st.SaveLibraries(libs); err != nil {
		t.Fatal(err)
	}

	m1 := &domain.MediaItem{ID: "m1", Title: "Movie 1", AddedAt: 100, Type: domain.MediaTypeMovie}
	s1 := &domain.Show{ID: "s1", Title: "Show 1", AddedAt: 200}
	m2 := &domain.MediaItem{ID: "m2", Title: "Movie 2", AddedAt: 150, Type: domain.MediaTypeMovie}

	if err := st.SaveMovies("movies", []*domain.MediaItem{m1, m2}, 1); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveShows("shows", []*domain.Show{s1}, 1); err != nil {
		t.Fatal(err)
	}

	svc := NewService(nil, st, slog.Default())
	recent := svc.RecentlyAdded(0)

	if len(recent) != 3 {
		t.Fatalf("expected 3 items, got %d", len(recent))
	}

	// Should be ordered by AddedAt desc: s1 (200), m2 (150), m1 (100)
	if recent[0].GetID() != "s1" || recent[1].GetID() != "m2" || recent[2].GetID() != "m1" {
		t.Errorf("incorrect order: 0=%s, 1=%s, 2=%s", recent[0].GetID(), recent[1].GetID(), recent[2].GetID())
	}

	// Verify types
	if recent[0].GetItemType() != "show" {
		t.Errorf("expected index 0 to be show, got %s", recent[0].GetItemType())
	}
	if recent[1].GetItemType() != "movie" {
		t.Errorf("expected index 1 to be movie, got %s", recent[1].GetItemType())
	}
}
