package tui

import (
	"testing"

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
	model = model.refreshFindIndex()
	
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