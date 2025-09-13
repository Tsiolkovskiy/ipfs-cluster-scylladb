package scyllastate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"
)

func TestRetryPolicy_NewRetryPolicy(t *testing.T) {
	config := RetryPolicyConfig{
		NumRetries:    5,
		MinRetryDelay: 200 * time.Millisecond,
		MaxRetryDelay: 30 * time.Second,
	}

	rp := NewRetryPolicy(config)

	assert.NotNil(t, rp)
	assert.Equal(t, 5, rp.numRetries)
	assert.Equal(t, 200*time.Millisecond, rp.minDelay)
	assert.Equal(t, 30*time.Second, rp.maxDelay)
	assert.Equal(t, 2.0, rp.backoffFactor)
}

func TestRetryPolicy_CalculateDelay(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      100 * time.Millisecond,
		maxDelay:      10 * time.Second,
		backoffFactor: 2.0,
	}

	tests := []struct {
		name     string
		attempt  int
		expected time.Duration
	}{
		{
			name:     "attempt 0 should return 0",
			attempt:  0,
			expected: 0,
		},
		{
			name:     "negative attempt should return 0",
			attempt:  -1,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := rp.calculateDelay(tt.attempt)
			assert.Equal(t, tt.expected, delay)
		})
	}

	// Test positive attempts with bounds checking
	for attempt := 1; attempt <= 5; attempt++ {
		delay := rp.calculateDelay(attempt)
		assert.GreaterOrEqual(t, delay, rp.minDelay, "Delay should be at least minDelay for attempt %d", attempt)
		assert.LessOrEqual(t, delay, rp.maxDelay, "Delay should not exceed maxDelay for attempt %d", attempt)
	}
}

func TestRetryPolicy_CalculateDelayExponentialBackoff(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    5,
		minDelay:      100 * time.Millisecond,
		maxDelay:      60 * time.Second, // Large max to test exponential growth
		backoffFactor: 2.0,
	}

	// Test that delays generally increase (accounting for jitter)
	// We'll run this multiple times to account for jitter variance
	increasingCount := 0
	totalRuns := 20

	for i := 0; i < totalRuns; i++ {
		delay1 := rp.calculateDelay(1)
		delay2 := rp.calculateDelay(2)
		delay3 := rp.calculateDelay(3)

		// Check that delays are generally increasing
		if delay2 >= delay1*0.8 && delay3 >= delay2*0.8 { // Allow for jitter
			increasingCount++
		}
	}

	// At least 70% should show increasing pattern due to exponential backoff
	assert.GreaterOrEqual(t, increasingCount, totalRuns*7/10,
		"Expected at least 70%% of runs to show increasing delay pattern")
}

func TestRetryPolicy_CalculateDelayMaxCap(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    10,
		minDelay:      100 * time.Millisecond,
		maxDelay:      1 * time.Second, // Small max delay to test capping
		backoffFactor: 2.0,
	}

	// High attempt numbers should be capped at maxDelay
	for attempt := 5; attempt <= 10; attempt++ {
		delay := rp.calculateDelay(attempt)
		assert.LessOrEqual(t, delay, rp.maxDelay,
			"Delay should be capped at maxDelay for attempt %d", attempt)
		assert.GreaterOrEqual(t, delay, rp.minDelay,
			"Delay should be at least minDelay for attempt %d", attempt)
	}
}

func TestRetryPolicy_CalculateDelayJitter(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      1 * time.Second,
		maxDelay:      10 * time.Second,
		backoffFactor: 2.0,
	}

	// Test that jitter produces different values
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = rp.calculateDelay(2) // Same attempt number
	}

	// Check that we get some variation due to jitter
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}

	assert.False(t, allSame, "Jitter should produce some variation in delays")
}

