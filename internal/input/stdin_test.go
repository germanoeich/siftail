package input

import (
	"context"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/germanoeich/siftail/internal/core"
)

func TestStdinReader_LongLines(t *testing.T) {
	// Create a very long line (> 64KB) to test beyond Scanner's default limit
	longLine := strings.Repeat("a", 100*1024) // 100KB line
	input := longLine + "\nshort line\n"

	reader := NewStdinReaderFromReader(strings.NewReader(input))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eventCh, errCh := reader.Start(ctx)

	var events []core.LogEvent
	var errors []error

	done := make(chan bool)
	go func() {
		defer close(done)
		for {
			select {
			case event, ok := <-eventCh:
				if !ok {
					eventCh = nil
					if errCh == nil {
						return
					}
					continue
				}
				events = append(events, event)
			case err, ok := <-errCh:
				if !ok {
					errCh = nil
					if eventCh == nil {
						return
					}
					continue
				}
				errors = append(errors, err)
			case <-ctx.Done():
				return
			}
		}
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}

	// Should have no errors
	if len(errors) != 0 {
		t.Errorf("Expected no errors, got %d: %v", len(errors), errors)
	}

	// Should have exactly 2 events
	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}

	// First event should be the long line
	if len(events[0].Line) != 100*1024 {
		t.Errorf("Expected first line length %d, got %d", 100*1024, len(events[0].Line))
	}
	if events[0].Line != longLine {
		t.Error("First line content doesn't match expected long line")
	}

	// Second event should be the short line
	if events[1].Line != "short line" {
		t.Errorf("Expected second line 'short line', got %q", events[1].Line)
	}

	// Verify properties of events
	for i, event := range events {
		if event.Source != core.SourceStdin {
			t.Errorf("Event %d: expected source SourceStdin, got %v", i, event.Source)
		}
		if event.Container != "" {
			t.Errorf("Event %d: expected empty container, got %q", i, event.Container)
		}
		if event.Seq != uint64(i+1) {
			t.Errorf("Event %d: expected seq %d, got %d", i, i+1, event.Seq)
		}
		if event.Time.IsZero() {
			t.Errorf("Event %d: expected non-zero timestamp", i)
		}
	}
}

func TestStdinReader_StopsOnEOF(t *testing.T) {
	// Track goroutines before test
	initialGoroutines := runtime.NumGoroutine()

	input := "line1\nline2\nline3"
	reader := NewStdinReaderFromReader(strings.NewReader(input))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eventCh, errCh := reader.Start(ctx)

	var events []core.LogEvent
	var errors []error

	done := make(chan bool)
	go func() {
		defer close(done)
		for {
			select {
			case event, ok := <-eventCh:
				if !ok {
					eventCh = nil
					if errCh == nil {
						return
					}
					continue
				}
				events = append(events, event)
			case err, ok := <-errCh:
				if !ok {
					errCh = nil
					if eventCh == nil {
						return
					}
					continue
				}
				errors = append(errors, err)
			case <-ctx.Done():
				t.Error("Context cancelled before EOF")
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Test timed out - should have stopped on EOF")
	}

	// Should have no errors
	if len(errors) != 0 {
		t.Errorf("Expected no errors, got %d: %v", len(errors), errors)
	}

	// Should have exactly 3 events (including the line without trailing newline)
	if len(events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(events))
	}

	expectedLines := []string{"line1", "line2", "line3"}
	for i, expected := range expectedLines {
		if events[i].Line != expected {
			t.Errorf("Event %d: expected line %q, got %q", i, expected, events[i].Line)
		}
	}

	// Give goroutines time to exit
	time.Sleep(100 * time.Millisecond)

	// Check for goroutine leaks
	finalGoroutines := runtime.NumGoroutine()
	if finalGoroutines > initialGoroutines+2 { // allow for small variance in test framework
		t.Errorf("Potential goroutine leak: started with %d, ended with %d goroutines",
			initialGoroutines, finalGoroutines)
	}
}

func TestStdinReader_NewlineHandling(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "unix newlines",
			input:    "line1\nline2\nline3\n",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "windows newlines",
			input:    "line1\r\nline2\r\nline3\r\n",
			expected: []string{"line1\r", "line2\r", "line3\r"},
		},
		{
			name:     "mixed newlines",
			input:    "line1\nline2\r\nline3\n",
			expected: []string{"line1", "line2\r", "line3"},
		},
		{
			name:     "no trailing newline",
			input:    "line1\nline2",
			expected: []string{"line1", "line2"},
		},
		{
			name:     "empty lines",
			input:    "line1\n\nline3\n",
			expected: []string{"line1", "", "line3"},
		},
		{
			name:     "single line no newline",
			input:    "single line",
			expected: []string{"single line"},
		},
		{
			name:     "only newline",
			input:    "\n",
			expected: []string{""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := NewStdinReaderFromReader(strings.NewReader(tc.input))

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			eventCh, errCh := reader.Start(ctx)

			var events []core.LogEvent
			var errors []error

			done := make(chan bool)
			go func() {
				defer close(done)
				for {
					select {
					case event, ok := <-eventCh:
						if !ok {
							eventCh = nil
							if errCh == nil {
								return
							}
							continue
						}
						events = append(events, event)
					case err, ok := <-errCh:
						if !ok {
							errCh = nil
							if eventCh == nil {
								return
							}
							continue
						}
						errors = append(errors, err)
					case <-ctx.Done():
						return
					}
				}
			}()

			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("Test timed out")
			}

			// Should have no errors
			if len(errors) != 0 {
				t.Errorf("Expected no errors, got %d: %v", len(errors), errors)
			}

			// Should have correct number of events
			if len(events) != len(tc.expected) {
				t.Fatalf("Expected %d events, got %d", len(tc.expected), len(events))
			}

			// Check each line
			for i, expected := range tc.expected {
				if events[i].Line != expected {
					t.Errorf("Event %d: expected line %q, got %q", i, expected, events[i].Line)
				}
			}
		})
	}
}

