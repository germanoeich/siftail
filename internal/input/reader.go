package input

import (
	"context"
	"sync"

	"github.com/germanoeich/siftail/internal/core"
)

// Reader defines the interface for log input sources (stdin, file, docker)
type Reader interface {
	// Start returns immediately; goroutine pumps events until ctx done.
	Start(ctx context.Context) (<-chan core.LogEvent, <-chan error)
}

// FanIn multiplexes multiple readers into a single stream
type FanIn struct {
	readers []Reader
}

// NewFanIn creates a new FanIn multiplexer
func NewFanIn(readers ...Reader) *FanIn {
	return &FanIn{readers: readers}
}

// Start starts all readers and multiplexes their output into single channels
// Cancelling ctx cleanly stops all readers
func (f *FanIn) Start(ctx context.Context) (<-chan core.LogEvent, <-chan error) {
	eventCh := make(chan core.LogEvent, 100) // buffered to prevent blocking
	errCh := make(chan error, 10)            // buffered for error reporting

	var wg sync.WaitGroup

	// Start all readers
	for _, reader := range f.readers {
		wg.Add(1)
		go func(r Reader) {
			defer wg.Done()

			readerEvents, readerErrs := r.Start(ctx)

			// Forward events and errors until context is cancelled or channels close
			for {
				select {
				case <-ctx.Done():
					return
				case event, ok := <-readerEvents:
					if !ok {
						return // reader events channel closed
					}
					select {
					case eventCh <- event:
					case <-ctx.Done():
						return
					}
				case err, ok := <-readerErrs:
					if !ok {
						return // reader errors channel closed
					}
					select {
					case errCh <- err:
					case <-ctx.Done():
						return
					}
				}
			}
		}(reader)
	}

	// Close output channels when all readers are done
	go func() {
		wg.Wait()
		close(eventCh)
		close(errCh)
	}()

	return eventCh, errCh
}
