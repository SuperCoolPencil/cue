package store

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/SuperCoolPencil/cue/internal/domain"
)

func TestInvalidateAllPreservesLocalQueue(t *testing.T) {
	for _, persistent := range []bool{false, true} {
		name, base := "memory", ""
		if persistent {
			name, base = "disk", t.TempDir()
		}
		t.Run(name, func(t *testing.T) {
			st, err := NewLibraryStore(base, "server", "user")
			if err != nil {
				t.Fatal(err)
			}
			queue := []*domain.MediaItem{{ID: "queued-movie"}}
			if err := st.SaveQueueItems(queue); err != nil {
				t.Fatal(err)
			}
			if err := st.SaveMovies("movies", queue, 1); err != nil {
				t.Fatal(err)
			}
			st.InvalidateAll()
			if _, ok := st.GetMovies("movies"); ok {
				t.Fatal("server content survived invalidation")
			}
			if persistent {
				if err := st.Close(); err != nil {
					t.Fatal(err)
				}
				st, err = NewLibraryStore(base, "server", "user")
				if err != nil {
					t.Fatal(err)
				}
			}
			defer func() { _ = st.Close() }()
			items, ok := st.GetQueueItems()
			if !ok || len(items) != 1 || items[0].ID != queue[0].ID {
				t.Fatalf("queue lost: %#v", items)
			}
			st.InvalidateQueue()
			if _, ok := st.GetQueueItems(); ok {
				t.Fatal("explicit queue clearing failed")
			}
		})
	}
}

