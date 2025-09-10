package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/germanoeich/siftail/internal/core"
)

func TestModel_Update_ResizeAdjustsViewport(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)

	// Initial size
	if model.width != 80 || model.height != 24 {
		t.Errorf("Expected initial size 80x24, got %dx%d", model.width, model.height)
	}

	// Resize message
	resizeMsg := tea.WindowSizeMsg{Width: 120, Height: 40}
	newModel, _ := model.Update(resizeMsg)
	model = newModel.(Model)

	// Check new size
	if model.width != 120 || model.height != 40 {
		t.Errorf("Expected resized size 120x40, got %dx%d", model.width, model.height)
	}

	// Check viewport was adjusted (height - 3 for status/toolbar)
	expectedVpHeight := 40 - 3
	if model.vp.Height != expectedVpHeight {
		t.Errorf("Expected viewport height %d, got %d", expectedVpHeight, model.vp.Height)
	}

	if model.vp.Width != 120 {
		t.Errorf("Expected viewport width 120, got %d", model.vp.Width)
	}
}

func TestModel_FollowTailBehavior(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)

	// Initially should follow tail
	if !model.followTail {
		t.Error("Expected initial followTail to be true")
	}

	// When viewport is at bottom, should continue following
	model.vp.GotoBottom()
	model = model.updateFollowTail()

	if !model.followTail {
		t.Error("Expected followTail to remain true when at bottom")
	}

	// Simulate scrolling up (not at bottom)
	// Note: In a real test we'd set up the viewport content and scroll
	// For now, we'll just test the basic follow tail update logic
}

func TestModel_PromptFocusAndApply(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)

	// Initially not in prompt mode
	if model.inPrompt {
		t.Error("Expected initial inPrompt to be false")
	}

	// Start highlight prompt
	model = model.startPrompt(PromptHighlight, "Highlight: ")

	if !model.inPrompt {
		t.Error("Expected inPrompt to be true after starting prompt")
	}

	if model.promptKind != PromptHighlight {
		t.Errorf("Expected prompt kind to be PromptHighlight, got %v", model.promptKind)
	}

	// Cancel prompt
	model = model.cancelPrompt()

	if model.inPrompt {
		t.Error("Expected inPrompt to be false after canceling prompt")
	}

	// Test prompt submission
	model = model.startPrompt(PromptHighlight, "Highlight: ")
	model.input.SetValue("test pattern")

	initialHighlights := len(model.filters.Highlights)
	model = model.handlePromptSubmit()

	if model.inPrompt {
		t.Error("Expected inPrompt to be false after submitting prompt")
	}

	if len(model.filters.Highlights) != initialHighlights+1 {
		t.Errorf("Expected %d highlights after submission, got %d",
			initialHighlights+1, len(model.filters.Highlights))
	}
}

func TestModel_SeverityToggle(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)

	// Initially all levels should be enabled
	if !model.levels.IsEnabled(core.SevDebug) {
		t.Error("Expected DEBUG level to be initially enabled")
	}

	// Toggle level 1 (DEBUG)
	model.levels.Toggle(1)

	if model.levels.IsEnabled(core.SevDebug) {
		t.Error("Expected DEBUG level to be disabled after toggle")
	}

	// Toggle again
	model.levels.Toggle(1)

	if !model.levels.IsEnabled(core.SevDebug) {
		t.Error("Expected DEBUG level to be enabled after second toggle")
	}
}

func TestModel_FindNavigation(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)

	// Add some test events to the ring
	detector := core.NewDefaultSeverityDetector(levels)
	events := []string{
		"This is a test message",
		"Another log entry",
		"Test pattern here",
		"Final test line",
	}

	for _, line := range events {
		levelStr, level, _ := detector.Detect(line)
		event := core.LogEvent{
			Line:     line,
			LevelStr: levelStr,
			Level:    level,
		}
		ring.Append(event)
	}

	// Start find with "test" pattern
	matcher, _ := core.NewMatcher("test")
	search.SetMatcher(matcher)
	search.SetActive(true)

	// Rebuild find index
	model.refreshFindIndex()

	// Should have found matches
	if search.Count() == 0 {
		t.Error("Expected to find matches for 'test' pattern")
	}

	// Test navigation
	firstHit := search.JumpToFirst()
	if firstHit == 0 {
		t.Error("Expected first hit to return non-zero sequence")
	}

	// Test next/prev navigation
	nextHit := search.Next()
	if nextHit == 0 {
		t.Error("Expected next hit to return non-zero sequence")
	}

	prevHit := search.Prev()
	if prevHit != firstHit {
		t.Error("Expected prev hit to return to first hit")
	}
}

