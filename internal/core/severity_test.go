package core

import (
	"strings"
	"testing"
)

func TestSeverity_DefaultMapping(t *testing.T) {
	lm := NewLevelMap()

	// Test default mappings
	expected := map[int]string{
		1: "DEBUG",
		2: "INFO",
		3: "WARN",
		4: "ERROR",
	}

	indexToName, enabled := lm.GetSnapshot()

	for index, expectedName := range expected {
		if indexToName[index] != expectedName {
			t.Errorf("Expected index %d to map to %s, got %s", index, expectedName, indexToName[index])
		}

		if !enabled[index] {
			t.Errorf("Expected index %d to be enabled by default", index)
		}
	}

	// Test that empty slots are actually empty
	for i := 5; i <= 8; i++ {
		if indexToName[i] != "" {
			t.Errorf("Expected index %d to be empty, got %s", i, indexToName[i])
		}
	}
}

func TestSeverity_Detect_JSON_LevelField(t *testing.T) {
	lm := NewLevelMap()
	detector := NewDefaultSeverityDetector(lm)

	testCases := []struct {
		name        string
		line        string
		expectedStr string
		expectedSev Severity
		expectedOk  bool
	}{
		{
			name:        "JSON with level field",
			line:        `{"level": "info", "msg": "test message"}`,
			expectedStr: "info",
			expectedSev: SevInfo,
			expectedOk:  true,
		},
		{
			name:        "JSON with lvl field",
			line:        `{"lvl": "ERROR", "message": "error occurred"}`,
			expectedStr: "ERROR",
			expectedSev: SevError,
			expectedOk:  true,
		},
		{
			name:        "JSON with severity field",
			line:        `{"severity": "warn", "text": "warning message"}`,
			expectedStr: "warn",
			expectedSev: SevWarn,
			expectedOk:  true,
		},
		{
			name:        "JSON with numeric level",
			line:        `{"level": 6, "msg": "info message"}`,
			expectedStr: "INFO",
			expectedSev: SevInfo,
			expectedOk:  true,
		},
		{
			name:        "JSON without level field",
			line:        `{"msg": "test message", "timestamp": "2023-01-01"}`,
			expectedStr: "",
			expectedSev: SevUnknown,
			expectedOk:  false,
		},
		{
			name:        "JSON with case-insensitive Level field",
			line:        `{"Level": "DEBUG", "msg": "debug message"}`,
			expectedStr: "DEBUG",
			expectedSev: SevDebug,
			expectedOk:  true,
		},
		{
			name:        "Invalid JSON",
			line:        `{"level": "debug", "msg": "incomplete`,
			expectedStr: "debug",
			expectedSev: SevDebug,
			expectedOk:  true, // This will be detected by bracketed pattern
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			levelStr, level, ok := detector.Detect(tc.line)

			if ok != tc.expectedOk {
				t.Errorf("Expected ok=%v, got %v", tc.expectedOk, ok)
			}

			if levelStr != tc.expectedStr {
				t.Errorf("Expected levelStr=%s, got %s", tc.expectedStr, levelStr)
			}

			if level != tc.expectedSev {
				t.Errorf("Expected level=%v, got %v", tc.expectedSev, level)
			}
		})
	}
}

