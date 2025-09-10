package core

import (
	"testing"
)

func TestFilterIn_ApplyAndStack(t *testing.T) {
	filters := NewFilters()

	// Test lines
	testLines := []string{
		"error occurred in module A",
		"info message",
		"warning about something",
		"error in module B",
		"debug info",
		"another error message",
	}

	// Initially, with no include filters, all lines should be shown
	for i, line := range testLines {
		if !filters.ShouldShowLine(line) {
			t.Errorf("Line %d should be shown with no filters: %s", i, line)
		}
	}

	// Add first include filter for "error"
	errorMatcher, err := NewMatcher("error")
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}
	filters.AddInclude(errorMatcher)

	// Now only lines containing "error" should be shown
	expectedShown := []bool{true, false, false, true, false, true}
	for i, line := range testLines {
		shown := filters.ShouldShowLine(line)
		if shown != expectedShown[i] {
			t.Errorf("Line %d: expected %t, got %t for '%s'", i, expectedShown[i], shown, line)
		}
	}

	// Add second include filter for "info" (should OR with error)
	infoMatcher, err := NewMatcher("info")
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}
	filters.AddInclude(infoMatcher)

	// Now lines containing "error" OR "info" should be shown
	expectedShown = []bool{true, true, false, true, true, true}
	for i, line := range testLines {
		shown := filters.ShouldShowLine(line)
		if shown != expectedShown[i] {
			t.Errorf("Line %d after adding info filter: expected %t, got %t for '%s'", i, expectedShown[i], shown, line)
		}
	}

	// Add third include filter for "warning"
	warningMatcher, err := NewMatcher("warning")
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}
	filters.AddInclude(warningMatcher)

	// Now lines containing "error" OR "info" OR "warning" should be shown
	expectedShown = []bool{true, true, true, true, true, true}
	for i, line := range testLines {
		shown := filters.ShouldShowLine(line)
		if shown != expectedShown[i] {
			t.Errorf("Line %d after adding warning filter: expected %t, got %t for '%s'", i, expectedShown[i], shown, line)
		}
	}

	// Test regex include filter
	regexMatcher, err := NewMatcher("/module [AB]/")
	if err != nil {
		t.Fatalf("Failed to create regex matcher: %v", err)
	}

	// Clear existing filters and add regex
	filters.ClearIncludes()
	filters.AddInclude(regexMatcher)

	// Only lines matching the regex should be shown
	expectedShown = []bool{true, false, false, true, false, false}
	for i, line := range testLines {
		shown := filters.ShouldShowLine(line)
		if shown != expectedShown[i] {
			t.Errorf("Line %d with regex filter: expected %t, got %t for '%s'", i, expectedShown[i], shown, line)
		}
	}
}

func TestFilterOut_ApplyAndStack(t *testing.T) {
	filters := NewFilters()

	// Test lines
	testLines := []string{
		"error occurred",
		"info message",
		"debug ignore this",
		"warning occurred",
		"error ignore also",
		"info normal message",
	}

	// Initially, with no exclude filters, all lines should be shown
	for i, line := range testLines {
		if !filters.ShouldShowLine(line) {
			t.Errorf("Line %d should be shown with no filters: %s", i, line)
		}
	}

	// Add first exclude filter for "ignore"
	ignoreMatcher, err := NewMatcher("ignore")
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}
	filters.AddExclude(ignoreMatcher)

	// Lines containing "ignore" should be hidden
	expectedShown := []bool{true, true, false, true, false, true}
	for i, line := range testLines {
		shown := filters.ShouldShowLine(line)
		if shown != expectedShown[i] {
			t.Errorf("Line %d: expected %t, got %t for '%s'", i, expectedShown[i], shown, line)
		}
	}

	// Add second exclude filter for "debug"
	debugMatcher, err := NewMatcher("debug")
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}
	filters.AddExclude(debugMatcher)

	// Lines containing "ignore" OR "debug" should be hidden
	expectedShown = []bool{true, true, false, true, false, true}
	for i, line := range testLines {
		shown := filters.ShouldShowLine(line)
		if shown != expectedShown[i] {
			t.Errorf("Line %d after adding debug filter: expected %t, got %t for '%s'", i, expectedShown[i], shown, line)
		}
	}

	// Add third exclude filter for "error"
	errorMatcher, err := NewMatcher("error")
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}
	filters.AddExclude(errorMatcher)

	// Lines containing "ignore" OR "debug" OR "error" should be hidden
	expectedShown = []bool{false, true, false, true, false, true}
	for i, line := range testLines {
		shown := filters.ShouldShowLine(line)
		if shown != expectedShown[i] {
			t.Errorf("Line %d after adding error filter: expected %t, got %t for '%s'", i, expectedShown[i], shown, line)
		}
	}

	// Test regex exclude filter
	regexMatcher, err := NewMatcher("/message$/")
	if err != nil {
		t.Fatalf("Failed to create regex matcher: %v", err)
	}

	// Clear existing filters and add regex
	filters.ClearExcludes()
	filters.AddExclude(regexMatcher)

	// Lines ending with "message" should be hidden
	expectedShown = []bool{true, false, true, true, true, false}
	for i, line := range testLines {
		shown := filters.ShouldShowLine(line)
		if shown != expectedShown[i] {
			t.Errorf("Line %d with regex exclude filter: expected %t, got %t for '%s'", i, expectedShown[i], shown, line)
		}
	}
}

