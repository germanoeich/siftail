package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/germanoeich/siftail/internal/core"
)

// Ensure the help overlay renders when opened and can be closed by key.
func TestHelpOverlay_RenderAndClose(t *testing.T) {
	ring := core.NewRing(10)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	m := *NewModel(ring, filters, search, levels, ModeFile)

	// Set a stable window size so overlay renders deterministically
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = nm.(Model)

	// Open help via '?' key
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = nm.(Model)

	if !m.helpOpen {
		t.Fatalf("expected helpOpen=true after '?' key")
	}

	view := m.View()
	if !strings.Contains(view, "Help — Key Bindings") {
		t.Fatalf("expected help overlay to render, got view: %q", view)
	}

	// Close help via Esc
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(Model)
	if m.helpOpen {
		t.Fatalf("expected helpOpen=false after Esc")
	}
}
