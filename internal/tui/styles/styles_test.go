package styles

import (
	"testing"
)

func TestThemeSwitching(t *testing.T) {
	initial := ActiveTheme().Name

	if !SetTheme("surge") {
		t.Errorf("expected SetTheme('surge') to return true")
	}

	if ActiveTheme().Name != "surge" {
		t.Errorf("expected active theme to be 'surge', got '%s'", ActiveTheme().Name)
	}

	if SetTheme("nonexistent") {
		t.Errorf("expected SetTheme('nonexistent') to return false")
	}

	// Restore initial theme
	SetTheme(initial)
}

func TestThemeNamesAndNext(t *testing.T) {
	names := ThemeNames()
	if len(names) == 0 {
		t.Fatalf("expected non-empty theme names")
	}

	foundSurge := false
	for _, n := range names {
		if n == "surge" {
			foundSurge = true
		}
	}

	if !foundSurge {
		t.Errorf("expected 'surge' in theme names")
	}

	next := NextThemeName("gruvbox")
	if next != "surge" {
		t.Errorf("expected next after gruvbox to be surge, got %s", next)
	}
}
