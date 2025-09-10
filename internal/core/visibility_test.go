package core

import (
	"testing"
)

func TestSeverityToggle_FiltersView(t *testing.T) {
	levelMap := NewLevelMap()
	
	// Create test events with different severity levels
	events := []LogEvent{
		{Seq: 1, Line: "debug message", Level: SevDebug},
		{Seq: 2, Line: "info message", Level: SevInfo},
		{Seq: 3, Line: "warning message", Level: SevWarn},
		{Seq: 4, Line: "error message", Level: SevError},
		{Seq: 5, Line: "another debug", Level: SevDebug},
		{Seq: 6, Line: "another error", Level: SevError},
	}
	
	// Initially all levels should be enabled
	visible := FilterEventsByLevel(events, levelMap)
	if len(visible) != len(events) {
		t.Errorf("Expected all %d events visible initially, got %d", len(events), len(visible))
	}
	
	// Toggle DEBUG off (index 1)
	levelMap.Toggle(1)
	visible = FilterEventsByLevel(events, levelMap)
	
	// Should hide debug messages (events 1 and 5)
	expected := []uint64{2, 3, 4, 6}
	if len(visible) != len(expected) {
		t.Fatalf("Expected %d events after toggling DEBUG off, got %d", len(expected), len(visible))
	}
	
	for i, event := range visible {
		if event.Seq != expected[i] {
			t.Errorf("Event %d: expected seq %d, got %d", i, expected[i], event.Seq)
		}
	}
	
	// Toggle ERROR off (index 4)
	levelMap.Toggle(4)
	visible = FilterEventsByLevel(events, levelMap)
	
	// Should hide debug and error messages (events 1, 4, 5, 6)
	expected = []uint64{2, 3}
	if len(visible) != len(expected) {
		t.Fatalf("Expected %d events after toggling ERROR off, got %d", len(expected), len(visible))
	}
	
	for i, event := range visible {
		if event.Seq != expected[i] {
			t.Errorf("Event %d: expected seq %d, got %d", i, expected[i], event.Seq)
		}
	}
	
	// Toggle DEBUG back on (index 1)
	levelMap.Toggle(1)
	visible = FilterEventsByLevel(events, levelMap)
	
	// Should show debug but still hide error messages
	expected = []uint64{1, 2, 3, 5}
	if len(visible) != len(expected) {
		t.Fatalf("Expected %d events after toggling DEBUG back on, got %d", len(expected), len(visible))
	}
	
	for i, event := range visible {
		if event.Seq != expected[i] {
			t.Errorf("Event %d: expected seq %d, got %d", i, expected[i], event.Seq)
		}
	}
}

func TestSeverity_DiscoveryUpdatesToolbar(t *testing.T) {
	levelMap := NewLevelMap()
	detector := NewDefaultSeverityDetector(levelMap)
	
	// Initially should have default mappings
	indexToName, enabled := levelMap.GetSnapshot()
	
	// Check default mappings
	expectedDefaults := []string{"", "DEBUG", "INFO", "WARN", "ERROR", "", "", "", "", ""}
	for i, expected := range expectedDefaults {
		if indexToName[i] != expected {
			t.Errorf("Default mapping %d: expected '%s', got '%s'", i, expected, indexToName[i])
		}
	}
	
	// All default levels should be enabled
	for i := 1; i <= 4; i++ {
		if !enabled[i] {
			t.Errorf("Default level %d should be enabled", i)
		}
	}
	
	// Discover new levels
	testLines := []string{
		"[TRACE] trace message",
		"NOTICE: notice message", 
		"ALERT something happened",
		"[CRITICAL] critical message",
	}
	
	for _, line := range testLines {
		detector.Detect(line)
	}
	
	// Get updated mappings
	indexToName, enabled = levelMap.GetSnapshot()
	
	// New levels should be assigned to slots 5-8
	expectedMappings := map[int]string{
		1: "DEBUG",
		2: "INFO", 
		3: "WARN",
		4: "ERROR",
		5: "TRACE",
		6: "NOTICE",
		7: "ALERT", 
		8: "CRITICAL",
		9: "", // Should still be empty
	}
	
	for index, expectedName := range expectedMappings {
		if indexToName[index] != expectedName {
			t.Errorf("Mapping %d: expected '%s', got '%s'", index, expectedName, indexToName[index])
		}
	}
	
	// New levels should be enabled by default
	for i := 5; i <= 8; i++ {
		if !enabled[i] {
			t.Errorf("New level %d should be enabled by default", i)
		}
	}
	
	// Test overflow to OTHER
	moreLines := []string{
		"[CUSTOM1] message",
		"[CUSTOM2] message", 
		"[CUSTOM3] message", // This should trigger OTHER
	}
	
	for _, line := range moreLines {
		detector.Detect(line)
	}
	
	// Get final mappings
	indexToName, _ = levelMap.GetSnapshot()
	
	// Slot 9 should now be OTHER
	if indexToName[9] != "OTHER" {
		t.Errorf("Slot 9 should be OTHER after overflow, got '%s'", indexToName[9])
	}
}

