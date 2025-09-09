package core

import (
	"sync"
)

// Ring implements a thread-safe circular buffer for LogEvents with constant-time append
// and memory bounded by capacity. When full, new entries overwrite the oldest ones.
type Ring struct {
	mu   sync.RWMutex
	cap  int
	buf  []LogEvent
	head int    // next write position
	size int    // current number of elements (0 <= size <= cap)
	seq  uint64 // monotonically increasing sequence number
}

// NewRing creates a new ring buffer with the specified capacity
func NewRing(cap int) *Ring {
	if cap <= 0 {
		cap = 10000 // default capacity as specified in requirements
	}

	return &Ring{
		cap: cap,
		buf: make([]LogEvent, cap),
	}
}

// Append adds a new LogEvent to the ring buffer, assigning it a sequence number.
// Returns the event with sequence number assigned. When the buffer is full,
// the oldest entry is overwritten.
func (r *Ring) Append(e LogEvent) LogEvent {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Assign sequence number
	r.seq++
	e.Seq = r.seq

	// Store in the buffer
	r.buf[r.head] = e

	// Advance head position (wraps around)
	r.head = (r.head + 1) % r.cap

	// Update size (capped at capacity)
	if r.size < r.cap {
		r.size++
	}

	return e
}

// Snapshot returns a stable copy of all current events in chronological order
// (oldest to newest). The returned slice is independent of the internal buffer
// and safe to use without locking.
func (r *Ring) Snapshot() []LogEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.size == 0 {
		return nil
	}

	result := make([]LogEvent, r.size)

	if r.size < r.cap {
		// Buffer not yet full, events are from 0 to size-1
		copy(result, r.buf[:r.size])
	} else {
		// Buffer is full, events wrap around
		// Oldest events start at head position, newest end at head-1
		oldestIdx := r.head

		// Copy from oldestIdx to end of buffer
		copy(result, r.buf[oldestIdx:])

		// Copy from start of buffer to head-1
		if oldestIdx > 0 {
			copy(result[r.cap-oldestIdx:], r.buf[:oldestIdx])
		}
	}

	return result
}

// GetBySeq retrieves an event by its sequence number.
// Returns the event and true if found, or zero event and false if not found.
// Events may not be found if they have been overwritten due to buffer wrapping.
func (r *Ring) GetBySeq(seq uint64) (LogEvent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.size == 0 || seq == 0 {
		return LogEvent{}, false
	}

	// Check if sequence number is too old (overwritten)
	oldestSeq := r.seq - uint64(r.size) + 1
	if seq < oldestSeq || seq > r.seq {
		return LogEvent{}, false
	}

	// Calculate the position in the buffer
	var idx int
	if r.size < r.cap {
		// Buffer not full, sequential from 0
		idx = int(seq - oldestSeq)
	} else {
		// Buffer is full, calculate wrapped position
		offset := int(seq - oldestSeq)
		idx = (r.head + offset) % r.cap
	}

	event := r.buf[idx]
	if event.Seq == seq {
		return event, true
	}

	return LogEvent{}, false
}

// Capacity returns the maximum number of events the ring can hold
func (r *Ring) Capacity() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cap
}

// Size returns the current number of events in the ring
func (r *Ring) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.size
}

// CurrentSeq returns the current sequence number (last assigned)
func (r *Ring) CurrentSeq() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.seq
}

// OldestSeq returns the sequence number of the oldest event in the buffer,
// or 0 if the buffer is empty
func (r *Ring) OldestSeq() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.size == 0 {
		return 0
	}

	return r.seq - uint64(r.size) + 1
}