func TestPlaybackProgressUpdatesEveryCachedCopy(t *testing.T) {
	st, err := NewLibraryStore(t.TempDir(), "server", "user")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	item := &domain.MediaItem{ID: "item", Type: domain.MediaTypeEpisode}
	if err := st.SaveMovies("lib", []*domain.MediaItem{item}, 1); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveMixedContent("mixed", []domain.ListItem{item}, 1); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveEpisodes("lib", "show", "season", []*domain.MediaItem{item}); err != nil {
		t.Fatal(err)
	}
	if err := st.SavePlaylistItems("playlist", []*domain.MediaItem{item}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveQueueItems([]*domain.MediaItem{item}); err != nil {
		t.Fatal(err)
	}
	st.SetPlaybackProgress(item.ID, 45*time.Second, 1234)
	movies, _ := st.GetMovies("lib")
	mixed, _ := st.GetMixedContent("mixed")
	episodes, _ := st.GetEpisodes("lib", "show", "season")
	playlist, _ := st.GetPlaylistItems("playlist")
	queue, _ := st.GetQueueItems()
	for _, copy := range []*domain.MediaItem{movies[0], mixed[0].(*domain.MediaItem), episodes[0], playlist[0], queue[0]} {
		if copy.ViewOffset != 45*time.Second || copy.LastPlayedAt != 1234 {
			t.Fatalf("stale playback copy: %#v", copy)
		}
	}
}

func TestLibraryStorePersistsAndInvalidatesContent(t *testing.T) {
	base := t.TempDir()
	st, err := NewLibraryStore(base, "http://server", "user")
	if err != nil {
		t.Fatal(err)
	}

	libs := []domain.Library{{ID: "movies", Name: "Movies", Type: "movie", UpdatedAt: 10}}
	movies := []*domain.MediaItem{{ID: "m1", Title: "Movie"}}
	if err := st.SaveLibraries(libs); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveMovies("movies", movies, 10); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewLibraryStore(base, "http://server", "user")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	gotLibs, ok := reopened.GetLibraries()
	if !ok || len(gotLibs) != 1 || gotLibs[0].ID != "movies" {
		t.Fatalf("libraries = %#v, %v", gotLibs, ok)
	}
	gotMovies, ok := reopened.GetMovies("movies")
	if !ok || len(gotMovies) != 1 || gotMovies[0].ID != "m1" {
		t.Fatalf("movies = %#v, %v", gotMovies, ok)
	}
	if !reopened.IsValid("movies", 10) {
		t.Fatal("cache should be valid at matching timestamp")
	}
	reopened.InvalidateLibrary("movies")
	if _, ok := reopened.GetMovies("movies"); ok {
		t.Fatal("movies should be invalidated")
	}
	if reopened.IsValid("movies", 10) {
		t.Fatal("timestamp should be invalidated")
	}
}

func TestLibraryStoreMixedContentAndQueue(t *testing.T) {
	st, err := NewLibraryStore("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	movie := &domain.MediaItem{ID: "m1", Title: "Movie"}
	show := &domain.Show{ID: "s1", Title: "Show"}
	if err := st.SaveMixedContent("mixed", []domain.ListItem{movie, show}, 1); err != nil {
		t.Fatal(err)
	}
	content, ok := st.GetMixedContent("mixed")
	if !ok || len(content) != 2 {
		t.Fatalf("mixed content = %#v, %v", content, ok)
	}
	if _, ok := content[0].(*domain.MediaItem); !ok {
		t.Fatalf("first item type = %T", content[0])
	}
	if _, ok := content[1].(*domain.Show); !ok {
		t.Fatalf("second item type = %T", content[1])
	}

	if err := st.SaveQueueItems([]*domain.MediaItem{movie}); err != nil {
		t.Fatal(err)
	}
	queue, ok := st.GetQueueItems()
	if !ok || len(queue) != 1 || queue[0].ID != "m1" {
		t.Fatalf("queue = %#v, %v", queue, ok)
	}
	st.InvalidateQueue()
	if _, ok := st.GetQueueItems(); ok {
		t.Fatal("queue should be invalidated")
	}
}

func seedStore(t *testing.T, dir string) *LibraryStore {
	t.Helper()
	s, err := NewLibraryStore(dir, "http://test", "user1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	movie := &domain.MediaItem{ID: "mov1", Title: "Movie One", Type: domain.MediaTypeMovie}
	episode := &domain.MediaItem{
		ID: "ep1", Title: "Episode One", Type: domain.MediaTypeEpisode,
		ShowID: "show1", ParentID: "season1", ViewOffset: 300,
	}
	show := &domain.Show{ID: "show1", Title: "Show One", EpisodeCount: 10, UnwatchedCount: 5}
	season := &domain.Season{ID: "season1", ShowID: "show1", EpisodeCount: 10, UnwatchedCount: 5}

	if err := s.SaveMovies("lib1", []*domain.MediaItem{movie}, 100); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveShows("lib2", []*domain.Show{show}, 100); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSeasons("lib2", "show1", []*domain.Season{season}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveEpisodes("lib2", "show1", "season1", []*domain.MediaItem{episode}); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePlaylistItems("pl1", []*domain.MediaItem{
		{ID: "mov1", Title: "Movie One", Type: domain.MediaTypeMovie},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveQueueItems([]*domain.MediaItem{
		{ID: "mov1", Title: "Movie One", Type: domain.MediaTypeMovie},
	}); err != nil {
		t.Fatal(err)
	}
	return s
}

func testWatchState(t *testing.T, s *LibraryStore) {
	t.Helper()

	// Toggle an episode watched: item patched, season + show counters drop
	s.SetWatchState("ep1", true)

	eps, ok := s.GetEpisodes("lib2", "show1", "season1")
	if !ok || len(eps) != 1 {
		t.Fatal("episodes missing after patch")
	}
	if !eps[0].IsPlayed || eps[0].ViewOffset != 0 {
		t.Fatalf("episode not patched: %+v", eps[0])
	}

	seasons, _ := s.GetSeasons("lib2", "show1")
	if seasons[0].UnwatchedCount != 4 {
		t.Fatalf("season unwatched = %d, want 4", seasons[0].UnwatchedCount)
	}
	shows, _ := s.GetShows("lib2")
	if shows[0].UnwatchedCount != 4 {
		t.Fatalf("show unwatched = %d, want 4", shows[0].UnwatchedCount)
	}

	// Toggling the same state again must not shift counters
	s.SetWatchState("ep1", true)
	seasons, _ = s.GetSeasons("lib2", "show1")
	if seasons[0].UnwatchedCount != 4 {
		t.Fatalf("idempotency broken: season unwatched = %d", seasons[0].UnwatchedCount)
	}

	// Unwatch restores the counters
	s.SetWatchState("ep1", false)
	seasons, _ = s.GetSeasons("lib2", "show1")
	shows, _ = s.GetShows("lib2")
	if seasons[0].UnwatchedCount != 5 || shows[0].UnwatchedCount != 5 {
		t.Fatalf("counters not restored: season=%d show=%d",
			seasons[0].UnwatchedCount, shows[0].UnwatchedCount)
	}

	// A movie is patched in the library list AND its playlist copy
	s.SetWatchState("mov1", true)
	movies, _ := s.GetMovies("lib1")
	if !movies[0].IsPlayed {
		t.Fatal("movie not patched in library list")
	}
	plItems, _ := s.GetPlaylistItems("pl1")
	if !plItems[0].IsPlayed {
		t.Fatal("movie not patched in playlist items")
	}
	queueItems, _ := s.GetQueueItems()
	if !queueItems[0].IsPlayed {
		t.Fatal("movie not patched in queue items")
	}

	// Freshness timestamp untouched — no invalidation happened
	if !s.IsValid("lib1", 100) {
		t.Fatal("watch-state patch must not invalidate the library")
	}
}

func TestSetWatchStateBolt(t *testing.T) {
	testWatchState(t, seedStore(t, t.TempDir()))
}

// Seasons/episodes have no server-side freshness signal; entries past the
// TTL must read as cache misses so drill-downs refetch.
func TestTVCacheTTL(t *testing.T) {
	s := seedStore(t, "")

	// Fresh write: served
	if _, ok := s.GetEpisodes("lib2", "show1", "season1"); !ok {
		t.Fatal("fresh episodes not served")
	}
	if _, ok := s.GetSeasons("lib2", "show1"); !ok {
		t.Fatal("fresh seasons not served")
	}

	// Force-expire the episodes entry by rewriting its wrapper timestamp
	data, _ := json.Marshal([]*domain.MediaItem{{ID: "ep1"}})
	expired := timestamped{
		FetchedAt: time.Now().Add(-tvCacheTTL - time.Hour).Unix(),
		Data:      data,
	}
	if err := s.set(bucketEpisodes, "lib:lib2:show:show1:season:season1", expired); err != nil {
		t.Fatal(err)
	}

	if _, ok := s.GetEpisodes("lib2", "show1", "season1"); ok {
		t.Fatal("expired episodes served from cache")
	}

	// Pre-TTL cache format (bare array, no wrapper) reads as a miss
	if err := s.set(bucketEpisodes, "lib:l:show:s:season:x", []*domain.MediaItem{{ID: "old"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetEpisodes("l", "s", "x"); ok {
		t.Fatal("legacy unwrapped entry served instead of miss")
	}
}

func TestSetWatchStateMemoryOnly(t *testing.T) {
	testWatchState(t, seedStore(t, ""))
}
