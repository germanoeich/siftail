# siftail

A Go + Bubble Tea TUI for tailing and exploring logs from multiple sources.

## Quick Start

**siftail** supports three input modes:

### File Mode
Tail a file with rotation and truncation awareness:
```bash
siftail /var/log/app.log
```

Notes:
- By default, siftail reads the entire file from the beginning, then continues tailing.
- To show only the last N lines initially and tail from the end, use `-n N` (or `--num-lines N`).

### Docker Mode  
Stream logs from all running containers:
```bash
siftail docker
```

### Stdin Mode
Read piped input as a live stream:
```bash
journalctl -f -u my.service | siftail
```

## Features

- Live, scrollable viewport with nano-style toolbar
- **Highlight** text without scrolling
- **Find** text and jump between matches  
- **Filter-in** to show only matching lines
- **Filter-out** to hide matching lines
- **Dynamic severity detection** with toggleable levels (1-9)
- **Docker container management** with presets
- Handles file rotation, long lines, and high-volume input
- Ignores destructive terminal control sequences (spinners/clears) for stable rendering
- Soft-wraps long lines to the viewport width (no ellipses)

## Build

```bash
# Build the binary
make build

# Run tests
make test

# Run with race detector
make race
```

## Requirements

- Go 1.22+
- For Docker mode: Docker daemon access (socket permissions apply)

## Notes on terminal control sequences

Some tools (e.g., build/code generators) emit dynamic terminal control sequences to
update a single line in place (spinners) or to clear regions of the screen.
These sequences can wreak havoc in a TUI viewport. siftail sanitizes incoming lines
from all inputs (stdin, files, and Docker) and strips such sequences, converting
inline carriage returns to spaces while preserving a trailing CR (from CRLF) so
content remains readable and scrollback stays consistent.

## License

MIT