func TestRetryPolicy_IsRetryableError(t *testing.T) {
	rp := &RetryPolicy{}

	tests := []struct {
		name     string
		err      error
		expected bool
		category string
	}{
		// Nil and basic errors
		{
			name:     "nil error",
			err:      nil,
			expected: false,
			category: "none",
		},
		{
			name:     "generic error",
			err:      errors.New("some generic error"),
			expected: false,
			category: "other",
		},

		// Timeout errors (requirement 3.1)
		{
			name:     "timeout error",
			err:      errors.New("connection timeout"),
			expected: true,
			category: "timeout",
		},
		{
			name:     "timed out error",
			err:      errors.New("operation timed out"),
			expected: true,
			category: "timeout",
		},
		{
			name:     "request timeout",
			err:      errors.New("request timeout occurred"),
			expected: true,
			category: "timeout",
		},

		// Node failure errors (requirement 3.1)
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: true,
			category: "node_failure",
		},
		{
			name:     "connection reset",
			err:      errors.New("connection reset by peer"),
			expected: true,
			category: "node_failure",
		},
		{
			name:     "no hosts available",
			err:      errors.New("no hosts available in the pool"),
			expected: true,
			category: "node_failure",
		},
		{
			name:     "host down",
			err:      errors.New("host is down"),
			expected: true,
			category: "node_failure",
		},
		{
			name:     "host unreachable",
			err:      errors.New("host unreachable"),
			expected: true,
			category: "node_failure",
		},
		{
			name:     "network unreachable",
			err:      errors.New("network unreachable"),
			expected: true,
			category: "node_failure",
		},

		// Consistency errors (requirement 3.2)
		{
			name:     "unavailable error",
			err:      errors.New("service unavailable"),
			expected: true,
			category: "consistency",
		},
		{
			name:     "not enough replicas",
			err:      errors.New("not enough replicas available"),
			expected: true,
			category: "consistency",
		},
		{
			name:     "cannot achieve consistency",
			err:      errors.New("cannot achieve consistency level"),
			expected: true,
			category: "consistency",
		},
		{
			name:     "insufficient replicas",
			err:      errors.New("insufficient replicas for operation"),
			expected: true,
			category: "consistency",
		},
		{
			name:     "quorum not available",
			err:      errors.New("quorum not available"),
			expected: true,
			category: "consistency",
		},
		{
			name:     "consistency level error",
			err:      errors.New("consistency level cannot be satisfied"),
			expected: true,
			category: "consistency",
		},

		// Overload errors
		{
			name:     "overloaded",
			err:      errors.New("server overloaded"),
			expected: true,
			category: "overload",
		},
		{
			name:     "too many requests",
			err:      errors.New("too many requests"),
			expected: true,
			category: "overload",
		},
		{
			name:     "rate limit",
			err:      errors.New("rate limit exceeded"),
			expected: true,
			category: "overload",
		},
		{
			name:     "throttled",
			err:      errors.New("request throttled"),
			expected: true,
			category: "overload",
		},

		// Non-retryable errors
		{
			name:     "syntax error",
			err:      errors.New("syntax error in CQL"),
			expected: false,
			category: "other",
		},
		{
			name:     "invalid query",
			err:      errors.New("invalid query format"),
			expected: false,
			category: "other",
		},
		{
			name:     "authentication error",
			err:      errors.New("authentication failed"),
			expected: false,
			category: "other",
		},
		{
			name:     "authorization error",
			err:      errors.New("authorization denied"),
			expected: false,
			category: "other",
		},
		{
			name:     "keyspace error",
			err:      errors.New("keyspace does not exist"),
			expected: false,
			category: "other",
		},
		{
			name:     "table error",
			err:      errors.New("table not found"),
			expected: false,
			category: "other",
		},
		{
			name:     "column error",
			err:      errors.New("column does not exist"),
			expected: false,
			category: "other",
		},
		{
			name:     "schema error",
			err:      errors.New("schema mismatch"),
			expected: false,
			category: "other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rp.isRetryableError(tt.err)
			assert.Equal(t, tt.expected, result, "Error: %v", tt.err)

			category := rp.GetErrorCategory(tt.err)
			assert.Equal(t, tt.category, category, "Error category for: %v", tt.err)
		})
	}
}

