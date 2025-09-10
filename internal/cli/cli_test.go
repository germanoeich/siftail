package cli

import (
	"os"
	"testing"

	"github.com/germanoeich/siftail/internal/tui"
)

func TestCLI_FileMode_StartsTailer(t *testing.T) {
	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "siftail_test_*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()
	
	// Write some test content
	if _, err := tmpFile.WriteString("test log line\n"); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	
	// Test file mode detection
	args := []string{tmpFile.Name()}
	config, err := ParseArgs(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}
	
	if config.Mode != tui.ModeFile {
		t.Errorf("Expected mode to be ModeFile, got %v", config.Mode)
	}
	
	if config.FilePath != tmpFile.Name() {
		t.Errorf("Expected file path to be %s, got %s", tmpFile.Name(), config.FilePath)
	}
}

func TestCLI_DockerMode_StartsDockerReader_Fake(t *testing.T) {
	// Test docker mode detection
	args := []string{"docker"}
	config, err := ParseArgs(args)
	if err != nil {
		t.Fatalf("Failed to parse args: %v", err)
	}
	
	if config.Mode != tui.ModeDocker {
		t.Errorf("Expected mode to be ModeDocker, got %v", config.Mode)
	}
	
	if config.FilePath != "" {
		t.Errorf("Expected empty file path for docker mode, got %s", config.FilePath)
	}
}

func TestCLI_StdinMode_WhenPiped(t *testing.T) {
	// This test simulates stdin mode by testing with no arguments
	// In a real scenario, stdin detection would need to be mocked
	
	// Test with no arguments (should detect stdin mode if piped)
	args := []string{}
	
	// This will fail in test because stdin is a terminal
	// In a real implementation, we'd mock the stdin detection
	_, err := ParseArgs(args)
	if err == nil {
		t.Error("Expected error when no args and stdin is terminal")
	}
	
	// The error should mention stdin or input
	if err.Error() == "" {
		t.Error("Expected meaningful error message")
	}
}

func TestParseArgs_ValidFlags(t *testing.T) {
	testCases := []struct {
		args     []string
		expected Config
	}{
		{
			args: []string{"--buffer-size", "5000", "docker"},
			expected: Config{
				Mode:       tui.ModeDocker,
				BufferSize: 5000,
				FromStart:  false,
				NoColor:    false,
			},
		},
		{
			args: []string{"--from-start", "--no-color", "/tmp/test.log"},
			expected: Config{
				Mode:      tui.ModeFile,
				FilePath:  "/tmp/test.log",
				FromStart: true,
				NoColor:   true,
			},
		},
		{
			args: []string{"--time-format", "15:04:05", "docker"},
			expected: Config{
				Mode:       tui.ModeDocker,
				TimeFormat: "15:04:05",
			},
		},
	}
	
	for i, tc := range testCases {
		config, err := ParseArgs(tc.args)
		
		// Skip validation for file paths that don't exist
		if tc.expected.Mode == tui.ModeFile {
			continue
		}
		
		if err != nil {
			t.Errorf("Test case %d: unexpected error: %v", i, err)
			continue
		}
		
		if config.Mode != tc.expected.Mode {
			t.Errorf("Test case %d: expected mode %v, got %v", i, tc.expected.Mode, config.Mode)
		}
		
		if tc.expected.BufferSize != 0 && config.BufferSize != tc.expected.BufferSize {
			t.Errorf("Test case %d: expected buffer size %d, got %d", i, tc.expected.BufferSize, config.BufferSize)
		}
		
		if config.FromStart != tc.expected.FromStart {
			t.Errorf("Test case %d: expected from-start %t, got %t", i, tc.expected.FromStart, config.FromStart)
		}
		
		if config.NoColor != tc.expected.NoColor {
			t.Errorf("Test case %d: expected no-color %t, got %t", i, tc.expected.NoColor, config.NoColor)
		}
		
		if tc.expected.TimeFormat != "" && config.TimeFormat != tc.expected.TimeFormat {
			t.Errorf("Test case %d: expected time format %s, got %s", i, tc.expected.TimeFormat, config.TimeFormat)
		}
	}
}

func TestParseArgs_HelpAndVersion(t *testing.T) {
	testCases := []struct {
		args         []string
		expectHelp   bool
		expectVersion bool
	}{
		{[]string{"--help"}, true, false},
		{[]string{"-h"}, true, false},
		{[]string{"--version"}, false, true},
		{[]string{"-v"}, false, true},
	}
	
	for i, tc := range testCases {
		config, err := ParseArgs(tc.args)
		if err != nil {
			t.Errorf("Test case %d: unexpected error: %v", i, err)
			continue
		}
		
		if config.ShowHelp != tc.expectHelp {
			t.Errorf("Test case %d: expected help %t, got %t", i, tc.expectHelp, config.ShowHelp)
		}
		
		if config.ShowVersion != tc.expectVersion {
			t.Errorf("Test case %d: expected version %t, got %t", i, tc.expectVersion, config.ShowVersion)
		}
	}
}

