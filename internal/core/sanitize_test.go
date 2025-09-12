package core

import "testing"

func TestSanitizeLine_RemovesCSIAndKeepsText(t *testing.T) {
	in := "start\x1b[2K\x1b[1A\x1b[31mred\x1b[0m end"
	out := SanitizeLine(in)
	if out != "startred end" { // SGRs and cursor/erase removed
		t.Fatalf("unexpected sanitize: %q", out)
	}
}

func TestSanitizeLine_InlineCRBecomesSpace(t *testing.T) {
	in := "foo\rbar"
	out := SanitizeLine(in)
	if out != "foo bar" {
		t.Fatalf("expected inline CR to become space, got %q", out)
	}
}

func TestSanitizeLine_TrailingCRIsPreserved(t *testing.T) {
	in := "line with CR\r"
	out := SanitizeLine(in)
	if out != "line with CR\r" {
		t.Fatalf("expected trailing CR preserved, got %q", out)
	}
}

func TestSanitizeLine_OSCAndDCSRemoved(t *testing.T) {
	// OSC: ESC ] ... BEL
	osc := "\x1b]0;title\x07visible"
	// DCS-like: ESC P ... ST
	dcs := "\x1bPqSECRET\x1b\\ok"
	out := SanitizeLine(osc + dcs)
	if out != "visibleok" {
		t.Fatalf("expected OSC/DCS stripped, got %q", out)
	}
}