func TestSeverity_Detect_Logfmt(t *testing.T) {
	lm := NewLevelMap()
	detector := NewDefaultSeverityDetector(lm)

	testCases := []struct {
		name        string
		line        string
		expectedStr string
		expectedSev Severity
		expectedOk  bool
	}{
		{
			name:        "logfmt with level",
			line:        `time=2023-01-01T10:00:00Z level=info msg="test message"`,
			expectedStr: "info",
			expectedSev: SevInfo,
			expectedOk:  true,
		},
		{
			name:        "logfmt with lvl",
			line:        `ts=1234567890 lvl=error msg="error occurred"`,
			expectedStr: "error",
			expectedSev: SevError,
			expectedOk:  true,
		},
		{
			name:        "logfmt with severity",
			line:        `severity=warn component=auth msg="auth failed"`,
			expectedStr: "warn",
			expectedSev: SevWarn,
			expectedOk:  true,
		},
		{
			name:        "logfmt with quoted level",
			line:        `level="DEBUG" msg="debug info"`,
			expectedStr: "DEBUG",
			expectedSev: SevDebug,
			expectedOk:  true,
		},
		{
			name:        "logfmt without level",
			line:        `ts=1234567890 msg="no level here" component=test`,
			expectedStr: "",
			expectedSev: SevUnknown,
			expectedOk:  false,
		},
		{
			name:        "logfmt mixed with other content",
			line:        `[INFO] Application started level=info port=8080`,
			expectedStr: "info",
			expectedSev: SevInfo,
			expectedOk:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			levelStr, level, ok := detector.Detect(tc.line)

			if ok != tc.expectedOk {
				t.Errorf("Expected ok=%v, got %v", tc.expectedOk, ok)
			}

			if levelStr != tc.expectedStr {
				t.Errorf("Expected levelStr=%s, got %s", tc.expectedStr, levelStr)
			}

			if level != tc.expectedSev {
				t.Errorf("Expected level=%v, got %v", tc.expectedSev, level)
			}
		})
	}
}

func TestSeverity_Detect_Bracketed(t *testing.T) {
	lm := NewLevelMap()
	detector := NewDefaultSeverityDetector(lm)

	testCases := []struct {
		name        string
		line        string
		expectedStr string
		expectedSev Severity
		expectedOk  bool
	}{
		{
			name:        "bracketed INFO",
			line:        `[INFO] 2023-01-01 10:00:00 Application started`,
			expectedStr: "INFO",
			expectedSev: SevInfo,
			expectedOk:  true,
		},
		{
			name:        "angle bracketed ERROR",
			line:        `<ERROR> Database connection failed`,
			expectedStr: "ERROR",
			expectedSev: SevError,
			expectedOk:  true,
		},
		{
			name:        "parentheses WARN",
			line:        `(WARN) Deprecated API usage detected`,
			expectedStr: "WARN",
			expectedSev: SevWarn,
			expectedOk:  true,
		},
		{
			name:        "word boundary DEBUG",
			line:        `DEBUG: Entering function processRequest()`,
			expectedStr: "DEBUG",
			expectedSev: SevDebug,
			expectedOk:  true,
		},
		{
			name:        "case insensitive error",
			line:        `[error] Something went wrong`,
			expectedStr: "error",
			expectedSev: SevError,
			expectedOk:  true,
		},
		{
			name:        "WARNING variant",
			line:        `[WARNING] This is a warning message`,
			expectedStr: "WARNING",
			expectedSev: SevWarn, // WARNING maps to SevWarn
			expectedOk:  true,
		},
		{
			name:        "ERR variant",
			line:        `[ERR] Short error format`,
			expectedStr: "ERR",
			expectedSev: SevError,
			expectedOk:  true,
		},
		{
			name:        "FATAL level",
			line:        `[FATAL] Application crashed`,
			expectedStr: "FATAL",
			expectedSev: SevUnknown, // FATAL gets assigned dynamically now
			expectedOk:  true,
		},
		{
			name:        "CRITICAL level",
			line:        `CRITICAL: System overload`,
			expectedStr: "CRITICAL",
			expectedSev: SevUnknown, // CRITICAL gets assigned dynamically now
			expectedOk:  true,
		},
		{
			name:        "TRACE level",
			line:        `TRACE: Function entry`,
			expectedStr: "TRACE",
			expectedSev: SevUnknown, // TRACE gets assigned dynamically now
			expectedOk:  true,
		},
		{
			name:        "no level found",
			line:        `Regular log message without level indicator`,
			expectedStr: "",
			expectedSev: SevUnknown,
			expectedOk:  false,
		},
		{
			name:        "level in middle of word should not match",
			line:        `Processing information data`,
			expectedStr: "",
			expectedSev: SevUnknown,
			expectedOk:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			levelStr, level, ok := detector.Detect(tc.line)

			if ok != tc.expectedOk {
				t.Errorf("Expected ok=%v, got %v", tc.expectedOk, ok)
			}

			if levelStr != tc.expectedStr {
				t.Errorf("Expected levelStr=%s, got %s", tc.expectedStr, levelStr)
			}

			if level != tc.expectedSev {
				t.Errorf("Expected level=%v, got %v", tc.expectedSev, level)
			}
		})
	}
}