func TestComputeVisible_OrderOfOps(t *testing.T) {
	levelMap := NewLevelMap()
	filters := NewFilters()
	
	// Create include filter for "error"
	errorMatcher, _ := NewMatcher("error")
	filters.AddInclude(errorMatcher)
	
	// Create exclude filter for "ignore"
	ignoreMatcher, _ := NewMatcher("ignore")
	filters.AddExclude(ignoreMatcher)
	
	// Toggle DEBUG off
	levelMap.Toggle(1)
	
	events := []LogEvent{
		{Seq: 1, Line: "debug message", Level: SevDebug, Source: SourceDocker, Container: "app1"},
		{Seq: 2, Line: "error occurred", Level: SevError, Source: SourceDocker, Container: "app1"},
		{Seq: 3, Line: "info error message", Level: SevInfo, Source: SourceDocker, Container: "app2"},
		{Seq: 4, Line: "error ignore this", Level: SevError, Source: SourceDocker, Container: "app1"},
		{Seq: 5, Line: "warning error", Level: SevWarn, Source: SourceDocker, Container: "app1"},
	}
	
	plan := VisiblePlan{
		Include:  filters,
		LevelMap: levelMap,
		DockerVisible: map[string]bool{
			"app1": true,
			"app2": false, // app2 is not visible
		},
	}
	
	visible := ComputeVisible(events, plan)
	
	// Expected results:
	// Event 1: DEBUG disabled -> hidden
	// Event 2: ERROR enabled, contains "error", from app1 (visible), no "ignore" -> shown
	// Event 3: INFO enabled, contains "error", from app2 (not visible) -> hidden
	// Event 4: ERROR enabled, contains "error", from app1 (visible), contains "ignore" -> hidden
	// Event 5: WARN enabled, contains "error", from app1 (visible), no "ignore" -> shown
	
	expectedSeqs := []uint64{2, 5}
	if len(visible) != len(expectedSeqs) {
		t.Fatalf("Expected %d visible events, got %d", len(expectedSeqs), len(visible))
	}
	
	for i, event := range visible {
		if event.Seq != expectedSeqs[i] {
			t.Errorf("Visible event %d: expected seq %d, got %d", i, expectedSeqs[i], event.Seq)
		}
	}
}

func TestComputeVisible_DockerAndSeverity(t *testing.T) {
	levelMap := NewLevelMap()
	
	// Toggle INFO off
	levelMap.Toggle(2)
	
	events := []LogEvent{
		{Seq: 1, Line: "debug from app1", Level: SevDebug, Source: SourceDocker, Container: "app1"},
		{Seq: 2, Line: "info from app1", Level: SevInfo, Source: SourceDocker, Container: "app1"},
		{Seq: 3, Line: "error from app2", Level: SevError, Source: SourceDocker, Container: "app2"},
		{Seq: 4, Line: "warn from app3", Level: SevWarn, Source: SourceDocker, Container: "app3"},
		{Seq: 5, Line: "file debug", Level: SevDebug, Source: SourceFile, Container: ""},
		{Seq: 6, Line: "file info", Level: SevInfo, Source: SourceFile, Container: ""},
	}
	
	plan := VisiblePlan{
		LevelMap: levelMap,
		DockerVisible: map[string]bool{
			"app1": true,
			"app2": true,
			"app3": false, // app3 not visible
		},
	}
	
	visible := ComputeVisible(events, plan)
	
	// Expected results:
	// Event 1: DEBUG enabled, from visible app1 -> shown
	// Event 2: INFO disabled -> hidden
	// Event 3: ERROR enabled, from visible app2 -> shown  
	// Event 4: WARN enabled, from non-visible app3 -> hidden
	// Event 5: DEBUG enabled, from file (non-docker) -> shown
	// Event 6: INFO disabled -> hidden
	
	expectedSeqs := []uint64{1, 3, 5}
	if len(visible) != len(expectedSeqs) {
		t.Fatalf("Expected %d visible events, got %d", len(expectedSeqs), len(visible))
	}
	
	for i, event := range visible {
		if event.Seq != expectedSeqs[i] {
			t.Errorf("Visible event %d: expected seq %d, got %d", i, expectedSeqs[i], event.Seq)
		}
	}
}

