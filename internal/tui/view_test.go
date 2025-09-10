package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/germanoeich/siftail/internal/core"
)

func TestRender_SeverityBadges(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)
	model.width = 80
	model.height = 24

	// Test different severity levels
	testCases := []struct {
		level    core.Severity
		levelStr string
		expected string
	}{
		{core.SevDebug, "DEBUG", "DEBUG"},
		{core.SevInfo, "INFO", "INFO "},
		{core.SevWarn, "WARN", "WARN "},
		{core.SevError, "ERROR", "ERROR"},
	}

	for _, tc := range testCases {
		badge := model.renderSeverityBadge(tc.level, tc.levelStr)

		// Badge should contain the level string
		if !strings.Contains(badge, tc.levelStr) {
			t.Errorf("Expected badge to contain %s, got %s", tc.levelStr, badge)
		}
	}
}

func TestRender_HighlightSpansNoOverlap(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)

	// Add a highlight pattern
	matcher, _ := core.NewMatcher("test")
	filters.AddHighlight(matcher)

	// Test line with the pattern
	testLine := "This is a test line with test patterns"

	result := model.applyAllHighlights(testLine)

	// Result should contain the original text
	// Note: The exact styling would depend on the terminal,
	// but we can check that the function doesn't crash and returns something
	if result == "" {
		t.Error("Expected non-empty result from highlight application")
	}

	// Should contain the original word somewhere (even if styled)
	if !strings.Contains(result, "test") {
		t.Error("Expected result to contain original text")
	}
}

func TestRender_ContainerPrefixing(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeDocker)
	model.width = 80
	model.height = 24

	// Create a test event with container info
	event := core.LogEvent{
		Source:    core.SourceDocker,
		Container: "test-container",
		Line:      "Test log message",
		Level:     core.SevInfo,
		LevelStr:  "INFO",
		Time:      time.Now(),
	}

	result := model.renderEventWithFullStyling(event)

	// Should contain the container name
	if !strings.Contains(result, "test-container") {
		t.Errorf("Expected result to contain container name, got: %s", result)
	}

	// Should contain the log message
	if !strings.Contains(result, "Test log message") {
		t.Errorf("Expected result to contain log message, got: %s", result)
	}
}

func TestRender_ToolbarGeneration(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)
	model.width = 120
	model.height = 30

	// Render toolbar
	toolbar := model.renderToolbar()

	// Should contain hotkeys
	expectedHotkeys := []string{"Quit", "Highlight", "Find", "Filter"}
	for _, hotkey := range expectedHotkeys {
		if !strings.Contains(toolbar, hotkey) {
			t.Errorf("Expected toolbar to contain %s, got: %s", hotkey, toolbar)
		}
	}

	// Should contain level mapping
	if !strings.Contains(toolbar, "DEBUG") {
		t.Error("Expected toolbar to contain DEBUG level")
	}
}

func TestRender_StatusLine(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	// Test different modes
	testCases := []struct {
		mode     Mode
		expected string
	}{
		{ModeFile, "[FILE]"},
		{ModeStdin, "[STDIN]"},
		{ModeDocker, "[DOCKER]"},
	}

	for _, tc := range testCases {
		model := *NewModel(ring, filters, search, levels, tc.mode)
		model.width = 80
		model.height = 24

		statusLine := model.renderStatusLine()

		if !strings.Contains(statusLine, tc.expected) {
			t.Errorf("Expected status line to contain %s for mode %v, got: %s",
				tc.expected, tc.mode, statusLine)
		}

		// Should contain line count
		if !strings.Contains(statusLine, "Lines:") {
			t.Error("Expected status line to contain line count")
		}
	}
}

func TestRender_PromptDisplay(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)
	model.width = 80
	model.height = 24

	// Test different prompt types
	testCases := []struct {
		promptKind PromptKind
		expected   string
	}{
		{PromptFind, "Find: "},
		{PromptHighlight, "Highlight: "},
		{PromptFilterIn, "Filter In: "},
		{PromptFilterOut, "Filter Out: "},
		{PromptPresetName, "Preset Name: "},
	}

	for _, tc := range testCases {
		model.inPrompt = true
		model.promptKind = tc.promptKind

		prompt := model.renderPrompt()

		if !strings.Contains(prompt, tc.expected) {
			t.Errorf("Expected prompt to contain %s for kind %v, got: %s",
				tc.expected, tc.promptKind, prompt)
		}
	}
}

func TestRender_LevelMapping(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)

	// Add a custom level
	levels.GetOrAssignIndex("CUSTOM")

	levelMapping := model.renderLevelMapping()

	// Should contain default levels
	if !strings.Contains(levelMapping, "DEBUG") {
		t.Error("Expected level mapping to contain DEBUG")
	}

	if !strings.Contains(levelMapping, "INFO") {
		t.Error("Expected level mapping to contain INFO")
	}

	// Should show enabled/disabled status
	if !strings.Contains(levelMapping, "[on]") {
		t.Error("Expected level mapping to show enabled status")
	}

	// Toggle a level and check
	levels.Toggle(1) // Toggle DEBUG
	levelMapping = model.renderLevelMapping()

	if !strings.Contains(levelMapping, "[off]") {
		t.Error("Expected level mapping to show disabled status after toggle")
	}
}

func TestRender_EmptyContent(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)
	model.width = 80
	model.height = 24

	// Render empty events
	var events []core.LogEvent
	result := model.renderEventsWithFullStyling(events)

	// Should contain empty message
	if !strings.Contains(result, "No log entries") {
		t.Errorf("Expected empty message, got: %s", result)
	}
}

func TestRender_TruncateToWidth(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)

	// Test truncation
	longLine := strings.Repeat("a", 100)
	truncated := model.truncateToWidth(longLine, 50)

	// Should be shortened
	if len(truncated) > 50 {
		t.Errorf("Expected truncated line to be <= 50 chars, got %d", len(truncated))
	}

	// Test line that fits
	shortLine := "short"
	notTruncated := model.truncateToWidth(shortLine, 50)

	if notTruncated != shortLine {
		t.Error("Expected short line to remain unchanged")
	}
}

func TestRender_SubstringHighlight(t *testing.T) {
	// Setup
	ring := core.NewRing(100)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	model := *NewModel(ring, filters, search, levels, ModeFile)

	// Create a test matcher
	matcher, _ := core.NewMatcher("test")

	// Apply highlighting
	testLine := "This is a test line"
	result := model.applySubstringHighlight(testLine, matcher, highlightStyle)

	// Result should not be empty
	if result == "" {
		t.Error("Expected non-empty result from substring highlighting")
	}

	// Should contain the original text content
	if !strings.Contains(result, "This is a") || !strings.Contains(result, "line") {
		t.Error("Expected result to contain original text content")
	}
}