func TestFilter_Composition_IncludeAndExclude(t *testing.T) {
	filters := NewFilters()

	// Test lines
	testLines := []string{
		"error occurred in system",
		"info error message",
		"debug normal message",
		"error ignore this one",
		"warning error happened",
		"info ignore normal",
		"error in production",
	}

	// Set up include filter for "error"
	errorMatcher, err := NewMatcher("error")
	if err != nil {
		t.Fatalf("Failed to create error matcher: %v", err)
	}
	filters.AddInclude(errorMatcher)

	// Set up exclude filter for "ignore"
	ignoreMatcher, err := NewMatcher("ignore")
	if err != nil {
		t.Fatalf("Failed to create ignore matcher: %v", err)
	}
	filters.AddExclude(ignoreMatcher)

	// Only lines that contain "error" AND do NOT contain "ignore" should be shown
	expectedShown := []bool{true, true, false, false, true, false, true}
	for i, line := range testLines {
		shown := filters.ShouldShowLine(line)
		if shown != expectedShown[i] {
			t.Errorf("Line %d: expected %t, got %t for '%s'", i, expectedShown[i], shown, line)
		}
	}

	// Add second include filter for "info"
	infoMatcher, err := NewMatcher("info")
	if err != nil {
		t.Fatalf("Failed to create info matcher: %v", err)
	}
	filters.AddInclude(infoMatcher)

	// Lines that contain ("error" OR "info") AND do NOT contain "ignore"
	expectedShown = []bool{true, true, false, false, true, false, true}
	for i, line := range testLines {
		shown := filters.ShouldShowLine(line)
		if shown != expectedShown[i] {
			t.Errorf("Line %d after adding info include: expected %t, got %t for '%s'", i, expectedShown[i], shown, line)
		}
	}

	// Add second exclude filter for "production"
	prodMatcher, err := NewMatcher("production")
	if err != nil {
		t.Fatalf("Failed to create production matcher: %v", err)
	}
	filters.AddExclude(prodMatcher)

	// Lines that contain ("error" OR "info") AND do NOT contain ("ignore" OR "production")
	expectedShown = []bool{true, true, false, false, true, false, false}
	for i, line := range testLines {
		shown := filters.ShouldShowLine(line)
		if shown != expectedShown[i] {
			t.Errorf("Line %d after adding production exclude: expected %t, got %t for '%s'", i, expectedShown[i], shown, line)
		}
	}
}