func TestRetryPolicy_IsRetryableError_GocqlTypes(t *testing.T) {
	rp := &RetryPolicy{}

	tests := []struct {
		name     string
		err      error
		expected bool
		category string
	}{
		{
			name:     "RequestErrUnavailable",
			err:      &gocql.RequestErrUnavailable{},
			expected: true,
			category: "consistency",
		},
		{
			name:     "RequestErrReadTimeout",
			err:      &gocql.RequestErrReadTimeout{},
			expected: true,
			category: "timeout",
		},
		{
			name:     "RequestErrWriteTimeout",
			err:      &gocql.RequestErrWriteTimeout{},
			expected: true,
			category: "timeout",
		},
		{
			name: "RequestErrReadFailure with sufficient responses",
			err: &gocql.RequestErrReadFailure{
				Consistency: gocql.Quorum,
				Received:    2,
				BlockFor:    3,
			},
			expected: true,
			category: "other",
		},
		{
			name: "RequestErrReadFailure with insufficient responses",
			err: &gocql.RequestErrReadFailure{
				Consistency: gocql.Quorum,
				Received:    1,
				BlockFor:    3,
			},
			expected: false,
			category: "other",
		},
		{
			name: "RequestErrWriteFailure with sufficient responses",
			err: &gocql.RequestErrWriteFailure{
				Consistency: gocql.Quorum,
				Received:    2,
				BlockFor:    3,
			},
			expected: true,
			category: "other",
		},
		{
			name: "RequestErrWriteFailure with insufficient responses",
			err: &gocql.RequestErrWriteFailure{
				Consistency: gocql.Quorum,
				Received:    0,
				BlockFor:    3,
			},
			expected: false,
			category: "other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rp.isRetryableError(tt.err)
			assert.Equal(t, tt.expected, result, "Error: %v", tt.err)

			category := rp.GetErrorCategory(tt.err)
			assert.Equal(t, tt.category, category, "Error category for: %v", tt.err)
		})
	}
}

func TestRetryPolicy_CalculateDelayForError(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      100 * time.Millisecond,
		maxDelay:      10 * time.Second,
		backoffFactor: 2.0,
	}

	baseDelay := rp.calculateDelay(2) // Get base delay for attempt 2

	tests := []struct {
		name           string
		err            error
		expectedFactor float64 // Expected multiplier relative to base delay
	}{
		{
			name:           "nil error",
			err:            nil,
			expectedFactor: 1.0,
		},
		{
			name:           "node failure - longer delay",
			err:            errors.New("host down"),
			expectedFactor: 1.5,
		},
		{
			name:           "connection refused - longer delay",
			err:            errors.New("connection refused"),
			expectedFactor: 1.5,
		},
		{
			name:           "no hosts available - longer delay",
			err:            errors.New("no hosts available"),
			expectedFactor: 1.5,
		},
		{
			name:           "consistency issue - shorter delay",
			err:            errors.New("not enough replicas"),
			expectedFactor: 0.75,
		},
		{
			name:           "cannot achieve consistency - shorter delay",
			err:            errors.New("cannot achieve consistency"),
			expectedFactor: 0.75,
		},
		{
			name:           "quorum not available - shorter delay",
			err:            errors.New("quorum not available"),
			expectedFactor: 0.75,
		},
		{
			name:           "RequestErrUnavailable - shorter delay",
			err:            &gocql.RequestErrUnavailable{},
			expectedFactor: 0.75,
		},
		{
			name:           "RequestErrWriteTimeout - normal delay",
			err:            &gocql.RequestErrWriteTimeout{},
			expectedFactor: 1.0,
		},
		{
			name:           "RequestErrReadTimeout - shorter delay",
			err:            &gocql.RequestErrReadTimeout{},
			expectedFactor: 0.8,
		},
		{
			name:           "RequestErrReadFailure - longer delay",
			err:            &gocql.RequestErrReadFailure{},
			expectedFactor: 1.2,
		},
		{
			name:           "RequestErrWriteFailure - longer delay",
			err:            &gocql.RequestErrWriteFailure{},
			expectedFactor: 1.2,
		},
		{
			name:           "generic error - normal delay",
			err:            errors.New("some other error"),
			expectedFactor: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := rp.calculateDelayForError(2, tt.err)

			// Calculate expected delay range (accounting for jitter)
			expectedDelay := time.Duration(float64(baseDelay) * tt.expectedFactor)

			// Allow for jitter variance (±25% plus some tolerance)
			tolerance := time.Duration(float64(expectedDelay) * 0.4) // 40% tolerance for jitter
			minExpected := expectedDelay - tolerance
			maxExpected := expectedDelay + tolerance

			// Ensure delay is within reasonable bounds
			assert.GreaterOrEqual(t, delay, rp.minDelay, "Delay should be at least minDelay")
			assert.LessOrEqual(t, delay, rp.maxDelay, "Delay should not exceed maxDelay")

			// For non-nil errors with specific factors, check the range
			if tt.err != nil && tt.expectedFactor != 1.0 {
				if minExpected >= rp.minDelay && maxExpected <= rp.maxDelay {
					assert.GreaterOrEqual(t, delay, minExpected,
						"Delay should be at least %v for error type", minExpected)
					assert.LessOrEqual(t, delay, maxExpected,
						"Delay should be at most %v for error type", maxExpected)
				}
			}
		})
	}
}

