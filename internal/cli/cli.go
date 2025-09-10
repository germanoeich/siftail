package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/germanoeich/siftail/internal/core"
	"github.com/germanoeich/siftail/internal/tui"
)

// Config holds the parsed command-line configuration
type Config struct {
	Mode         tui.Mode
	FilePath     string
	BufferSize   int
	FromStart    bool
	NoColor      bool
	TimeFormat   string
	ShowHelp     bool
	ShowVersion  bool
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() Config {
	return Config{
		BufferSize: 10000,
		TimeFormat: "15:04:05.000",
		NoColor:    false,
		FromStart:  false,
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
	fs.BoolVar(&config.FromStart, "from-start", config.FromStart, "start reading from beginning of file (file mode only)")
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

	// Configure based on CLI options
	if config.NoColor {
		// TODO: Implement color disabling
	}

	// Initialize data source based on mode
	switch config.Mode {
	case tui.ModeFile:
		if err := startFileReader(config.FilePath, config.FromStart, ring); err != nil {
			return fmt.Errorf("failed to start file reader: %w", err)
		}

	case tui.ModeStdin:
		if err := startStdinReader(ring); err != nil {
			return fmt.Errorf("failed to start stdin reader: %w", err)
		}

	case tui.ModeDocker:
		if err := startDockerReader(ring); err != nil {
			return fmt.Errorf("failed to start docker reader: %w", err)
		}
	}

	// Start the TUI
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

// startFileReader initializes file tailing for the given path
func startFileReader(filePath string, fromStart bool, ring *core.Ring) error {
	// TODO: Implement file reader using internal/input package
	// This is a placeholder implementation
	fmt.Printf("Would start file reader for: %s (fromStart: %v)\n", filePath, fromStart)
	return nil
}

// startStdinReader initializes stdin streaming
func startStdinReader(ring *core.Ring) error {
	// TODO: Implement stdin reader using internal/input package
	// This is a placeholder implementation
	fmt.Println("Would start stdin reader")
	return nil
}

// startDockerReader initializes docker container streaming
func startDockerReader(ring *core.Ring) error {
	// TODO: Implement docker reader using internal/input package
	// This is a placeholder implementation
	fmt.Println("Would start docker reader")
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
  --from-start                 start reading from beginning of file (file mode)
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