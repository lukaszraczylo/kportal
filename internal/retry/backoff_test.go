package retry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBackoff_Next(t *testing.T) {
	tests := []struct {
		name     string
		attempt  int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{
			name:     "first attempt returns ~1s",
			attempt:  0,
			minDelay: 900 * time.Millisecond,  // 1s - 10% jitter
			maxDelay: 1100 * time.Millisecond, // 1s + 10% jitter
		},
		{
			name:     "second attempt returns ~2s",
			attempt:  1,
			minDelay: 1800 * time.Millisecond, // 2s - 10% jitter
			maxDelay: 2200 * time.Millisecond, // 2s + 10% jitter
		},
		{
			name:     "third attempt returns ~4s",
			attempt:  2,
			minDelay: 3600 * time.Millisecond, // 4s - 10% jitter
			maxDelay: 4400 * time.Millisecond, // 4s + 10% jitter
		},
		{
			name:     "fourth attempt returns ~8s",
			attempt:  3,
			minDelay: 7200 * time.Millisecond, // 8s - 10% jitter
			maxDelay: 8800 * time.Millisecond, // 8s + 10% jitter
		},
		{
			name:     "fifth attempt returns ~10s (max)",
			attempt:  4,
			minDelay: 9000 * time.Millisecond,  // 10s - 10% jitter
			maxDelay: 11000 * time.Millisecond, // 10s + 10% jitter
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBackoff()

			// Advance to the desired attempt
			for i := 0; i < tt.attempt; i++ {
				b.Next()
			}

			delay := b.Next()

			assert.GreaterOrEqual(t, delay, tt.minDelay, "delay should be >= min with jitter")
			assert.LessOrEqual(t, delay, tt.maxDelay, "delay should be <= max with jitter")
		})
	}
}

func TestBackoff_Next_StaysAtMax(t *testing.T) {
	b := NewBackoff()

	// Go through several attempts past the max
	for i := 0; i < 10; i++ {
		delay := b.Next()

		// After the 5th attempt (index 4), should always be at max (10s ± jitter)
		if i >= 4 {
			assert.GreaterOrEqual(t, delay, 9*time.Second, "should stay at max delay")
			assert.LessOrEqual(t, delay, 11*time.Second, "should stay at max delay with jitter")
		}
	}
}

func TestBackoff_Reset(t *testing.T) {
	b := NewBackoff()

	// Advance through several attempts
	for i := 0; i < 5; i++ {
		b.Next()
	}

	// Should be at attempt 5
	assert.Equal(t, 5, b.Attempt(), "should be at attempt 5")

	// Reset
	b.Reset()

	// Should be back to attempt 0
	assert.Equal(t, 0, b.Attempt(), "should be reset to attempt 0")

	// Next call should return ~1s (first attempt)
	delay := b.Next()
	assert.GreaterOrEqual(t, delay, 900*time.Millisecond, "after reset should return ~1s")
	assert.LessOrEqual(t, delay, 1100*time.Millisecond, "after reset should return ~1s with jitter")
}

func TestBackoff_Jitter_IsWithinExpectedRange(t *testing.T) {
	b := NewBackoff()

	// Test multiple times to ensure jitter varies
	delays := make([]time.Duration, 20)
	for i := 0; i < 20; i++ {
		b.Reset()
		delays[i] = b.Next()
	}

	// All delays should be within the jitter range for 1s
	for _, delay := range delays {
		assert.GreaterOrEqual(t, delay, 900*time.Millisecond, "jitter should not go below 10%")
		assert.LessOrEqual(t, delay, 1100*time.Millisecond, "jitter should not go above 10%")
	}

	// Check that not all delays are identical (jitter is working)
	allSame := true
	first := delays[0]
	for _, d := range delays[1:] {
		if d != first {
			allSame = false
			break
		}
	}

	assert.False(t, allSame, "jitter should produce varying delays")
}

func TestBackoff_Attempt(t *testing.T) {
	b := NewBackoff()

	assert.Equal(t, 0, b.Attempt(), "initial attempt should be 0")

	b.Next()
	assert.Equal(t, 1, b.Attempt(), "attempt should increment after Next()")

	b.Next()
	assert.Equal(t, 2, b.Attempt(), "attempt should increment after Next()")

	b.Reset()
	assert.Equal(t, 0, b.Attempt(), "attempt should reset to 0")
}

func TestBackoff_ExponentialProgression(t *testing.T) {
	b := NewBackoff()

	// Track the progression
	var delays []time.Duration
	for i := 0; i < 5; i++ {
		delays = append(delays, b.Next())
	}

	// Verify exponential growth (each should be roughly 2x the previous)
	// We allow for jitter by checking a range
	for i := 1; i < len(delays)-1; i++ {
		// Each delay should be roughly double the previous (accounting for jitter)
		// With 10% jitter on each value, worst case: (2.0 * 1.1) / 0.9 = 2.44
		// We use 1.7x to 2.5x as a reasonable range with 10% jitter on each
		ratio := float64(delays[i]) / float64(delays[i-1])
		assert.GreaterOrEqual(t, ratio, 1.7, "exponential growth should be ~2x")
		assert.LessOrEqual(t, ratio, 2.5, "exponential growth should be ~2x")
	}
}
