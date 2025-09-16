package tui

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
)

// clipboardResultMsg communicates the outcome of an attempted copy.
type clipboardResultMsg struct {
	message string
}

// copySelectionCmd copies text to both OSC52 and the system clipboard (if available).

func copySelectionCmd(text string) tea.Cmd {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	return func() tea.Msg {
		termenv.Copy(text)

		if clipboard.Unsupported {
			return clipboardResultMsg{message: clipboardUnsupportedHint(detectClipboardEnv())}
		}

		if err := clipboard.WriteAll(text); err != nil {
			return clipboardResultMsg{message: fmt.Sprintf("Copy failed: %v", err)}
		}

		return clipboardResultMsg{message: "Copied selection to clipboard"}
	}
}

// clipboardEnv captures environment details that influence clipboard hints.
type clipboardEnv struct {
	wayland       bool
	gnomeTerminal bool
	os            string
}

func detectClipboardEnv() clipboardEnv {
	return clipboardEnv{
		wayland:       os.Getenv("WAYLAND_DISPLAY") != "",
		gnomeTerminal: os.Getenv("GNOME_TERMINAL_SCREEN") != "",
		os:            runtime.GOOS,
	}
}

func clipboardUnsupportedHint(env clipboardEnv) string {
	if env.os == "linux" && env.gnomeTerminal {
		return "Clipboard blocked: GNOME Terminal needs wl-clipboard or OSC52 clipboard support."
	}
	if env.os == "linux" && env.wayland {
		return "Clipboard blocked: install wl-clipboard or enable OSC52 clipboard support in your terminal."
	}
	if env.os == "linux" {
		return "Clipboard blocked: install xclip or xsel, or enable OSC52 clipboard support in your terminal."
	}
	return "Clipboard blocked: your terminal prevented the copy operation."
}
