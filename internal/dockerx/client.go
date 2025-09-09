package dockerx

import (
	"context"
	"io"
)

// Client abstracts Docker SDK operations for testing
type Client interface {
	ListContainers(ctx context.Context) ([]Container, error)
	StreamLogs(ctx context.Context, id string, since string) (io.ReadCloser, error)
	ContainerName(ctx context.Context, id string) (string, error) // convenience
}

// Container represents a Docker container
type Container struct {
	ID    string
	Name  string // without leading '/'
	State string // running, etc
}
