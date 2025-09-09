package core

import (
	"sync"
	"testing"
	"time"
)

// TestRing_AppendAndWrap verifies wrapping behavior when buffer fills
func TestRing_AppendAndWrap(t *testing.T) {
	ring := NewRing(3) // small capacity for easy testing

	// Test initial state
	if ring.Size() != 0 {
		t.Errorf("Expected size 0, got %d", ring.Size())
	}
	if ring.Capacity() != 3 {
		t.Errorf("Expected capacity 3, got %d", ring.Capacity())
	}
	if ring.CurrentSeq() != 0 {
		t.Errorf("Expected seq 0, got %d", ring.CurrentSeq())
	}

	// Add events up to capacity
	events := []LogEvent{
		{Line: "event1", Time: time.Now()},
		{Line: "event2", Time: time.Now()},
		{Line: "event3", Time: time.Now()},
	}

	for _, e := range events {
		stored := ring.Append(e)

		// Verify sequence numbers are assigned
		if stored.Seq == 0 {
			t.Error("Expected non-zero sequence number")
		}

		// Verify the original event is not modified
		if e.Seq != 0 {
			t.Error("Original event should not be modified")
		}
	}

	// Verify size and sequence
	if ring.Size() != 3 {
		t.Errorf("Expected size 3, got %d", ring.Size())
	}
	if ring.CurrentSeq() != 3 {
		t.Errorf("Expected seq 3, got %d", ring.CurrentSeq())
	}
	if ring.OldestSeq() != 1 {
		t.Errorf("Expected oldest seq 1, got %d", ring.OldestSeq())
	}

	// Verify snapshot returns events in order
	snapshot := ring.Snapshot()
	if len(snapshot) != 3 {
		t.Errorf("Expected snapshot length 3, got %d", len(snapshot))
	}

	for i, event := range snapshot {
		expectedSeq := uint64(i + 1)
		if event.Seq != expectedSeq {
			t.Errorf("Expected sequence %d, got %d", expectedSeq, event.Seq)
		}
		if event.Line != events[i].Line {
			t.Errorf("Expected line %s, got %s", events[i].Line, event.Line)
		}
	}

	// Test wrapping - add one more event
	wrapEvent := LogEvent{Line: "event4", Time: time.Now()}
	_ = ring.Append(wrapEvent)

	// Size should remain 3, but sequence increases
	if ring.Size() != 3 {
		t.Errorf("Expected size 3 after wrap, got %d", ring.Size())
	}
	if ring.CurrentSeq() != 4 {
		t.Errorf("Expected seq 4 after wrap, got %d", ring.CurrentSeq())
	}
	if ring.OldestSeq() != 2 {
		t.Errorf("Expected oldest seq 2 after wrap, got %d", ring.OldestSeq())
	}

	// Verify oldest event was overwritten
	snapshot = ring.Snapshot()
	if len(snapshot) != 3 {
		t.Errorf("Expected snapshot length 3 after wrap, got %d", len(snapshot))
	}

	// Should now contain events 2, 3, 4
	expectedLines := []string{"event2", "event3", "event4"}
	for i, event := range snapshot {
		expectedSeq := uint64(i + 2)
		if event.Seq != expectedSeq {
			t.Errorf("Expected sequence %d, got %d", expectedSeq, event.Seq)
		}
		if event.Line != expectedLines[i] {
			t.Errorf("Expected line %s, got %s", expectedLines[i], event.Line)
		}
	}

	// Verify GetBySeq works correctly
	if _, found := ring.GetBySeq(1); found {
		t.Error("Expected seq 1 to be overwritten")
	}

	if event, found := ring.GetBySeq(2); !found || event.Line != "event2" {
		t.Errorf("Expected to find event2 at seq 2, found=%v, line=%s", found, event.Line)
	}

	if event, found := ring.GetBySeq(4); !found || event.Line != "event4" {
		t.Errorf("Expected to find event4 at seq 4, found=%v, line=%s", found, event.Line)
	}
}

