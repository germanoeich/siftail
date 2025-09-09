package core

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"time"
)

// SourceKind represents the input source type
type SourceKind int

const (
	SourceStdin SourceKind = iota
	SourceFile
	SourceDocker
)

// Severity represents the severity level of a log entry
type Severity uint8

const (
	SevUnknown Severity = iota
	SevDebug
	SevInfo
	SevWarn
	SevError
	// Additional levels learned dynamically map to 5..9
)

// LogEvent represents a single log entry
type LogEvent struct {
	Seq       uint64
	Time      time.Time
	Source    SourceKind
	Container string // docker only; empty otherwise
	Line      string // raw
	LevelStr  string // original parsed token, e.g. "warn", "TRACE"
	Level     Severity
}

// LevelMap manages the dynamic mapping between level names and numeric indices 1-9
type LevelMap struct {
	mu          sync.RWMutex
	IndexToName []string       // positions 1..9 (0 unused)
	NameToIndex map[string]int // uppercased -> 1..9
	Enabled     map[int]bool   // current visibility by index (default true)
}

// NewLevelMap creates a new LevelMap with default mappings
func NewLevelMap() *LevelMap {
	lm := &LevelMap{
		IndexToName: make([]string, 10), // 0-9, but we only use 1-9
		NameToIndex: make(map[string]int),
		Enabled:     make(map[int]bool),
	}

	// Set up default mappings: 1=DEBUG, 2=INFO, 3=WARN, 4=ERROR
	defaults := []string{"", "DEBUG", "INFO", "WARN", "ERROR", "", "", "", "", ""}
	copy(lm.IndexToName, defaults)

	for i := 1; i <= 9; i++ {
		if lm.IndexToName[i] != "" {
			lm.NameToIndex[lm.IndexToName[i]] = i
		}
		lm.Enabled[i] = true // default all enabled
	}

	return lm
}

// GetOrAssignIndex returns the index for a level name, assigning a new slot if needed
func (lm *LevelMap) GetOrAssignIndex(levelStr string) int {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	normalized := strings.ToUpper(strings.Trim(levelStr, "[]<>: "))

	// Check if we already have this level
	if index, exists := lm.NameToIndex[normalized]; exists {
		return index
	}

	// Find next available slot (5-9)
	for i := 5; i <= 9; i++ {
		if lm.IndexToName[i] == "" {
			lm.IndexToName[i] = normalized
			lm.NameToIndex[normalized] = i
			lm.Enabled[i] = true // default enabled
			return i
		}
	}

	// All slots full, assign to OTHER (slot 9)
	if lm.IndexToName[9] != "OTHER" {
		// Need to move any existing level 9 to OTHER bucket
		if oldName := lm.IndexToName[9]; oldName != "" && oldName != "OTHER" {
			delete(lm.NameToIndex, oldName)
		}
		lm.IndexToName[9] = "OTHER"
		lm.NameToIndex["OTHER"] = 9
		lm.Enabled[9] = true
	}
	lm.NameToIndex[normalized] = 9

	return 9
}

// IsEnabled returns whether a severity level is currently enabled for display
func (lm *LevelMap) IsEnabled(level Severity) bool {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	index := lm.severityToIndex(level)
	return lm.Enabled[index]
}

// Toggle enables/disables a severity level by index (1-9)
func (lm *LevelMap) Toggle(index int) {
	if index < 1 || index > 9 {
		return
	}

	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.Enabled[index] = !lm.Enabled[index]
}

// GetSnapshot returns a read-only snapshot of the current state
func (lm *LevelMap) GetSnapshot() (indexToName []string, enabled map[int]bool) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	indexToName = make([]string, len(lm.IndexToName))
	copy(indexToName, lm.IndexToName)

	enabled = make(map[int]bool)
	for k, v := range lm.Enabled {
		enabled[k] = v
	}

	return
}

// severityToIndex maps the severity enum to an index (1-4 for defaults)
func (lm *LevelMap) severityToIndex(level Severity) int {
	switch level {
	case SevDebug:
		return 1
	case SevInfo:
		return 2
	case SevWarn:
		return 3
	case SevError:
		return 4
	default:
		return 9 // Unknown maps to OTHER
	}
}

// SeverityDetector defines the interface for detecting log levels
type SeverityDetector interface {
	Detect(line string) (levelStr string, level Severity, ok bool)
}

// DefaultSeverityDetector implements the standard severity detection logic
type DefaultSeverityDetector struct {
	levelMap      *LevelMap
	bracketedRe   *regexp.Regexp
	customBracketRe *regexp.Regexp
}

// NewDefaultSeverityDetector creates a new detector with the given level map
func NewDefaultSeverityDetector(levelMap *LevelMap) *DefaultSeverityDetector {
	// Regex for known level patterns (case-insensitive)
	bracketedRe := regexp.MustCompile(`(?i)(?:[\[\(<]|\b)(DEBUG|TRACE|INFO|NOTICE|WARN|WARNING|ERROR|ERR|FATAL|CRITICAL|ALERT|EMERG|EMERGENCY)(?:[\]\)>]|\b|:)`)
	
	// Regex for custom levels in brackets - any uppercase word in brackets
	customBracketRe := regexp.MustCompile(`\[([A-Z][A-Z0-9]*)\]`)

	return &DefaultSeverityDetector{
		levelMap:        levelMap,
		bracketedRe:     bracketedRe,
		customBracketRe: customBracketRe,
	}
}

