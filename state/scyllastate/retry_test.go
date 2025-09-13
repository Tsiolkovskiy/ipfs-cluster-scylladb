package scyllastate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"
)

func TestNewRetryPolicyAdvanced(t *testing.T) {
	config := RetryPolicyConfig{
		NumRetries:    5,
		MinRetryDelay: 200 * time.Millisecond,
		MaxRetryDelay: 30 * time.Second,
	}

	rp := NewRetryPolicy(config)

	assert.Equal(t, 5, rp.numRetries)
	assert.Equal(t, 200*time.Millisecond, rp.minDelay)
	assert.Equal(t, 30*time.Second, rp.maxDelay)
	assert.Equal(t, 2.0, rp.backoffFactor)
}

func TestRetryPolicy_CalculateDelayAdvanced(t *testing.T) {
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

	// Test positive attempts
	delay1 := rp.calculateDelay(1)
	assert.GreaterOrEqual(t, delay1, rp.minDelay)
	assert.LessOrEqual(t, delay1, rp.maxDelay)

	delay2 := rp.calculateDelay(2)
	assert.GreaterOrEqual(t, delay2, rp.minDelay)
	assert.LessOrEqual(t, delay2, rp.maxDelay)

	delay3 := rp.calculateDelay(3)
	assert.GreaterOrEqual(t, delay3, rp.minDelay)
	assert.LessOrEqual(t, delay3, rp.maxDelay)

	// Test that delays generally increase (accounting for jitter)
	// We'll run this multiple times to account for jitter variance
	increasingCount := 0
	for i := 0; i < 10; i++ {
		d1 := rp.calculateDelay(1)
		d2 := rp.calculateDelay(2)
		if d2 >= d1 {
			increasingCount++
		}
	}
	// At least 70% should be increasing due to exponential backoff
	assert.GreaterOrEqual(t, increasingCount, 7)
}

func TestRetryPolicy_CalculateDelayMaxCap(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    10,
		minDelay:      100 * time.Millisecond,
		maxDelay:      1 * time.Second, // Small max delay
		backoffFactor: 2.0,
	}

	// High attempt number should be capped at maxDelay
	delay := rp.calculateDelay(10)
	assert.LessOrEqual(t, delay, rp.maxDelay)
	assert.GreaterOrEqual(t, delay, rp.minDelay)
}

func TestRetryPolicy_IsRetryableError(t *testing.T) {
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
			name:     "timeout error",
			err:      errors.New("connection timeout"),
			expected: true,
		},
		{
			name:     "timed out error",
			err:      errors.New("operation timed out"),
			expected: true,
		},
		{
			name:     "unavailable error",
			err:      errors.New("service unavailable"),
			expected: true,
		},
		{
			name:     "not enough replicas",
			err:      errors.New("not enough replicas available"),
			expected: true,
		},
		{
			name:     "cannot achieve consistency",
			err:      errors.New("cannot achieve consistency level"),
			expected: true,
		},
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "connection reset",
			err:      errors.New("connection reset by peer"),
			expected: true,
		},
		{
			name:     "no hosts available",
			err:      errors.New("no hosts available in the pool"),
			expected: true,
		},
		{
			name:     "host down",
			err:      errors.New("host is down"),
			expected: true,
		},
		{
			name:     "overloaded",
			err:      errors.New("server overloaded"),
			expected: true,
		},
		{
			name:     "too many requests",
			err:      errors.New("too many requests"),
			expected: true,
		},
		{
			name:     "syntax error - not retryable",
			err:      errors.New("syntax error in CQL"),
			expected: false,
		},
		{
			name:     "authentication error - not retryable",
			err:      errors.New("authentication failed"),
			expected: false,
		},
		{
			name:     "generic error - not retryable",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rp.isRetryableError(tt.err)
			assert.Equal(t, tt.expected, result, "Error: %v", tt.err)
		})
	}
}

func TestRetryPolicy_IsRetryableError_GocqlTypes(t *testing.T) {
	rp := &RetryPolicy{}

	// Test gocql specific error types
	unavailableErr := &gocql.RequestErrUnavailable{}
	assert.True(t, rp.isRetryableError(unavailableErr))

	readTimeoutErr := &gocql.RequestErrReadTimeout{}
	assert.True(t, rp.isRetryableError(readTimeoutErr))

	writeTimeoutErr := &gocql.RequestErrWriteTimeout{}
	assert.True(t, rp.isRetryableError(writeTimeoutErr))
}

func TestRetryPolicy_ExecuteWithRetry_Success(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      10 * time.Millisecond,
		maxDelay:      100 * time.Millisecond,
		backoffFactor: 2.0,
	}

	ctx := context.Background()
	callCount := 0

	operation := func() error {
		callCount++
		return nil // Success on first try
	}

	err := rp.ExecuteWithRetry(ctx, operation)
	assert.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestRetryPolicy_ExecuteWithRetry_SuccessAfterRetries(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      10 * time.Millisecond,
		maxDelay:      100 * time.Millisecond,
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
	// Should have taken some time due to delays
	assert.Greater(t, duration, 20*time.Millisecond)
}