// TestRing_SnapshotConsistency ensures snapshots are stable copies
func TestRing_SnapshotConsistency(t *testing.T) {
	ring := NewRing(5)

	// Add some events
	for i := 1; i <= 3; i++ {
		ring.Append(LogEvent{
			Line: "event" + string(rune('0'+i)),
			Time: time.Now(),
		})
	}

	// Take a snapshot
	snapshot1 := ring.Snapshot()

	// Modify the ring
	ring.Append(LogEvent{Line: "event4", Time: time.Now()})

	// Take another snapshot
	snapshot2 := ring.Snapshot()

	// Original snapshot should be unchanged
	if len(snapshot1) != 3 {
		t.Errorf("Expected original snapshot length 3, got %d", len(snapshot1))
	}
	if len(snapshot2) != 4 {
		t.Errorf("Expected new snapshot length 4, got %d", len(snapshot2))
	}

	// Verify snapshots are independent
	for i := range snapshot1 {
		if snapshot1[i].Seq != snapshot2[i].Seq {
			continue // expected - different snapshots
		}
		if snapshot1[i].Line != snapshot2[i].Line {
			t.Error("Snapshot content should not change after modification")
		}
	}

	// Modify the snapshot directly - should not affect ring
	if len(snapshot1) > 0 {
		originalLine := snapshot1[0].Line
		snapshot1[0].Line = "modified"

		// Ring should be unaffected
		current := ring.Snapshot()
		if current[0].Line != originalLine {
			t.Error("Modifying snapshot should not affect ring contents")
		}
	}
}

// TestRing_ConcurrentReaders tests concurrent access with race detector
func TestRing_ConcurrentReaders(t *testing.T) {
	ring := NewRing(100)

	// Number of goroutines and operations
	numWriters := 5
	numReaders := 10
	opsPerGoroutine := 50

	var wg sync.WaitGroup

	// Start writers
	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				event := LogEvent{
					Line:      "writer" + string(rune('0'+writerID)) + "_event" + string(rune('0'+i)),
					Time:      time.Now(),
					Source:    SourceStdin,
					LevelStr:  "INFO",
					Level:     SevInfo,
					Container: "",
				}
				ring.Append(event)
			}
		}(w)
	}

	// Start readers
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				// Mix different read operations
				switch i % 4 {
				case 0:
					snapshot := ring.Snapshot()
					_ = len(snapshot) // use the snapshot
				case 1:
					_ = ring.Size()
				case 2:
					_ = ring.CurrentSeq()
				case 3:
					seq := ring.OldestSeq()
					if seq > 0 {
						ring.GetBySeq(seq)
					}
				}
			}
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify final state is consistent
	finalSnapshot := ring.Snapshot()
	if len(finalSnapshot) > ring.Capacity() {
		t.Errorf("Snapshot length %d exceeds capacity %d", len(finalSnapshot), ring.Capacity())
	}

	// Verify sequence numbers are increasing
	for i := 1; i < len(finalSnapshot); i++ {
		if finalSnapshot[i].Seq <= finalSnapshot[i-1].Seq {
			t.Errorf("Sequence numbers not increasing: %d <= %d",
				finalSnapshot[i].Seq, finalSnapshot[i-1].Seq)
		}
	}
}

