package core

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// TextMatcher provides fast case-insensitive substring matching with optional regex support.
// Patterns wrapped in /.../  are treated as regular expressions.
type TextMatcher struct {
	raw     string         // original user input
	isRegex bool           // true if pattern is wrapped in /.../
	pattern *regexp.Regexp // compiled regex (nil for substring matching)
	lowered string         // lowercased version for case-insensitive substring matching
}

// NewMatcher creates a new TextMatcher from user input.
// Patterns wrapped in /.../  are treated as regular expressions.
// All other patterns are treated as case-insensitive substrings.
func NewMatcher(s string) (TextMatcher, error) {
	// Keep original input for Raw() method
	original := s
	s = strings.TrimSpace(s)

	if s == "" {
		return TextMatcher{raw: original}, nil
	}

	// Check if this is a regex pattern (wrapped in /.../
	if len(s) >= 3 && strings.HasPrefix(s, "/") && strings.HasSuffix(s, "/") {
		pattern := s[1 : len(s)-1]                     // extract pattern between slashes
		regex, err := regexp.Compile("(?i)" + pattern) // case-insensitive regex
		if err != nil {
			return TextMatcher{}, err
		}
		return TextMatcher{
			raw:     original,
			isRegex: true,
			pattern: regex,
		}, nil
	}

	// Substring matching - store lowercased version of trimmed string
	return TextMatcher{
		raw:     original,
		isRegex: false,
		lowered: strings.ToLower(s),
	}, nil
}

// Match returns true if the line matches this matcher's pattern.
// Uses case-insensitive substring matching for non-regex patterns.
func (m TextMatcher) Match(line string) bool {
	if m.isRegex {
		// For regex patterns, let the compiled regex decide (empty regex matches everything)
		return m.pattern.MatchString(line)
	}

	// For substring patterns, empty or whitespace-only patterns don't match anything
	if strings.TrimSpace(m.raw) == "" {
		return false
	}

	// Case-insensitive substring matching
	return strings.Contains(strings.ToLower(line), m.lowered)
}

// Raw returns the original user input used to create this matcher
func (m TextMatcher) Raw() string {
	return m.raw
}

// IsRegex returns true if this matcher uses regular expression matching
func (m TextMatcher) IsRegex() bool {
	return m.isRegex
}

// Filters manages the three types of text filtering: include, exclude, and highlight.
// Include filters: line is shown if it matches ANY include pattern (OR logic)
// Exclude filters: line is hidden if it matches ANY exclude pattern (OR logic)
// Highlights: visual marking without affecting line visibility
type Filters struct {
	Include    []TextMatcher // OR over includes - line shown if matches any
	Exclude    []TextMatcher // OR over excludes - line hidden if matches any
	Highlights []TextMatcher // visual highlighting only, no effect on visibility
}

// NewFilters creates an empty Filters struct
func NewFilters() *Filters {
	return &Filters{
		Include:    make([]TextMatcher, 0),
		Exclude:    make([]TextMatcher, 0),
		Highlights: make([]TextMatcher, 0),
	}
}

// ShouldShowLine determines if a line should be visible based on include/exclude filters.
// Returns true if:
// - No include filters are set, OR the line matches at least one include filter
// AND
// - The line does not match any exclude filter
func (f *Filters) ShouldShowLine(line string) bool {
	// Check exclude filters first (if any exclude matches, hide the line)
	for _, exclude := range f.Exclude {
		if exclude.Match(line) {
			return false
		}
	}

	// If no include filters, show the line (as long as it didn't match excludes)
	if len(f.Include) == 0 {
		return true
	}

	// Check if line matches any include filter
	for _, include := range f.Include {
		if include.Match(line) {
			return true
		}
	}

	// Has include filters but line didn't match any
	return false
}

// ShouldHighlight returns true if the line matches any highlight pattern
func (f *Filters) ShouldHighlight(line string) bool {
	for _, highlight := range f.Highlights {
		if highlight.Match(line) {
			return true
		}
	}
	return false
}

// AddInclude adds a new include filter
func (f *Filters) AddInclude(matcher TextMatcher) {
	f.Include = append(f.Include, matcher)
}

// AddExclude adds a new exclude filter
func (f *Filters) AddExclude(matcher TextMatcher) {
	f.Exclude = append(f.Exclude, matcher)
}

// AddHighlight adds a new highlight pattern
func (f *Filters) AddHighlight(matcher TextMatcher) {
	f.Highlights = append(f.Highlights, matcher)
}

// ClearIncludes removes all include filters
func (f *Filters) ClearIncludes() {
	f.Include = f.Include[:0]
}

// ClearExcludes removes all exclude filters
func (f *Filters) ClearExcludes() {
	f.Exclude = f.Exclude[:0]
}

// ClearHighlights removes all highlight patterns
func (f *Filters) ClearHighlights() {
	f.Highlights = f.Highlights[:0]
}

