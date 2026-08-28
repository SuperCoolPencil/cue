package library

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/store"
)

type fakeLibraryClient struct {
	libs          []domain.Library
	moviePages    [][]*domain.MediaItem
	showPages     [][]*domain.Show
	mixedPages    [][]domain.ListItem
	seasons       []*domain.Season
	episodes      []*domain.MediaItem
	episodesMap   map[string][]*domain.MediaItem // seasonID -> episodes
	continueItems []*domain.MediaItem
	itemCount     int
	countErr      error
	libraryCalls  int
	movieCalls    int
	countCalls    int
}

func (f *fakeLibraryClient) GetLibraries(context.Context) ([]domain.Library, error) {
	f.libraryCalls++
	return f.libs, nil
}

func (f *fakeLibraryClient) GetMovies(_ context.Context, _ string, offset, limit int) ([]*domain.MediaItem, int, error) {
	f.movieCalls++
	idx := offset / limit
	if idx >= len(f.moviePages) {
		return nil, len(flattenMovies(f.moviePages)), nil
	}
	return f.moviePages[idx], len(flattenMovies(f.moviePages)), nil
}

func (f *fakeLibraryClient) GetShows(_ context.Context, _ string, _, _ int) ([]*domain.Show, int, error) {
	return flattenShows(f.showPages), len(flattenShows(f.showPages)), nil
}

func (f *fakeLibraryClient) GetMixedContent(_ context.Context, _ string, _, _ int) ([]domain.ListItem, int, error) {
	return flattenMixed(f.mixedPages), len(flattenMixed(f.mixedPages)), nil
}

func (f *fakeLibraryClient) GetSeasons(context.Context, string) ([]*domain.Season, error) {
	return f.seasons, nil
}

func (f *fakeLibraryClient) GetEpisodes(_ context.Context, seasonID string) ([]*domain.MediaItem, error) {
	if f.episodesMap != nil {
		if eps, ok := f.episodesMap[seasonID]; ok {
			return eps, nil
		}
	}
	return f.episodes, nil
}

func (f *fakeLibraryClient) GetContinueWatching(context.Context) ([]*domain.MediaItem, error) {
	return f.continueItems, nil
}

func (f *fakeLibraryClient) GetLibraryItemCount(context.Context, string, string) (int, error) {
	f.countCalls++
	return f.itemCount, f.countErr
}

func TestFetchLibrariesSavesToStore(t *testing.T) {
	st, _ := store.NewLibraryStore("", "", "")
	client := &fakeLibraryClient{libs: []domain.Library{{ID: "lib", Name: "Movies", Type: "movie"}}}
	svc := NewService(client, st, slog.Default())

	libs, err := svc.FetchLibraries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(libs) != 1 || client.libraryCalls != 1 {
		t.Fatalf("libs=%#v calls=%d", libs, client.libraryCalls)
	}
	cached, ok := st.GetLibraries()
	if !ok || len(cached) != 1 || cached[0].ID != "lib" {
		t.Fatalf("cached libraries = %#v, %v", cached, ok)
	}
}

