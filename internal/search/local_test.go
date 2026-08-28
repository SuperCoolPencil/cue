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