func TestRetryPolicy_ExecuteWithRetry_NonRetryableError(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      10 * time.Millisecond,
		maxDelay:      100 * time.Millisecond,
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
}

func TestRetryPolicy_ExecuteWithRetry_ExhaustedRetries(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    2,
		minDelay:      10 * time.Millisecond,
		maxDelay:      100 * time.Millisecond,
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
}

func TestRetryPolicy_ExecuteWithRetry_ContextCancellation(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    5,
		minDelay:      100 * time.Millisecond,
		maxDelay:      1 * time.Second,
		backoffFactor: 2.0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	callCount := 0
	operation := func() error {
		callCount++
		return errors.New("timeout error") // Retryable error
	}

	err := rp.ExecuteWithRetry(ctx, operation)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
	// Should have been cancelled before exhausting all retries
	assert.LessOrEqual(t, callCount, 3)
}

func TestRetryPolicy_ExecuteWithRetry_ContextCancellationDuringDelay(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      200 * time.Millisecond,
		maxDelay:      1 * time.Second,
		backoffFactor: 2.0,
	}

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	operation := func() error {
		callCount++
		if callCount == 1 {
			// Cancel context after first failure, during delay
			go func() {
				time.Sleep(50 * time.Millisecond)
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

func TestRetryPolicy_ExecuteWithRetryAndMetricsAdvanced(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      10 * time.Millisecond,
		maxDelay:      100 * time.Millisecond,
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

func TestRetryPolicy_Attempt_GocqlInterface(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      10 * time.Millisecond,
		maxDelay:      100 * time.Millisecond,
		backoffFactor: 2.0,
	}

	// Mock query that implements gocql.RetryableQuery
	mockQuery := &mockRetryableQueryAdvanced{attempts: 0, consistency: gocql.Quorum}

	// First few attempts should return true
	assert.True(t, rp.Attempt(mockQuery))

	mockQuery.attempts = 1
	assert.True(t, rp.Attempt(mockQuery))

	mockQuery.attempts = 3
	assert.True(t, rp.Attempt(mockQuery))

	// After numRetries, should return false
	mockQuery.attempts = 4
	assert.False(t, rp.Attempt(mockQuery))
}

func TestRetryPolicy_GetRetryType_GocqlInterface(t *testing.T) {
	rp := &RetryPolicy{}

	// Test retryable errors
	timeoutErr := errors.New("timeout error")
	assert.Equal(t, gocql.Retry, rp.GetRetryType(timeoutErr))

	unavailableErr := &gocql.RequestErrUnavailable{}
	assert.Equal(t, gocql.Retry, rp.GetRetryType(unavailableErr))

	// Test non-retryable errors
	syntaxErr := errors.New("syntax error")
	assert.Equal(t, gocql.Rethrow, rp.GetRetryType(syntaxErr))
}

// Mock implementation of gocql.RetryableQuery for testing
type mockRetryableQueryAdvanced struct {
	attempts    int
	consistency gocql.Consistency
}

func (m *mockRetryableQueryAdvanced) Attempts() int {
	return m.attempts
}

func (m *mockRetryableQueryAdvanced) GetConsistency() gocql.Consistency {
	return m.consistency
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

// Test enhanced error classification for requirements 3.1 and 3.2
func TestRetryPolicy_EnhancedErrorClassification(t *testing.T) {
	rp := &RetryPolicy{}

	// Test gocql specific error types
	tests := []struct {
		name      string
		err       error
		retryable bool
		category  string
	}{
		{
			name:      "RequestErrUnavailable",
			err:       &gocql.RequestErrUnavailable{},
			retryable: true,
			category:  "consistency",
		},
		{
			name:      "RequestErrReadTimeout",
			err:       &gocql.RequestErrReadTimeout{},
			retryable: true,
			category:  "timeout",
		},
		{
			name:      "RequestErrWriteTimeout",
			err:       &gocql.RequestErrWriteTimeout{},
			retryable: true,
			category:  "timeout",
		},
		{
			name: "RequestErrReadFailure with sufficient responses",
			err: &gocql.RequestErrReadFailure{
				Consistency: gocql.Quorum,
				Received:    2,
				BlockFor:    3,
			},
			retryable: true,
			category:  "other",
		},
		{
			name: "RequestErrReadFailure with insufficient responses",
			err: &gocql.RequestErrReadFailure{
				Consistency: gocql.Quorum,
				Received:    1,
				BlockFor:    3,
			},
			retryable: false,
			category:  "other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.retryable, rp.isRetryableError(tt.err))
			assert.Equal(t, tt.category, rp.GetErrorCategory(tt.err))
		})
	}
}

// Test ExecuteWithRetry with different error types
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
}

// Benchmark adaptive delay calculation
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