// Detect attempts to extract the severity level from a log line
func (d *DefaultSeverityDetector) Detect(line string) (levelStr string, level Severity, ok bool) {
	trimmed := strings.TrimSpace(line)
	
	// Try JSON first (fast check)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		if levelStr, level, ok := d.detectJSON(trimmed); ok {
			return levelStr, level, true
		}
	}

	// Try logfmt
	if levelStr, level, ok := d.detectLogfmt(line); ok {
		return levelStr, level, true
	}

	// Try bracketed/common patterns
	if levelStr, level, ok := d.detectBracketed(line); ok {
		return levelStr, level, true
	}

	return "", SevUnknown, false
}

// detectJSON tries to parse the line as JSON and extract level
func (d *DefaultSeverityDetector) detectJSON(line string) (string, Severity, bool) {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		return "", SevUnknown, false
	}

	// Check various level field names (case-insensitive by converting keys)
	levelKeys := []string{"level", "lvl", "severity", "sev", "log.level", "priority"}
	
	for _, key := range levelKeys {
		// Check exact match first
		if val, exists := obj[key]; exists {
			if levelStr := d.extractStringValue(val); levelStr != "" {
				return levelStr, d.stringToSeverity(levelStr), true
			}
		}
		
		// Check case-insensitive
		for objKey, val := range obj {
			if strings.EqualFold(objKey, key) {
				if levelStr := d.extractStringValue(val); levelStr != "" {
					return levelStr, d.stringToSeverity(levelStr), true
				}
			}
		}
	}

	return "", SevUnknown, false
}

// detectLogfmt tries to parse key=value pairs and extract level
func (d *DefaultSeverityDetector) detectLogfmt(line string) (string, Severity, bool) {
	// Simple logfmt parsing - split by spaces and look for key=value
	parts := strings.Fields(line)
	
	for _, part := range parts {
		if strings.Contains(part, "=") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				key := strings.ToLower(strings.TrimSpace(kv[0]))
				value := strings.TrimSpace(kv[1])
				
				// Skip if value is empty after trimming
				if value == "" {
					continue
				}
				
				// Remove quotes if present
				value = strings.Trim(value, `"'`)
				
				// Skip if value is empty after removing quotes
				if value == "" {
					continue
				}
				
				// Check if this is a level key
				levelKeys := []string{"level", "lvl", "severity", "sev", "priority"}
				for _, levelKey := range levelKeys {
					if key == levelKey {
						return value, d.stringToSeverity(value), true
					}
				}
			}
		}
	}
	
	return "", SevUnknown, false
}

// detectBracketed uses regex to find bracketed or word-boundary level indicators
func (d *DefaultSeverityDetector) detectBracketed(line string) (string, Severity, bool) {
	// Try known patterns first
	matches := d.bracketedRe.FindStringSubmatch(line)
	if len(matches) > 1 {
		// Find first non-empty match
		for i := 1; i < len(matches); i++ {
			if matches[i] != "" {
				levelStr := matches[i]
				return levelStr, d.stringToSeverity(levelStr), true
			}
		}
	}
	
	// Try custom bracket patterns
	matches = d.customBracketRe.FindStringSubmatch(line)
	if len(matches) > 1 && matches[1] != "" {
		levelStr := matches[1]
		return levelStr, d.stringToSeverity(levelStr), true
	}
	
	return "", SevUnknown, false
}

// extractStringValue converts interface{} to string
func (d *DefaultSeverityDetector) extractStringValue(val interface{}) string {
	switch v := val.(type) {
	case string:
		return v
	case float64:
		// Handle numeric levels (syslog-style)
		switch int(v) {
		case 0, 7: // DEBUG
			return "DEBUG"
		case 1, 6: // INFO
			return "INFO"
		case 2, 3, 4: // WARN
			return "WARN"
		case 5: // ERROR
			return "ERROR"
		default:
			return "OTHER"
		}
	default:
		return ""
	}
}

// stringToSeverity converts a level string to a Severity enum
func (d *DefaultSeverityDetector) stringToSeverity(levelStr string) Severity {
	normalized := strings.ToUpper(strings.Trim(levelStr, "[]<>: "))
	
	// Map to default severities only for exact matches of the 4 main levels
	switch normalized {
	case "DEBUG":
		return SevDebug
	case "INFO":
		return SevInfo
	case "WARN":
		return SevWarn
	case "ERROR":
		return SevError
	case "WARNING":
		// Special case: WARNING maps to WARN severity but also gets dynamic slot
		d.levelMap.GetOrAssignIndex(normalized)
		return SevWarn
	case "ERR":
		// Special case: ERR maps to ERROR severity but also gets dynamic slot  
		d.levelMap.GetOrAssignIndex(normalized)
		return SevError
	default:
		// For all other levels, register with level map for dynamic assignment
		d.levelMap.GetOrAssignIndex(normalized)
		return SevUnknown
	}
}