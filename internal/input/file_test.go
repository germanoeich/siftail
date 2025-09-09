package input

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/germanoeich/siftail/internal/core"
)

// testHelper provides utilities for file tailer tests
type testHelper struct {
	t       *testing.T
	tempDir string
	file    *os.File
}

// newTestHelper creates a new test helper with a temporary directory and file
func newTestHelper(t *testing.T) *testHelper {
	tempDir, err := os.MkdirTemp("", "siftail_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	filePath := filepath.Join(tempDir, "test.log")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	return &testHelper{
		t:       t,
		tempDir: tempDir,
		file:    file,
	}
}

// cleanup removes the temporary directory and files
func (h *testHelper) cleanup() {
	if h.file != nil {
		h.file.Close()
	}
	os.RemoveAll(h.tempDir)
}

// writeLines writes lines to the test file
func (h *testHelper) writeLines(lines ...string) {
	for _, line := range lines {
		_, err := h.file.WriteString(line + "\n")
		if err != nil {
			h.t.Fatalf("Failed to write line: %v", err)
		}
	}
	h.file.Sync() // Ensure data is written to disk
}

// filePath returns the path to the test file
func (h *testHelper) filePath() string {
	return h.file.Name()
}

// collectEvents collects events from a channel with a timeout
func collectEvents(t *testing.T, eventCh <-chan core.LogEvent, count int, timeout time.Duration) []core.LogEvent {
	var events []core.LogEvent
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for len(events) < count {
		select {
		case event, ok := <-eventCh:
			if !ok {
				t.Fatalf("Event channel closed unexpectedly after %d events", len(events))
			}
			events = append(events, event)
		case <-timer.C:
			t.Fatalf("Timeout waiting for events. Got %d, expected %d", len(events), count)
		}
	}

	return events
}