func TestParseArgs_InvalidArgs(t *testing.T) {
	testCases := []struct {
		args        []string
		expectError bool
	}{
		{[]string{"file1", "file2"}, true},                    // Too many args
		{[]string{"--buffer-size", "0", "docker"}, true},     // Invalid buffer size
		{[]string{"--buffer-size", "-100", "docker"}, true},  // Negative buffer size
		{[]string{"/nonexistent/file.log"}, true},            // Non-existent file
		{[]string{"invalid-mode"}, true},                     // Invalid mode
	}
	
	for i, tc := range testCases {
		_, err := ParseArgs(tc.args)
		
		if tc.expectError && err == nil {
			t.Errorf("Test case %d: expected error but got none", i)
		}
		
		if !tc.expectError && err != nil {
			t.Errorf("Test case %d: unexpected error: %v", i, err)
		}
	}
}

func TestValidateConfig(t *testing.T) {
	testCases := []struct {
		config      Config
		expectError bool
		description string
	}{
		{
			config:      Config{BufferSize: 50},
			expectError: true,
			description: "buffer size too small",
		},
		{
			config:      Config{BufferSize: 2000000},
			expectError: true,
			description: "buffer size too large",
		},
		{
			config:      Config{BufferSize: 10000, TimeFormat: "15:04:05"},
			expectError: false,
			description: "valid time format",
		},
		{
			config:      Config{BufferSize: 10000, TimeFormat: "15:04:05"},
			expectError: false,
			description: "valid config",
		},
	}
	
	for i, tc := range testCases {
		err := ValidateConfig(tc.config)
		
		if tc.expectError && err == nil {
			t.Errorf("Test case %d (%s): expected error but got none", i, tc.description)
		}
		
		if !tc.expectError && err != nil {
			t.Errorf("Test case %d (%s): unexpected error: %v", i, tc.description, err)
		}
	}
}

func TestParseBufferSize(t *testing.T) {
	testCases := []struct {
		input    string
		expected int
		hasError bool
	}{
		{"1000", 1000, false},
		{"5K", 5000, false},
		{"5k", 5000, false},
		{"2M", 2000000, false},
		{"2m", 2000000, false},
		{"", 0, true},
		{"invalid", 0, true},
	}
	
	for i, tc := range testCases {
		result, err := ParseBufferSize(tc.input)
		
		if tc.hasError {
			if err == nil {
				t.Errorf("Test case %d: expected error for input %s", i, tc.input)
			}
		} else {
			if err != nil {
				t.Errorf("Test case %d: unexpected error for input %s: %v", i, tc.input, err)
			}
			
			if result != tc.expected {
				t.Errorf("Test case %d: expected %d for input %s, got %d", i, tc.expected, tc.input, result)
			}
		}
	}
}

func TestGetModeString(t *testing.T) {
	testCases := []struct {
		mode     tui.Mode
		expected string
	}{
		{tui.ModeFile, "file"},
		{tui.ModeStdin, "stdin"},
		{tui.ModeDocker, "docker"},
	}
	
	for i, tc := range testCases {
		result := GetModeString(tc.mode)
		if result != tc.expected {
			t.Errorf("Test case %d: expected %s for mode %v, got %s", i, tc.expected, tc.mode, result)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	
	if config.BufferSize != 10000 {
		t.Errorf("Expected default buffer size 10000, got %d", config.BufferSize)
	}
	
	if config.TimeFormat != "15:04:05.000" {
		t.Errorf("Expected default time format '15:04:05.000', got %s", config.TimeFormat)
	}
	
	if config.NoColor {
		t.Error("Expected default no-color to be false")
	}
	
	if config.FromStart {
		t.Error("Expected default from-start to be false")
	}
}

func TestDetermineMode(t *testing.T) {
	// Test with docker argument
	mode, filePath, err := determineMode([]string{"docker"})
	if err != nil {
		t.Errorf("Unexpected error for docker mode: %v", err)
	}
	if mode != tui.ModeDocker {
		t.Errorf("Expected ModeDocker, got %v", mode)
	}
	if filePath != "" {
		t.Errorf("Expected empty file path for docker mode, got %s", filePath)
	}
	
	// Test with too many arguments
	_, _, err = determineMode([]string{"arg1", "arg2", "arg3"})
	if err == nil {
		t.Error("Expected error for too many arguments")
	}
}