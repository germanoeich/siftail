package input

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/germanoeich/siftail/internal/core"
	"github.com/germanoeich/siftail/internal/dockerx"
)

// VisibleSet manages which containers are visible/hidden with thread safety
type VisibleSet struct {
	mu sync.RWMutex
	On map[string]bool // containerID -> visible?
}

// NewVisibleSet creates a new VisibleSet
func NewVisibleSet() *VisibleSet {
	return &VisibleSet{
		On: make(map[string]bool),
	}
}

// IsVisible returns whether a container is visible
func (vs *VisibleSet) IsVisible(containerID string) bool {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	visible, exists := vs.On[containerID]
	if !exists {
		return true // default to visible for new containers
	}
	return visible
}

// Toggle flips the visibility state of a container
func (vs *VisibleSet) Toggle(containerID string) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	current, exists := vs.On[containerID]
	if !exists {
		current = true // default is visible
	}
	vs.On[containerID] = !current
}

// SetVisible sets the visibility state of a container
func (vs *VisibleSet) SetVisible(containerID string, visible bool) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.On[containerID] = visible
}

// ToggleAll sets all containers to the same visibility state
// If all are currently on, turn them all off, otherwise turn them all on
func (vs *VisibleSet) ToggleAll(containerIDs []string) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// Check if all are currently visible (default to visible if not set)
	allVisible := true
	for _, id := range containerIDs {
		visible, exists := vs.On[id]
		if !exists {
			visible = true // default is visible
		}
		if !visible {
			allVisible = false
			break
		}
	}

	// Set all to the opposite state
	newState := !allVisible
	for _, id := range containerIDs {
		vs.On[id] = newState
	}
}

// GetSnapshot returns a copy of the current visibility state
func (vs *VisibleSet) GetSnapshot() map[string]bool {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	
	snapshot := make(map[string]bool)
	for k, v := range vs.On {
		snapshot[k] = v
	}
	return snapshot
}

// DockerReader reads logs from all running Docker containers
type DockerReader struct {
	client      dockerx.Client
	levelDetect core.SeverityDetector
	visible     *VisibleSet
	
	// Internal state
	mu              sync.RWMutex
	containers      []dockerx.Container
	activeStreams   map[string]context.CancelFunc // containerID -> cancel func
	streamWG        sync.WaitGroup                // tracks active processStream goroutines
}

// NewDockerReader creates a new Docker log reader
func NewDockerReader(client dockerx.Client, levelDetect core.SeverityDetector) *DockerReader {
	return &DockerReader{
		client:        client,
		levelDetect:   levelDetect,
		visible:       NewVisibleSet(),
		activeStreams: make(map[string]context.CancelFunc),
	}
}

// GetVisibleSet returns the visibility control for container toggles
func (dr *DockerReader) GetVisibleSet() *VisibleSet {
	return dr.visible
}

// GetContainers returns a snapshot of currently tracked containers
func (dr *DockerReader) GetContainers() []dockerx.Container {
	dr.mu.RLock()
	defer dr.mu.RUnlock()
	
	result := make([]dockerx.Container, len(dr.containers))
	copy(result, dr.containers)
	return result
}

// Start implements the Reader interface
// Enumerates running containers and starts streaming logs from each
func (dr *DockerReader) Start(ctx context.Context) (<-chan core.LogEvent, <-chan error) {
	eventCh := make(chan core.LogEvent, 100)
	errCh := make(chan error, 10)

	go dr.run(ctx, eventCh, errCh)

	return eventCh, errCh
}

// run is the main goroutine that manages container discovery and log streaming
func (dr *DockerReader) run(ctx context.Context, eventCh chan<- core.LogEvent, errCh chan<- error) {
	defer func() {
		// Stop all streams and wait for them to exit before closing channels
		dr.stopAllStreams()
		dr.streamWG.Wait()
		close(eventCh)
		close(errCh)
	}()

	// Initial container discovery
	if err := dr.refreshContainers(ctx); err != nil {
		select {
		case errCh <- fmt.Errorf("failed to list containers: %w", err):
		case <-ctx.Done():
			return
		}
		return
	}

	// Start streaming from all running containers
	dr.startAllStreams(ctx, eventCh, errCh)

	// Set up periodic container refresh (every 30 seconds)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Refresh container list and start new streams as needed
			if err := dr.refreshContainers(ctx); err != nil {
				select {
				case errCh <- fmt.Errorf("failed to refresh containers: %w", err):
				case <-ctx.Done():
					return
				}
			}
			dr.startAllStreams(ctx, eventCh, errCh)
		}
	}
}