func TestFilterEventsByContainer(t *testing.T) {
	events := []LogEvent{
		{Seq: 1, Source: SourceDocker, Container: "app1", Line: "message 1"},
		{Seq: 2, Source: SourceDocker, Container: "app2", Line: "message 2"},
		{Seq: 3, Source: SourceFile, Container: "", Line: "file message"},
		{Seq: 4, Source: SourceDocker, Container: "app3", Line: "message 3"},
	}
	
	// Test with specific containers visible
	dockerVisible := map[string]bool{
		"app1": true,
		"app2": false,
		"app3": true,
	}
	
	visible := FilterEventsByContainer(events, dockerVisible)
	
	// Should show app1, app3, and the file message (non-docker always visible)
	expectedSeqs := []uint64{1, 3, 4}
	if len(visible) != len(expectedSeqs) {
		t.Fatalf("Expected %d visible events, got %d", len(expectedSeqs), len(visible))
	}
	
	for i, event := range visible {
		if event.Seq != expectedSeqs[i] {
			t.Errorf("Visible event %d: expected seq %d, got %d", i, expectedSeqs[i], event.Seq)
		}
	}
	
	// Test with nil map (all visible)
	visible = FilterEventsByContainer(events, nil)
	if len(visible) != len(events) {
		t.Errorf("Expected all events visible with nil map, got %d", len(visible))
	}
	
	// Test with empty map (all visible)
	visible = FilterEventsByContainer(events, map[string]bool{})
	if len(visible) != len(events) {
		t.Errorf("Expected all events visible with empty map, got %d", len(visible))
	}
}

func TestShouldShowEvent(t *testing.T) {
	levelMap := NewLevelMap()
	filters := NewFilters()
	
	// Set up filters
	matcher, _ := NewMatcher("error")
	filters.AddInclude(matcher)
	
	excludeMatcher, _ := NewMatcher("ignore")
	filters.AddExclude(excludeMatcher)
	
	// Toggle DEBUG off
	levelMap.Toggle(1)
	
	plan := VisiblePlan{
		Include:  filters,
		LevelMap: levelMap,
		DockerVisible: map[string]bool{
			"app1": true,
			"app2": false,
		},
	}
	
	testCases := []struct {
		name     string
		event    LogEvent
		expected bool
	}{
		{
			name: "disabled severity level",
			event: LogEvent{
				Line:      "debug error message",
				Level:     SevDebug,
				Source:    SourceDocker,
				Container: "app1",
			},
			expected: false, // DEBUG is disabled
		},
		{
			name: "non-visible container",
			event: LogEvent{
				Line:      "error occurred",
				Level:     SevError,
				Source:    SourceDocker,
				Container: "app2",
			},
			expected: false, // app2 is not visible
		},
		{
			name: "excluded by filter",
			event: LogEvent{
				Line:      "error ignore this",
				Level:     SevError,
				Source:    SourceDocker,
				Container: "app1",
			},
			expected: false, // contains "ignore"
		},
		{
			name: "doesn't match include filter",
			event: LogEvent{
				Line:      "info message",
				Level:     SevInfo,
				Source:    SourceDocker,
				Container: "app1",
			},
			expected: false, // doesn't contain "error"
		},
		{
			name: "passes all filters",
			event: LogEvent{
				Line:      "error occurred",
				Level:     SevError,
				Source:    SourceDocker,
				Container: "app1",
			},
			expected: true, // ERROR enabled, app1 visible, contains "error", no "ignore"
		},
		{
			name: "file source ignores docker visibility",
			event: LogEvent{
				Line:   "error in file",
				Level:  SevError,
				Source: SourceFile,
			},
			expected: true, // File source ignores docker visibility
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ShouldShowEvent(tc.event, plan)
			if result != tc.expected {
				t.Errorf("Expected %t, got %t", tc.expected, result)
			}
		})
	}
}