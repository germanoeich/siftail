package dockerx

import (
	"bufio"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestDockerClient_Fake_ListAndLogs(t *testing.T) {
	ctx := context.Background()
	fake := NewFakeClient()

	// Add test containers
	fake.AddContainer("container1", "web-server", "running")
	fake.AddContainer("container2", "database", "running")
	fake.AddContainer("container3", "worker", "exited")

	// Add log lines for containers
	fake.AddLogLines("container1", []string{
		"INFO Starting web server on port 8080",
		"DEBUG Request received: GET /health",
		"WARN High memory usage detected",
	})
	fake.AddLogLines("container2", []string{
		"INFO Database initialized",
		"ERROR Connection timeout",
	})

	// Test ListContainers
	containers, err := fake.ListContainers(ctx)
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}

	if len(containers) != 3 {
		t.Fatalf("Expected 3 containers, got %d", len(containers))
	}

	// Verify container details
	expectedContainers := map[string]Container{
		"container1": {ID: "container1", Name: "web-server", State: "running"},
		"container2": {ID: "container2", Name: "database", State: "running"},
		"container3": {ID: "container3", Name: "worker", State: "exited"},
	}

	for _, container := range containers {
		expected, exists := expectedContainers[container.ID]
		if !exists {
			t.Errorf("Unexpected container: %s", container.ID)
			continue
		}

		if container.Name != expected.Name {
			t.Errorf("Container %s: expected name %s, got %s", container.ID, expected.Name, container.Name)
		}
		if container.State != expected.State {
			t.Errorf("Container %s: expected state %s, got %s", container.ID, expected.State, container.State)
		}
	}

	// Test StreamLogs for container1
	logStream, err := fake.StreamLogs(ctx, "container1", "")
	if err != nil {
		t.Fatalf("StreamLogs failed: %v", err)
	}
	defer logStream.Close()

	// Read log lines
	scanner := bufio.NewScanner(logStream)
	var logLines []string

	// Use a timeout to avoid hanging
	done := make(chan bool)
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			logLines = append(logLines, line)
			if len(logLines) >= 3 { // We expect 3 log lines
				break
			}
		}
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for log lines")
	}

	if len(logLines) != 3 {
		t.Fatalf("Expected 3 log lines, got %d", len(logLines))
	}

	// Verify log content (checking that our test messages are in the logs)
	expectedMessages := []string{
		"INFO Starting web server on port 8080",
		"DEBUG Request received: GET /health",
		"WARN High memory usage detected",
	}

	for i, expected := range expectedMessages {
		if !strings.Contains(logLines[i], expected) {
			t.Errorf("Log line %d: expected to contain %q, got %q", i, expected, logLines[i])
		}
	}

	// Test ContainerName
	name, err := fake.ContainerName(ctx, "container1")
	if err != nil {
		t.Fatalf("ContainerName failed: %v", err)
	}
	if name != "web-server" {
		t.Errorf("Expected name 'web-server', got %q", name)
	}

	// Test error case for non-existent container
	_, err = fake.StreamLogs(ctx, "nonexistent", "")
	if err == nil {
		t.Error("Expected error for non-existent container, got nil")
	}
}

func TestDockerLogReader_DemuxStdoutStderr(t *testing.T) {
	fake := NewFakeClient()
	fake.AddContainer("test-container", "test", "running")

	// Add mixed stdout/stderr logs (in real Docker, these would be demuxed)
	fake.AddLogLines("test-container", []string{
		"[stdout] Application started successfully",
		"[stderr] Warning: deprecated API used",
		"[stdout] Processing request #1",
		"[stderr] Error: failed to connect to database",
		"[stdout] Request completed",
	})

	ctx := context.Background()
	logStream, err := fake.StreamLogs(ctx, "test-container", "")
	if err != nil {
		t.Fatalf("StreamLogs failed: %v", err)
	}
	defer logStream.Close()

	// Read all log lines
	content, err := io.ReadAll(logStream)
	if err != nil {
		t.Fatalf("Failed to read log stream: %v", err)
	}

	logContent := string(content)
	lines := strings.Split(strings.TrimSpace(logContent), "\n")

	if len(lines) != 5 {
		t.Fatalf("Expected 5 log lines, got %d", len(lines))
	}

	// Verify that all expected messages are present
	expectedMessages := []string{
		"Application started successfully",
		"Warning: deprecated API used",
		"Processing request #1",
		"Error: failed to connect to database",
		"Request completed",
	}

	for i, expected := range expectedMessages {
		if !strings.Contains(lines[i], expected) {
			t.Errorf("Line %d: expected to contain %q, got %q", i, expected, lines[i])
		}
	}

	// Verify timestamp format (should be RFC3339Nano)
	for i, line := range lines {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			t.Errorf("Line %d: expected timestamp + message format, got %q", i, line)
			continue
		}

		timestamp := parts[0]
		_, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			t.Errorf("Line %d: invalid timestamp format %q: %v", i, timestamp, err)
		}
	}
}

func TestFakeClient_ErrorHandling(t *testing.T) {
	fake := NewFakeClient()
	ctx := context.Background()

	// Test ListContainers error
	fake.SetError("ListContainers", io.ErrUnexpectedEOF)
	_, err := fake.ListContainers(ctx)
	if err != io.ErrUnexpectedEOF {
		t.Errorf("Expected UnexpectedEOF error, got %v", err)
	}

	// Reset error and test StreamLogs error
	fake.SetError("ListContainers", nil)
	fake.SetError("StreamLogs", io.ErrClosedPipe)
	_, err = fake.StreamLogs(ctx, "test", "")
	if err != io.ErrClosedPipe {
		t.Errorf("Expected ClosedPipe error, got %v", err)
	}

	// Test ContainerName error
	fake.SetError("StreamLogs", nil)
	fake.SetError("ContainerName", io.ErrShortBuffer)
	_, err = fake.ContainerName(ctx, "test")
	if err != io.ErrShortBuffer {
		t.Errorf("Expected ShortBuffer error, got %v", err)
	}
}
