package tui

import (
	"testing"
	"time"

	"github.com/SuperCoolPencil/cue/internal/config"
	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
)

func TestEnterOnResumableItemShowsResumeConfirmation(t *testing.T) {
	item := &domain.MediaItem{ID: "m1", Title: "Movie", ViewOffset: 10 * time.Minute}
	col := components.NewListColumn(components.ColumnTypeMovies, "Movies")
	col.SetItems([]*domain.MediaItem{item})
	col.SetFocused(true)

	model := Model{
		State:       StateBrowsing,
		ColumnStack: NewColumnStack(),
	}
	model.ColumnStack.Push(col, 0)

	updated, _ := model.handleEnter()
	got := updated.(Model)
	if got.State != StateConfirmResume {
		t.Fatalf("state = %v, want StateConfirmResume", got.State)
	}
	if got.pendingPlayback == nil || got.pendingPlayback.ID != "m1" {
		t.Fatalf("pending playback = %#v", got.pendingPlayback)
	}
}

func TestResumeConfirmationCancel(t *testing.T) {
	model := Model{
		State:           StateConfirmResume,
		pendingPlayback: &domain.MediaItem{ID: "m1", Title: "Movie"},
	}

	updated, _ := model.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(Model)
	if got.State != StateBrowsing {
		t.Fatalf("state = %v, want StateBrowsing", got.State)
	}
	if got.pendingPlayback != nil {
		t.Fatalf("pending playback should be cleared")
	}
}

func TestNextEpisodeKeybinding(t *testing.T) {
	col := components.NewListColumn(components.ColumnTypeEpisodes, "Episodes")
	col.SetItems([]*domain.MediaItem{
		{ID: "ep1", Title: "Ep 1", Type: domain.MediaTypeEpisode, IsPlayed: true},
		{ID: "ep2", Title: "Ep 2", Type: domain.MediaTypeEpisode, IsPlayed: false},
		{ID: "ep3", Title: "Ep 3", Type: domain.MediaTypeEpisode, IsPlayed: false},
	})
	col.SetFocused(true)

	model := Model{
		State:       StateBrowsing,
		ColumnStack: NewColumnStack(),
		UIConfig:    config.UIConfig{Autoplay: true},
	}
	model.ColumnStack.Push(col, 0)

	// Cursor starts on ep1 (watched). Pressing N should go to ep2.
	updated, _ := model.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
	got := updated.(Model)

	top := got.ColumnStack.Top()
	if top.SelectedIndex() != 1 {
		t.Fatalf("selected index after N = %d, want 1 (Ep 2)", top.SelectedIndex())
	}
	if item := top.SelectedMediaItem(); item == nil || item.ID != "ep2" {
		t.Fatalf("selected item = %#v, want ep2", item)
	}
}

func TestEnterOnShowDirectPlaysWhenEnabled(t *testing.T) {
	col := components.NewListColumn(components.ColumnTypeShows, "Shows")
	col.SetItems([]*domain.Show{{ID: "show1", Title: "Show", LibraryID: "library1"}})
	col.SetFocused(true)

	model := Model{
		State:       StateBrowsing,
		ColumnStack: NewColumnStack(),
		UIConfig:    config.UIConfig{PlayNextOnSelect: true},
	}
	model.ColumnStack.Push(col, 0)

	updated, cmd := model.handleEnter()
	got := updated.(Model)
	if cmd == nil {
		t.Fatal("handleEnter() returned nil command")
	}
	if got.ColumnStack.Len() != 1 {
		t.Fatalf("column stack length = %d, want 1 (show should not be opened)", got.ColumnStack.Len())
	}
	if got.StatusMsg != "Finding next episode for Show..." {
		t.Fatalf("status = %q", got.StatusMsg)
	}
}
