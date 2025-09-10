package core

import (
	"sync"
)

// SearchState manages the state for find navigation and highlighting.
// This provides "h" highlight (no scrolling) and "f" find (highlight + arrow up/down jump).
type SearchState struct {
	mu      sync.RWMutex
	Active  bool        // whether find mode is currently active
	Matcher TextMatcher // current find pattern
	HitSeqs []uint64    // sorted sequence numbers of matching events
	Cursor  int         // current index into HitSeqs (-1 if none)
}

// NewSearchState creates a new SearchState
func NewSearchState() *SearchState {
	return &SearchState{
		HitSeqs: make([]uint64, 0),
		Cursor:  -1,
	}
}

// SetActive enables or disables find mode
func (s *SearchState) SetActive(active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Active = active
}

// IsActive returns whether find mode is currently active
func (s *SearchState) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Active
}

// SetMatcher sets the find pattern and clears existing hits
func (s *SearchState) SetMatcher(matcher TextMatcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.Matcher = matcher
	s.HitSeqs = s.HitSeqs[:0] // clear existing hits
	s.Cursor = -1
}

// GetMatcher returns the current find matcher
func (s *SearchState) GetMatcher() TextMatcher {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Matcher
}

// AddHit adds a matching sequence number to the hits list.
// Maintains sorted order for efficient navigation.
func (s *SearchState) AddHit(seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Find insertion position using binary search
	left, right := 0, len(s.HitSeqs)
	for left < right {
		mid := (left + right) / 2
		if s.HitSeqs[mid] < seq {
			left = mid + 1
		} else {
			right = mid
		}
	}
	
	// Check if sequence already exists
	if left < len(s.HitSeqs) && s.HitSeqs[left] == seq {
		return // already exists
	}
	
	// Insert at the correct position
	s.HitSeqs = append(s.HitSeqs, 0)
	copy(s.HitSeqs[left+1:], s.HitSeqs[left:])
	s.HitSeqs[left] = seq
}

// RemoveOldHits removes sequence numbers older than the given threshold.
// This should be called when the ring buffer overwrites old entries.
func (s *SearchState) RemoveOldHits(oldestSeq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Find the first sequence number >= oldestSeq
	cutoff := 0
	for cutoff < len(s.HitSeqs) && s.HitSeqs[cutoff] < oldestSeq {
		cutoff++
	}
	
	if cutoff > 0 {
		// Remove old entries
		copy(s.HitSeqs, s.HitSeqs[cutoff:])
		s.HitSeqs = s.HitSeqs[:len(s.HitSeqs)-cutoff]
		
		// Adjust cursor position
		if s.Cursor >= 0 {
			s.Cursor -= cutoff
			if s.Cursor < 0 {
				s.Cursor = -1
			}
		}
	}
}

// Next moves to the next hit and returns its sequence number.
// Returns 0 if there are no hits or navigation is at the end.
func (s *SearchState) Next() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if len(s.HitSeqs) == 0 {
		return 0
	}
	
	if s.Cursor < len(s.HitSeqs)-1 {
		s.Cursor++
		return s.HitSeqs[s.Cursor]
	}
	
	// Wrap around to the beginning
	s.Cursor = 0
	return s.HitSeqs[s.Cursor]
}

// Prev moves to the previous hit and returns its sequence number.
// Returns 0 if there are no hits.
func (s *SearchState) Prev() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if len(s.HitSeqs) == 0 {
		return 0
	}
	
	if s.Cursor > 0 {
		s.Cursor--
		return s.HitSeqs[s.Cursor]
	}
	
	// Wrap around to the end
	s.Cursor = len(s.HitSeqs) - 1
	return s.HitSeqs[s.Cursor]
}

// Current returns the sequence number of the current hit.
// Returns 0 if no hit is currently selected.
func (s *SearchState) Current() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if s.Cursor >= 0 && s.Cursor < len(s.HitSeqs) {
		return s.HitSeqs[s.Cursor]
	}
	return 0
}

// Count returns the total number of hits
func (s *SearchState) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.HitSeqs)
}

// Position returns the current position (1-based) and total count.
// Returns (0, 0) if no hits or no current selection.
func (s *SearchState) Position() (current, total int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	total = len(s.HitSeqs)
	if s.Cursor >= 0 && s.Cursor < total {
		current = s.Cursor + 1 // convert to 1-based
	}
	return
}

// JumpToFirst moves to the first hit and returns its sequence number.
// Returns 0 if there are no hits.
func (s *SearchState) JumpToFirst() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if len(s.HitSeqs) == 0 {
		return 0
	}
	
	s.Cursor = 0
	return s.HitSeqs[s.Cursor]
}

// SetCurrentBySeq sets the current position to the hit with the given sequence number.
// Returns true if the sequence was found, false otherwise.
func (s *SearchState) SetCurrentBySeq(seq uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Binary search for the sequence
	left, right := 0, len(s.HitSeqs)
	for left < right {
		mid := (left + right) / 2
		if s.HitSeqs[mid] < seq {
			left = mid + 1
		} else {
			right = mid
		}
	}
	
	if left < len(s.HitSeqs) && s.HitSeqs[left] == seq {
		s.Cursor = left
		return true
	}
	
	return false
}

// Clear clears all hits and resets the cursor
func (s *SearchState) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.HitSeqs = s.HitSeqs[:0]
	s.Cursor = -1
}

// GetSnapshot returns a read-only snapshot of the current state
func (s *SearchState) GetSnapshot() (active bool, hitSeqs []uint64, cursor int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	active = s.Active
	hitSeqs = make([]uint64, len(s.HitSeqs))
	copy(hitSeqs, s.HitSeqs)
	cursor = s.Cursor
	
	return
}