func TestSyncLibraryUsesFreshCache(t *testing.T) {
	st, _ := store.NewLibraryStore("", "", "")
	if err := st.SaveMovies("lib", []*domain.MediaItem{{ID: "cached"}}, 100); err != nil {
		t.Fatal(err)
	}
	client := &fakeLibraryClient{itemCount: 1}
	svc := NewService(client, st, slog.Default())

	result, err := svc.SyncLibrary(context.Background(), domain.Library{ID: "lib", Type: "movie", UpdatedAt: 50}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.FromCache || result.Count != 1 || client.movieCalls != 0 {
		t.Fatalf("result=%#v calls=%d", result, client.movieCalls)
	}
}

func TestSyncLibraryRefetchesWhenServerCountChanges(t *testing.T) {
	st, _ := store.NewLibraryStore("", "", "")
	if err := st.SaveMovies("lib", []*domain.MediaItem{{ID: "cached"}}, 100); err != nil {
		t.Fatal(err)
	}
	client := &fakeLibraryClient{
		itemCount:  2,
		moviePages: [][]*domain.MediaItem{{{ID: "cached"}, {ID: "new"}}},
	}
	svc := NewService(client, st, slog.Default())

	result, err := svc.SyncLibrary(context.Background(), domain.Library{ID: "lib", Type: "movie", UpdatedAt: 50}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.FromCache || result.Count != 2 || client.movieCalls != 1 {
		t.Fatalf("result=%#v fetch calls=%d", result, client.movieCalls)
	}
}

func TestSyncLibraryRetainsCacheWhenCountValidationFails(t *testing.T) {
	st, _ := store.NewLibraryStore("", "", "")
	if err := st.SaveMovies("lib", []*domain.MediaItem{{ID: "cached"}}, 100); err != nil {
		t.Fatal(err)
	}
	client := &fakeLibraryClient{countErr: errors.New("offline")}
	svc := NewService(client, st, slog.Default())

	result, err := svc.SyncLibrary(context.Background(), domain.Library{ID: "lib", Type: "movie", UpdatedAt: 50}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.FromCache || result.Count != 1 || client.movieCalls != 0 {
		t.Fatalf("result=%#v fetch calls=%d", result, client.movieCalls)
	}
}

func TestFetchMoviesPaginatesAndReportsProgress(t *testing.T) {
	st, _ := store.NewLibraryStore("", "", "")
	client := &fakeLibraryClient{moviePages: [][]*domain.MediaItem{
		{{ID: "1"}},
		{{ID: "2"}},
		{{ID: "3"}},
	}}
	svc := NewService(client, st, slog.Default())
	var progress []int

	movies, err := svc.FetchMovies(context.Background(), "lib", 0, func(loaded, total int) {
		progress = append(progress, loaded)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(movies) != 3 || client.movieCalls != 3 {
		t.Fatalf("movies=%#v calls=%d", movies, client.movieCalls)
	}
	if len(progress) != 3 || progress[0] != 1 || progress[2] != 3 {
		t.Fatalf("progress=%v", progress)
	}
}

func TestFetchContinueWatching(t *testing.T) {
	st, _ := store.NewLibraryStore("", "", "")
	expected := []*domain.MediaItem{{ID: "1", Title: "In Progress"}}
	client := &fakeLibraryClient{continueItems: expected}
	svc := NewService(client, st, slog.Default())

	items, err := svc.FetchContinueWatching(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "In Progress" {
		t.Fatalf("unexpected items: %#v", items)
	}
}

func TestFetchAllDeduplicatesPages(t *testing.T) {
	pages := [][]*domain.MediaItem{
		{{ID: "a"}, {ID: "b"}},
		{{ID: "b"}, {ID: "c"}},
	}
	items, err := fetchAll(context.Background(), func(_ context.Context, offset, limit int) ([]*domain.MediaItem, int, error) {
		index := offset / limit
		if index >= len(pages) {
			return nil, 4, nil
		}
		return pages[index], 4, nil
	}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("deduplicated item count = %d, want 3", len(items))
	}
}

func TestFetchAllContinuesWhenTotalIsZero(t *testing.T) {
	pages := [][]*domain.MediaItem{
		{{ID: "a"}, {ID: "b"}},
		{{ID: "c"}},
	}
	items, err := fetchAll(context.Background(), func(_ context.Context, offset, limit int) ([]*domain.MediaItem, int, error) {
		index := offset / limit
		if index >= len(pages) {
			return nil, 0, nil
		}
		return pages[index], 0, nil
	}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("item count = %d, want 3", len(items))
	}
}

func flattenMovies(pages [][]*domain.MediaItem) []*domain.MediaItem {
	var out []*domain.MediaItem
	for _, page := range pages {
		out = append(out, page...)
	}
	return out
}

func flattenShows(pages [][]*domain.Show) []*domain.Show {
	var out []*domain.Show
	for _, page := range pages {
		out = append(out, page...)
	}
	return out
}

func flattenMixed(pages [][]domain.ListItem) []domain.ListItem {
	var out []domain.ListItem
	for _, page := range pages {
		out = append(out, page...)
	}
	return out
}
