package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/germanoeich/siftail/internal/core"
	"github.com/germanoeich/siftail/internal/dockerx"
	"github.com/germanoeich/siftail/internal/input"
	"github.com/germanoeich/siftail/internal/tui"
)

// Config holds the parsed command-line configuration
type Config struct {
	Mode        tui.Mode
	FilePath    string
	BufferSize  int
	FromStart   bool
	NumLines    int // file mode prefill; if <0, read whole file
	Theme       string
	NoColor     bool
	TimeFormat  string
	ShowHelp    bool
	ShowVersion bool
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() Config {
	return Config{
		BufferSize: 10000,
		TimeFormat: "15:04:05.000",
		NoColor:    false,
		FromStart:  true, // default to read entire file
		NumLines:   -1,   // unset
		Theme:      "dark",
	}
}

// ParseArgs parses command-line arguments and returns a configuration
func ParseArgs(args []string) (Config, error) {
	config := DefaultConfig()

	// Create a new flag set for parsing
	fs := flag.NewFlagSet("siftail", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Print(usage)
	}

	// Define flags
	fs.IntVar(&config.BufferSize, "buffer-size", config.BufferSize, "ring buffer size for log events")
	fs.BoolVar(&config.FromStart, "from-start", config.FromStart, "start reading from beginning of file (file mode only; default true)")
	fs.IntVar(&config.NumLines, "n", config.NumLines, "prefill last N lines (file mode only; overrides --from-start)")
	fs.IntVar(&config.NumLines, "num-lines", config.NumLines, "prefill last N lines (file mode only; overrides --from-start)")
	fs.StringVar(&config.Theme, "theme", config.Theme, "UI theme (dark, dracula, nord, light)")
	fs.BoolVar(&config.NoColor, "no-color", config.NoColor, "disable colored output")
	fs.StringVar(&config.TimeFormat, "time-format", config.TimeFormat, "timestamp format for display")
	fs.BoolVar(&config.ShowHelp, "h", config.ShowHelp, "show help message")
	fs.BoolVar(&config.ShowHelp, "help", config.ShowHelp, "show help message")
	fs.BoolVar(&config.ShowVersion, "v", config.ShowVersion, "show version information")
	fs.BoolVar(&config.ShowVersion, "version", config.ShowVersion, "show version information")

	// Parse the arguments
	if err := fs.Parse(args); err != nil {
		return config, err
	}

	// Handle help and version flags
	if config.ShowHelp {
		fs.Usage()
		return config, nil
	}

	if config.ShowVersion {
		fmt.Println("siftail version 0.1.0")
		return config, nil
	}

	// Validate buffer size
	if config.BufferSize <= 0 {
		return config, errors.New("buffer-size must be positive")
	}

	// Determine mode based on remaining arguments
	remaining := fs.Args()
	mode, filePath, err := determineMode(remaining)
	if err != nil {
		return config, err
	}

	config.Mode = mode
	config.FilePath = filePath

	return config, nil
}

// determineMode analyzes arguments and stdin to determine the operational mode
func determineMode(args []string) (tui.Mode, string, error) {
	// Check if stdin has data (piped input)
	stat, err := os.Stdin.Stat()
	hasStdinData := err == nil && (stat.Mode()&os.ModeCharDevice) == 0

	switch {
	case len(args) == 0:
		if hasStdinData {
			return tui.ModeStdin, "", nil
		} else {
			return 0, "", errors.New("no input specified and stdin is a terminal. Use -h for help")
		}

	case len(args) == 1 && args[0] == "docker":
		if hasStdinData {
			return 0, "", errors.New("cannot use docker mode with piped input")
		}
		return tui.ModeDocker, "", nil

	case len(args) == 1:
		if hasStdinData {
			return 0, "", errors.New("cannot specify file path with piped input")
		}
		// Validate file exists or is accessible
		filePath := args[0]
		if err := validateFilePath(filePath); err != nil {
			return 0, "", fmt.Errorf("file access error: %w", err)
		}
		return tui.ModeFile, filePath, nil

	default:
		return 0, "", errors.New("too many arguments. Use -h for help")
	}
}

// validateFilePath checks if a file path is accessible
func validateFilePath(path string) error {
	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file does not exist: %s", path)
		}
		return fmt.Errorf("cannot access file: %w", err)
	}

	// Check if it's a regular file
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", path)
	}

	// Check if file is readable
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot read file: %w", err)
	}
	file.Close()

	return nil
}

