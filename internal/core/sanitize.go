package core

import (
	"regexp"
	"strings"
)

// Sanitizer removes terminal control sequences and problematic control
// characters from incoming log lines so they render deterministically
// inside the TUI. It leaves tabs intact and preserves a trailing \r (CR)
// so tests that assert on CRLF behavior continue to pass.

// Patterns adapted from common ANSI/VT escape taxonomies:
// - CSI: ESC [ ... final byte in @-~
// - OSC: ESC ] ... BEL or ST (ESC \\)
// - DCS/SOS/PM/APC: ESC P/^/_ ... ST
// - Single-escape sequences like ESC 7 / ESC 8, etc.

var (
	// CSI (Control Sequence Introducer) — covers erase line/screen, cursor moves, SGR, etc.
	reCSI = regexp.MustCompile("\x1b\x5b[0-?]*[ -/]*[@-~]")

	// OSC (Operating System Command): ESC ] ... (BEL | ST)
	reOSC = regexp.MustCompile("\x1b\x5d[\x20-\x7e]*(?:\x07|\x1b\\\\)")

	// DCS, SOS, PM, APC — ESC P / ESC X / ESC ^ / ESC _ ... ST
	// Use DOTALL via a non-capturing group to match any content lazily until ST or BEL.
	reDCSLike = regexp.MustCompile("\x1b[P^_X](?s:.*?)(?:\x1b\\\\|\x07)")

	// Single ESC sequences that occasionally appear (e.g., ESC 7/8 save/restore)
	reSingleESC = regexp.MustCompile("\x1b[0-9A-Za-z]")
)

// SanitizeLine removes terminal control sequences and replaces in-line CR/BS
// that would otherwise mutate previously rendered content. It preserves a
// trailing CR (to keep Windows CRLF tests stable) but removes other control
// characters. This function is idempotent.
func SanitizeLine(s string) string {
	if s == "" {
		return s
	}

	// Remove OSC/DCS-like blocks first (they can contain CSI-like bytes inside).
	s = reOSC.ReplaceAllString(s, "")
	s = reDCSLike.ReplaceAllString(s, "")

	// Remove all CSI sequences (including SGR). If in future we want to allow
	// SGR colors from input, we can special-case final 'm'. For stability we
	// strip everything now.
	s = reCSI.ReplaceAllString(s, "")

	// Remove simple ESC+char sequences
	s = reSingleESC.ReplaceAllString(s, "")

	// Normalize CR: preserve a single trailing CR (Windows CRLF artifact in tests),
	// but convert any other CR occurrences to spaces so they don't act like
	// carriage returns in the terminal.
	if strings.Contains(s, "\r") {
		// If trailing CR exists, remember it and temporarily drop it for processing
		keepTrailingCR := strings.HasSuffix(s, "\r")
		if keepTrailingCR {
			s = s[:len(s)-1]
		}
		// Replace in-line CR with space
		s = strings.ReplaceAll(s, "\r", " ")
		if keepTrailingCR {
			s += "\r"
		}
	}

	// Remove backspaces entirely; most terminals would combine them with
	// overstrikes which is not desirable in a log viewer.
	s = strings.ReplaceAll(s, "\b", "")

	// Remove other C0 control chars except TAB (\t) — keep text readable.
	// This targets: NUL..US (0x00..0x1F) excluding TAB (0x09) and CR already handled.
	// Replace with a single space to avoid accidental joins.
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < 0x20 { // C0
			if ch == '\t' || ch == '\n' || ch == '\r' { // keep tab and already-handled newline/cr
				b.WriteByte(ch)
			} else {
				// replace with space
				b.WriteByte(' ')
			}
		} else {
			b.WriteByte(ch)
		}
	}
	s = b.String()
	return s
}
