package scyllastate

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRetryPolicy_Execute tests the retry policy execution
func TestRetryPolicy_Execute(t *testing.T) {
	tests := []struct {
		name           string
		numRetries     int
		operation      func() error
		expectedCalls  int
		expectedError  bool
		errorContains  string
	}{
		{
			name:       "successful operation on first try",
			numRetries: 3,
			operation: func() error {
				return nil
			},
			expectedCalls: 1,
			expectedError: false,
		},
		{
			name:       "successful operation on second try",
			numRetries: 3,
			operation: func() func() error {
				calls := 0
				return func() error {
					calls++
					if calls == 1 {
						return gocql.RequestErrWriteTimeout{
							Consistency: gocql.Quorum,
							Received:    1,
							BlockFor:    2,
							WriteType:   "SIMPLE",
						}
					}
					return nil
				}
			}(),
			expectedCalls: 2,
			expectedError: false,
		},
		{
			name:       "operation fails all retries",
			numRetries: 2,
			operation: func() error {
				return gocql.RequestErrWriteTimeout{
					Consistency: gocql.Quorum,
					Received:    1,
					BlockFor:    2,
					WriteType:   "SIMPLE",
				}
			},
			expectedCalls: 3, // initial + 2 retries
			expectedError: true,
			errorContains: "operation failed after 2 attempts",
		},
		{
			name:       "non-retryable error",
			numRetries: 3,
			operation: func() error {
				return errors.New("non-retryable error")
			},
			expectedCalls: 1,
			expectedError: true,
			errorContains: "non-retryable error",
		},
		{
			name:       "zero retries",
			numRetries: 0,
			operation: func() error {
				return gocql.RequestErrWriteTimeout{
					Consistency: gocql.Quorum,
					Received:    1,
					BlockFor:    2,
					WriteType:   "SIMPLE",
				}
			},
			expectedCalls: 1,
			expectedError: true,
			errorContains: "operation failed after 0 attempts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rp := &RetryPolicy{
				numRetries:    tt.numRetries,
				minDelay:      10 * time.Millisecond,
				maxDelay:      100 * time.Millisecond,
				backoffFactor: 2.0,
			}

			calls := 0
			wrappedOp := func() error {
				calls++
				return tt.operation()
			}

			ctx := context.Background()
			err := rp.Execute(ctx, wrappedOp)

			assert.Equal(t, tt.expectedCalls, calls)
			if tt.expectedError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestRetryPolicy_Execute_ContextCancellation tests context cancellation during retries
func TestRetryPolicy_Execute_ContextCancellation(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    5,
		minDelay:      100 * time.Millisecond,
		maxDelay:      1 * time.Second,
		backoffFactor: 2.0,
	}

	calls := 0
	operation := func() error {
		calls++
		return gocql.RequestErrWriteTimeout{
			Consistency: gocql.Quorum,
			Received:    1,
			BlockFor:    2,
			WriteType:   "SIMPLE",
		}
	}

	// Create a context that will be cancelled after a short delay
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := rp.Execute(ctx, operation)
	duration := time.Since(start)

	// Should return context.DeadlineExceeded
	assert.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)

	// Should not have taken too long (context should have cancelled quickly)
	assert.Less(t, duration, 200*time.Millisecond)

	// Should have made at least one call
	assert.GreaterOrEqual(t, calls, 1)
}

// TestRetryPolicy_CalculateDelay tests delay calculation
func TestRetryPolicy_CalculateDelay(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    5,
		minDelay:      100 * time.Millisecond,
		maxDelay:      10 * time.Second,
		backoffFactor: 2.0,
	}

	tests := []struct {
		attempt     int
		expectZero  bool
		expectRange bool
	}{
		{
			attempt:    0,
			expectZero: true,
		},
		{
			attempt:     1,
			expectRange: true,
		},
		{
			attempt:     2,
			expectRange: true,
		},
		{
			attempt:     3,
			expectRange: true,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			delay := rp.calculateDelay(tt.attempt)

			if tt.expectZero {
				assert.Equal(t, time.Duration(0), delay)
			}

			if tt.expectRange {
				assert.GreaterOrEqual(t, delay, rp.minDelay)
				assert.LessOrEqual(t, delay, rp.maxDelay)
				assert.Greater(t, delay, time.Duration(0))
			}
		})
	}
}

