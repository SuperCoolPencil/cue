package playlist

import (
	"log/slog"
	"testing"

	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/store"
)

func TestQueueOperations(t *testing.T) {
	st, err := store.NewLibraryStore("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	svc := NewService(nil, st, slog.Default())
	item := &domain.MediaItem{ID: "m1", Title: "Movie"}

	if err := svc.AddToQueue(item); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddToQueue(item); err != nil {
		t.Fatal(err)
	}
	if got := svc.QueueItems(); len(got) != 1 {
		t.Fatalf("expected duplicate suppression, got %d", len(got))
	}

	if err := svc.RemoveFromQueue("m1"); err != nil {
		t.Fatal(err)
	}
	if got := svc.QueueItems(); len(got) != 0 {
		t.Fatalf("expected empty queue, got %d", len(got))
	}
}
