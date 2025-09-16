package tui

import (
	"strings"
	"testing"
)

func TestCopySelectionCmdReturnsNilForEmptyText(t *testing.T) {
	if cmd := copySelectionCmd(" \n\t"); cmd != nil {
		t.Fatalf("expected nil command for whitespace selection")
	}
}

func TestClipboardUnsupportedHintVariants(t *testing.T) {
	tests := []struct {
		name string
		env  clipboardEnv
		want string
	}{
		{
			name: "gnome",
			env:  clipboardEnv{gnomeTerminal: true, wayland: true, os: "linux"},
			want: "GNOME Terminal",
		},
		{
			name: "wayland",
			env:  clipboardEnv{wayland: true, os: "linux"},
			want: "wl-clipboard",
		},
		{
			name: "linux",
			env:  clipboardEnv{os: "linux"},
			want: "xclip",
		},
		{
			name: "other",
			env:  clipboardEnv{os: "darwin"},
			want: "terminal prevented",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := clipboardUnsupportedHint(tt.env)
			if !strings.Contains(msg, tt.want) {
				t.Fatalf("expected hint to mention %q, got %q", tt.want, msg)
			}
		})
	}
}
