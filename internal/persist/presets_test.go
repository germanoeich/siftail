package persist

import (
	"os"
	"path/filepath"
	"testing"
	"runtime"
)

func TestPresets_SaveAndLoad(t *testing.T) {
	// Use temporary directory for testing
	tempDir, err := os.MkdirTemp("", "siftail_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create manager with custom path
	manager := &PresetsManager{
		configPath: filepath.Join(tempDir, "presets.json"),
	}

	// Test saving and loading presets
	testPresets := []Preset{
		{
			Name: "web-services",
			Visible: map[string]bool{
				"nginx":  true,
				"apache": true,
				"mysql":  false,
			},
		},
		{
			Name: "databases",
			Visible: map[string]bool{
				"mysql":    true,
				"postgres": true,
				"redis":    false,
			},
		},
	}

	// Save presets
	if err := manager.SavePresets(testPresets); err != nil {
		t.Fatalf("Failed to save presets: %v", err)
	}

	// Load presets
	loadedPresets, err := manager.LoadPresets()
	if err != nil {
		t.Fatalf("Failed to load presets: %v", err)
	}

	// Verify loaded presets match
	if len(loadedPresets) != len(testPresets) {
		t.Fatalf("Expected %d presets, got %d", len(testPresets), len(loadedPresets))
	}

	for i, preset := range loadedPresets {
		if preset.Name != testPresets[i].Name {
			t.Errorf("Preset name mismatch: expected %s, got %s", testPresets[i].Name, preset.Name)
		}
		
		if len(preset.Visible) != len(testPresets[i].Visible) {
			t.Errorf("Preset visibility length mismatch for %s", preset.Name)
		}
		
		for container, visible := range testPresets[i].Visible {
			if preset.Visible[container] != visible {
				t.Errorf("Preset visibility mismatch for %s.%s: expected %v, got %v",
					preset.Name, container, visible, preset.Visible[container])
			}
		}
	}
}

func TestPresets_ApplyByName(t *testing.T) {
	preset := Preset{
		Name: "test-preset",
		Visible: map[string]bool{
			"web-server": true,
			"database":   false,
			"cache":      true,
		},
	}

	currentContainers := map[string]bool{
		"web-server": false, // Should be changed to true
		"database":   true,  // Should be changed to false
		"cache":      false, // Should be changed to true
		"other":      true,  // Should remain unchanged
	}

	result := ApplyPreset(preset, currentContainers)

	expected := map[string]bool{
		"web-server": true,
		"database":   false,
		"cache":      true,
		"other":      true, // Unchanged
	}

	for container, expectedVisible := range expected {
		if result[container] != expectedVisible {
			t.Errorf("Container %s: expected %v, got %v", container, expectedVisible, result[container])
		}
	}
}

func TestPresets_IgnoresMissingContainers(t *testing.T) {
	preset := Preset{
		Name: "test-preset",
		Visible: map[string]bool{
			"existing-container":    true,
			"non-existing-container": true,
		},
	}

	currentContainers := map[string]bool{
		"existing-container": false,
	}

	result := ApplyPreset(preset, currentContainers)

	// Should only affect existing container
	if len(result) != 1 {
		t.Errorf("Expected 1 container in result, got %d", len(result))
	}

	if result["existing-container"] != true {
		t.Errorf("Expected existing-container to be true, got %v", result["existing-container"])
	}

	// Non-existing container should not be added
	if _, exists := result["non-existing-container"]; exists {
		t.Errorf("Non-existing container should not be added to result")
	}
}

func TestPresets_SaveSinglePreset(t *testing.T) {
	// Use temporary directory for testing
	tempDir, err := os.MkdirTemp("", "siftail_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	manager := &PresetsManager{
		configPath: filepath.Join(tempDir, "presets.json"),
	}

	// Save initial preset
	preset1 := Preset{
		Name: "preset1",
		Visible: map[string]bool{
			"container1": true,
		},
	}

	if err := manager.SavePreset(preset1); err != nil {
		t.Fatalf("Failed to save preset1: %v", err)
	}

	// Save second preset
	preset2 := Preset{
		Name: "preset2",
		Visible: map[string]bool{
			"container2": false,
		},
	}

	if err := manager.SavePreset(preset2); err != nil {
		t.Fatalf("Failed to save preset2: %v", err)
	}

	// Replace first preset
	preset1Updated := Preset{
		Name: "preset1",
		Visible: map[string]bool{
			"container1": false,
			"container3": true,
		},
	}

	if err := manager.SavePreset(preset1Updated); err != nil {
		t.Fatalf("Failed to save updated preset1: %v", err)
	}

	// Verify final state
	presets, err := manager.LoadPresets()
	if err != nil {
		t.Fatalf("Failed to load presets: %v", err)
	}

	if len(presets) != 2 {
		t.Fatalf("Expected 2 presets, got %d", len(presets))
	}

	// Find updated preset1
	var foundPreset1 *Preset
	for _, p := range presets {
		if p.Name == "preset1" {
			foundPreset1 = &p
			break
		}
	}

	if foundPreset1 == nil {
		t.Fatalf("Updated preset1 not found")
	}

	if len(foundPreset1.Visible) != 2 {
		t.Errorf("Expected preset1 to have 2 containers, got %d", len(foundPreset1.Visible))
	}

	if foundPreset1.Visible["container1"] != false {
		t.Errorf("Expected container1 to be false, got %v", foundPreset1.Visible["container1"])
	}

	if foundPreset1.Visible["container3"] != true {
		t.Errorf("Expected container3 to be true, got %v", foundPreset1.Visible["container3"])
	}
}

func TestPresets_DeletePreset(t *testing.T) {
	// Use temporary directory for testing
	tempDir, err := os.MkdirTemp("", "siftail_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	manager := &PresetsManager{
		configPath: filepath.Join(tempDir, "presets.json"),
	}

	// Save some presets
	presets := []Preset{
		{Name: "preset1", Visible: map[string]bool{"c1": true}},
		{Name: "preset2", Visible: map[string]bool{"c2": false}},
		{Name: "preset3", Visible: map[string]bool{"c3": true}},
	}

	if err := manager.SavePresets(presets); err != nil {
		t.Fatalf("Failed to save initial presets: %v", err)
	}

	// Delete middle preset
	if err := manager.DeletePreset("preset2"); err != nil {
		t.Fatalf("Failed to delete preset2: %v", err)
	}

	// Verify result
	remaining, err := manager.LoadPresets()
	if err != nil {
		t.Fatalf("Failed to load presets after deletion: %v", err)
	}

	if len(remaining) != 2 {
		t.Fatalf("Expected 2 presets after deletion, got %d", len(remaining))
	}

	names := make([]string, len(remaining))
	for i, p := range remaining {
		names[i] = p.Name
	}

	expectedNames := []string{"preset1", "preset3"}
	for _, expected := range expectedNames {
		found := false
		for _, name := range names {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected preset %s to remain, but it was not found", expected)
		}
	}
}

func TestPresets_GetPreset(t *testing.T) {
	// Use temporary directory for testing
	tempDir, err := os.MkdirTemp("", "siftail_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	manager := &PresetsManager{
		configPath: filepath.Join(tempDir, "presets.json"),
	}

	// Save a preset
	originalPreset := Preset{
		Name: "test-preset",
		Visible: map[string]bool{
			"container1": true,
			"container2": false,
		},
	}

	if err := manager.SavePreset(originalPreset); err != nil {
		t.Fatalf("Failed to save preset: %v", err)
	}

	// Get existing preset
	retrieved, err := manager.GetPreset("test-preset")
	if err != nil {
		t.Fatalf("Failed to get preset: %v", err)
	}

	if retrieved == nil {
		t.Fatalf("Expected to retrieve preset, got nil")
	}

	if retrieved.Name != originalPreset.Name {
		t.Errorf("Name mismatch: expected %s, got %s", originalPreset.Name, retrieved.Name)
	}

	// Get non-existing preset
	notFound, err := manager.GetPreset("non-existing")
	if err != nil {
		t.Fatalf("Failed to get non-existing preset: %v", err)
	}

	if notFound != nil {
		t.Errorf("Expected nil for non-existing preset, got %v", notFound)
	}
}

func TestCreatePresetFromCurrent(t *testing.T) {
	currentContainers := map[string]bool{
		"web":   true,
		"db":    false,
		"cache": true,
	}

	preset := CreatePresetFromCurrent("my-preset", currentContainers)

	if preset.Name != "my-preset" {
		t.Errorf("Expected name 'my-preset', got %s", preset.Name)
	}

	if len(preset.Visible) != len(currentContainers) {
		t.Errorf("Expected %d containers in preset, got %d", len(currentContainers), len(preset.Visible))
	}

	for name, visible := range currentContainers {
		if preset.Visible[name] != visible {
			t.Errorf("Container %s: expected %v, got %v", name, visible, preset.Visible[name])
		}
	}
}

func TestPresets_LoadEmptyFile(t *testing.T) {
	// Use temporary directory for testing
	tempDir, err := os.MkdirTemp("", "siftail_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	manager := &PresetsManager{
		configPath: filepath.Join(tempDir, "nonexistent.json"),
	}

	// Load from non-existent file should return empty slice
	presets, err := manager.LoadPresets()
	if err != nil {
		t.Fatalf("Failed to load from non-existent file: %v", err)
	}

	if len(presets) != 0 {
		t.Errorf("Expected empty slice from non-existent file, got %d presets", len(presets))
	}
}

func TestGetConfigPath(t *testing.T) {
	path, err := getConfigPath()
	if err != nil {
		t.Fatalf("Failed to get config path: %v", err)
	}

	if path == "" {
		t.Errorf("Config path should not be empty")
	}

	// Check that path contains expected components
	if runtime.GOOS == "windows" {
		if !filepath.IsAbs(path) {
			t.Errorf("Config path should be absolute: %s", path)
		}
		if !contains(path, "siftail") {
			t.Errorf("Config path should contain 'siftail': %s", path)
		}
	} else {
		// Linux/macOS
		if !filepath.IsAbs(path) {
			t.Errorf("Config path should be absolute: %s", path)
		}
		if !contains(path, "siftail") {
			t.Errorf("Config path should contain 'siftail': %s", path)
		}
		if !contains(path, "presets.json") {
			t.Errorf("Config path should contain 'presets.json': %s", path)
		}
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}