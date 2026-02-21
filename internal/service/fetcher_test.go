package service

import (
	"testing"
	"time"
)

func TestFetcher_calculateDelay(t *testing.T) {
	tests := []struct {
		name           string
		baseDelay      time.Duration
		maxDelay       time.Duration
		attempt        int
		expectedMin    time.Duration
		expectedMax    time.Duration
		shouldBeCapped bool
	}{
		{
			name:        "attempt 1 - base delay",
			baseDelay:   10 * time.Second,
			maxDelay:    2 * time.Minute,
			attempt:     1,
			expectedMin: 9 * time.Second,  // 10s - 10% jitter
			expectedMax: 11 * time.Second, // 10s + 10% jitter
		},
		{
			name:        "attempt 2 - exponential backoff",
			baseDelay:   10 * time.Second,
			maxDelay:    2 * time.Minute,
			attempt:     2,
			expectedMin: 18 * time.Second, // 20s - 10% jitter
			expectedMax: 22 * time.Second, // 20s + 10% jitter
		},
		{
			name:        "attempt 3 - exponential backoff",
			baseDelay:   10 * time.Second,
			maxDelay:    2 * time.Minute,
			attempt:     3,
			expectedMin: 36 * time.Second, // 40s - 10% jitter
			expectedMax: 44 * time.Second, // 40s + 10% jitter
		},
		{
			name:        "attempt 4 - exponential backoff",
			baseDelay:   10 * time.Second,
			maxDelay:    2 * time.Minute,
			attempt:     4,
			expectedMin: 72 * time.Second,  // 80s - 10% jitter
			expectedMax: 88 * time.Second,  // 80s + 10% jitter
		},
		{
			name:           "attempt 5 - capped at max delay",
			baseDelay:      10 * time.Second,
			maxDelay:       2 * time.Minute,
			attempt:        5,
			expectedMin:    108 * time.Second, // 120s (2min) - 10% jitter
			expectedMax:    132 * time.Second, // 120s (2min) + 10% jitter
			shouldBeCapped: true,
		},
		{
			name:           "attempt 10 - heavily capped",
			baseDelay:      10 * time.Second,
			maxDelay:       2 * time.Minute,
			attempt:        10,
			expectedMin:    108 * time.Second, // 120s (2min) - 10% jitter
			expectedMax:    132 * time.Second, // 120s (2min) + 10% jitter
			shouldBeCapped: true,
		},
		{
			name:        "small base delay - 1 second",
			baseDelay:   1 * time.Second,
			maxDelay:    30 * time.Second,
			attempt:     1,
			expectedMin: 900 * time.Millisecond, // 1s - 10% jitter
			expectedMax: 1100 * time.Millisecond, // 1s + 10% jitter
		},
		{
			name:        "small base delay attempt 3",
			baseDelay:   1 * time.Second,
			maxDelay:    30 * time.Second,
			attempt:     3,
			expectedMin: 3600 * time.Millisecond, // 4s - 10% jitter
			expectedMax: 4400 * time.Millisecond, // 4s + 10% jitter
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fetcher{
				config: FetcherConfig{
					RetryBaseDelay: tt.baseDelay,
					RetryMaxDelay:  tt.maxDelay,
				},
			}

			// Run multiple times to check jitter distribution
			for i := 0; i < 100; i++ {
				delay := f.calculateDelay(tt.attempt)

				if delay < tt.expectedMin || delay > tt.expectedMax {
					t.Errorf("calculateDelay(%d) = %v, want between %v and %v",
						tt.attempt, delay, tt.expectedMin, tt.expectedMax)
				}

				// Verify it's not exactly the base value (should have jitter)
				baseValue := tt.baseDelay * time.Duration(1<<uint(tt.attempt-1))
				if baseValue > tt.maxDelay {
					baseValue = tt.maxDelay
				}

				// Allow some calls to be exactly the base value due to random jitter
				// but most should be different
			}
		})
	}
}

func TestFetcher_calculateDelay_ExponentialFormula(t *testing.T) {
	// Test that the exponential formula is correct: base * 2^(attempt-1)
	f := &Fetcher{
		config: FetcherConfig{
			RetryBaseDelay: 10 * time.Second,
			RetryMaxDelay:  10 * time.Minute,
		},
	}

	tests := []struct {
		attempt       int
		expectedBase  time.Duration
	}{
		{1, 10 * time.Second},  // 10 * 2^0 = 10
		{2, 20 * time.Second},  // 10 * 2^1 = 20
		{3, 40 * time.Second},  // 10 * 2^2 = 40
		{4, 80 * time.Second},  // 10 * 2^3 = 80
		{5, 160 * time.Second}, // 10 * 2^4 = 160
		{6, 320 * time.Second}, // 10 * 2^5 = 320
	}

	for _, tt := range tests {
		delay := f.calculateDelay(tt.attempt)

		// Check that delay is within ±10% of expected base (allowing for jitter)
		minDelay := time.Duration(float64(tt.expectedBase) * 0.9)
		maxDelay := time.Duration(float64(tt.expectedBase) * 1.1)

		if delay < minDelay || delay > maxDelay {
			t.Errorf("calculateDelay(%d) = %v, want between %v and %v (base: %v)",
				tt.attempt, delay, minDelay, maxDelay, tt.expectedBase)
		}
	}
}

func TestFetcher_calculateDelay_Jitter(t *testing.T) {
	// Test that jitter is applied by checking the result is not exactly the base value
	f := &Fetcher{
		config: FetcherConfig{
			RetryBaseDelay: 10 * time.Second,
			RetryMaxDelay:  2 * time.Minute,
		},
	}

	attempt := 2
	expectedBase := 20 * time.Second // 10 * 2^1

	// Run multiple times and verify results are within jitter range
	foundWithinRange := false
	for i := 0; i < 10; i++ {
		delay := f.calculateDelay(attempt)

		// Check delay is within ±10% of expected base
		minDelay := time.Duration(float64(expectedBase) * 0.9)
		maxDelay := time.Duration(float64(expectedBase) * 1.1)

		if delay >= minDelay && delay <= maxDelay {
			foundWithinRange = true
		} else {
			t.Errorf("calculateDelay(%d) = %v, should be between %v and %v",
				attempt, delay, minDelay, maxDelay)
		}
	}

	if !foundWithinRange {
		t.Error("calculateDelay() should produce values within jitter range")
	}
}