func TestFilter_EdgeCases(t *testing.T) {
	filters := NewFilters()

	// Test empty matcher
	emptyMatcher, err := NewMatcher("")
	if err != nil {
		t.Fatalf("Failed to create empty matcher: %v", err)
	}

	// Empty matchers shouldn't match anything
	if emptyMatcher.Match("any line") {
		t.Error("Empty matcher should not match any line")
	}

	// Adding empty matchers means no lines will match
	filters.AddInclude(emptyMatcher)
	if filters.ShouldShowLine("test line") {
		t.Error("Line should NOT be shown when only empty include filters exist (no matches)")
	}

	filters.ClearIncludes()
	filters.AddExclude(emptyMatcher)
	if !filters.ShouldShowLine("test line") {
		t.Error("Line should be shown when only empty exclude filters exist")
	}

	// Test whitespace-only matcher
	whitespaceMatcher, err := NewMatcher("   ")
	if err != nil {
		t.Fatalf("Failed to create whitespace matcher: %v", err)
	}

	if whitespaceMatcher.Match("any line") {
		t.Error("Whitespace-only matcher should not match any line")
	}

	// Test case sensitivity
	filters.ClearExcludes()
	caseMatcher, err := NewMatcher("ERROR")
	if err != nil {
		t.Fatalf("Failed to create case matcher: %v", err)
	}
	filters.AddInclude(caseMatcher)

	// Should match case-insensitively
	testCases := []string{"ERROR occurred", "error occurred", "Error occurred", "an error here"}
	for _, testCase := range testCases {
		if !filters.ShouldShowLine(testCase) {
			t.Errorf("Case-insensitive matching failed for: %s", testCase)
		}
	}
}

func TestFilter_ClearOperations(t *testing.T) {
	filters := NewFilters()

	// Add some include filters
	errorMatcher, _ := NewMatcher("error")
	infoMatcher, _ := NewMatcher("info")
	filters.AddInclude(errorMatcher)
	filters.AddInclude(infoMatcher)

	// Add some exclude filters
	ignoreMatcher, _ := NewMatcher("ignore")
	debugMatcher, _ := NewMatcher("debug")
	filters.AddExclude(ignoreMatcher)
	filters.AddExclude(debugMatcher)

	// Verify filters are working
	if filters.ShouldShowLine("debug message") {
		t.Error("Debug line should be filtered out")
	}
	if !filters.ShouldShowLine("error occurred") {
		t.Error("Error line should be shown")
	}

	// Clear includes
	filters.ClearIncludes()

	// After clearing includes, all lines should be shown (except excludes)
	if !filters.ShouldShowLine("warning message") {
		t.Error("Warning should be shown after clearing includes")
	}
	if filters.ShouldShowLine("debug message") {
		t.Error("Debug should still be excluded")
	}

	// Clear excludes
	filters.ClearExcludes()

	// After clearing excludes, all lines should be shown
	if !filters.ShouldShowLine("debug message") {
		t.Error("Debug should be shown after clearing excludes")
	}
	if !filters.ShouldShowLine("any message") {
		t.Error("Any line should be shown after clearing all filters")
	}
}

func TestFilter_Integration_WithVisibility(t *testing.T) {
	// Test that filters integrate properly with the visibility system
	filters := NewFilters()
	levelMap := NewLevelMap()

	// Set up include filter
	errorMatcher, _ := NewMatcher("error")
	filters.AddInclude(errorMatcher)

	// Set up exclude filter
	ignoreMatcher, _ := NewMatcher("ignore")
	filters.AddExclude(ignoreMatcher)

	// Toggle DEBUG off
	levelMap.Toggle(1)

	// Create test events
	events := []LogEvent{
		{Seq: 1, Line: "debug error message", Level: SevDebug, Source: SourceFile},
		{Seq: 2, Line: "info error occurred", Level: SevInfo, Source: SourceFile},
		{Seq: 3, Line: "error ignore this", Level: SevError, Source: SourceFile},
		{Seq: 4, Line: "error in system", Level: SevError, Source: SourceFile},
		{Seq: 5, Line: "warning message", Level: SevWarn, Source: SourceFile},
	}

	// Create visibility plan
	plan := VisiblePlan{
		Include:  filters,
		LevelMap: levelMap,
	}

	visible := ComputeVisible(events, plan)

	// Expected:
	// Event 1: DEBUG disabled -> hidden
	// Event 2: INFO enabled, contains "error", no "ignore" -> shown
	// Event 3: ERROR enabled, contains "error", contains "ignore" -> hidden
	// Event 4: ERROR enabled, contains "error", no "ignore" -> shown
	// Event 5: WARN enabled, no "error" -> hidden

	expectedSeqs := []uint64{2, 4}
	if len(visible) != len(expectedSeqs) {
		t.Fatalf("Expected %d visible events, got %d", len(expectedSeqs), len(visible))
	}

	for i, event := range visible {
		if event.Seq != expectedSeqs[i] {
			t.Errorf("Visible event %d: expected seq %d, got %d", i, expectedSeqs[i], event.Seq)
		}
	}
}