// FindIndex maintains a sorted list of sequence numbers for events that match
// the current find pattern. This enables efficient prev/next navigation.
type FindIndex struct {
	mu      sync.RWMutex
	matcher TextMatcher // current find pattern
	matches []uint64    // sorted sequence numbers of matching events
	current int         // current position in matches slice (-1 if none)
	maxCap  int         // maximum capacity (bounded by ring capacity)
}

// NewFindIndex creates a new FindIndex with the given capacity limit
func NewFindIndex(maxCapacity int) *FindIndex {
	return &FindIndex{
		matches: make([]uint64, 0),
		current: -1,
		maxCap:  maxCapacity,
	}
}

// SetMatcher updates the find pattern and clears existing matches
func (fi *FindIndex) SetMatcher(matcher TextMatcher) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	fi.matcher = matcher
	fi.matches = fi.matches[:0] // clear existing matches
	fi.current = -1
}

// GetMatcher returns the current find matcher
func (fi *FindIndex) GetMatcher() TextMatcher {
	fi.mu.RLock()
	defer fi.mu.RUnlock()
	return fi.matcher
}

// AddMatch adds a matching sequence number to the index.
// Maintains sorted order and enforces capacity limits.
func (fi *FindIndex) AddMatch(seq uint64) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	// Insert in sorted order
	pos := sort.Search(len(fi.matches), func(i int) bool {
		return fi.matches[i] >= seq
	})

	// If sequence already exists, don't add duplicate
	if pos < len(fi.matches) && fi.matches[pos] == seq {
		return
	}

	// Insert the sequence number at the correct position
	fi.matches = append(fi.matches, 0)
	copy(fi.matches[pos+1:], fi.matches[pos:])
	fi.matches[pos] = seq

	// Enforce capacity limit by removing oldest (smallest) entries
	if len(fi.matches) > fi.maxCap {
		// Remove oldest entries from the beginning
		removeCount := len(fi.matches) - fi.maxCap
		copy(fi.matches, fi.matches[removeCount:])
		fi.matches = fi.matches[:fi.maxCap]

		// Adjust current position
		if fi.current >= 0 {
			fi.current -= removeCount
			if fi.current < 0 {
				fi.current = -1
			}
		}
	}
}

// RemoveOldMatches removes sequence numbers older than the given threshold.
// This should be called when the ring buffer overwrites old entries.
func (fi *FindIndex) RemoveOldMatches(oldestSeq uint64) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	// Find the first sequence number >= oldestSeq
	cutoff := sort.Search(len(fi.matches), func(i int) bool {
		return fi.matches[i] >= oldestSeq
	})

	if cutoff > 0 {
		// Remove entries before the cutoff
		copy(fi.matches, fi.matches[cutoff:])
		fi.matches = fi.matches[:len(fi.matches)-cutoff]

		// Adjust current position
		if fi.current >= 0 {
			fi.current -= cutoff
			if fi.current < 0 {
				fi.current = -1
			}
		}
	}
}

// Next moves to the next match and returns its sequence number.
// Returns 0 if there are no more matches.
func (fi *FindIndex) Next() uint64 {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	if len(fi.matches) == 0 {
		return 0
	}

	if fi.current < len(fi.matches)-1 {
		fi.current++
		return fi.matches[fi.current]
	}

	// Wrap around to the beginning
	fi.current = 0
	return fi.matches[fi.current]
}

// Prev moves to the previous match and returns its sequence number.
// Returns 0 if there are no matches.
func (fi *FindIndex) Prev() uint64 {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	if len(fi.matches) == 0 {
		return 0
	}

	if fi.current > 0 {
		fi.current--
		return fi.matches[fi.current]
	}

	// Wrap around to the end
	fi.current = len(fi.matches) - 1
	return fi.matches[fi.current]
}

// Current returns the sequence number of the current match.
// Returns 0 if no match is currently selected.
func (fi *FindIndex) Current() uint64 {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	if fi.current >= 0 && fi.current < len(fi.matches) {
		return fi.matches[fi.current]
	}
	return 0
}

// Count returns the total number of matches
func (fi *FindIndex) Count() int {
	fi.mu.RLock()
	defer fi.mu.RUnlock()
	return len(fi.matches)
}

// Position returns the current position (1-based) and total count.
// Returns (0, 0) if no matches or no current selection.
func (fi *FindIndex) Position() (current, total int) {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	total = len(fi.matches)
	if fi.current >= 0 && fi.current < total {
		current = fi.current + 1 // convert to 1-based
	}
	return
}

// SetCurrentBySeq sets the current position to the match with the given sequence number.
// Returns true if the sequence was found, false otherwise.
func (fi *FindIndex) SetCurrentBySeq(seq uint64) bool {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	pos := sort.Search(len(fi.matches), func(i int) bool {
		return fi.matches[i] >= seq
	})

	if pos < len(fi.matches) && fi.matches[pos] == seq {
		fi.current = pos
		return true
	}

	return false
}
