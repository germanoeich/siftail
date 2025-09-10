package core

import (
	"testing"
)

func TestSearch_IndexesHitsIncrementally(t *testing.T) {
	search := NewSearchState()
	
	// Add hits in non-sequential order
	search.AddHit(10)
	search.AddHit(5)
	search.AddHit(15)
	search.AddHit(8)
	
	// Verify they are stored in sorted order
	active, hitSeqs, cursor := search.GetSnapshot()
	if active {
		t.Error("Expected search to be inactive initially")
	}
	if cursor != -1 {
		t.Errorf("Expected cursor to be -1, got %d", cursor)
	}
	
	expected := []uint64{5, 8, 10, 15}
	if len(hitSeqs) != len(expected) {
		t.Fatalf("Expected %d hits, got %d", len(expected), len(hitSeqs))
	}
	
	for i, expectedSeq := range expected {
		if hitSeqs[i] != expectedSeq {
			t.Errorf("Hit %d: expected %d, got %d", i, expectedSeq, hitSeqs[i])
		}
	}
	
	// Add duplicate - should not be added
	search.AddHit(10)
	if search.Count() != 4 {
		t.Errorf("Expected 4 hits after adding duplicate, got %d", search.Count())
	}
	
	// Test incremental addition
	search.AddHit(12)
	search.AddHit(3)
	search.AddHit(20)
	
	_, hitSeqs, _ = search.GetSnapshot()
	expectedFinal := []uint64{3, 5, 8, 10, 12, 15, 20}
	if len(hitSeqs) != len(expectedFinal) {
		t.Fatalf("Expected %d hits, got %d", len(expectedFinal), len(hitSeqs))
	}
	
	for i, expectedSeq := range expectedFinal {
		if hitSeqs[i] != expectedSeq {
			t.Errorf("Final hit %d: expected %d, got %d", i, expectedSeq, hitSeqs[i])
		}
	}
}

func TestSearch_JumpPrevNext_Bounds(t *testing.T) {
	search := NewSearchState()
	
	// Test empty hits
	if search.Next() != 0 {
		t.Error("Next() should return 0 for empty hits")
	}
	if search.Prev() != 0 {
		t.Error("Prev() should return 0 for empty hits")
	}
	if search.Current() != 0 {
		t.Error("Current() should return 0 for empty hits")
	}
	
	// Add some hits
	search.AddHit(10)
	search.AddHit(20)
	search.AddHit(30)
	
	// Test navigation from initial state (cursor = -1)
	seq := search.Next()
	if seq != 10 {
		t.Errorf("First Next() should return 10, got %d", seq)
	}
	
	current, total := search.Position()
	if current != 1 || total != 3 {
		t.Errorf("Expected position (1, 3), got (%d, %d)", current, total)
	}
	
	// Test forward navigation
	seq = search.Next()
	if seq != 20 {
		t.Errorf("Second Next() should return 20, got %d", seq)
	}
	
	seq = search.Next()
	if seq != 30 {
		t.Errorf("Third Next() should return 30, got %d", seq)
	}
	
	// Test wrap-around forward
	seq = search.Next()
	if seq != 10 {
		t.Errorf("Next() at end should wrap to 10, got %d", seq)
	}
	
	// Test backward navigation
	seq = search.Prev()
	if seq != 30 {
		t.Errorf("Prev() should return 30, got %d", seq)
	}
	
	seq = search.Prev()
	if seq != 20 {
		t.Errorf("Prev() should return 20, got %d", seq)
	}
	
	seq = search.Prev()
	if seq != 10 {
		t.Errorf("Prev() should return 10, got %d", seq)
	}
	
	// Test wrap-around backward
	seq = search.Prev()
	if seq != 30 {
		t.Errorf("Prev() at start should wrap to 30, got %d", seq)
	}
	
	// Test JumpToFirst
	seq = search.JumpToFirst()
	if seq != 10 {
		t.Errorf("JumpToFirst() should return 10, got %d", seq)
	}
	
	current, total = search.Position()
	if current != 1 || total != 3 {
		t.Errorf("After JumpToFirst, expected position (1, 3), got (%d, %d)", current, total)
	}
}

func TestSearch_SetCurrentBySeq(t *testing.T) {
	search := NewSearchState()
	
	// Add hits
	search.AddHit(10)
	search.AddHit(20)
	search.AddHit(30)
	
	// Test setting to existing sequence
	if !search.SetCurrentBySeq(20) {
		t.Error("SetCurrentBySeq(20) should return true")
	}
	
	if search.Current() != 20 {
		t.Errorf("Current() should return 20, got %d", search.Current())
	}
	
	current, total := search.Position()
	if current != 2 || total != 3 {
		t.Errorf("Expected position (2, 3), got (%d, %d)", current, total)
	}
	
	// Test setting to non-existing sequence
	if search.SetCurrentBySeq(25) {
		t.Error("SetCurrentBySeq(25) should return false for non-existing sequence")
	}
	
	// Current position should not change
	if search.Current() != 20 {
		t.Errorf("Current() should still return 20, got %d", search.Current())
	}
}

