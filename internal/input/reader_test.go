package input

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/germanoeich/siftail/internal/core"
)

// mockReader is a test implementation of Reader
type mockReader struct {
	events []core.LogEvent
	errors []error
	delay  time.Duration
}

func (m *mockReader) Start(ctx context.Context) (<-chan core.LogEvent, <-chan error) {
	eventCh := make(chan core.LogEvent, len(m.events)+1) // extra buffer
	errCh := make(chan error, len(m.errors)+1)           // extra buffer

	go func() {
		defer close(eventCh)
		defer close(errCh)

		// Send events with optional delay
		for _, event := range m.events {
			if m.delay > 0 {
				time.Sleep(m.delay)
			}
			select {
			case eventCh <- event:
			case <-ctx.Done():
				return
			}
		}

		// Send errors
		for _, err := range m.errors {
			select {
			case errCh <- err:
			case <-ctx.Done():
				return
			}
		}

		// Add a small delay to ensure events are forwarded before closing
		time.Sleep(10 * time.Millisecond)
	}()

	return eventCh, errCh
}

func TestFanIn_Multiplexes(t *testing.T) {
	// Create mock readers with different events
	reader1 := &mockReader{
		events: []core.LogEvent{
			{Seq: 1, Line: "event1-1", Source: core.SourceStdin},
			{Seq: 2, Line: "event1-2", Source: core.SourceStdin},
		},
	}

	reader2 := &mockReader{
		events: []core.LogEvent{
			{Seq: 1, Line: "event2-1", Source: core.SourceFile},
			{Seq: 2, Line: "event2-2", Source: core.SourceFile},
		},
	}

	reader3 := &mockReader{
		events: []core.LogEvent{
			{Seq: 1, Line: "event3-1", Source: core.SourceDocker},
		},
	}

	fanIn := NewFanIn(reader1, reader2, reader3)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventCh, errCh := fanIn.Start(ctx)

	// Collect all events
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

	// Wait for completion or timeout
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for events")
	}

	// Verify we got all expected events
	if len(events) != 5 {
		t.Errorf("Expected 5 events, got %d", len(events))
		for i, event := range events {
			t.Logf("Event %d: %+v", i, event)
		}
	}

	// Verify we got events from all sources
	sourceCounts := make(map[core.SourceKind]int)
	for _, event := range events {
		sourceCounts[event.Source]++
	}

	if sourceCounts[core.SourceStdin] != 2 {
		t.Errorf("Expected 2 stdin events, got %d", sourceCounts[core.SourceStdin])
	}
	if sourceCounts[core.SourceFile] != 2 {
		t.Errorf("Expected 2 file events, got %d", sourceCounts[core.SourceFile])
	}
	if sourceCounts[core.SourceDocker] != 1 {
		t.Errorf("Expected 1 docker event, got %d", sourceCounts[core.SourceDocker])
	}

	// Should not have any errors
	if len(errors) != 0 {
		t.Errorf("Expected no errors, got %d: %v", len(errors), errors)
	}
}

func TestFanIn_CancelStopsAll(t *testing.T) {
	// Track goroutines before test
	initialGoroutines := runtime.NumGoroutine()

	// Create readers that would run indefinitely without cancellation
	reader1 := &mockReader{
		events: []core.LogEvent{
			{Seq: 1, Line: "event1", Source: core.SourceStdin},
		},
		delay: 100 * time.Millisecond, // slow down to ensure cancellation works
	}

	reader2 := &mockReader{
		events: []core.LogEvent{
			{Seq: 1, Line: "event2", Source: core.SourceFile},
		},
		delay: 100 * time.Millisecond,
	}

	fanIn := NewFanIn(reader1, reader2)

	ctx, cancel := context.WithCancel(context.Background())

	eventCh, errCh := fanIn.Start(ctx)

	// Read a few events to ensure readers started
	var eventCount int
	timeout := time.After(500 * time.Millisecond)

readLoop:
	for {
		select {
		case _, ok := <-eventCh:
			if !ok {
				break readLoop
			}
			eventCount++
			if eventCount >= 2 {
				// Cancel after getting some events
				cancel()
			}
		case <-errCh:
			// Ignore errors for this test
		case <-timeout:
			if eventCount == 0 {
				t.Fatal("No events received before timeout")
			}
			cancel()
			break readLoop
		}
	}

	// Wait for channels to close after cancellation
	channelsClosed := make(chan bool)
	go func() {
		// Drain remaining events/errors until channels close
		for eventCh != nil || errCh != nil {
			select {
			case _, ok := <-eventCh:
				if !ok {
					eventCh = nil
				}
			case _, ok := <-errCh:
				if !ok {
					errCh = nil
				}
			}
		}
		close(channelsClosed)
	}()

	// Wait for channels to close or timeout
	select {
	case <-channelsClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("Channels did not close after cancellation")
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

func TestFanIn_EmptyReaders(t *testing.T) {
	fanIn := NewFanIn() // no readers

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	eventCh, errCh := fanIn.Start(ctx)

	// Should close immediately
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
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Test timed out - channels should have closed immediately")
	}

	if len(events) != 0 {
		t.Errorf("Expected no events, got %d", len(events))
	}
	if len(errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(errors))
	}
}

func TestFanIn_ErrorPropagation(t *testing.T) {
	// Create a reader that sends errors
	testErr := &testError{"test error"}
	readerWithError := &mockReader{
		errors: []error{testErr},
	}

	normalReader := &mockReader{
		events: []core.LogEvent{
			{Seq: 1, Line: "normal event", Source: core.SourceStdin},
		},
	}

	fanIn := NewFanIn(readerWithError, normalReader)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eventCh, errCh := fanIn.Start(ctx)

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

	// Should have received the error
	if len(errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errors))
	} else if errors[0] != testErr {
		t.Errorf("Expected test error, got %v", errors[0])
	}

	// Should still have received the normal event
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	} else if events[0].Line != "normal event" {
		t.Errorf("Expected 'normal event', got %q", events[0].Line)
	}
}

// testError is a custom error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