func TestRetryPolicy_ErrorClassification(t *testing.T) {
	rp := &RetryPolicy{}

	tests := []struct {
		name      string
		err       error
		isNode    bool
		isConsist bool
	}{
		{
			name:      "host down - node failure",
			err:       errors.New("host down"),
			isNode:    true,
			isConsist: false,
		},
		{
			name:      "connection refused - node failure",
			err:       errors.New("connection refused"),
			isNode:    true,
			isConsist: false,
		},
		{
			name:      "network unreachable - node failure",
			err:       errors.New("network unreachable"),
			isNode:    true,
			isConsist: false,
		},
		{
			name:      "not enough replicas - consistency",
			err:       errors.New("not enough replicas"),
			isNode:    false,
			isConsist: true,
		},
		{
			name:      "quorum not available - consistency",
			err:       errors.New("quorum not available"),
			isNode:    false,
			isConsist: true,
		},
		{
			name:      "RequestErrUnavailable - consistency",
			err:       &gocql.RequestErrUnavailable{},
			isNode:    false,
			isConsist: true,
		},
		{
			name:      "generic error - neither",
			err:       errors.New("some other error"),
			isNode:    false,
			isConsist: false,
		},
		{
			name:      "nil error - neither",
			err:       nil,
			isNode:    false,
			isConsist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isNode := rp.IsNodeFailureError(tt.err)
			isConsist := rp.IsConsistencyError(tt.err)

			assert.Equal(t, tt.isNode, isNode, "Node failure classification for: %v", tt.err)
			assert.Equal(t, tt.isConsist, isConsist, "Consistency error classification for: %v", tt.err)
		})
	}
}

func TestRetryPolicy_ExecuteWithRetry_Success(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      1 * time.Millisecond, // Very short for testing
		maxDelay:      10 * time.Millisecond,
		backoffFactor: 2.0,
	}

	ctx := context.Background()
	callCount := 0

	operation := func() error {
		callCount++
		return nil // Success on first try
	}

	start := time.Now()
	err := rp.ExecuteWithRetry(ctx, operation)
	duration := time.Since(start)

	assert.NoError(t, err)
	assert.Equal(t, 1, callCount)
	assert.Less(t, duration, 50*time.Millisecond, "Should complete quickly on first success")
}

func TestRetryPolicy_ExecuteWithRetry_SuccessAfterRetries(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      1 * time.Millisecond,
		maxDelay:      10 * time.Millisecond,
		backoffFactor: 2.0,
	}

	ctx := context.Background()
	callCount := 0

	operation := func() error {
		callCount++
		if callCount < 3 {
			return errors.New("timeout error") // Retryable error
		}
		return nil // Success on third try
	}

	start := time.Now()
	err := rp.ExecuteWithRetry(ctx, operation)
	duration := time.Since(start)

	assert.NoError(t, err)
	assert.Equal(t, 3, callCount)
	assert.Greater(t, duration, 2*time.Millisecond, "Should take some time due to delays")
}

func TestRetryPolicy_ExecuteWithRetry_NonRetryableError(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      1 * time.Millisecond,
		maxDelay:      10 * time.Millisecond,
		backoffFactor: 2.0,
	}

	ctx := context.Background()
	callCount := 0

	operation := func() error {
		callCount++
		return errors.New("syntax error") // Non-retryable error
	}

	err := rp.ExecuteWithRetry(ctx, operation)

	assert.Error(t, err)
	assert.Equal(t, 1, callCount) // Should not retry
	assert.Contains(t, err.Error(), "non-retryable error")
	assert.Contains(t, err.Error(), "syntax error")
}

func TestRetryPolicy_ExecuteWithRetry_ExhaustedRetries(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    2,
		minDelay:      1 * time.Millisecond,
		maxDelay:      10 * time.Millisecond,
		backoffFactor: 2.0,
	}

	ctx := context.Background()
	callCount := 0

	operation := func() error {
		callCount++
		return errors.New("timeout error") // Always retryable error
	}

	err := rp.ExecuteWithRetry(ctx, operation)

	assert.Error(t, err)
	assert.Equal(t, 3, callCount) // Initial attempt + 2 retries
	assert.Contains(t, err.Error(), "operation failed after 3 attempts")
	assert.Contains(t, err.Error(), "timeout error")
}

