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
- By default, siftail tails from the end and prefills the view with up to the last 200 lines (reading at most ~512KB). Use `--from-start` to load the entire existing file before tailing.

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

## License

MIT
