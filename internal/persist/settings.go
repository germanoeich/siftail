package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// Settings represents user-adjustable UI preferences.
type Settings struct {
	ShowTimestamps bool   `json:"showTimestamps"`
	Theme          string `json:"theme"`
}

// SettingsManager handles persistence of settings.
type SettingsManager struct {
	path string
}

// NewSettingsManager returns a manager bound to the platform config path.
func NewSettingsManager() (*SettingsManager, error) {
	p, err := getSettingsPath()
	if err != nil {
		return nil, err
	}
	return &SettingsManager{path: p}, nil
}

// getSettingsPath returns siftail's settings.json path under XDG/AppData.
func getSettingsPath() (string, error) {
	var configDir string

	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", os.ErrNotExist
		}
		configDir = filepath.Join(appData, "siftail")
	default:
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			xdg = filepath.Join(home, ".config")
		}
		configDir = filepath.Join(xdg, "siftail")
	}

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.json"), nil
}

// Load reads settings from disk, returning defaults if file does not exist.
func (m *SettingsManager) Load() (Settings, error) {
	// Defaults
	s := Settings{ShowTimestamps: true, Theme: "dark"}

	if _, err := os.Stat(m.path); os.IsNotExist(err) {
		return s, nil
	}
	data, err := os.ReadFile(m.path)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	// Basic sanity
	if s.Theme == "" {
		s.Theme = "dark"
	}
	return s, nil
}

// Save writes settings to disk.
func (m *SettingsManager) Save(s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0o644)
}
