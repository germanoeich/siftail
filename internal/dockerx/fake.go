package dockerx

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// FakeClient implements Client for testing
type FakeClient struct {
	containers []Container
	logStreams map[string][]string // containerID -> log lines
	errors     map[string]error    // method -> error to return
}

// NewFakeClient creates a new fake Docker client for testing
func NewFakeClient() *FakeClient {
	return &FakeClient{
		containers: []Container{},
		logStreams: make(map[string][]string),
		errors:     make(map[string]error),
	}
}

// AddContainer adds a container to the fake client
func (f *FakeClient) AddContainer(id, name, state string) {
	f.containers = append(f.containers, Container{
		ID:    id,
		Name:  name,
		State: state,
	})
}

// AddLogLines adds log lines for a container
func (f *FakeClient) AddLogLines(containerID string, lines []string) {
	f.logStreams[containerID] = append(f.logStreams[containerID], lines...)
}

// SetError sets an error to return for a specific method
func (f *FakeClient) SetError(method string, err error) {
	f.errors[method] = err
}

// ListContainers returns the fake containers
func (f *FakeClient) ListContainers(ctx context.Context) ([]Container, error) {
	if err, exists := f.errors["ListContainers"]; exists {
		return nil, err
	}

	result := make([]Container, len(f.containers))
	copy(result, f.containers)
	return result, nil
}

// StreamLogs returns a fake log stream
func (f *FakeClient) StreamLogs(ctx context.Context, id string, since string) (io.ReadCloser, error) {
	if err, exists := f.errors["StreamLogs"]; exists {
		return nil, err
	}

	lines, exists := f.logStreams[id]
	if !exists {
		return nil, fmt.Errorf("container not found: %s", id)
	}

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		for _, line := range lines {
			select {
			case <-ctx.Done():
				return
			default:
				// Add timestamp prefix to simulate Docker log format
				timestamp := time.Now().UTC().Format(time.RFC3339Nano)
				formatted := fmt.Sprintf("%s %s\n", timestamp, line)
				if _, err := pw.Write([]byte(formatted)); err != nil {
					return
				}

				// Small delay to simulate streaming
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	return pr, nil
}

// ContainerName returns the container name by ID
func (f *FakeClient) ContainerName(ctx context.Context, id string) (string, error) {
	if err, exists := f.errors["ContainerName"]; exists {
		return "", err
	}

	for _, container := range f.containers {
		if strings.HasPrefix(container.ID, id) || container.ID == id {
			return container.Name, nil
		}
	}

	return "", fmt.Errorf("container not found: %s", id)
}