// Run executes the application with the given configuration
func Run(config Config) error {
	// Initialize core components
	ring := core.NewRing(config.BufferSize)
	filters := core.NewFilters()
	search := core.NewSearchState()
	levels := core.NewLevelMap()

	// Create TUI model
	model := tui.NewModel(ring, filters, search, levels, config.Mode)

	// Bubble Tea program (created before starting readers so we can send refresh msgs)
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Wire input -> ring and notify UI
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize data source based on mode
	switch config.Mode {
	case tui.ModeFile:
		if err := startFileReader(ctx, config.FilePath, config.FromStart, config.NumLines, ring, program); err != nil {
			return fmt.Errorf("failed to start file reader: %w", err)
		}

	case tui.ModeStdin:
		if err := startStdinReader(ctx, ring, program); err != nil {
			return fmt.Errorf("failed to start stdin reader: %w", err)
		}

	case tui.ModeDocker:
		if err := startDockerReader(ctx, ring, levels, program); err != nil {
			return fmt.Errorf("failed to start docker reader: %w", err)
		}
	}

	// Apply theme prior to run
	if config.Theme != "" {
		model.SetTheme(config.Theme)
	}

	// Run the TUI (blocks until exit)
	_, err := program.Run()

	// Ensure readers are stopped
	cancel()
	return err
}

// uiRefresher is the minimal interface we need from a Bubble Tea program
type uiRefresher interface {
	Send(msg tea.Msg)
}

// wireEventStream pumps events from a reader into the ring and notifies the UI
func wireEventStream(ctx context.Context, events <-chan core.LogEvent, errs <-chan error, ring *core.Ring, ui uiRefresher) {
	// Events
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-events:
				if !ok {
					return
				}
				e = ring.Append(e)
				// Notify UI of the new event (so find can index incrementally)
				if ui != nil {
					ui.Send(tui.LogAppendedMsg{Event: e})
					ui.Send(tui.RefreshCmd()())
				}
			}
		}
	}()

	// Errors
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-errs:
				if !ok {
					return
				}
				// Print to stderr; model also shows count via status if desired later
				fmt.Fprintf(os.Stderr, "input error: %v\n", err)
			}
		}
	}()
}

// startFileReader initializes file tailing for the given path
func startFileReader(ctx context.Context, filePath string, fromStart bool, numLines int, ring *core.Ring, ui uiRefresher) error {
	// If numLines specified, prefill last N lines and then tail from end
	if numLines >= 0 {
		_ = prefillLastLines(filePath, numLines, 16*1024*1024, ring, ui)
		fromStart = false
	}

	reader := input.NewFileReader(filePath, fromStart)
	events, errs := reader.Start(ctx)
	wireEventStream(ctx, events, errs, ring, ui)
	return nil
}

// startStdinReader initializes stdin streaming
func startStdinReader(ctx context.Context, ring *core.Ring, ui uiRefresher) error {
	reader := input.NewStdinReader()
	events, errs := reader.Start(ctx)
	wireEventStream(ctx, events, errs, ring, ui)
	return nil
}

// startDockerReader initializes docker container streaming
func startDockerReader(ctx context.Context, ring *core.Ring, levels *core.LevelMap, ui uiRefresher) error {
	// Create real docker client
	real, err := dockerx.NewRealClient()
	if err != nil {
		// Surface a recoverable error to the UI
		if ui != nil {
			ui.Send(tui.DockerErrorMsg{Error: err, Recoverable: true})
		}
		return err
	}

	detector := core.NewDefaultSeverityDetector(levels)
	reader := input.NewDockerReader(real, detector)

	events, errs := reader.Start(ctx)
	wireEventStream(ctx, events, errs, ring, ui)

	// Periodically push container list snapshots to the UI
	go func() {
		// Send an initial snapshot soon after start
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			// Build name->visible map (default visible=true)
			containers := reader.GetContainers()
			m := make(map[string]bool, len(containers))
			for _, c := range containers {
				if c.Name != "" {
					m[c.Name] = true
				} else {
					m[c.ID] = true
				}
			}
			if ui != nil {
				ui.Send(tui.DockerContainersMsg{Containers: m})
			}

			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()

	return nil
}

// prefillLastLines reads up to the last N lines (bounded by maxBytes) and appends them to the ring.
// This does not affect the tailer position; it's just an initial snapshot for user context.
func prefillLastLines(path string, maxLines int, maxBytes int64, ring *core.Ring, ui uiRefresher) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return err
	}

	size := st.Size()
	if size == 0 {
		return nil
	}

	// Determine how many bytes to read from the end
	readBytes := size
	if readBytes > maxBytes {
		readBytes = maxBytes
	}

	// Seek to start position for reading
	start := size - readBytes
	if _, err := f.Seek(start, 0); err != nil {
		return err
	}

	// Read chunk
	buf := make([]byte, readBytes)
	if _, err := io.ReadFull(f, buf); err != nil && err != io.ErrUnexpectedEOF {
		return err
	}

	// Split into lines; if we started mid-line, drop the first partial
	lines := bufio.NewScanner(bytes.NewReader(buf))
	lines.Split(bufio.ScanLines)
	var all []string
	for lines.Scan() {
		all = append(all, lines.Text())
	}
	if len(all) == 0 {
		return nil
	}
	// If we did not start at byte 0, the first scanned line is partial; drop it
	if start > 0 {
		all = all[1:]
	}
	// Keep only the last maxLines
	if len(all) > maxLines {
		all = all[len(all)-maxLines:]
	}

	// Append to ring in order
	for _, line := range all {
		ring.Append(core.LogEvent{
			Time:      time.Now(),
			Source:    core.SourceFile,
			Line:      line,
			Level:     core.SevUnknown,
			LevelStr:  "",
			Container: "",
		})
	}
	if ui != nil && len(all) > 0 {
		ui.Send(tui.RefreshCmd()())
	}
	return nil
}

