package search

import (
	"context"
	"testing"

	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/store"
)

type fakeRemoteSearch struct {
	items []*domain.MediaItem
}

func (f fakeRemoteSearch) Search(context.Context, string) ([]*domain.MediaItem, error) {
	return f.items, nil
}

func TestSearchRemote(t *testing.T) {
	st, err := store.NewLibraryStore("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	svc := NewService(st)
	svc.SetRemote(fakeRemoteSearch{items: []*domain.MediaItem{{ID: "r1", Title: "Remote", Type: domain.MediaTypeMovie, LibraryID: "lib"}}})

	results, err := svc.SearchRemote(context.Background(), "remote")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Title != "Remote" || results[0].LibraryID != "lib" {
		t.Fatalf("unexpected results: %#v", results)
	}
}
