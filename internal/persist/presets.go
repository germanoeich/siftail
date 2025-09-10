package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// Preset represents a named configuration of container visibility settings
type Preset struct {
	Name    string          `json:"name"`
	Visible map[string]bool `json:"visible"` // container name -> visible
}

// PresetsFile represents the structure of the presets configuration file
type PresetsFile struct {
	Presets []Preset `json:"presets"`
}

// PresetsManager handles persistence of Docker container visibility presets
type PresetsManager struct {
	configPath string
}

// NewPresetsManager creates a new presets manager with the appropriate config path
func NewPresetsManager() (*PresetsManager, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	return &PresetsManager{
		configPath: configPath,
	}, nil
}

// getConfigPath returns the platform-specific config directory path for siftail
func getConfigPath() (string, error) {
	var configDir string

	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", os.ErrNotExist
		}
		configDir = filepath.Join(appData, "siftail")
	default:
		// Linux/macOS - use XDG config directory
		xdgConfig := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfig == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			xdgConfig = filepath.Join(homeDir, ".config")
		}
		configDir = filepath.Join(xdgConfig, "siftail")
	}

	// Ensure the config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}

	return filepath.Join(configDir, "presets.json"), nil
}

// LoadPresets loads all presets from disk
func (p *PresetsManager) LoadPresets() ([]Preset, error) {
	if _, err := os.Stat(p.configPath); os.IsNotExist(err) {
		// File doesn't exist, return empty slice
		return []Preset{}, nil
	}

	data, err := os.ReadFile(p.configPath)
	if err != nil {
		return nil, err
	}

	var presetsFile PresetsFile
	if err := json.Unmarshal(data, &presetsFile); err != nil {
		return nil, err
	}

	return presetsFile.Presets, nil
}

// SavePresets saves all presets to disk
func (p *PresetsManager) SavePresets(presets []Preset) error {
	presetsFile := PresetsFile{
		Presets: presets,
	}

	data, err := json.MarshalIndent(presetsFile, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(p.configPath, data, 0644)
}

// SavePreset saves a single preset, replacing any existing preset with the same name
func (p *PresetsManager) SavePreset(preset Preset) error {
	presets, err := p.LoadPresets()
	if err != nil {
		return err
	}

	// Remove existing preset with same name
	filtered := make([]Preset, 0, len(presets))
	for _, existing := range presets {
		if existing.Name != preset.Name {
			filtered = append(filtered, existing)
		}
	}

	// Add the new/updated preset
	filtered = append(filtered, preset)

	return p.SavePresets(filtered)
}

// DeletePreset removes a preset by name
func (p *PresetsManager) DeletePreset(name string) error {
	presets, err := p.LoadPresets()
	if err != nil {
		return err
	}

	// Filter out the preset to delete
	filtered := make([]Preset, 0, len(presets))
	for _, preset := range presets {
		if preset.Name != name {
			filtered = append(filtered, preset)
		}
	}

	return p.SavePresets(filtered)
}

// GetPreset retrieves a preset by name
func (p *PresetsManager) GetPreset(name string) (*Preset, error) {
	presets, err := p.LoadPresets()
	if err != nil {
		return nil, err
	}

	for _, preset := range presets {
		if preset.Name == name {
			return &preset, nil
		}
	}

	return nil, nil // Not found
}

// ApplyPreset applies a preset to the current container visibility settings
// It maps by container name first, then falls back to ID if name lookup fails
func ApplyPreset(preset Preset, currentContainers map[string]bool) map[string]bool {
	result := make(map[string]bool)

	// Start with current settings
	for name, visible := range currentContainers {
		result[name] = visible
	}

	// Apply preset settings (by container name)
	for containerName, visible := range preset.Visible {
		if _, exists := result[containerName]; exists {
			result[containerName] = visible
		}
		// Note: We ignore containers in the preset that don't exist in current containers
		// This allows presets to work across different environments where not all containers exist
	}

	return result
}

// CreatePresetFromCurrent creates a new preset from the current container visibility state
func CreatePresetFromCurrent(name string, currentContainers map[string]bool) Preset {
	visible := make(map[string]bool)

	// Copy current visibility settings
	for containerName, isVisible := range currentContainers {
		visible[containerName] = isVisible
	}

	return Preset{
		Name:    name,
		Visible: visible,
	}
}
