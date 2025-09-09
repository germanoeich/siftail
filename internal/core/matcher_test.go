package core

import (
	"testing"
)

func TestMatcher_SubstringAndRegex(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		line      string
		expected  bool
		expectErr bool
	}{
		// Substring matching tests
		{
			name:     "case insensitive substring match",
			pattern:  "error",
			line:     "This is an ERROR message",
			expected: true,
		},
		{
			name:     "case insensitive substring no match",
			pattern:  "error",
			line:     "This is a warning message",
			expected: false,
		},
		{
			name:     "substring match with special characters",
			pattern:  "user@domain.com",
			line:     "Login failed for user@domain.com",
			expected: true,
		},
		{
			name:     "empty pattern",
			pattern:  "",
			line:     "any line",
			expected: false,
		},
		{
			name:     "whitespace pattern",
			pattern:  "   ",
			line:     "any line",
			expected: false,
		},
		{
			name:     "pattern with whitespace",
			pattern:  " error ",
			line:     "This has error in it",
			expected: true,
		},

		// Regex tests
		{
			name:     "simple regex match",
			pattern:  "/error.*/",
			line:     "error occurred at runtime",
			expected: true,
		},
		{
			name:     "regex no match",
			pattern:  "/error.*/",
			line:     "warning: something happened",
			expected: false,
		},
		{
			name:     "regex case insensitive",
			pattern:  "/ERROR.*/",
			line:     "error occurred",
			expected: true,
		},
		{
			name:     "regex word boundary",
			pattern:  "/\\berror\\b/",
			line:     "error message",
			expected: true,
		},
		{
			name:     "regex word boundary no match",
			pattern:  "/\\berror\\b/",
			line:     "terrrorist message",
			expected: false,
		},
		{
			name:     "regex with numbers",
			pattern:  "/\\d{3}-\\d{3}-\\d{4}/",
			line:     "Call us at 555-123-4567",
			expected: true,
		},
		{
			name:     "regex with character classes",
			pattern:  "/[A-Z]{2,}/",
			line:     "This has UPPERCASE text",
			expected: true,
		},
		{
			name:      "invalid regex",
			pattern:   "/[unclosed/",
			line:      "any line",
			expected:  false,
			expectErr: true,
		},
		{
			name:     "two slashes as substring",
			pattern:  "//",
			line:     "http://example.com",
			expected: true, // should match as substring
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := NewMatcher(tt.pattern)

			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error for pattern %q, but got none", tt.pattern)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error creating matcher for pattern %q: %v", tt.pattern, err)
				return
			}

			result := matcher.Match(tt.line)
			if result != tt.expected {
				t.Errorf("Match(%q) with pattern %q = %v, expected %v", tt.line, tt.pattern, result, tt.expected)
			}

			// Test that Raw() returns the original pattern
			if matcher.Raw() != tt.pattern {
				t.Errorf("Raw() = %q, expected %q", matcher.Raw(), tt.pattern)
			}

			// Test IsRegex()
			expectedRegex := len(tt.pattern) >= 3 && tt.pattern[0] == '/' && tt.pattern[len(tt.pattern)-1] == '/'
			if matcher.IsRegex() != expectedRegex {
				t.Errorf("IsRegex() = %v, expected %v for pattern %q", matcher.IsRegex(), expectedRegex, tt.pattern)
			}
		})
	}
}