// usage string for the CLI
const usage = `siftail - a TUI for tailing and exploring logs

USAGE:
  siftail [flags] [file]       # file mode - tail a file
  siftail docker               # docker mode - stream from all running containers
  <command> | siftail          # stdin mode - read piped input as live stream

EXAMPLES:
  siftail /var/log/app.log     # tail a file with rotation awareness
  siftail docker               # stream from all Docker containers
  journalctl -f | siftail      # tail systemd journal via stdin

FLAGS:
  -h, --help                   show this help message
  -v, --version                show version information
  --buffer-size N              ring buffer size (default: 10000)
  --from-start                 start reading from beginning of file (file mode; default)
  -n, --num-lines N            prefill last N lines (file mode; overrides --from-start)
  --theme NAME                 UI theme (dark, dracula, nord, light)
  --no-color                   disable colored output
  --time-format FORMAT         timestamp format (default: "15:04:05.000")

HOTKEYS (once running):
  q, Ctrl+C                    quit
  h                            highlight text (no scroll)
  f                            find text (jump to matches with Up/Down)
  F                            filter-in (show only matching lines)
  U                            filter-out (hide matching lines)
  1-9                          toggle severity levels
  l                            list containers (docker mode)
  P                            manage presets (docker mode)

SEVERITY LEVELS:
  Default mapping: 1=DEBUG, 2=INFO, 3=WARN, 4=ERROR
  Custom levels automatically assigned to slots 5-9
  Overflow levels grouped into 9:OTHER
`

// ValidateConfig performs additional validation on the parsed configuration
func ValidateConfig(config Config) error {
	// Validate buffer size bounds
	if config.BufferSize < 100 {
		return errors.New("buffer-size too small (minimum: 100)")
	}
	if config.BufferSize > 1000000 {
		return errors.New("buffer-size too large (maximum: 1,000,000)")
	}

	// Validate time format
	if config.TimeFormat != "" {
		// Try to format a test time to validate the format
		_, err := parseTimeFormat(config.TimeFormat)
		if err != nil {
			return fmt.Errorf("invalid time format: %w", err)
		}
	}

	return nil
}

// parseTimeFormat validates a time format string
func parseTimeFormat(format string) (string, error) {
	// This is a simplified validation - in a real implementation,
	// we would use time.Parse with the Go reference time
	// Mon Jan 2 15:04:05 MST 2006, which is Unix time 1136239445
	if len(format) == 0 {
		return "", errors.New("empty time format")
	}

	return format, nil
}

// GetModeString returns a human-readable string for the mode
func GetModeString(mode tui.Mode) string {
	switch mode {
	case tui.ModeFile:
		return "file"
	case tui.ModeStdin:
		return "stdin"
	case tui.ModeDocker:
		return "docker"
	default:
		return "unknown"
	}
}

// PrintConfig prints the current configuration (useful for debugging)
func PrintConfig(config Config) {
	fmt.Printf("Configuration:\n")
	fmt.Printf("  Mode: %s\n", GetModeString(config.Mode))
	if config.FilePath != "" {
		fmt.Printf("  File Path: %s\n", config.FilePath)
	}
	fmt.Printf("  Buffer Size: %d\n", config.BufferSize)
	fmt.Printf("  From Start: %t\n", config.FromStart)
	fmt.Printf("  No Color: %t\n", config.NoColor)
	fmt.Printf("  Time Format: %s\n", config.TimeFormat)
}

// ParseBufferSize parses a buffer size string with optional suffixes (K, M)
func ParseBufferSize(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty buffer size")
	}

	// Check for suffix
	suffix := s[len(s)-1:]
	var multiplier int64 = 1
	var numStr = s

	switch suffix {
	case "K", "k":
		multiplier = 1000
		numStr = s[:len(s)-1]
	case "M", "m":
		multiplier = 1000000
		numStr = s[:len(s)-1]
	}

	num, err := strconv.ParseInt(numStr, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid buffer size: %w", err)
	}

	result := num * multiplier
	if result > int64(^uint32(0)) { // Check for overflow
		return 0, errors.New("buffer size too large")
	}

	return int(result), nil
}
