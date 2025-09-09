package dockerx

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// RealClient implements Client using the actual Docker SDK
type RealClient struct {
	client *client.Client
}

// NewRealClient creates a new real Docker client
func NewRealClient() (*RealClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	// Test connection
	_, err = cli.Ping(context.Background())
	if err != nil {
		return nil, fmt.Errorf("docker daemon not reachable: %w", err)
	}

	return &RealClient{client: cli}, nil
}

// ListContainers returns all running containers
func (c *RealClient) ListContainers(ctx context.Context) ([]Container, error) {
	containers, err := c.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	result := make([]Container, len(containers))
	for i, ctr := range containers {
		name := ""
		if len(ctr.Names) > 0 {
			name = strings.TrimPrefix(ctr.Names[0], "/")
		}

		result[i] = Container{
			ID:    ctr.ID,
			Name:  name,
			State: ctr.State,
		}
	}

	return result, nil
}

// StreamLogs returns a log stream for the given container
func (c *RealClient) StreamLogs(ctx context.Context, id string, since string) (io.ReadCloser, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
	}

	if since != "" {
		options.Since = since
	}

	logs, err := c.client.ContainerLogs(ctx, id, options)
	if err != nil {
		return nil, fmt.Errorf("failed to get container logs: %w", err)
	}

	// Create a pipe to demultiplex stdout/stderr
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		defer logs.Close()

		// Use stdcopy to demultiplex the Docker log stream
		_, err := stdcopy.StdCopy(pw, pw, logs)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("log demux error: %w", err))
		}
	}()

	return pr, nil
}

// ContainerName returns the name of the container by ID
func (c *RealClient) ContainerName(ctx context.Context, id string) (string, error) {
	inspect, err := c.client.ContainerInspect(ctx, id)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	return strings.TrimPrefix(inspect.Name, "/"), nil
}

// Close closes the Docker client connection
func (c *RealClient) Close() error {
	return c.client.Close()
}