// TestTailer_AppendsDetected tests that the tailer detects when new content is appended to a file
func TestTailer_AppendsDetected(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.cleanup()

	// Write initial content
	helper.writeLines("initial line 1", "initial line 2")

	// Create tailer starting from beginning
	tailer := NewFileReader(helper.filePath(), true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh, errCh := tailer.Start(ctx)

	// Collect initial events
	events := collectEvents(t, eventCh, 2, 2*time.Second)

	// Verify initial events
	if events[0].Line != "initial line 1" {
		t.Errorf("Expected 'initial line 1', got '%s'", events[0].Line)
	}
	if events[1].Line != "initial line 2" {
		t.Errorf("Expected 'initial line 2', got '%s'", events[1].Line)
	}

	// Verify source is correct
	for i, event := range events {
		if event.Source != core.SourceFile {
			t.Errorf("Event %d: Expected source %v, got %v", i, core.SourceFile, event.Source)
		}
	}

	// Append new content
	helper.writeLines("appended line 1", "appended line 2")

	// Collect appended events
	newEvents := collectEvents(t, eventCh, 2, 2*time.Second)

	// Verify appended events
	if newEvents[0].Line != "appended line 1" {
		t.Errorf("Expected 'appended line 1', got '%s'", newEvents[0].Line)
	}
	if newEvents[1].Line != "appended line 2" {
		t.Errorf("Expected 'appended line 2', got '%s'", newEvents[1].Line)
	}

	// Check for errors
	select {
	case err := <-errCh:
		t.Fatalf("Unexpected error: %v", err)
	default:
		// No error, which is expected
	}
}

// TestTailer_CopyTruncateRotation tests the copytruncate rotation strategy
func TestTailer_CopyTruncateRotation(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.cleanup()

	// Write initial content
	helper.writeLines("line 1", "line 2")

	// Create tailer starting from beginning
	tailer := NewFileReader(helper.filePath(), true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh, errCh := tailer.Start(ctx)

	// Collect initial events
	initialEvents := collectEvents(t, eventCh, 2, 2*time.Second)

	// Verify initial events
	if initialEvents[0].Line != "line 1" {
		t.Errorf("Expected 'line 1', got '%s'", initialEvents[0].Line)
	}
	if initialEvents[1].Line != "line 2" {
		t.Errorf("Expected 'line 2', got '%s'", initialEvents[1].Line)
	}

	// Simulate copytruncate rotation:
	// 1. Copy current content (this would normally be done by logrotate)
	// 2. Truncate the file
	helper.file.Close()

	// Truncate file (copytruncate behavior)
	err := os.Truncate(helper.filePath(), 0)
	if err != nil {
		t.Fatalf("Failed to truncate file: %v", err)
	}

	// Reopen file for writing new content
	helper.file, err = os.OpenFile(helper.filePath(), os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		t.Fatalf("Failed to reopen file: %v", err)
	}

	// Wait for the tailer to detect the truncation
	time.Sleep(500 * time.Millisecond)

	// Write new content to the truncated file
	helper.writeLines("new line 1", "new line 2")

	// Give extra time for fsnotify to detect the writes
	time.Sleep(200 * time.Millisecond)

	// Collect new events after rotation
	newEvents := collectEvents(t, eventCh, 2, 3*time.Second)

	// Verify new events
	if newEvents[0].Line != "new line 1" {
		t.Errorf("Expected 'new line 1', got '%s'", newEvents[0].Line)
	}
	if newEvents[1].Line != "new line 2" {
		t.Errorf("Expected 'new line 2', got '%s'", newEvents[1].Line)
	}

	// Check for errors
	select {
	case err := <-errCh:
		t.Fatalf("Unexpected error: %v", err)
	default:
		// No error, which is expected
	}
}

// TestTailer_RenameCreateRotation tests the rename+create rotation strategy
func TestTailer_RenameCreateRotation(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.cleanup()

	// Write initial content
	helper.writeLines("original line 1", "original line 2")

	// Create tailer starting from beginning
	tailer := NewFileReader(helper.filePath(), true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh, errCh := tailer.Start(ctx)

	// Collect initial events
	initialEvents := collectEvents(t, eventCh, 2, 2*time.Second)

	// Verify initial events
	if initialEvents[0].Line != "original line 1" {
		t.Errorf("Expected 'original line 1', got '%s'", initialEvents[0].Line)
	}
	if initialEvents[1].Line != "original line 2" {
		t.Errorf("Expected 'original line 2', got '%s'", initialEvents[1].Line)
	}

	// Simulate rename+create rotation:
	// 1. Rename current file to a backup name
	// 2. Create a new file with the original name
	originalPath := helper.filePath()
	backupPath := originalPath + ".bak"

	helper.file.Close()

	// Rename original file
	err := os.Rename(originalPath, backupPath)
	if err != nil {
		t.Fatalf("Failed to rename file: %v", err)
	}

	// Wait for the tailer to detect the rename
	time.Sleep(500 * time.Millisecond)

	// Create new file with original name
	helper.file, err = os.Create(originalPath)
	if err != nil {
		t.Fatalf("Failed to create new file: %v", err)
	}

	// Write new content to the new file
	helper.writeLines("rotated line 1", "rotated line 2")

	// Give extra time for fsnotify to detect the writes
	time.Sleep(200 * time.Millisecond)

	// Collect new events after rotation
	newEvents := collectEvents(t, eventCh, 2, 3*time.Second)

	// Verify new events
	if newEvents[0].Line != "rotated line 1" {
		t.Errorf("Expected 'rotated line 1', got '%s'", newEvents[0].Line)
	}
	if newEvents[1].Line != "rotated line 2" {
		t.Errorf("Expected 'rotated line 2', got '%s'", newEvents[1].Line)
	}

	// Check for errors
	select {
	case err := <-errCh:
		t.Fatalf("Unexpected error: %v", err)
	default:
		// No error, which is expected
	}

	// Cleanup backup file
	os.Remove(backupPath)
}

// TestTailer_FromEndBehavior tests that tailer starts from end when fromStart is false
func TestTailer_FromEndBehavior(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.cleanup()

	// Write initial content
	helper.writeLines("existing line 1", "existing line 2")

	// Create tailer starting from end (fromStart = false)
	tailer := NewFileReader(helper.filePath(), false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh, errCh := tailer.Start(ctx)

	// Wait a moment to ensure tailer is ready
	time.Sleep(100 * time.Millisecond)

	// Should not receive existing lines since we start from end
	select {
	case event := <-eventCh:
		t.Errorf("Unexpected event from existing content: %s", event.Line)
	case <-time.After(500 * time.Millisecond):
		// Expected - no events from existing content
	}

	// Append new content
	helper.writeLines("new line after start")

	// Should receive the new content
	events := collectEvents(t, eventCh, 1, 2*time.Second)

	if events[0].Line != "new line after start" {
		t.Errorf("Expected 'new line after start', got '%s'", events[0].Line)
	}

	// Check for errors
	select {
	case err := <-errCh:
		t.Fatalf("Unexpected error: %v", err)
	default:
		// No error, which is expected
	}
}

// TestTailer_ContextCancellation tests that tailer stops gracefully when context is cancelled
func TestTailer_ContextCancellation(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.cleanup()

	// Write initial content
	helper.writeLines("line 1")

	// Create tailer
	tailer := NewFileReader(helper.filePath(), true)
	ctx, cancel := context.WithCancel(context.Background())

	eventCh, errCh := tailer.Start(ctx)

	// Collect initial event
	events := collectEvents(t, eventCh, 1, 2*time.Second)
	if events[0].Line != "line 1" {
		t.Errorf("Expected 'line 1', got '%s'", events[0].Line)
	}

	// Cancel context
	cancel()

	// Channels should close
	select {
	case _, ok := <-eventCh:
		if ok {
			t.Error("Event channel should be closed after context cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Error("Event channel should close within reasonable time")
	}

	select {
	case _, ok := <-errCh:
		if ok {
			t.Error("Error channel should be closed after context cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Error("Error channel should close within reasonable time")
	}
}
