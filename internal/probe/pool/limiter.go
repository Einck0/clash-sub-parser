package pool

import (
	"context"
	"sync"
)

// DynamicLimiter manages a dynamic concurrency window using token queues.
// It supports runtime dynamic limit adjustment, context cancellation, and zero leaks.
type DynamicLimiter struct {
	mu      sync.Mutex
	limit   int
	active  int
	waiters []chan struct{}
}

// NewLimiter creates a new DynamicLimiter with the given concurrency limit.
func NewLimiter(limit int) *DynamicLimiter {
	if limit < 1 {
		limit = 1
	}
	return &DynamicLimiter{
		limit: limit,
	}
}

// Limit returns the current concurrency limit.
func (l *DynamicLimiter) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit
}

// Active returns the count of currently active concurrency tokens.
func (l *DynamicLimiter) Active() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active
}

// Waiters returns the count of goroutines currently waiting for a token.
func (l *DynamicLimiter) Waiters() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.waiters)
}

// SetLimit dynamically updates the concurrency limit.
// If the limit increases, pending waiters are immediately granted tokens up to the new limit.
// If the limit decreases, active tokens drain naturally upon Release.
func (l *DynamicLimiter) SetLimit(newLimit int) {
	if newLimit < 1 {
		newLimit = 1
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.limit = newLimit
	// Wake up waiters if we now have available slots
	for l.active < l.limit && len(l.waiters) > 0 {
		l.active++
		w := l.waiters[0]
		l.waiters = l.waiters[1:]
		close(w)
	}
}

// Acquire requests a concurrency slot. If all slots are occupied, it blocks until
// a slot is released or the context is cancelled.
func (l *DynamicLimiter) Acquire(ctx context.Context) error {
	l.mu.Lock()
	if l.active < l.limit {
		l.active++
		l.mu.Unlock()
		return nil
	}

	// Must wait in queue
	ch := make(chan struct{})
	l.waiters = append(l.waiters, ch)
	l.mu.Unlock()

	select {
	case <-ctx.Done():
		l.mu.Lock()
		for i, w := range l.waiters {
			if w == ch {
				l.waiters = append(l.waiters[:i], l.waiters[i+1:]...)
				l.mu.Unlock()
				return ctx.Err()
			}
		}
		// Token was already assigned to ch right before ctx.Done()
		l.releaseLocked()
		l.mu.Unlock()
		return ctx.Err()
	case <-ch:
		return nil
	}
}

// Release yields a concurrency slot back to the limiter.
func (l *DynamicLimiter) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.releaseLocked()
}

func (l *DynamicLimiter) releaseLocked() {
	if len(l.waiters) > 0 && l.active <= l.limit {
		// Hand off directly to next waiter; active count stays the same
		w := l.waiters[0]
		l.waiters = l.waiters[1:]
		close(w)
	} else if l.active > 0 {
		l.active--
	}
}