func TestFilters_IncludeExclude(t *testing.T) {
	tests := []struct {
		name       string
		includes   []string
		excludes   []string
		line       string
		shouldShow bool
	}{
		{
			name:       "no filters - show everything",
			includes:   []string{},
			excludes:   []string{},
			line:       "any log line",
			shouldShow: true,
		},
		{
			name:       "include filter match",
			includes:   []string{"error"},
			excludes:   []string{},
			line:       "error occurred",
			shouldShow: true,
		},
		{
			name:       "include filter no match",
			includes:   []string{"error"},
			excludes:   []string{},
			line:       "info message",
			shouldShow: false,
		},
		{
			name:       "multiple includes - match first",
			includes:   []string{"error", "warning"},
			excludes:   []string{},
			line:       "error occurred",
			shouldShow: true,
		},
		{
			name:       "multiple includes - match second",
			includes:   []string{"error", "warning"},
			excludes:   []string{},
			line:       "warning: low disk space",
			shouldShow: true,
		},
		{
			name:       "multiple includes - no match",
			includes:   []string{"error", "warning"},
			excludes:   []string{},
			line:       "info message",
			shouldShow: false,
		},
		{
			name:       "exclude filter match",
			includes:   []string{},
			excludes:   []string{"debug"},
			line:       "debug: verbose output",
			shouldShow: false,
		},
		{
			name:       "exclude filter no match",
			includes:   []string{},
			excludes:   []string{"debug"},
			line:       "info message",
			shouldShow: true,
		},
		{
			name:       "include and exclude - include wins",
			includes:   []string{"error"},
			excludes:   []string{"debug"},
			line:       "error in debug mode",
			shouldShow: false, // exclude takes precedence
		},
		{
			name:       "include and exclude - both match, exclude wins",
			includes:   []string{"important"},
			excludes:   []string{"debug"},
			line:       "important debug information",
			shouldShow: false,
		},
		{
			name:       "include and exclude - only include matches",
			includes:   []string{"error"},
			excludes:   []string{"debug"},
			line:       "error occurred",
			shouldShow: true,
		},
		{
			name:       "multiple excludes",
			includes:   []string{},
			excludes:   []string{"debug", "trace"},
			line:       "trace execution",
			shouldShow: false,
		},
		{
			name:       "case insensitive include",
			includes:   []string{"ERROR"},
			excludes:   []string{},
			line:       "error occurred",
			shouldShow: true,
		},
		{
			name:       "case insensitive exclude",
			includes:   []string{},
			excludes:   []string{"DEBUG"},
			line:       "debug message",
			shouldShow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters := NewFilters()

			// Add include filters
			for _, include := range tt.includes {
				matcher, err := NewMatcher(include)
				if err != nil {
					t.Fatalf("Failed to create include matcher for %q: %v", include, err)
				}
				filters.AddInclude(matcher)
			}

			// Add exclude filters
			for _, exclude := range tt.excludes {
				matcher, err := NewMatcher(exclude)
				if err != nil {
					t.Fatalf("Failed to create exclude matcher for %q: %v", exclude, err)
				}
				filters.AddExclude(matcher)
			}

			result := filters.ShouldShowLine(tt.line)
			if result != tt.shouldShow {
				t.Errorf("ShouldShowLine(%q) = %v, expected %v", tt.line, result, tt.shouldShow)
				t.Logf("Includes: %v, Excludes: %v", tt.includes, tt.excludes)
			}
		})
	}
}

func TestFilters_Highlights(t *testing.T) {
	filters := NewFilters()

	// Add some highlight patterns
	errorMatcher, _ := NewMatcher("error")
	warnMatcher, _ := NewMatcher("/warn.*/")
	filters.AddHighlight(errorMatcher)
	filters.AddHighlight(warnMatcher)

	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "matches error highlight",
			line:     "error occurred",
			expected: true,
		},
		{
			name:     "matches warning regex highlight",
			line:     "warning: disk full",
			expected: true,
		},
		{
			name:     "no highlight match",
			line:     "info message",
			expected: false,
		},
		{
			name:     "case insensitive highlight",
			line:     "ERROR occurred",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filters.ShouldHighlight(tt.line)
			if result != tt.expected {
				t.Errorf("ShouldHighlight(%q) = %v, expected %v", tt.line, result, tt.expected)
			}
		})
	}
}

func TestFilters_ClearOperations(t *testing.T) {
	filters := NewFilters()

	// Add filters
	matcher1, _ := NewMatcher("test1")
	matcher2, _ := NewMatcher("test2")
	matcher3, _ := NewMatcher("test3")

	filters.AddInclude(matcher1)
	filters.AddExclude(matcher2)
	filters.AddHighlight(matcher3)

	// Verify they're added
	if len(filters.Include) != 1 {
		t.Errorf("Expected 1 include filter, got %d", len(filters.Include))
	}
	if len(filters.Exclude) != 1 {
		t.Errorf("Expected 1 exclude filter, got %d", len(filters.Exclude))
	}
	if len(filters.Highlights) != 1 {
		t.Errorf("Expected 1 highlight filter, got %d", len(filters.Highlights))
	}

	// Test clearing
	filters.ClearIncludes()
	if len(filters.Include) != 0 {
		t.Errorf("Expected 0 include filters after clear, got %d", len(filters.Include))
	}

	filters.ClearExcludes()
	if len(filters.Exclude) != 0 {
		t.Errorf("Expected 0 exclude filters after clear, got %d", len(filters.Exclude))
	}

	filters.ClearHighlights()
	if len(filters.Highlights) != 0 {
		t.Errorf("Expected 0 highlight filters after clear, got %d", len(filters.Highlights))
	}
}