func TestRetryPolicy_ExecuteWithRetry_ContextCancellation(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    5,
		minDelay:      10 * time.Millisecond,
		maxDelay:      100 * time.Millisecond,
		backoffFactor: 2.0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	callCount := 0
	operation := func() error {
		callCount++
		return errors.New("timeout error") // Retryable error
	}

	start := time.Now()
	err := rp.ExecuteWithRetry(ctx, operation)
	duration := time.Since(start)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
	assert.Less(t, duration, 100*time.Millisecond, "Should be cancelled before completing all retries")
	assert.LessOrEqual(t, callCount, 3, "Should not complete all retries due to cancellation")
}

func TestRetryPolicy_ExecuteWithRetry_ContextCancellationDuringDelay(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      50 * time.Millisecond,
		maxDelay:      200 * time.Millisecond,
		backoffFactor: 2.0,
	}

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	operation := func() error {
		callCount++
		if callCount == 1 {
			// Cancel context after first failure, during delay
			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()
		}
		return errors.New("timeout error") // Retryable error
	}

	err := rp.ExecuteWithRetry(ctx, operation)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled during retry delay")
	assert.Equal(t, 1, callCount) // Should only be called once
}

func TestRetryPolicy_ExecuteWithRetryAndMetrics(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      1 * time.Millisecond,
		maxDelay:      10 * time.Millisecond,
		backoffFactor: 2.0,
	}

	ctx := context.Background()
	callCount := 0
	retryCallbacks := []struct {
		attempt int
		err     error
	}{}

	operation := func() error {
		callCount++
		if callCount < 3 {
			return errors.New("timeout error")
		}
		return nil
	}

	onRetry := func(attempt int, err error) {
		retryCallbacks = append(retryCallbacks, struct {
			attempt int
			err     error
		}{attempt, err})
	}

	err := rp.ExecuteWithRetryAndMetrics(ctx, operation, onRetry)

	assert.NoError(t, err)
	assert.Equal(t, 3, callCount)
	assert.Len(t, retryCallbacks, 2) // Two retry callbacks (attempts 2 and 3)

	// Check retry callback data
	assert.Equal(t, 2, retryCallbacks[0].attempt)
	assert.Contains(t, retryCallbacks[0].err.Error(), "timeout error")
	assert.Equal(t, 3, retryCallbacks[1].attempt)
	assert.Contains(t, retryCallbacks[1].err.Error(), "timeout error")
}

func TestRetryPolicy_ExecuteWithRetryAndMetrics_NoCallback(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    2,
		minDelay:      1 * time.Millisecond,
		maxDelay:      10 * time.Millisecond,
		backoffFactor: 2.0,
	}

	ctx := context.Background()
	callCount := 0

	operation := func() error {
		callCount++
		if callCount < 2 {
			return errors.New("timeout error")
		}
		return nil
	}

	// Test with nil callback
	err := rp.ExecuteWithRetryAndMetrics(ctx, operation, nil)

	assert.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

// Test different error types with ExecuteWithRetry
func TestRetryPolicy_ExecuteWithRetryErrorTypes(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    2,
		minDelay:      1 * time.Millisecond,
		maxDelay:      10 * time.Millisecond,
		backoffFactor: 2.0,
	}

	ctx := context.Background()

	// Test node failure recovery (requirement 3.1)
	t.Run("node failure recovery", func(t *testing.T) {
		callCount := 0
		operation := func() error {
			callCount++
			if callCount < 3 {
				return errors.New("host down") // Node failure
			}
			return nil // Recovery
		}

		err := rp.ExecuteWithRetry(ctx, operation)
		assert.NoError(t, err)
		assert.Equal(t, 3, callCount)
	})

	// Test consistency error recovery (requirement 3.2)
	t.Run("consistency error recovery", func(t *testing.T) {
		callCount := 0
		operation := func() error {
			callCount++
			if callCount < 2 {
				return &gocql.RequestErrUnavailable{} // Consistency issue
			}
			return nil // Recovery
		}

		err := rp.ExecuteWithRetry(ctx, operation)
		assert.NoError(t, err)
		assert.Equal(t, 2, callCount)
	})

	// Test permanent failure (should not retry)
	t.Run("permanent failure", func(t *testing.T) {
		callCount := 0
		operation := func() error {
			callCount++
			return errors.New("syntax error in CQL") // Non-retryable
		}

		err := rp.ExecuteWithRetry(ctx, operation)
		assert.Error(t, err)
		assert.Equal(t, 1, callCount) // Should not retry
		assert.Contains(t, err.Error(), "non-retryable error")
	})

	// Test mixed error types
	t.Run("mixed error types", func(t *testing.T) {
		callCount := 0
		operation := func() error {
			callCount++
			switch callCount {
			case 1:
				return errors.New("timeout error") // Retryable
			case 2:
				return &gocql.RequestErrUnavailable{} // Retryable
			default:
				return nil // Success
			}
		}

		err := rp.ExecuteWithRetry(ctx, operation)
		assert.NoError(t, err)
		assert.Equal(t, 3, callCount)
	})
}

