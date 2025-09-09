package input

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/germanoeich/siftail/internal/core"
)

// FileReader tails a file and handles rotation scenarios
type FileReader struct {
	path      string
	fromStart bool
	seq       uint64
	file      *os.File
	watcher   *fsnotify.Watcher
	lastStat  os.FileInfo
}

// NewFileReader creates a new file tailer
func NewFileReader(path string, fromStart bool) *FileReader {
	return &FileReader{
		path:      path,
		fromStart: fromStart,
	}
}

// Start implements the Reader interface
func (f *FileReader) Start(ctx context.Context) (<-chan core.LogEvent, <-chan error) {
	eventCh := make(chan core.LogEvent, 50)
	errCh := make(chan error, 5)

	go func() {
		defer close(eventCh)
		defer close(errCh)
		defer f.cleanup()

		if err := f.initialize(); err != nil {
			select {
			case errCh <- fmt.Errorf("failed to initialize file reader: %w", err):
			case <-ctx.Done():
			}
			return
		}

		f.run(ctx, eventCh, errCh)
	}()

	return eventCh, errCh
}

// initialize sets up the file handle and watcher
func (f *FileReader) initialize() error {
	var err error

	// Open the file
	f.file, err = os.Open(f.path)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", f.path, err)
	}

	// Get initial file stats
	f.lastStat, err = f.file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", f.path, err)
	}

	// Position cursor
	if !f.fromStart {
		// Seek to end unless fromStart is requested
		if _, err := f.file.Seek(0, io.SeekEnd); err != nil {
			return fmt.Errorf("failed to seek to end of file: %w", err)
		}
	}

	// Set up fsnotify watcher
	f.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	// Watch the file
	if err := f.watcher.Add(f.path); err != nil {
		return fmt.Errorf("failed to watch file %s: %w", f.path, err)
	}

	return nil
}

// run is the main event loop
func (f *FileReader) run(ctx context.Context, eventCh chan<- core.LogEvent, errCh chan<- error) {
	reader := bufio.NewReader(f.file)
	backoffTimer := time.NewTimer(0)
	backoffTimer.Stop()

	// If starting from beginning, read existing content first
	if f.fromStart {
		f.readAvailableLines(reader, eventCh, errCh, ctx)
	}

	for {
		select {
		case <-ctx.Done():
			return

		case <-backoffTimer.C:
			// Retry after backoff
			f.readAvailableLines(reader, eventCh, errCh, ctx)

		case event, ok := <-f.watcher.Events:
			if !ok {
				return // watcher closed
			}

			switch {
			case event.Has(fsnotify.Write):
				// File was written to - check if it was truncated first
				if f.checkForTruncation() {
					// File was truncated - handle as rotation
					if err := f.handleRotation(reader, eventCh, errCh); err != nil {
						select {
						case errCh <- fmt.Errorf("truncation handling failed: %w", err):
						case <-ctx.Done():
							return
						}
						backoffTimer.Reset(100 * time.Millisecond)
					} else {
						// After successful rotation, read new content
						f.readAvailableLines(reader, eventCh, errCh, ctx)
					}
				} else {
					// Normal write - read new content
					f.readAvailableLines(reader, eventCh, errCh, ctx)
				}

			case event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove):
				// File was rotated/removed - handle rotation
				if err := f.handleRotation(reader, eventCh, errCh); err != nil {
					select {
					case errCh <- fmt.Errorf("rotation handling failed: %w", err):
					case <-ctx.Done():
						return
					}
					// Start backoff on rotation error
					backoffTimer.Reset(100 * time.Millisecond)
				} else {
					// After successful rotation, read new content
					f.readAvailableLines(reader, eventCh, errCh, ctx)
				}

			case event.Has(fsnotify.Create):
				// File was created (could be after rename rotation)
				f.readAvailableLines(reader, eventCh, errCh, ctx)
			}

		case err, ok := <-f.watcher.Errors:
			if !ok {
				return // watcher closed
			}
			select {
			case errCh <- fmt.Errorf("watcher error: %w", err):
			case <-ctx.Done():
				return
			}
		}
	}
}