func TestStdinReader_ContextCancellation(t *testing.T) {
	// Create a pipe to simulate slow input
	pr, pw := io.Pipe()
	reader := NewStdinReaderFromReader(pr)

	ctx, cancel := context.WithCancel(context.Background())

	eventCh, errCh := reader.Start(ctx)

	// Start a goroutine to write data slowly
	go func() {
		defer pw.Close()
		pw.Write([]byte("line1\n"))
		time.Sleep(100 * time.Millisecond)
		pw.Write([]byte("line2\n"))
		time.Sleep(100 * time.Millisecond)
		// Cancel before writing more
		cancel()
		// Don't write more data after cancellation as it may still get processed
		// The test is about context cancellation, not about dropping buffered data
	}()

	var events []core.LogEvent
	var errors []error

	done := make(chan bool)
	go func() {
		defer close(done)
		for {
			select {
			case event, ok := <-eventCh:
				if !ok {
					eventCh = nil
					if errCh == nil {
						return
					}
					continue
				}
				events = append(events, event)
			case err, ok := <-errCh:
				if !ok {
					errCh = nil
					if eventCh == nil {
						return
					}
					continue
				}
				errors = append(errors, err)
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Test timed out")
	}

	// Should have received some events before cancellation
	if len(events) == 0 {
		t.Error("Expected at least one event before cancellation")
	}

	// Should have received at most 2 events (line1 and line2)
	if len(events) > 2 {
		t.Errorf("Expected at most 2 events, got %d", len(events))
	}
}

func TestStdinReader_SequenceNumbers(t *testing.T) {
	input := "line1\nline2\nline3\n"
	reader := NewStdinReaderFromReader(strings.NewReader(input))

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	eventCh, errCh := reader.Start(ctx)

	var events []core.LogEvent

	done := make(chan bool)
	go func() {
		defer close(done)
		for {
			select {
			case event, ok := <-eventCh:
				if !ok {
					return
				}
				events = append(events, event)
			case <-errCh:
				// Ignore errors for this test
			case <-ctx.Done():
				return
			}
		}
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Test timed out")
	}

	// Verify sequence numbers increment correctly
	for i, event := range events {
		expectedSeq := uint64(i + 1)
		if event.Seq != expectedSeq {
			t.Errorf("Event %d: expected seq %d, got %d", i, expectedSeq, event.Seq)
		}
	}
}