func TestNewModel(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	// Test all modes
	modes := []Mode{ModeFile, ModeStdin, ModeDocker}

	for _, mode := range modes {
		model := *NewModel(ring, filters, search, levels, mode)

		if model.mode != mode {
			t.Errorf("Expected mode %v, got %v", mode, model.mode)
		}

		if model.ring != ring {
			t.Error("Expected ring to be set correctly")
		}

		if model.filters != filters {
			t.Error("Expected filters to be set correctly")
		}

		if model.search != search {
			t.Error("Expected search to be set correctly")
		}

		if model.levels != levels {
			t.Error("Expected levels to be set correctly")
		}

		if !model.followTail {
			t.Error("Expected followTail to be initially true")
		}

		if model.inPrompt {
			t.Error("Expected inPrompt to be initially false")
		}
	}
}

func TestDockerUI_ToggleSingle(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeDocker)

	// Setup containers
	model.dockerUI.Containers = map[string]bool{
		"nginx":    true,
		"postgres": false,
		"redis":    true,
	}

	// Open container list
	model.dockerUI.ContainerListOpen = true
	model.dockerUI.SelectedContainer = 0 // Should be "nginx" when sorted

	// Test toggling selected container (nginx should become false)
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	newModel, _ := model.Update(keyMsg)
	model = newModel.(Model)

	// nginx should now be false
	if model.dockerUI.Containers["nginx"] != false {
		t.Errorf("Expected nginx to be false after toggle, got %v", model.dockerUI.Containers["nginx"])
	}

	// Other containers should remain unchanged
	if model.dockerUI.Containers["postgres"] != false {
		t.Errorf("Expected postgres to remain false, got %v", model.dockerUI.Containers["postgres"])
	}

	if model.dockerUI.Containers["redis"] != true {
		t.Errorf("Expected redis to remain true, got %v", model.dockerUI.Containers["redis"])
	}
}

func TestDockerUI_ToggleAll(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeDocker)

	// Setup containers with mixed visibility
	model.dockerUI.Containers = map[string]bool{
		"nginx":    true,
		"postgres": false,
		"redis":    true,
	}

	// Open container list and select "All" (index -1)
	model.dockerUI.ContainerListOpen = true
	model.dockerUI.SelectedContainer = -1 // "All" option
	model.dockerUI.AllToggle = true

	// Test toggling all containers (should make all false)
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	newModel, _ := model.Update(keyMsg)
	model = newModel.(Model)

	// All containers should now be false
	for name, visible := range model.dockerUI.Containers {
		if visible != false {
			t.Errorf("Expected container %s to be false after toggle all, got %v", name, visible)
		}
	}

	// AllToggle should now be false
	if model.dockerUI.AllToggle != false {
		t.Errorf("Expected AllToggle to be false, got %v", model.dockerUI.AllToggle)
	}

	// Test toggling all again (should make all true)
	keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	newModel, _ = model.Update(keyMsg)
	model = newModel.(Model)

	// All containers should now be true
	for name, visible := range model.dockerUI.Containers {
		if visible != true {
			t.Errorf("Expected container %s to be true after second toggle all, got %v", name, visible)
		}
	}

	// AllToggle should now be true
	if model.dockerUI.AllToggle != true {
		t.Errorf("Expected AllToggle to be true, got %v", model.dockerUI.AllToggle)
	}
}