// Benchmark tests
func BenchmarkRetryPolicy_CalculateDelay(b *testing.B) {
	rp := &RetryPolicy{
		numRetries:    5,
		minDelay:      100 * time.Millisecond,
		maxDelay:      10 * time.Second,
		backoffFactor: 2.0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rp.calculateDelay(3)
	}
}

func BenchmarkRetryPolicy_IsRetryableError(b *testing.B) {
	rp := &RetryPolicy{}
	err := errors.New("timeout error")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rp.isRetryableError(err)
	}
}

func BenchmarkRetryPolicy_CalculateDelayForError(b *testing.B) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      100 * time.Millisecond,
		maxDelay:      10 * time.Second,
		backoffFactor: 2.0,
	}
	err := errors.New("host down")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rp.calculateDelayForError(2, err)
	}
}

func BenchmarkRetryPolicy_GetErrorCategory(b *testing.B) {
	rp := &RetryPolicy{}
	err := errors.New("timeout error")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rp.GetErrorCategory(err)
	}
}

// Test edge cases and error conditions
func TestRetryPolicy_EdgeCases(t *testing.T) {
	t.Run("zero retries", func(t *testing.T) {
		rp := &RetryPolicy{
			numRetries:    0,
			minDelay:      100 * time.Millisecond,
			maxDelay:      1 * time.Second,
			backoffFactor: 2.0,
		}

		ctx := context.Background()
		callCount := 0

		operation := func() error {
			callCount++
			return errors.New("timeout error")
		}

		err := rp.ExecuteWithRetry(ctx, operation)
		assert.Error(t, err)
		assert.Equal(t, 1, callCount) // Only initial attempt
		assert.Contains(t, err.Error(), "operation failed after 1 attempts")
	})

	t.Run("very small delays", func(t *testing.T) {
		rp := &RetryPolicy{
			numRetries:    2,
			minDelay:      1 * time.Nanosecond,
			maxDelay:      1 * time.Microsecond,
			backoffFactor: 2.0,
		}

		ctx := context.Background()
		callCount := 0

		operation := func() error {
			callCount++
			if callCount < 3 {
				return errors.New("timeout error")
			}
			return nil
		}

		start := time.Now()
		err := rp.ExecuteWithRetry(ctx, operation)
		duration := time.Since(start)

		assert.NoError(t, err)
		assert.Equal(t, 3, callCount)
		assert.Less(t, duration, 10*time.Millisecond, "Should complete quickly with tiny delays")
	})

	t.Run("case insensitive error matching", func(t *testing.T) {
		rp := &RetryPolicy{}

		tests := []struct {
			err      error
			expected bool
		}{
			{errors.New("TIMEOUT ERROR"), true},
			{errors.New("Timeout Error"), true},
			{errors.New("timeout error"), true},
			{errors.New("CONNECTION REFUSED"), true},
			{errors.New("Connection Refused"), true},
			{errors.New("UNAVAILABLE"), true},
			{errors.New("Unavailable"), true},
		}

		for _, tt := range tests {
			result := rp.isRetryableError(tt.err)
			assert.Equal(t, tt.expected, result, "Case insensitive matching failed for: %v", tt.err)
		}
	})

	t.Run("error message substrings", func(t *testing.T) {
		rp := &RetryPolicy{}

		// Test that error detection works with substrings
		tests := []string{
			"operation timeout occurred",
			"connection was refused by server",
			"service is currently unavailable",
			"not enough replicas are available",
			"host node1 is down",
		}

		for _, errMsg := range tests {
			err := errors.New(errMsg)
			result := rp.isRetryableError(err)
			assert.True(t, result, "Should be retryable: %s", errMsg)
		}
	})
}