// refreshContainers updates the list of running containers
func (dr *DockerReader) refreshContainers(ctx context.Context) error {
	containers, err := dr.client.ListContainers(ctx)
	if err != nil {
		return err
	}

	// Filter to only running containers
	var running []dockerx.Container
	for _, container := range containers {
		if container.State == "running" {
			running = append(running, container)
		}
	}

	dr.mu.Lock()
	dr.containers = running
	dr.mu.Unlock()

	return nil
}

// startAllStreams starts log streams for any containers that don't have active streams
func (dr *DockerReader) startAllStreams(ctx context.Context, eventCh chan<- core.LogEvent, errCh chan<- error) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	for _, container := range dr.containers {
		// Check if we already have a stream for this container
		if _, exists := dr.activeStreams[container.ID]; exists {
			continue
		}

		// Start a new stream
		streamCtx, cancel := context.WithCancel(ctx)
		dr.activeStreams[container.ID] = cancel

		go dr.streamContainer(streamCtx, container, eventCh, errCh)
	}
}

// stopAllStreams cancels all active container log streams
func (dr *DockerReader) stopAllStreams() {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	for _, cancel := range dr.activeStreams {
		cancel()
	}
	dr.activeStreams = make(map[string]context.CancelFunc)
}

// streamContainer streams logs from a single container
func (dr *DockerReader) streamContainer(ctx context.Context, container dockerx.Container, eventCh chan<- core.LogEvent, errCh chan<- error) {
	defer func() {
		// Clean up this stream from activeStreams when it exits
		dr.mu.Lock()
		delete(dr.activeStreams, container.ID)
		dr.mu.Unlock()
	}()

	// Start streaming logs from "now" to avoid old logs
	stream, err := dr.client.StreamLogs(ctx, container.ID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		select {
		case errCh <- fmt.Errorf("failed to stream logs for container %s (%s): %w", container.Name, container.ID, err):
		case <-ctx.Done():
		}
		return
	}
	defer stream.Close()

	// For simplicity, treat the stream as a single output (stdout)
	// TODO: In a real implementation with real Docker daemon, use stdcopy.StdCopy to demultiplex
	// Docker's multiplexed stream format. The fake client outputs plain text for testing.
	// Real implementation would look like:
	//   stdoutReader, stdoutWriter := io.Pipe()
	//   stderrReader, stderrWriter := io.Pipe()
	//   go func() {
	//     defer stdoutWriter.Close()
	//     defer stderrWriter.Close()
	//     stdcopy.StdCopy(stdoutWriter, stderrWriter, stream)
	//   }()
	//   go dr.processStream(ctx, stdoutReader, container, "stdout", eventCh, errCh)
	//   go dr.processStream(ctx, stderrReader, container, "stderr", eventCh, errCh)
	dr.streamWG.Add(1)
	go dr.processStream(ctx, stream, container, "stdout", eventCh, errCh)

	// Wait for context cancellation
	<-ctx.Done()
}

// processStream processes a single stream (stdout or stderr) from a container
func (dr *DockerReader) processStream(ctx context.Context, reader io.ReadCloser, container dockerx.Container, streamType string, eventCh chan<- core.LogEvent, errCh chan<- error) {
	defer dr.streamWG.Done()
	defer reader.Close()
	
	scanner := bufio.NewScanner(reader)
	var seq uint64

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		
		// Parse timestamp from Docker log format if present
		// Docker logs format: "2023-01-01T12:00:00.000000000Z message"
		timestamp := time.Now()
		message := line
		
		// Try to extract timestamp from Docker format
		if len(line) > 30 && line[29] == 'Z' && line[30] == ' ' {
			if t, err := time.Parse(time.RFC3339Nano, line[:30]); err == nil {
				timestamp = t
				message = line[31:]
			}
		}

		// Detect severity level
		levelStr, level, _ := dr.levelDetect.Detect(message)

		// Create log event
		event := core.LogEvent{
			Seq:       seq,
			Time:      timestamp,
			Source:    core.SourceDocker,
			Container: container.Name,
			Line:      message,
			LevelStr:  levelStr,
			Level:     level,
		}
		seq++

		// Always send the event - visibility filtering is applied at the view layer
		// This prevents backpressure when containers are toggled off
		select {
		case eventCh <- event:
		case <-ctx.Done():
			return
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		select {
		case errCh <- fmt.Errorf("error reading %s from container %s: %w", streamType, container.Name, err):
		case <-ctx.Done():
		}
	}
}