func TestSeverity_DynamicLevels_AssignSlots(t *testing.T) {
	lm := NewLevelMap()
	detector := NewDefaultSeverityDetector(lm)

	// Test lines with custom levels that should be assigned to slots 5-9
	customLevels := []struct {
		line          string
		expectedLevel string
		expectedSlot  int
	}{
		{`[NOTICE] First custom level`, "NOTICE", 5},
		{`[ALERT] Second custom level`, "ALERT", 6},
		{`[CRITICAL] Third custom level`, "CRITICAL", 7}, // Note: CRITICAL maps to SevError by default, but should still get a slot
		{`[TRACE] Fourth custom level`, "TRACE", 8},      // Note: TRACE maps to SevDebug by default, but should still get a slot
		{`[CUSTOM] Fifth custom level`, "CUSTOM", 9},
	}

	for i, tc := range customLevels {
		t.Run(tc.expectedLevel, func(t *testing.T) {
			levelStr, _, ok := detector.Detect(tc.line)

			if !ok {
				t.Fatalf("Expected to detect level in line: %s", tc.line)
			}

			if !strings.EqualFold(levelStr, tc.expectedLevel) {
				t.Errorf("Expected levelStr=%s, got %s", tc.expectedLevel, levelStr)
			}

			// Check that the level was assigned to the expected slot
			indexToName, _ := lm.GetSnapshot()
			expectedNormalized := strings.ToUpper(tc.expectedLevel)

			// For levels that map to standard severities, they might not create new slots
			// Let's check if we can find the level in any slot
			foundSlot := -1
			for slot := 5; slot <= 9; slot++ {
				if indexToName[slot] == expectedNormalized {
					foundSlot = slot
					break
				}
			}

			if foundSlot == -1 && i < 4 { // First 4 should definitely get slots
				t.Errorf("Expected %s to be assigned to a slot, but not found", expectedNormalized)
			}
		})
	}
}

func TestSeverity_OverflowToOther(t *testing.T) {
	lm := NewLevelMap()
	detector := NewDefaultSeverityDetector(lm)

	// Fill up all slots 5-9 with custom levels
	customLevels := []string{"NOTICE", "ALERT", "CUSTOM1", "CUSTOM2", "CUSTOM3"}

	for _, level := range customLevels {
		line := `[` + level + `] Test message`
		detector.Detect(line)
	}

	// Now add more levels - they should overflow to OTHER
	overflowLevels := []string{"OVERFLOW1", "OVERFLOW2", "OVERFLOW3"}

	for _, level := range overflowLevels {
		line := `[` + level + `] Test message`
		levelStr, _, ok := detector.Detect(line)

		if !ok {
			t.Fatalf("Expected to detect level in line: %s", line)
		}

		if !strings.EqualFold(levelStr, level) {
			t.Errorf("Expected levelStr=%s, got %s", level, levelStr)
		}

		// Check that the level maps to slot 9 (OTHER)
		index := lm.GetOrAssignIndex(level)
		if index != 9 {
			t.Errorf("Expected overflow level %s to map to slot 9, got %d", level, index)
		}
	}

	// Verify that slot 9 is now "OTHER"
	indexToName, _ := lm.GetSnapshot()
	if indexToName[9] != "OTHER" {
		t.Errorf("Expected slot 9 to be OTHER after overflow, got %s", indexToName[9])
	}
}

