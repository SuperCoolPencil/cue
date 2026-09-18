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

	if !SetTheme("surge-light") {
		t.Errorf("expected SetTheme('surge-light') to return true")
	}

	if ActiveTheme().Name != "surge-light" {
		t.Errorf("expected active theme to be 'surge-light', got '%s'", ActiveTheme().Name)
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
	foundSurgeLight := false
	for _, n := range names {
		if n == "surge" {
			foundSurge = true
		}
		if n == "surge-light" {
			foundSurgeLight = true
		}
	}

	if !foundSurge {
		t.Errorf("expected 'surge' in theme names")
	}
	if !foundSurgeLight {
		t.Errorf("expected 'surge-light' in theme names")
	}

	next := NextThemeName("surge")
	if next != "surge-light" {
		t.Errorf("expected next after surge to be surge-light, got %s", next)
	}
}
