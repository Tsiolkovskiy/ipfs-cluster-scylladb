package scyllastate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gocql/gocql"
)

// TestRetryPolicy_Requirements tests that the retry policy implementation
// satisfies requirements 3.1 and 3.2 from the specification
func TestRetryPolicy_Requirements(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      10 * time.Millisecond,
		maxDelay:      100 * time.Millisecond,
		backoffFactor: 2.0,
	}

	ctx := context.Background()

	// Requirement 3.1: System MUST continue working with remaining available nodes
	// when ScyllaDB nodes become unavailable
	t.Run("Requirement 3.1 - Node unavailability handling", func(t *testing.T) {
		nodeFailureErrors := []error{
			errors.New("host down"),
			errors.New("connection refused"),
			errors.New("no hosts available in the pool"),
			errors.New("host unreachable"),
		}

		for _, nodeErr := range nodeFailureErrors {
			t.Run(nodeErr.Error(), func(t *testing.T) {
				callCount := 0
				operation := func() error {
					callCount++
					if callCount <= 2 {
						return nodeErr // Simulate node failure
					}
					return nil // Simulate recovery/failover to available node
				}

				err := rp.ExecuteWithRetry(ctx, operation)
				if err != nil {
					t.Errorf("Expected operation to succeed after node recovery, got: %v", err)
				}
				if callCount != 3 {
					t.Errorf("Expected 3 attempts (initial + 2 retries), got %d", callCount)
				}

				// Verify error is classified correctly
				if !rp.IsNodeFailureError(nodeErr) {
					t.Errorf("Error %v should be classified as node failure", nodeErr)
				}
				if !rp.isRetryableError(nodeErr) {
					t.Errorf("Error %v should be retryable", nodeErr)
				}
			})
		}
	})

	// Requirement 3.2: System MUST retry with appropriate delays when write operations
	// fail due to insufficient replicas and eventually return error if consistency cannot be achieved
	t.Run("Requirement 3.2 - Consistency failure handling", func(t *testing.T) {
		consistencyErrors := []error{
			errors.New("not enough replicas available"),
			errors.New("cannot achieve consistency level"),
			&gocql.RequestErrUnavailable{},
		}

		for _, consistencyErr := range consistencyErrors {
			t.Run(consistencyErr.Error(), func(t *testing.T) {
				// Test successful recovery after consistency issues
				t.Run("recovery", func(t *testing.T) {
					callCount := 0
					operation := func() error {
						callCount++
						if callCount <= 2 {
							return consistencyErr // Simulate consistency failure
						}
						return nil // Simulate consistency recovery
					}

					err := rp.ExecuteWithRetry(ctx, operation)
					if err != nil {
						t.Errorf("Expected operation to succeed after consistency recovery, got: %v", err)
					}
					if callCount != 3 {
						t.Errorf("Expected 3 attempts, got %d", callCount)
					}
				})

				// Test eventual failure when consistency cannot be achieved
				t.Run("eventual failure", func(t *testing.T) {
					callCount := 0
					operation := func() error {
						callCount++
						return consistencyErr // Always fail
					}

					err := rp.ExecuteWithRetry(ctx, operation)
					if err == nil {
						t.Error("Expected operation to eventually fail when consistency cannot be achieved")
					}
					expectedAttempts := rp.numRetries + 1 // Initial attempt + retries
					if callCount != expectedAttempts {
						t.Errorf("Expected %d attempts, got %d", expectedAttempts, callCount)
					}
				})

				// Verify error is classified correctly
				if !rp.IsConsistencyError(consistencyErr) {
					t.Errorf("Error %v should be classified as consistency error", consistencyErr)
				}
				if !rp.isRetryableError(consistencyErr) {
					t.Errorf("Error %v should be retryable", consistencyErr)
				}
			})
		}
	})

	// Test that appropriate delays are used for different error types
	t.Run("Adaptive delay calculation", func(t *testing.T) {
		baseDelay := rp.calculateDelay(2)

		// Node failures should have longer delays to allow recovery
		nodeErr := errors.New("host down")
		nodeDelay := rp.calculateDelayForError(2, nodeErr)
		if nodeDelay <= baseDelay {
			t.Error("Node failure errors should have longer delays than base delay")
		}

		// Consistency errors should have shorter delays for quicker retry
		consistencyErr := errors.New("not enough replicas")
		consistencyDelay := rp.calculateDelayForError(2, consistencyErr)
		if consistencyDelay >= baseDelay {
			t.Error("Consistency errors should have shorter delays than base delay")
		}
	})

	// Test context cancellation support (important for graceful shutdown)
	t.Run("Context cancellation support", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		callCount := 0
		operation := func() error {
			callCount++
			return errors.New("timeout error") // Always retryable error
		}

		err := rp.ExecuteWithRetry(ctx, operation)
		if err == nil {
			t.Error("Expected error due to context cancellation")
		}
		if callCount > 2 {
			t.Errorf("Expected few attempts due to quick cancellation, got %d", callCount)
		}
	})
}

// TestRetryPolicy_NonRetryableErrors verifies that certain errors are never retried
func TestRetryPolicy_NonRetryableErrors(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      1 * time.Millisecond,
		maxDelay:      10 * time.Millisecond,
		backoffFactor: 2.0,
	}

	nonRetryableErrors := []error{
		errors.New("syntax error in CQL"),
		errors.New("authentication failed"),
		errors.New("authorization denied"),
		errors.New("keyspace does not exist"),
		errors.New("table not found"),
		errors.New("invalid query"),
	}

	for _, nonRetryableErr := range nonRetryableErrors {
		t.Run(nonRetryableErr.Error(), func(t *testing.T) {
			callCount := 0
			operation := func() error {
				callCount++
				return nonRetryableErr
			}

			err := rp.ExecuteWithRetry(context.Background(), operation)
			if err == nil {
				t.Error("Expected error for non-retryable error")
			}
			if callCount != 1 {
				t.Errorf("Expected exactly 1 attempt for non-retryable error, got %d", callCount)
			}
			if !contains(err.Error(), "non-retryable error") {
				t.Errorf("Expected error message to indicate non-retryable, got: %v", err)
			}

			// Verify error classification
			if rp.isRetryableError(nonRetryableErr) {
				t.Errorf("Error %v should not be retryable", nonRetryableErr)
			}
		})
	}
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsInner(s, substr))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
