package httpapi

import (
	"sync"
	"time"
)

// throttleStore is a small in-memory sliding window rate limiter keyed by an
// arbitrary string (IP, IP+email, ...).
type throttleStore struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newThrottleStore() *throttleStore {
	return &throttleStore{hits: map[string][]time.Time{}}
}

// Allow records an attempt and reports whether it is within the limit.
func (t *throttleStore) Allow(key string, max int, window time.Duration) (bool, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-window)
	list := t.hits[key][:0]
	for _, ts := range t.hits[key] {
		if ts.After(cutoff) {
			list = append(list, ts)
		}
	}
	if len(list) >= max {
		t.hits[key] = list
		retry := int(list[0].Add(window).Sub(now).Seconds())
		if retry < 1 {
			retry = 1
		}
		return false, retry
	}
	list = append(list, now)
	t.hits[key] = list
	return true, 0
}

// Count returns the number of attempts in the window.
func (t *throttleStore) Count(key string, window time.Duration) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	cutoff := time.Now().Add(-window)
	n := 0
	for _, ts := range t.hits[key] {
		if ts.After(cutoff) {
			n++
		}
	}
	return n
}

// Clear drops every recorded attempt for the key.
func (t *throttleStore) Clear(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.hits, key)
}

// AvailableIn returns the seconds before the key is allowed again.
func (t *throttleStore) AvailableIn(key string, window time.Duration) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.hits[key]) == 0 {
		return 0
	}
	d := t.hits[key][0].Add(window).Sub(time.Now())
	if d < 0 {
		return 1
	}
	return int(d.Seconds()) + 1
}
