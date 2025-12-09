package retry

import (
	"math"
	"math/rand"
	"time"
)

const (
	// Backoff intervals: 1s → 2s → 4s → 8s → 10s (max)
	initialDelay = 1 * time.Second
	maxDelay     = 10 * time.Second
	jitterPct    = 0.1 // 10% jitter
)

// Backoff implements exponential backoff with jitter for retry logic.
// The backoff sequence is: 1s → 2s → 4s → 8s → 10s (max, then stays at 10s).
type Backoff struct {
	attempt int
	rng     *rand.Rand
}

// NewBackoff creates a new Backoff instance with a seeded random number generator.
func NewBackoff() *Backoff {
	return &Backoff{
		attempt: 0,
		// #nosec G404 -- math/rand is appropriate for backoff jitter; cryptographic randomness not needed
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Next returns the next backoff duration and increments the attempt counter.
// The duration follows exponential backoff: 1s → 2s → 4s → 8s → 10s (max).
// A 10% jitter is added to prevent thundering herd effects.
func (b *Backoff) Next() time.Duration {
	// Calculate base delay: 2^attempt seconds
	exp := math.Pow(2, float64(b.attempt))
	delay := time.Duration(exp) * time.Second

	// Cap at max delay
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter (±10%)
	jitter := b.calculateJitter(delay)
	delay = delay + jitter

	b.attempt++
	return delay
}

// Reset resets the backoff to the initial state.
func (b *Backoff) Reset() {
	b.attempt = 0
}

// Attempt returns the current attempt number.
func (b *Backoff) Attempt() int {
	return b.attempt
}

// calculateJitter adds random jitter to prevent synchronized retries.
// Returns a value between -jitterPct*delay and +jitterPct*delay.
func (b *Backoff) calculateJitter(delay time.Duration) time.Duration {
	maxJitter := float64(delay) * jitterPct
	// Generate random value in range [-maxJitter, +maxJitter]
	jitter := (b.rng.Float64()*2 - 1) * maxJitter
	return time.Duration(jitter)
}
