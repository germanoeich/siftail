package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestMain_BuildsAndPrintsHelp(t *testing.T) {
	// Build the binary first
	buildCmd := exec.Command("go", "build", "-o", "siftail_test", ".")
	if err := buildCmd.Run(); t.Failed() {
		t.Fatalf("Failed to build siftail: %v", err)
	}

	// Clean up the test binary afterwards
	defer func() {
		_ = exec.Command("rm", "-f", "siftail_test").Run()
	}()

	// Execute siftail -h and capture output
	cmd := exec.Command("./siftail_test", "-h")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to execute siftail -h: %v", err)
	}

	usage := string(output)

	// Assert that usage contains all three modes
	if !strings.Contains(usage, "docker") {
		t.Error("Usage should mention 'docker' mode")
	}

	if !strings.Contains(usage, "stdin") {
		t.Error("Usage should mention 'stdin' mode")
	}

	if !strings.Contains(usage, "file") {
		t.Error("Usage should mention 'file' mode")
	}

	// Verify it shows the expected usage patterns
	expectedPatterns := []string{
		"siftail [flags] [file]",
		"siftail docker",
		"<command> | siftail",
	}

	for _, pattern := range expectedPatterns {
		if !strings.Contains(usage, pattern) {
			t.Errorf("Usage should contain pattern: %s", pattern)
		}
	}
}