// TestRing_EdgeCases tests various edge cases
func TestRing_EdgeCases(t *testing.T) {
	t.Run("ZeroCapacity", func(t *testing.T) {
		ring := NewRing(0)
		if ring.Capacity() != 10000 {
			t.Errorf("Expected default capacity 10000 for zero input, got %d", ring.Capacity())
		}
	})

	t.Run("NegativeCapacity", func(t *testing.T) {
		ring := NewRing(-5)
		if ring.Capacity() != 10000 {
			t.Errorf("Expected default capacity 10000 for negative input, got %d", ring.Capacity())
		}
	})

	t.Run("EmptyRing", func(t *testing.T) {
		ring := NewRing(10)

		snapshot := ring.Snapshot()
		if snapshot != nil {
			t.Errorf("Expected nil snapshot for empty ring, got %v", snapshot)
		}

		if ring.OldestSeq() != 0 {
			t.Errorf("Expected oldest seq 0 for empty ring, got %d", ring.OldestSeq())
		}

		if _, found := ring.GetBySeq(1); found {
			t.Error("Expected GetBySeq to return false for empty ring")
		}
	})

	t.Run("SingleCapacity", func(t *testing.T) {
		ring := NewRing(1)

		// Add first event
		event1 := ring.Append(LogEvent{Line: "first"})
		if event1.Seq != 1 {
			t.Errorf("Expected seq 1, got %d", event1.Seq)
		}

		snapshot := ring.Snapshot()
		if len(snapshot) != 1 || snapshot[0].Line != "first" {
			t.Errorf("Unexpected snapshot: %v", snapshot)
		}

		// Add second event (should overwrite)
		event2 := ring.Append(LogEvent{Line: "second"})
		if event2.Seq != 2 {
			t.Errorf("Expected seq 2, got %d", event2.Seq)
		}

		snapshot = ring.Snapshot()
		if len(snapshot) != 1 || snapshot[0].Line != "second" {
			t.Errorf("Unexpected snapshot after overwrite: %v", snapshot)
		}

		// First event should be gone
		if _, found := ring.GetBySeq(1); found {
			t.Error("Expected first event to be overwritten")
		}

		// Second event should be findable
		if event, found := ring.GetBySeq(2); !found || event.Line != "second" {
			t.Errorf("Expected to find second event, found=%v, line=%s", found, event.Line)
		}
	})

	t.Run("GetBySeqBounds", func(t *testing.T) {
		ring := NewRing(3)

		// Test with empty ring
		if _, found := ring.GetBySeq(0); found {
			t.Error("Expected seq 0 to not be found")
		}

		// Add events
		ring.Append(LogEvent{Line: "event1"})
		ring.Append(LogEvent{Line: "event2"})
		ring.Append(LogEvent{Line: "event3"})

		// Test bounds
		if _, found := ring.GetBySeq(0); found {
			t.Error("Expected seq 0 to not be found")
		}

		if _, found := ring.GetBySeq(4); found {
			t.Error("Expected seq 4 to not be found")
		}

		// Test valid range
		for seq := uint64(1); seq <= 3; seq++ {
			if _, found := ring.GetBySeq(seq); !found {
				t.Errorf("Expected seq %d to be found", seq)
			}
		}
	})
}

// BenchmarkRing_Append tests append performance
func BenchmarkRing_Append(b *testing.B) {
	ring := NewRing(10000)
	event := LogEvent{
		Line:     "benchmark event with some text content",
		Time:     time.Now(),
		Source:   SourceStdin,
		LevelStr: "INFO",
		Level:    SevInfo,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ring.Append(event)
	}
}

// BenchmarkRing_Snapshot tests snapshot performance
func BenchmarkRing_Snapshot(b *testing.B) {
	ring := NewRing(10000)
	event := LogEvent{
		Line:     "benchmark event with some text content",
		Time:     time.Now(),
		Source:   SourceStdin,
		LevelStr: "INFO",
		Level:    SevInfo,
	}

	// Fill the ring
	for i := 0; i < 10000; i++ {
		ring.Append(event)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snapshot := ring.Snapshot()
		_ = snapshot // prevent optimization
	}
}

// BenchmarkRing_GetBySeq tests GetBySeq performance
func BenchmarkRing_GetBySeq(b *testing.B) {
	ring := NewRing(10000)
	event := LogEvent{
		Line:     "benchmark event with some text content",
		Time:     time.Now(),
		Source:   SourceStdin,
		LevelStr: "INFO",
		Level:    SevInfo,
	}

	// Fill the ring
	for i := 0; i < 10000; i++ {
		ring.Append(event)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		seq := uint64((i % 10000) + 1)
		event, found := ring.GetBySeq(seq)
		_ = event
		_ = found
	}
}