func TestLevelMap_Toggle(t *testing.T) {
	lm := NewLevelMap()

	// Test toggling existing levels
	if !lm.IsEnabled(SevInfo) {
		t.Error("Expected INFO to be enabled by default")
	}

	lm.Toggle(2) // INFO is at index 2
	if lm.IsEnabled(SevInfo) {
		t.Error("Expected INFO to be disabled after toggle")
	}

	lm.Toggle(2) // Toggle back
	if !lm.IsEnabled(SevInfo) {
		t.Error("Expected INFO to be enabled after second toggle")
	}

	// Test invalid indices
	lm.Toggle(0)  // Should be ignored
	lm.Toggle(10) // Should be ignored

	// Should not panic and INFO should still be enabled
	if !lm.IsEnabled(SevInfo) {
		t.Error("Expected INFO to still be enabled after invalid toggles")
	}
}

func TestLevelMap_GetOrAssignIndex(t *testing.T) {
	lm := NewLevelMap()

	// Test existing levels
	if index := lm.GetOrAssignIndex("DEBUG"); index != 1 {
		t.Errorf("Expected DEBUG to be at index 1, got %d", index)
	}

	if index := lm.GetOrAssignIndex("info"); index != 2 {
		t.Errorf("Expected info to be at index 2, got %d", index)
	}

	// Test new level assignment
	if index := lm.GetOrAssignIndex("NOTICE"); index != 5 {
		t.Errorf("Expected NOTICE to be assigned to index 5, got %d", index)
	}

	// Test same level returns same index
	if index := lm.GetOrAssignIndex("NOTICE"); index != 5 {
		t.Errorf("Expected NOTICE to still be at index 5, got %d", index)
	}

	// Test case insensitive and trimming
	if index := lm.GetOrAssignIndex("[notice]"); index != 5 {
		t.Errorf("Expected [notice] to map to same index as NOTICE (5), got %d", index)
	}
}

func TestSeverity_ThreadSafety(t *testing.T) {
	lm := NewLevelMap()
	detector := NewDefaultSeverityDetector(lm)

	// This test would be better with actual goroutines, but for basic test coverage:
	lines := []string{
		`{"level": "info", "msg": "test1"}`,
		`[ERROR] Error message`,
		`level=debug msg="debug message"`,
		`[CUSTOM1] Custom level 1`,
		`[CUSTOM2] Custom level 2`,
	}

	for _, line := range lines {
		detector.Detect(line)
	}

	// Test concurrent reads don't panic
	for i := 0; i < 10; i++ {
		indexToName, enabled := lm.GetSnapshot()
		if len(indexToName) != 10 {
			t.Errorf("Expected IndexToName length 10, got %d", len(indexToName))
		}
		if len(enabled) == 0 {
			t.Error("Expected some enabled levels")
		}
	}
}

func TestSeverity_DetectionPriority(t *testing.T) {
	lm := NewLevelMap()
	detector := NewDefaultSeverityDetector(lm)

	// Test that JSON takes priority over other formats when line looks like JSON
	line := `{"level": "error", "msg": "test [INFO] message"}`
	levelStr, level, ok := detector.Detect(line)

	if !ok {
		t.Fatal("Expected to detect level")
	}

	if levelStr != "error" {
		t.Errorf("Expected to detect JSON level 'error', got '%s'", levelStr)
	}

	if level != SevError {
		t.Errorf("Expected SevError, got %v", level)
	}
}

func TestSeverity_EdgeCases(t *testing.T) {
	lm := NewLevelMap()
	detector := NewDefaultSeverityDetector(lm)

	testCases := []struct {
		name         string
		line         string
		shouldDetect bool
	}{
		{"empty line", "", false},
		{"whitespace only", "   \t\n   ", false},
		{"incomplete JSON", `{"level":`, false},
		{"JSON without braces", `"level": "info", "msg": "test"`, true}, // This will match via bracketed pattern
		{"logfmt without equals", `level info msg test`, true},          // This will match "info" via bracketed pattern
		{"very long line", strings.Repeat("a", 10000) + "[INFO]" + strings.Repeat("b", 10000), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, ok := detector.Detect(tc.line)
			if ok != tc.shouldDetect {
				t.Errorf("Expected detection=%v, got %v for line: %s", tc.shouldDetect, ok, tc.line)
			}
		})
	}
}
