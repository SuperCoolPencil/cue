package search

import (
	"testing"

	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/store"
)

func TestFilterLocalSearchesCachedLibraries(t *testing.T) {
	st, _ := store.NewLibraryStore("", "", "")
	libs := []domain.Library{
		{ID: "movies", Name: "Movies", Type: "movie"},
		{ID: "shows", Name: "Shows", Type: "show"},
	}
	_ = st.SaveMovies("movies", []*domain.MediaItem{{ID: "m1", Title: "The Matrix"}}, 1)
	_ = st.SaveShows("shows", []*domain.Show{{ID: "s1", Title: "The Expanse"}}, 1)
	svc := NewService(st)

	results := svc.FilterLocal("matrix", libs)
	if len(results) != 1 || results[0].Title != "The Matrix" || results[0].Type != domain.MediaTypeMovie {
		t.Fatalf("results = %#v", results)
	}
	results = svc.FilterLocal("expanse", libs)
	if len(results) != 1 || results[0].Title != "The Expanse" || results[0].Type != domain.MediaTypeShow {
		t.Fatalf("results = %#v", results)
	}
}

func TestFilterLocalSearchesSummary(t *testing.T) {
	st, _ := store.NewLibraryStore("", "", "")
	libs := []domain.Library{
		{ID: "movies", Name: "Movies", Type: "movie"},
	}
	_ = st.SaveMovies("movies", []*domain.MediaItem{
		{ID: "m1", Title: "Inception", Summary: "A thief who enters dreams of others."},
		{ID: "m2", Title: "Interstellar", Summary: "A team of explorers travel through a wormhole."},
	}, 1)
	svc := NewService(st)

	results := svc.FilterLocal("wormhole", libs)
	if len(results) != 1 || results[0].Title != "Interstellar" {
		t.Fatalf("expected Interstellar for query 'wormhole', got %#v", results)
	}
}