func TestFindIndexing_NextPrevAcrossStream(t *testing.T) {
	// Test find index operations with streaming data
	findIndex := NewFindIndex(100) // capacity of 100

	matcher, err := NewMatcher("error")
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}
	findIndex.SetMatcher(matcher)

	// Test empty index
	if findIndex.Count() != 0 {
		t.Errorf("Expected count 0 for empty index, got %d", findIndex.Count())
	}
	if findIndex.Current() != 0 {
		t.Errorf("Expected current 0 for empty index, got %d", findIndex.Current())
	}
	if findIndex.Next() != 0 {
		t.Errorf("Expected Next() to return 0 for empty index, got %d", findIndex.Next())
	}
	if findIndex.Prev() != 0 {
		t.Errorf("Expected Prev() to return 0 for empty index, got %d", findIndex.Prev())
	}

	// Add some matches
	sequences := []uint64{10, 25, 50, 75, 100}
	for _, seq := range sequences {
		findIndex.AddMatch(seq)
	}

	// Test count
	if findIndex.Count() != len(sequences) {
		t.Errorf("Expected count %d, got %d", len(sequences), findIndex.Count())
	}

	// Test navigation from initial state (no current selection)
	first := findIndex.Next()
	if first != sequences[0] {
		t.Errorf("Expected first Next() to return %d, got %d", sequences[0], first)
	}

	// Test forward navigation
	for i := 1; i < len(sequences); i++ {
		next := findIndex.Next()
		if next != sequences[i] {
			t.Errorf("Expected Next() to return %d, got %d", sequences[i], next)
		}
	}

	// Test wrap-around to beginning
	wrapped := findIndex.Next()
	if wrapped != sequences[0] {
		t.Errorf("Expected Next() to wrap to %d, got %d", sequences[0], wrapped)
	}

	// Test backward navigation
	for i := len(sequences) - 1; i >= 0; i-- {
		prev := findIndex.Prev()
		if prev != sequences[i] {
			t.Errorf("Expected Prev() to return %d, got %d", sequences[i], prev)
		}
	}

	// Test wrap-around to end
	wrapped = findIndex.Prev()
	if wrapped != sequences[len(sequences)-1] {
		t.Errorf("Expected Prev() to wrap to %d, got %d", sequences[len(sequences)-1], wrapped)
	}

	// Test Current()
	current := findIndex.Current()
	if current != sequences[len(sequences)-1] {
		t.Errorf("Expected Current() to return %d, got %d", sequences[len(sequences)-1], current)
	}

	// Test Position()
	currentPos, total := findIndex.Position()
	if currentPos != len(sequences) || total != len(sequences) {
		t.Errorf("Expected Position() to return (%d, %d), got (%d, %d)", len(sequences), len(sequences), currentPos, total)
	}

	// Test SetCurrentBySeq
	if !findIndex.SetCurrentBySeq(25) {
		t.Errorf("Expected SetCurrentBySeq(25) to return true")
	}
	if findIndex.Current() != 25 {
		t.Errorf("Expected Current() to return 25 after SetCurrentBySeq(25), got %d", findIndex.Current())
	}

	currentPos, total = findIndex.Position()
	if currentPos != 2 || total != 5 {
		t.Errorf("Expected Position() to return (2, 5) after SetCurrentBySeq(25), got (%d, %d)", currentPos, total)
	}

	// Test SetCurrentBySeq with non-existent sequence
	if findIndex.SetCurrentBySeq(999) {
		t.Errorf("Expected SetCurrentBySeq(999) to return false")
	}
	// Current should be unchanged
	if findIndex.Current() != 25 {
		t.Errorf("Expected Current() to remain 25 after failed SetCurrentBySeq, got %d", findIndex.Current())
	}
}

func TestFindIndex_AddMatchSorted(t *testing.T) {
	findIndex := NewFindIndex(10)
	matcher, _ := NewMatcher("test")
	findIndex.SetMatcher(matcher)

	// Add matches out of order
	sequences := []uint64{50, 10, 75, 25, 100}
	for _, seq := range sequences {
		findIndex.AddMatch(seq)
	}

	// Should be sorted internally
	expected := []uint64{10, 25, 50, 75, 100}
	for i, expectedSeq := range expected {
		findIndex.SetCurrentBySeq(expectedSeq)
		currentPos, _ := findIndex.Position()
		if currentPos != i+1 {
			t.Errorf("Expected position %d for sequence %d, got %d", i+1, expectedSeq, currentPos)
		}
	}

	// Test duplicate handling
	findIndex.AddMatch(50) // duplicate
	if findIndex.Count() != len(expected) {
		t.Errorf("Expected count to remain %d after adding duplicate, got %d", len(expected), findIndex.Count())
	}
}