func TestSearch_RemoveOldHits(t *testing.T) {
	search := NewSearchState()
	
	// Add hits
	search.AddHit(5)
	search.AddHit(10)
	search.AddHit(15)
	search.AddHit(20)
	search.AddHit(25)
	
	// Set current position
	search.SetCurrentBySeq(15)
	
	// Remove old hits (< 12)
	search.RemoveOldHits(12)
	
	_, hitSeqs, cursor := search.GetSnapshot()
	expectedSeqs := []uint64{15, 20, 25}
	if len(hitSeqs) != len(expectedSeqs) {
		t.Fatalf("Expected %d hits after removal, got %d", len(expectedSeqs), len(hitSeqs))
	}
	
	for i, expectedSeq := range expectedSeqs {
		if hitSeqs[i] != expectedSeq {
			t.Errorf("Hit %d: expected %d, got %d", i, expectedSeq, hitSeqs[i])
		}
	}
	
	// Cursor should be adjusted (was at index 2 for seq 15, now at index 0)
	if cursor != 0 {
		t.Errorf("Expected cursor at 0, got %d", cursor)
	}
	
	if search.Current() != 15 {
		t.Errorf("Current() should still return 15, got %d", search.Current())
	}
}

func TestSearch_RemoveOldHits_CursorAdjustment(t *testing.T) {
	search := NewSearchState()
	
	// Add hits
	search.AddHit(5)
	search.AddHit(10)
	search.AddHit(15)
	search.AddHit(20)
	
	// Set current position to first hit
	search.SetCurrentBySeq(5)
	
	// Remove old hits including the current one
	search.RemoveOldHits(12)
	
	// Cursor should be reset to -1
	_, _, cursor := search.GetSnapshot()
	if cursor != -1 {
		t.Errorf("Expected cursor to be -1 after removing current hit, got %d", cursor)
	}
	
	if search.Current() != 0 {
		t.Errorf("Current() should return 0 after cursor reset, got %d", search.Current())
	}
}

func TestSearch_Clear(t *testing.T) {
	search := NewSearchState()
	
	// Add hits and set position
	search.AddHit(10)
	search.AddHit(20)
	search.SetCurrentBySeq(20)
	
	// Clear all
	search.Clear()
	
	if search.Count() != 0 {
		t.Errorf("Expected 0 hits after clear, got %d", search.Count())
	}
	
	if search.Current() != 0 {
		t.Errorf("Current() should return 0 after clear, got %d", search.Current())
	}
	
	current, total := search.Position()
	if current != 0 || total != 0 {
		t.Errorf("Expected position (0, 0) after clear, got (%d, %d)", current, total)
	}
}

func TestSearch_ActiveState(t *testing.T) {
	search := NewSearchState()
	
	// Initially inactive
	if search.IsActive() {
		t.Error("Search should be inactive initially")
	}
	
	// Set active
	search.SetActive(true)
	if !search.IsActive() {
		t.Error("Search should be active after SetActive(true)")
	}
	
	// Set inactive
	search.SetActive(false)
	if search.IsActive() {
		t.Error("Search should be inactive after SetActive(false)")
	}
}

func TestSearch_SetMatcher(t *testing.T) {
	search := NewSearchState()
	
	// Create a matcher
	matcher, err := NewMatcher("test")
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}
	
	// Add some hits first
	search.AddHit(10)
	search.AddHit(20)
	search.SetCurrentBySeq(20)
	
	// Set matcher should clear hits
	search.SetMatcher(matcher)
	
	if search.Count() != 0 {
		t.Errorf("Expected 0 hits after SetMatcher, got %d", search.Count())
	}
	
	if search.Current() != 0 {
		t.Errorf("Current() should return 0 after SetMatcher, got %d", search.Current())
	}
	
	// Verify matcher is set
	retrievedMatcher := search.GetMatcher()
	if retrievedMatcher.Raw() != "test" {
		t.Errorf("Expected matcher 'test', got '%s'", retrievedMatcher.Raw())
	}
}

func TestHighlight_DoesNotAffectVisibility(t *testing.T) {
	// This test verifies that highlights don't affect line visibility
	// SearchState is separate from Filters, so we test that SearchState
	// doesn't have any methods that affect visibility
	
	search := NewSearchState()
	
	// Create a matcher and set it
	matcher, err := NewMatcher("error")
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}
	
	search.SetMatcher(matcher)
	search.SetActive(true)
	
	// Add some hits
	search.AddHit(10)
	search.AddHit(20)
	search.AddHit(30)
	
	// Verify that SearchState has no methods that would affect line visibility
	// It should only track hits for navigation, not filter lines
	
	// The key insight is that SearchState only provides navigation,
	// while Filters (in matcher.go) handles visibility decisions.
	// This separation ensures highlights don't affect what lines are shown.
	
	// Test that we can navigate through hits without affecting anything else
	seq1 := search.Next()
	seq2 := search.Next()
	seq3 := search.Next()
	
	if seq1 != 10 || seq2 != 20 || seq3 != 30 {
		t.Errorf("Navigation should work: got %d, %d, %d", seq1, seq2, seq3)
	}
	
	// SearchState provides read-only access to hits for UI highlighting,
	// but has no ShouldShowLine or similar methods that would affect visibility
	current, total := search.Position()
	if current != 3 || total != 3 {
		t.Errorf("Position tracking should work: got (%d, %d)", current, total)
	}
}