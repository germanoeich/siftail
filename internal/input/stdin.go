package input

import (
	"bufio"
	"context"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/germanoeich/siftail/internal/core"
)

// StdinReader reads from standard input using bufio.Reader to handle arbitrarily long lines
type StdinReader struct {
	reader io.Reader
	seq    uint64
}

// NewStdinReader creates a new STDIN reader
func NewStdinReader() *StdinReader {
	return &StdinReader{
		reader: os.Stdin,
	}
}

// NewStdinReaderFromReader creates a STDIN reader from a custom io.Reader (useful for testing)
func NewStdinReaderFromReader(reader io.Reader) *StdinReader {
	return &StdinReader{
		reader: reader,
	}
}

// Start implements the Reader interface
// Uses bufio.Reader.ReadBytes to handle arbitrarily long lines without Scanner's 64KB limit
func (s *StdinReader) Start(ctx context.Context) (<-chan core.LogEvent, <-chan error) {
	eventCh := make(chan core.LogEvent, 50) // buffered to prevent blocking
	errCh := make(chan error, 5)            // buffered for error reporting

	go func() {
		defer close(eventCh)
		defer close(errCh)

		bufReader := bufio.NewReader(s.reader)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				// ReadBytes handles arbitrarily long lines without the default Scanner limit
				lineBytes, err := bufReader.ReadBytes('\n')
				if err != nil {
					if err == io.EOF {
						// Process any remaining data before exiting
						if len(lineBytes) > 0 {
							line := string(lineBytes)
							// Don't trim trailing newline since EOF doesn't guarantee one
							event := s.createLogEvent(line)
							select {
							case eventCh <- event:
							case <-ctx.Done():
								return
							}
						}
						// EOF stops gracefully - exit if stdin mode
						return
					}

					// Other errors
					select {
					case errCh <- err:
					case <-ctx.Done():
						return
					}
					return
				}

				// Convert bytes to string and process
				line := string(lineBytes)

				// Trim trailing \n but keep \r if present
				if len(line) > 0 && line[len(line)-1] == '\n' {
					line = line[:len(line)-1]
				}

				event := s.createLogEvent(line)

				select {
				case eventCh <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return eventCh, errCh
}

// createLogEvent creates a LogEvent from a line of input
func (s *StdinReader) createLogEvent(line string) core.LogEvent {
	seq := atomic.AddUint64(&s.seq, 1)

	return core.LogEvent{
		Seq:       seq,
		Time:      time.Now(), // Stamp Time: time.Now() if no timestamp parsing
		Source:    core.SourceStdin,
		Container: "", // empty for stdin
		Line:      line,
		LevelStr:  "", // TODO: Add severity detection in future
		Level:     core.SevUnknown,
	}
}