func TestFindIndex_CapacityLimits(t *testing.T) {
	// Test with small capacity
	findIndex := NewFindIndex(3)
	matcher, _ := NewMatcher("test")
	findIndex.SetMatcher(matcher)

	// Add more matches than capacity
	sequences := []uint64{10, 20, 30, 40, 50}
	for _, seq := range sequences {
		findIndex.AddMatch(seq)
	}

	// Should only keep the 3 most recent (largest) sequences
	if findIndex.Count() != 3 {
		t.Errorf("Expected count 3 after exceeding capacity, got %d", findIndex.Count())
	}

	// Should keep sequences 30, 40, 50
	expected := []uint64{30, 40, 50}
	for i, expectedSeq := range expected {
		if !findIndex.SetCurrentBySeq(expectedSeq) {
			t.Errorf("Expected to find sequence %d in index", expectedSeq)
		}
		currentPos, _ := findIndex.Position()
		if currentPos != i+1 {
			t.Errorf("Expected position %d for sequence %d, got %d", i+1, expectedSeq, currentPos)
		}
	}

	// Old sequences should be gone
	if findIndex.SetCurrentBySeq(10) {
		t.Errorf("Expected sequence 10 to be removed from index")
	}
	if findIndex.SetCurrentBySeq(20) {
		t.Errorf("Expected sequence 20 to be removed from index")
	}
}

func TestFindIndex_RemoveOldMatches(t *testing.T) {
	findIndex := NewFindIndex(100)
	matcher, _ := NewMatcher("test")
	findIndex.SetMatcher(matcher)

	// Add some matches
	sequences := []uint64{10, 20, 30, 40, 50}
	for _, seq := range sequences {
		findIndex.AddMatch(seq)
	}

	// Set current to middle
	findIndex.SetCurrentBySeq(30)

	// Remove old matches (< 30)
	findIndex.RemoveOldMatches(30)

	// Should have sequences 30, 40, 50
	if findIndex.Count() != 3 {
		t.Errorf("Expected count 3 after RemoveOldMatches(30), got %d", findIndex.Count())
	}

	// Current should still be valid (sequence 30)
	if findIndex.Current() != 30 {
		t.Errorf("Expected Current() to return 30 after RemoveOldMatches, got %d", findIndex.Current())
	}

	// Removed sequences should not be found
	if findIndex.SetCurrentBySeq(10) {
		t.Errorf("Expected sequence 10 to be removed")
	}
	if findIndex.SetCurrentBySeq(20) {
		t.Errorf("Expected sequence 20 to be removed")
	}

	// Remaining sequences should be found
	for _, seq := range []uint64{30, 40, 50} {
		if !findIndex.SetCurrentBySeq(seq) {
			t.Errorf("Expected to find remaining sequence %d", seq)
		}
	}

	// Test removing matches that affect current position
	findIndex.SetCurrentBySeq(30)
	findIndex.RemoveOldMatches(40) // Remove 30

	// Current should be invalidated
	if findIndex.Current() != 0 {
		t.Errorf("Expected Current() to return 0 after current match removed, got %d", findIndex.Current())
	}

	// Should only have sequences 40, 50
	if findIndex.Count() != 2 {
		t.Errorf("Expected count 2 after RemoveOldMatches(40), got %d", findIndex.Count())
	}
}

func TestFindIndex_SetMatcher(t *testing.T) {
	findIndex := NewFindIndex(100)

	// Set initial matcher
	matcher1, _ := NewMatcher("error")
	findIndex.SetMatcher(matcher1)

	// Add some matches
	sequences := []uint64{10, 20, 30}
	for _, seq := range sequences {
		findIndex.AddMatch(seq)
	}
	findIndex.SetCurrentBySeq(20)

	// Verify state
	if findIndex.Count() != 3 {
		t.Errorf("Expected count 3, got %d", findIndex.Count())
	}
	if findIndex.Current() != 20 {
		t.Errorf("Expected current 20, got %d", findIndex.Current())
	}

	// Change matcher
	matcher2, _ := NewMatcher("warning")
	findIndex.SetMatcher(matcher2)

	// Should clear all matches and current position
	if findIndex.Count() != 0 {
		t.Errorf("Expected count 0 after SetMatcher, got %d", findIndex.Count())
	}
	if findIndex.Current() != 0 {
		t.Errorf("Expected current 0 after SetMatcher, got %d", findIndex.Current())
	}

	// Verify new matcher is set
	newMatcher := findIndex.GetMatcher()
	if newMatcher.Raw() != "warning" {
		t.Errorf("Expected new matcher pattern 'warning', got %q", newMatcher.Raw())
	}
}
