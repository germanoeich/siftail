package input

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/germanoeich/siftail/internal/core"
	"github.com/germanoeich/siftail/internal/dockerx"
)

func TestVisibleSet_ToggleAndAll(t *testing.T) {
	vs := NewVisibleSet()

	// Test initial state - new containers default to visible
	if !vs.IsVisible("container1") {
		t.Error("New containers should default to visible")
	}

	// Test single toggle
	vs.Toggle("container1")
	if vs.IsVisible("container1") {
		t.Error("Toggle should have set container1 to invisible")
	}

	vs.Toggle("container1")
	if !vs.IsVisible("container1") {
		t.Error("Second toggle should have set container1 back to visible")
	}

	// Test SetVisible
	vs.SetVisible("container2", false)
	if vs.IsVisible("container2") {
		t.Error("SetVisible(false) should make container2 invisible")
	}

	vs.SetVisible("container2", true)
	if !vs.IsVisible("container2") {
		t.Error("SetVisible(true) should make container2 visible")
	}

	// Test ToggleAll behavior
	containerIDs := []string{"container1", "container2", "container3"}

	// Initially all are visible (or default visible for container3)
	vs.SetVisible("container1", true)
	vs.SetVisible("container2", true)
	// container3 will default to visible

	// Since all are visible, ToggleAll should turn them all off
	vs.ToggleAll(containerIDs)
	for _, id := range containerIDs {
		if vs.IsVisible(id) {
			t.Errorf("After ToggleAll, container %s should be invisible", id)
		}
	}

	// Now none are visible, so ToggleAll should turn them all on
	vs.ToggleAll(containerIDs)
	for _, id := range containerIDs {
		if !vs.IsVisible(id) {
			t.Errorf("After second ToggleAll, container %s should be visible", id)
		}
	}

	// Test mixed state - if some are off, ToggleAll should turn all on
	vs.SetVisible("container1", false)
	vs.ToggleAll(containerIDs)
	for _, id := range containerIDs {
		if !vs.IsVisible(id) {
			t.Errorf("After ToggleAll with mixed state, container %s should be visible", id)
		}
	}

	// Test GetSnapshot
	vs.SetVisible("container1", true)
	vs.SetVisible("container2", false)
	snapshot := vs.GetSnapshot()

	if len(snapshot) < 2 {
		t.Error("Snapshot should contain at least 2 entries")
	}

	if snapshot["container1"] != true {
		t.Error("Snapshot should show container1 as visible")
	}

	if snapshot["container2"] != false {
		t.Error("Snapshot should show container2 as invisible")
	}

	// Modifying snapshot shouldn't affect original
	snapshot["container1"] = false
	if !vs.IsVisible("container1") {
		t.Error("Modifying snapshot should not affect original VisibleSet")
	}
}

func TestDockerReader_StreamsAllContainers_Fake(t *testing.T) {
	// Create fake client with some containers and log data
	fakeClient := dockerx.NewFakeClient()
	
	// Add running containers
	fakeClient.AddContainer("container1", "app1", "running")
	fakeClient.AddContainer("container2", "app2", "running")
	fakeClient.AddContainer("container3", "app3", "exited") // This should be ignored

	// Add log lines for running containers
	fakeClient.AddLogLines("container1", []string{
		"2023-01-01T12:00:00.000000000Z [INFO] App1 starting up",
		"2023-01-01T12:00:01.000000000Z [ERROR] App1 error occurred",
	})
	fakeClient.AddLogLines("container2", []string{
		"2023-01-01T12:00:00.500000000Z [WARN] App2 warning message",
		"2023-01-01T12:00:01.500000000Z [DEBUG] App2 debug info",
	})

	// Create severity detector
	levelMap := core.NewLevelMap()
	detector := core.NewDefaultSeverityDetector(levelMap)

	// Create docker reader
	reader := NewDockerReader(fakeClient, detector)

	// Start the reader
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	eventCh, errCh := reader.Start(ctx)

	// Collect events and errors
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

	// Wait for events to be processed
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Test timed out waiting for events")
	}

	// Verify we got events from both running containers
	if len(events) < 4 {
		t.Errorf("Expected at least 4 events, got %d", len(events))
	}

	// Verify no errors occurred
	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %d: %v", len(errors), errors)
	}

	// Verify events have correct source
	for _, event := range events {
		if event.Source != core.SourceDocker {
			t.Errorf("Expected SourceDocker, got %v", event.Source)
		}
		if event.Container == "" {
			t.Error("Expected container name to be set")
		}
		if event.Container != "app1" && event.Container != "app2" {
			t.Errorf("Unexpected container name: %s", event.Container)
		}
	}

	// Verify severity detection worked
	foundSeverities := make(map[core.Severity]bool)
	for _, event := range events {
		foundSeverities[event.Level] = true
	}

	expectedSeverities := []core.Severity{core.SevInfo, core.SevError, core.SevWarn, core.SevDebug}
	for _, expected := range expectedSeverities {
		if !foundSeverities[expected] {
			t.Errorf("Expected to find severity %v in events", expected)
		}
	}

	// Test container listing
	containers := reader.GetContainers()
	if len(containers) != 2 {
		t.Errorf("Expected 2 running containers, got %d", len(containers))
	}

	runningContainerNames := make(map[string]bool)
	for _, container := range containers {
		if container.State != "running" {
			t.Errorf("Expected only running containers, got %s with state %s", container.Name, container.State)
		}
		runningContainerNames[container.Name] = true
	}

	if !runningContainerNames["app1"] || !runningContainerNames["app2"] {
		t.Error("Expected to find app1 and app2 in running containers")
	}

	// Test visibility controls
	visibleSet := reader.GetVisibleSet()
	if visibleSet == nil {
		t.Fatal("Expected non-nil VisibleSet")
	}

	// Test that all containers are visible by default
	if !visibleSet.IsVisible("container1") || !visibleSet.IsVisible("container2") {
		t.Error("Expected all containers to be visible by default")
	}
}

