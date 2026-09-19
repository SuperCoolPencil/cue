package components

import (
	"testing"

	"github.com/SuperCoolPencil/cue/internal/domain"
	"github.com/SuperCoolPencil/cue/internal/search"
	tea "github.com/charmbracelet/bubbletea"
)

func TestGlobalSearchSelectionAndQueryChange(t *testing.T) {
	searchBox := NewGlobalSearch()
	searchBox.Show()
	searchBox, _, _ = searchBox.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("matrix")})
	if searchBox.Query() != "matrix" {
		t.Fatalf("query = %q", searchBox.Query())
	}
	if !searchBox.QueryChanged() || searchBox.QueryChanged() {
		t.Fatalf("query changed tracking failed")
	}
	searchBox.SetResults([]search.FilterResult{{
		FilterItem: search.FilterItem{Item: &domain.MediaItem{ID: "m1"}, Title: "The Matrix", Type: domain.MediaTypeMovie},
	}})
	_, _, selected := searchBox.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !selected {
		t.Fatal("enter should select result")
	}
	if got := searchBox.Selected(); got == nil || got.Title != "The Matrix" {
		t.Fatalf("selected = %#v", got)
	}
}

func TestGlobalSearchPreviewRendering(t *testing.T) {
	searchBox := NewGlobalSearch()
	searchBox.Show()
	searchBox.SetSize(100, 30)

	movie := &domain.MediaItem{
		ID:            "m1",
		Title:         "Inception",
		Year:          2010,
		Rating:        8.8,
		ContentRating: "PG-13",
		Summary:       "A thief who steals corporate secrets through dreams.",
		Type:          domain.MediaTypeMovie,
	}

	searchBox.SetResults([]search.FilterResult{{
		FilterItem: search.FilterItem{Item: movie, Title: "Inception", Type: domain.MediaTypeMovie},
	}})
	searchBox.SetPoster("ASCII_POSTER_ART")

	view := searchBox.View()
	if !contains(view, "PREVIEW") || !contains(view, "Inception") || !contains(view, "PG-13") || !contains(view, "ASCII_POSTER_ART") {
		t.Fatalf("search view missing preview content: %s", view)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || (len(s) >= len(substr) && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