// readAvailableLines reads all available lines from the current position
func (f *FileReader) readAvailableLines(reader *bufio.Reader, eventCh chan<- core.LogEvent, errCh chan<- error, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		lineBytes, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				// Process any remaining data without newline
				if len(lineBytes) > 0 {
					line := string(lineBytes)
					event := f.createLogEvent(line)
					select {
					case eventCh <- event:
					case <-ctx.Done():
						return
					}
				}
				// EOF means no more data currently available
				return
			}
			// Other read errors
			select {
			case errCh <- fmt.Errorf("read error: %w", err):
			case <-ctx.Done():
				return
			}
			return
		}

		// Convert bytes to string and trim newline
		line := string(lineBytes)
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}

		event := f.createLogEvent(line)
		select {
		case eventCh <- event:
		case <-ctx.Done():
			return
		}
	}
}

// handleRotation handles file rotation scenarios
func (f *FileReader) handleRotation(reader *bufio.Reader, eventCh chan<- core.LogEvent, errCh chan<- error) error {
	// Try to read any remaining data from the current file handle
	f.readAvailableLines(reader, eventCh, errCh, context.Background())

	// Close current file
	if f.file != nil {
		f.file.Close()
		f.file = nil
	}

	// Remove the old watch to avoid conflicts
	f.watcher.Remove(f.path)

	// Attempt to reopen the file (it might have been recreated)
	var err error
	retries := 0
	maxRetries := 20

	for retries < maxRetries {
		f.file, err = os.Open(f.path)
		if err == nil {
			break
		}

		// File might not exist yet after rotation, wait a bit
		time.Sleep(100 * time.Millisecond)
		retries++
	}

	if err != nil {
		return fmt.Errorf("failed to reopen file after rotation: %w", err)
	}

	// Get new file stats
	newStat, err := f.file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat reopened file: %w", err)
	}

	// Always treat rotation as a new file and start from beginning
	// This handles both rename+create and copytruncate scenarios
	f.lastStat = newStat
	f.file.Seek(0, io.SeekStart)
	reader.Reset(f.file)

	// Re-add to watcher
	if err := f.watcher.Add(f.path); err != nil {
		return fmt.Errorf("failed to re-watch file: %w", err)
	}

	return nil
}

// checkForTruncation checks if the file has been truncated (copytruncate rotation)
func (f *FileReader) checkForTruncation() bool {
	if f.file == nil || f.lastStat == nil {
		return false
	}

	// Get current file stats
	currentStat, err := f.file.Stat()
	if err != nil {
		return false
	}

	// Check if the file size decreased (indicates truncation)
	if currentStat.Size() < f.lastStat.Size() {
		return true
	}

	return false
}

// isNewFile checks if the file info represents a different file than before
func (f *FileReader) isNewFile(newStat os.FileInfo) bool {
	if f.lastStat == nil {
		return true
	}

	// Compare size, modification time, and underlying system info
	oldSys := f.lastStat.Sys()
	newSys := newStat.Sys()

	// Different modification time or size suggests it's different
	if !f.lastStat.ModTime().Equal(newStat.ModTime()) || f.lastStat.Size() != newStat.Size() {
		// If the new file is smaller or much newer, it's likely a new file
		if newStat.Size() < f.lastStat.Size() ||
			newStat.ModTime().Sub(f.lastStat.ModTime()) > time.Second {
			return true
		}
	}

	// On Unix systems, we can compare inodes
	if oldSys != nil && newSys != nil {
		// This is platform-specific, but works on Linux/macOS
		// Would need build tags for proper cross-platform support
		return fmt.Sprintf("%v", oldSys) != fmt.Sprintf("%v", newSys)
	}

	return false
}

// createLogEvent creates a LogEvent from a line of input
func (f *FileReader) createLogEvent(line string) core.LogEvent {
	seq := atomic.AddUint64(&f.seq, 1)

	return core.LogEvent{
		Seq:       seq,
		Time:      time.Now(),
		Source:    core.SourceFile,
		Container: "",
		Line:      line,
		LevelStr:  "", // TODO: Add severity detection in future
		Level:     core.SevUnknown,
	}
}

// cleanup closes file handles and watcher
func (f *FileReader) cleanup() {
	if f.watcher != nil {
		f.watcher.Close()
	}
	if f.file != nil {
		f.file.Close()
	}
}