func TestErrors_ShownInStatus(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)

	// Set an error
	model = model.setError("Test error message")

	// Check that error is set
	if model.errMsg != "Test error message" {
		t.Errorf("Expected error message to be set, got %s", model.errMsg)
	}

	// Check that error time is set
	if model.errTime.IsZero() {
		t.Error("Expected error time to be set")
	}

	// Check that error eventually expires
	model.errTime = model.errTime.Add(-6 * time.Second) // Make it 6 seconds old
	if !model.isErrorExpired() {
		t.Error("Expected error to be expired after 6 seconds")
	}
}

func TestDockerErrorMessage_NoManualRetryHint(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeDocker)

	// Simulate a recoverable Docker error message emitted by the reader
	errMsg := DockerErrorMsg{Error: fmt.Errorf("dial unix /var/run/docker.sock: connect: no such file"), Recoverable: true}
	newModel, _ := model.Update(errMsg)
	model = newModel.(Model)

	if model.errMsg == "" {
		t.Fatal("expected an error message to be set")
	}
	if strings.Contains(model.errMsg, "Press 'R'") {
		t.Fatalf("unexpected manual retry hint in error message: %q", model.errMsg)
	}
}

func TestLineLength_Truncation(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)
	model.perf.MaxLineLength = 10 // Very short for testing

	// Test truncating a long line
	longLine := "This is a very long line that should be truncated"
	truncated := model.truncateLine(longLine)

	// Should be truncated to 10 chars with "..."
	expected := "This is..."
	if truncated != expected {
		t.Errorf("Expected truncated line %q, got %q", expected, truncated)
	}

	// Test a short line (should not be truncated)
	shortLine := "Short"
	notTruncated := model.truncateLine(shortLine)
	if notTruncated != shortLine {
		t.Errorf("Expected short line to remain unchanged, got %q", notTruncated)
	}
}

// TestViewportScrollingAndFindJump ensures that the viewport receives full content
// (so it can scroll) and that find navigation jumps to off-screen matches.
func TestViewportScrollingAndFindJump(t *testing.T) {
	ring := core.NewRing(1000)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	m := *NewModel(ring, filters, search, levels, ModeFile)

	// Set terminal size so viewport height is a known value: height 13 => vp.Height = 10
	resize := tea.WindowSizeMsg{Width: 80, Height: 13}
	nm, _ := m.Update(resize)
	m = nm.(Model)
	if m.vp.Height != 10 {
		t.Fatalf("expected vp height 10, got %d", m.vp.Height)
	}

	// Append more than a page of events so scrolling is possible
	needleIdx := 20
	for i := 0; i < 100; i++ {
		line := fmt.Sprintf("line-%03d", i)
		if i == needleIdx {
			line = "MARK-NEEDLE-20"
		}
		e := core.LogEvent{Line: line}
		_ = ring.Append(e)
	}

	// Render content and auto-follow tail
	m = m.updateViewportContent()
	if !m.vp.AtBottom() {
		t.Fatal("expected viewport at bottom after render with followTail")
	}

	// Scroll up a page; should no longer be at bottom and YOffset should be > 0
	m.vp.PageUp()
	if m.vp.AtBottom() {
		t.Fatal("expected viewport not at bottom after PageUp; content may be truncated")
	}
	if m.vp.YOffset == 0 {
		t.Fatal("expected YOffset > 0 after PageUp")
	}

	// Activate Find to match the needle and navigate to it; viewport should jump
	matcher, _ := core.NewMatcher("NEEDLE")
	m.search.SetMatcher(matcher)
	m.search.SetActive(true)
	m = m.refreshFindIndex()

	// Jump to the first (and only) hit using our navigate helper
	m = m.navigateFind(false)

	// Expected position centers the match when possible
	expected := needleIdx - m.vp.Height/2
	if expected < 0 {
		expected = 0
	}
	if m.vp.YOffset != expected {
		t.Fatalf("expected YOffset %d after find jump, got %d", expected, m.vp.YOffset)
	}
	if m.followTail {
		t.Fatal("expected followTail to be disabled after jumping to a match")
	}
}