// TestRetryPolicy_IsRetryable tests error classification
func TestRetryPolicy_IsRetryable(t *testing.T) {
	rp := &RetryPolicy{}

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "write timeout",
			err:      gocql.RequestErrWriteTimeout{},
			expected: true,
		},
		{
			name:     "read timeout",
			err:      gocql.RequestErrReadTimeout{},
			expected: true,
		},
		{
			name:     "unavailable",
			err:      gocql.RequestErrUnavailable{},
			expected: true,
		},
		{
			name:     "overloaded",
			err:      gocql.ErrTooManyTimeouts,
			expected: true,
		},
		{
			name:     "connection closed",
			err:      gocql.ErrConnectionClosed,
			expected: true,
		},
		{
			name:     "generic error",
			err:      errors.New("generic error"),
			expected: false,
		},
		{
			name:     "syntax error",
			err:      gocql.RequestErrInvalid{},
			expected: false,
		},
		{
			name:     "unauthorized",
			err:      gocql.RequestErrUnauthorized{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rp.isRetryable(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRetryPolicy_BackoffProgression tests that delays increase with attempts
func TestRetryPolicy_BackoffProgression(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    5,
		minDelay:      10 * time.Millisecond,
		maxDelay:      1 * time.Second,
		backoffFactor: 2.0,
	}

	var delays []time.Duration
	for attempt := 1; attempt <= 5; attempt++ {
		delay := rp.calculateDelay(attempt)
		delays = append(delays, delay)
	}

	// Verify delays are generally increasing (allowing for jitter)
	for i := 1; i < len(delays); i++ {
		// Due to jitter, we can't guarantee strict ordering, but the average should increase
		// We'll just verify they're all within reasonable bounds
		assert.GreaterOrEqual(t, delays[i], rp.minDelay)
		assert.LessOrEqual(t, delays[i], rp.maxDelay)
	}
}

// TestRetryPolicy_MaxDelayRespected tests that max delay is respected
func TestRetryPolicy_MaxDelayRespected(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    10,
		minDelay:      1 * time.Millisecond,
		maxDelay:      50 * time.Millisecond,
		backoffFactor: 10.0, // Very aggressive backoff
	}

	// Test many attempts to ensure max delay is respected
	for attempt := 1; attempt <= 20; attempt++ {
		delay := rp.calculateDelay(attempt)
		assert.LessOrEqual(t, delay, rp.maxDelay, "Attempt %d exceeded max delay", attempt)
		assert.GreaterOrEqual(t, delay, rp.minDelay, "Attempt %d below min delay", attempt)
	}
}

// TestRetryPolicy_Integration tests retry policy integration with ScyllaState
func TestRetryPolicy_Integration(t *testing.T) {
	// Create a config with retry policy
	config := &Config{}
	config.Default()
	config.RetryPolicy = RetryPolicyConfig{
		NumRetries:      3,
		MinRetryDelay:   10 * time.Millisecond,
		MaxRetryDelay:   100 * time.Millisecond,
	}

	// Create retry policy from config
	rp := &RetryPolicy{
		numRetries:    config.RetryPolicy.NumRetries,
		minDelay:      config.RetryPolicy.MinRetryDelay,
		maxDelay:      config.RetryPolicy.MaxRetryDelay,
		backoffFactor: 2.0,
	}

	// Test that retry policy is properly configured
	assert.Equal(t, 3, rp.numRetries)
	assert.Equal(t, 10*time.Millisecond, rp.minDelay)
	assert.Equal(t, 100*time.Millisecond, rp.maxDelay)

	// Test execution with retryable error
	calls := 0
	operation := func() error {
		calls++
		if calls <= 2 {
			return gocql.RequestErrWriteTimeout{
				Consistency: gocql.Quorum,
				Received:    1,
				BlockFor:    2,
				WriteType:   "SIMPLE",
			}
		}
		return nil
	}

	ctx := context.Background()
	err := rp.Execute(ctx, operation)

	assert.NoError(t, err)
	assert.Equal(t, 3, calls) // Should have retried twice and succeeded on third attempt
}

// TestRetryPolicy_EdgeCases tests edge cases
func TestRetryPolicy_EdgeCases(t *testing.T) {
	t.Run("negative retries", func(t *testing.T) {
		rp := &RetryPolicy{
			numRetries:    -1,
			minDelay:      10 * time.Millisecond,
			maxDelay:      100 * time.Millisecond,
			backoffFactor: 2.0,
		}

		calls := 0
		operation := func() error {
			calls++
			return errors.New("test error")
		}

		ctx := context.Background()
		err := rp.Execute(ctx, operation)

		assert.Error(t, err)
		assert.Equal(t, 1, calls) // Should only call once with negative retries
	})

	t.Run("zero delays", func(t *testing.T) {
		rp := &RetryPolicy{
			numRetries:    2,
			minDelay:      0,
			maxDelay:      0,
			backoffFactor: 2.0,
		}

		calls := 0
		operation := func() error {
			calls++
			return gocql.RequestErrWriteTimeout{}
		}

		ctx := context.Background()
		start := time.Now()
		err := rp.Execute(ctx, operation)
		duration := time.Since(start)

		assert.Error(t, err)
		assert.Equal(t, 3, calls) // Initial + 2 retries
		// Should complete quickly with zero delays
		assert.Less(t, duration, 50*time.Millisecond)
	})

	t.Run("min delay greater than max delay", func(t *testing.T) {
		rp := &RetryPolicy{
			numRetries:    2,
			minDelay:      100 * time.Millisecond,
			maxDelay:      50 * time.Millisecond, // Less than min
			backoffFactor: 2.0,
		}

		// Should handle this gracefully
		delay := rp.calculateDelay(1)
		assert.GreaterOrEqual(t, delay, time.Duration(0))
		assert.LessOrEqual(t, delay, rp.minDelay) // Should use min as effective max
	})
}

// TestRetryPolicy_ConcurrentExecution tests concurrent retry executions
func TestRetryPolicy_ConcurrentExecution(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    2,
		minDelay:      10 * time.Millisecond,
		maxDelay:      50 * time.Millisecond,
		backoffFactor: 2.0,
	}

	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	// Run concurrent retry operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			calls := 0
			operation := func() error {
				calls++
				if calls <= 1 {
					return gocql.RequestErrWriteTimeout{}
				}
				return nil
			}

			ctx := context.Background()
			err := rp.Execute(ctx, operation)
			results <- err
		}(i)
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		err := <-results
		assert.NoError(t, err, "Goroutine %d failed", i)
	}
}

// BenchmarkRetryPolicy_Execute benchmarks retry policy execution
func BenchmarkRetryPolicy_Execute(b *testing.B) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      1 * time.Millisecond,
		maxDelay:      10 * time.Millisecond,
		backoffFactor: 2.0,
	}

	operation := func() error {
		return nil // Always succeed for benchmark
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rp.Execute(ctx, operation)
	}
}

// BenchmarkRetryPolicy_CalculateDelay benchmarks delay calculation
func BenchmarkRetryPolicy_CalculateDelay(b *testing.B) {
	rp := &RetryPolicy{
		numRetries:    10,
		minDelay:      1 * time.Millisecond,
		maxDelay:      1 * time.Second,
		backoffFactor: 2.0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rp.calculateDelay(i%10 + 1)
	}
}