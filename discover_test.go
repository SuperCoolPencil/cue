package main

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPlexctlSelectorCancelDoesNotSelectFirstServer(t *testing.T) {
	l := list.New(nil, plexctlSelectorDelegate{}, 60, 10)
	m := plexctlSelectorModel{list: l, choice: -1}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := updated.(plexctlSelectorModel)
	if !got.quitting {
		t.Fatal("cancel should quit selector")
	}
	if got.choice != -1 {
		t.Fatalf("cancel selected index %d, want no selection", got.choice)
	}
}

func TestPlexctlSelectorEnterSelectsCurrentServer(t *testing.T) {
	l := list.New([]list.Item{
		plexctlSelectorItem{title: "First"},
		plexctlSelectorItem{title: "Second"},
	}, plexctlSelectorDelegate{}, 60, 10)
	l.Select(1)
	m := plexctlSelectorModel{list: l, choice: -1}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(plexctlSelectorModel)
	if got.choice != 1 {
		t.Fatalf("enter selected index %d, want 1", got.choice)
	}
}
