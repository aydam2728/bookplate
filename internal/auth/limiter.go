package auth

import (
	"sync"
	"time"
)

// Login throttling. The form is reachable by anyone who can reach the
// instance, and household passwords are weak, so an unthrottled login is an
// open invitation to credential stuffing.
const (
	maxAttempts   = 10
	attemptWindow = 15 * time.Minute
)

// Limiter counts recent failed attempts per key, in memory.
//
// In memory is deliberate: a single instance serves one household, and
// persisting this would mean a write on every failed login, which is itself a
// way to hammer the database. Restarting clears it, which an attacker cannot
// cause.
type Limiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	now      func() time.Time
}

// NewLimiter returns an empty Limiter.
func NewLimiter() *Limiter {
	return &Limiter{attempts: map[string][]time.Time{}, now: time.Now}
}

// Allow reports whether another attempt may be made for key.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(key)) < maxAttempts
}

// Record notes a failed attempt.
func (l *Limiter) Record(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts[key] = append(l.recent(key), l.now())
}

// Reset clears the history for key, called after a successful login so one
// forgetful evening does not lock someone out for a quarter of an hour.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

// recent returns the attempts still inside the window, pruning the rest. The
// caller holds the lock.
func (l *Limiter) recent(key string) []time.Time {
	cutoff := l.now().Add(-attemptWindow)
	kept := l.attempts[key][:0]
	for _, at := range l.attempts[key] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	l.attempts[key] = kept
	return kept
}