func TestDockerReader_VisibilityToggling(t *testing.T) {
	// Create fake client with one container
	fakeClient := dockerx.NewFakeClient()
	fakeClient.AddContainer("container1", "app1", "running")
	fakeClient.AddLogLines("container1", []string{
		"2023-01-01T12:00:00.000000000Z [INFO] Test message",
	})

	// Create docker reader
	levelMap := core.NewLevelMap()
	detector := core.NewDefaultSeverityDetector(levelMap)
	reader := NewDockerReader(fakeClient, detector)

	// Test visibility controls
	visibleSet := reader.GetVisibleSet()

	// Test initial state
	if !visibleSet.IsVisible("container1") {
		t.Error("Container should be visible by default")
	}

	// Test toggle
	visibleSet.Toggle("container1")
	if visibleSet.IsVisible("container1") {
		t.Error("Container should be hidden after toggle")
	}

	// Test set visible
	visibleSet.SetVisible("container1", true)
	if !visibleSet.IsVisible("container1") {
		t.Error("Container should be visible after SetVisible(true)")
	}
}

func TestDockerReader_ErrorHandling(t *testing.T) {
	// Create fake client that returns errors
	fakeClient := dockerx.NewFakeClient()
	fakeClient.SetError("ListContainers", fmt.Errorf("mock list containers error"))

	// Create docker reader
	levelMap := core.NewLevelMap()
	detector := core.NewDefaultSeverityDetector(levelMap)
	reader := NewDockerReader(fakeClient, detector)

	// Start the reader
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	eventCh, errCh := reader.Start(ctx)

	// Should get an error
	select {
	case err := <-errCh:
		if err == nil {
			t.Error("Expected an error when ListContainers fails")
		}
	case <-eventCh:
		t.Error("Should not receive events when ListContainers fails")
	case <-time.After(2 * time.Second):
		t.Error("Test timed out waiting for error")
	}
}

func TestDockerReader_ContextCancellation(t *testing.T) {
	// Create fake client with containers
	fakeClient := dockerx.NewFakeClient()
	fakeClient.AddContainer("container1", "app1", "running")
	fakeClient.AddLogLines("container1", []string{
		"2023-01-01T12:00:00.000000000Z [INFO] Test message",
	})

	// Create docker reader
	levelMap := core.NewLevelMap()
	detector := core.NewDefaultSeverityDetector(levelMap)
	reader := NewDockerReader(fakeClient, detector)

	// Start the reader with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	eventCh, errCh := reader.Start(ctx)

	// Cancel the context immediately
	cancel()

	// Channels should close relatively quickly
	select {
	case <-eventCh:
		// Events channel should close
	case <-time.After(1 * time.Second):
		t.Error("Event channel should have closed after context cancellation")
	}

	select {
	case <-errCh:
		// Error channel should close
	case <-time.After(1 * time.Second):
		t.Error("Error channel should have closed after context cancellation")
	}
}