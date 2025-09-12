package persist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	tmp, err := os.MkdirTemp("", "siftail-settings-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	// Force XDG/APPDATA to temp so we don't touch user config
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	oldAPP := os.Getenv("APPDATA")
	_ = os.Setenv("XDG_CONFIG_HOME", tmp)
	_ = os.Setenv("APPDATA", filepath.Join(tmp, "AppData"))
	defer func() { _ = os.Setenv("XDG_CONFIG_HOME", oldXDG); _ = os.Setenv("APPDATA", oldAPP) }()

	sm, err := NewSettingsManager()
	if err != nil {
		t.Fatalf("NewSettingsManager: %v", err)
	}

	// Defaults when file missing
	s, err := sm.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !s.ShowTimestamps || s.Theme == "" {
		t.Fatalf("unexpected defaults: %+v", s)
	}

	want := Settings{ShowTimestamps: false, Theme: "nord"}
	if err := sm.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := sm.Load()
	if err != nil {
		t.Fatalf("Load(2): %v", err)
	}
	if got != want {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
	}
}
