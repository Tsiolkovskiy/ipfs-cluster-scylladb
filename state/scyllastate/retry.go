package scyllastate

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/gocql/gocql"
)

// RetryPolicyConfig configures retry behavior
type RetryPolicyConfig struct {
	NumRetries    int           `json:"num_retries"`
	MinRetryDelay time.Duration `json:"min_retry_delay"`
	MaxRetryDelay time.Duration `json:"max_retry_delay"`
}

// RetryPolicy implements gocql.RetryPolicy with exponential backoff
type RetryPolicy struct {
	numRetries    int
	minDelay      time.Duration
	maxDelay      time.Duration
	backoffFactor float64
}

// NewRetryPolicy creates a new retry policy with the given configuration
func NewRetryPolicy(config RetryPolicyConfig) *RetryPolicy {
	return &RetryPolicy{
		numRetries:    config.NumRetries,
		minDelay:      config.MinRetryDelay,
		maxDelay:      config.MaxRetryDelay,
		backoffFactor: 2.0, // Standard exponential backoff factor
	}
}

// Attempt implements gocql.RetryPolicy.Attempt
func (rp *RetryPolicy) Attempt(q gocql.RetryableQuery) bool {
	return q.Attempts() <= rp.numRetries
}

// GetRetryType implements gocql.RetryPolicy.GetRetryType
func (rp *RetryPolicy) GetRetryType(err error) gocql.RetryType {
	if rp.isRetryableError(err) {
		return gocql.Retry
	}
	return gocql.Rethrow
}

// ExecuteWithRetry executes an operation with retry logic and context cancellation support
// This implementation addresses requirements 3.1 and 3.2 for handling node failures and consistency issues
func (rp *RetryPolicy) ExecuteWithRetry(ctx context.Context, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= rp.numRetries; attempt++ {
		// Check for context cancellation before each attempt
		select {
		case <-ctx.Done():
			return fmt.Errorf("operation cancelled after %d attempts: %w", attempt, ctx.Err())
		default:
		}

		// Apply delay before retry attempts (not on first attempt)
		if attempt > 0 {
			delay := rp.calculateDelayForError(attempt, lastErr)

			// Use a timer that can be cancelled by context
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return fmt.Errorf("operation cancelled during retry delay after %d attempts: %w", attempt, ctx.Err())
			case <-timer.C:
				// Continue with the retry
			}
		}

		// Execute the operation
		if err := operation(); err != nil {
			lastErr = err

			// Check if this error is retryable
			if !rp.isRetryable(err) {
				return fmt.Errorf("non-retryable error on attempt %d: %w", attempt+1, err)
			}

			// If this was the last attempt, return the error with context
			if attempt == rp.numRetries {
				return fmt.Errorf("operation failed after %d attempts, last error: %w", rp.numRetries+1, err)
			}

			// Log retry attempt for debugging (requirement 3.1 and 3.2 observability)
			log.Printf("ScyllaDB operation failed on attempt %d/%d, will retry: %v",
				attempt+1, rp.numRetries+1, err)

			// Continue to next retry
			continue
		}

		// Operation succeeded
		if attempt > 0 {
			log.Printf("ScyllaDB operation succeeded after %d retries", attempt)
		}
		return nil
	}

	// This should never be reached, but just in case
	return fmt.Errorf("operation failed after %d attempts: %w", rp.numRetries+1, lastErr)
}

// ExecuteWithRetryAndMetrics executes an operation with retry logic and optional metrics callback
func (rp *RetryPolicy) ExecuteWithRetryAndMetrics(ctx context.Context, operation func() error, onRetry func(attempt int, err error)) error {
	var lastErr error

	for attempt := 0; attempt <= rp.numRetries; attempt++ {
		// Check for context cancellation before each attempt
		select {
		case <-ctx.Done():
			return fmt.Errorf("operation cancelled after %d attempts: %w", attempt, ctx.Err())
		default:
		}

		// Apply delay before retry attempts (not on first attempt)
		if attempt > 0 {
			delay := rp.calculateDelay(attempt)

			// Call retry callback if provided
			if onRetry != nil {
				onRetry(attempt, lastErr)
			}

			// Use a timer that can be cancelled by context
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return fmt.Errorf("operation cancelled during retry delay after %d attempts: %w", attempt, ctx.Err())
			case <-timer.C:
				// Continue with the retry
			}
		}

		// Execute the operation
		if err := operation(); err != nil {
			lastErr = err

			// Check if this error is retryable
			if !rp.isRetryable(err) {
				return fmt.Errorf("non-retryable error on attempt %d: %w", attempt+1, err)
			}

			// If this was the last attempt, return the error
			if attempt == rp.numRetries {
				return fmt.Errorf("operation failed after %d attempts: %w", rp.numRetries+1, err)
			}

			// Continue to next retry
			continue
		}

		// Operation succeeded
		return nil
	}

	// This should never be reached, but just in case
	return fmt.Errorf("operation failed after %d attempts: %w", rp.numRetries+1, lastErr)
}

// calculateDelay calculates the delay for a given attempt using exponential backoff with jitter
func (rp *RetryPolicy) calculateDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	// Exponential backoff: delay = minDelay * (backoffFactor ^ (attempt - 1))
	delay := float64(rp.minDelay) * math.Pow(rp.backoffFactor, float64(attempt-1))

	// Cap at maxDelay
	if delay > float64(rp.maxDelay) {
		delay = float64(rp.maxDelay)
	}

	// Add jitter (±25% of the delay) to avoid thundering herd
	jitterRange := delay * 0.25
	jitter := jitterRange * (rand.Float64()*2 - 1) // Random value between -jitterRange and +jitterRange
	finalDelay := time.Duration(delay + jitter)

	// Ensure we don't go below minimum delay
	if finalDelay < rp.minDelay {
		finalDelay = rp.minDelay
	}

	// Ensure we don't exceed maximum delay
	if finalDelay > rp.maxDelay {
		finalDelay = rp.maxDelay
	}

	return finalDelay
}

// calculateDelayForError calculates delay based on error type to address requirements 3.1 and 3.2
func (rp *RetryPolicy) calculateDelayForError(attempt int, err error) time.Duration {
	baseDelay := rp.calculateDelay(attempt)

	if err == nil {
		return baseDelay
	}

	errStr := strings.ToLower(err.Error())

	// Requirement 3.1: Node failures - use longer delays to allow nodes to recover
	if strings.Contains(errStr, "host down") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no hosts available") {
		// Use 1.5x longer delay for node failures to allow recovery time
		return time.Duration(float64(baseDelay) * 1.5)
	}

	// Requirement 3.2: Consistency issues - use shorter delays for quicker retry
	if strings.Contains(errStr, "not enough replicas") ||
		strings.Contains(errStr, "cannot achieve consistency") ||
		strings.Contains(errStr, "quorum not available") {
		// Use shorter delay for consistency issues as they may resolve quickly
		return time.Duration(float64(baseDelay) * 0.75)
	}

	// Handle specific gocql error types
	switch err.(type) {
	case *gocql.RequestErrUnavailable:
		// Requirement 3.2: Insufficient replicas - shorter delay
		return time.Duration(float64(baseDelay) * 0.75)
	case *gocql.RequestErrWriteTimeout:
		// Requirement 3.2: Write timeout - moderate delay
		return baseDelay
	case *gocql.RequestErrReadTimeout:
		// Requirement 3.1: Read timeout - shorter delay as reads are less critical
		return time.Duration(float64(baseDelay) * 0.8)
	case *gocql.RequestErrReadFailure, *gocql.RequestErrWriteFailure:
		// Node failure scenarios - longer delay
		return time.Duration(float64(baseDelay) * 1.2)
	}

	// Default delay for other retryable errors
	return baseDelay
}

// CalculateDelayForAttempt is a public method to calculate delay for testing
func (rp *RetryPolicy) CalculateDelayForAttempt(attempt int) time.Duration {
	return rp.calculateDelay(attempt)
}

// IsNodeFailureError determines if an error indicates node failure (requirement 3.1)
func (rp *RetryPolicy) IsNodeFailureError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "host down") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no hosts available") ||
		strings.Contains(errStr, "host unreachable") ||
		strings.Contains(errStr, "network unreachable")
}

// IsConsistencyError determines if an error indicates consistency issues (requirement 3.2)
func (rp *RetryPolicy) IsConsistencyError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	isConsistencyErr := strings.Contains(errStr, "not enough replicas") ||
		strings.Contains(errStr, "cannot achieve consistency") ||
		strings.Contains(errStr, "quorum not available") ||
		strings.Contains(errStr, "insufficient replicas")

	// Check gocql specific types
	switch err.(type) {
	case *gocql.RequestErrUnavailable:
		return true
	}

	return isConsistencyErr
}

// GetErrorCategory categorizes errors for better retry strategies
func (rp *RetryPolicy) GetErrorCategory(err error) string {
	if err == nil {
		return "none"
	}

	if rp.IsNodeFailureError(err) {
		return "node_failure"
	}

	if rp.IsConsistencyError(err) {
		return "consistency"
	}

	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "timed out") {
		return "timeout"
	}

	if strings.Contains(errStr, "overloaded") || strings.Contains(errStr, "throttled") {
		return "overload"
	}

	return "other"
}

// isRetryable determines if an error should be retried (for ExecuteWithRetry)
func (rp *RetryPolicy) isRetryable(err error) bool {
	return rp.isRetryableError(err)
}

// isRetryableError determines if an error should be retried based on error type
// This addresses requirements 3.1 and 3.2 for handling node failures and consistency issues
func (rp *RetryPolicy) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	errStrLower := strings.ToLower(errStr)

	// Requirement 3.1: Handle node unavailability - these errors indicate nodes are down
	// but the cluster may still be functional with remaining nodes
	if strings.Contains(errStrLower, "timeout") ||
		strings.Contains(errStrLower, "timed out") ||
		strings.Contains(errStrLower, "connection refused") ||
		strings.Contains(errStrLower, "connection reset") ||
		strings.Contains(errStrLower, "no hosts available") ||
		strings.Contains(errStrLower, "host down") ||
		strings.Contains(errStrLower, "host unreachable") ||
		strings.Contains(errStrLower, "network unreachable") {
		return true
	}

	// Requirement 3.2: Handle insufficient replicas and consistency failures
	// These errors indicate temporary consistency issues that may resolve with retry
	if strings.Contains(errStrLower, "unavailable") ||
		strings.Contains(errStrLower, "not enough replicas") ||
		strings.Contains(errStrLower, "cannot achieve consistency") ||
		strings.Contains(errStrLower, "insufficient replicas") ||
		strings.Contains(errStrLower, "consistency level") ||
		strings.Contains(errStrLower, "quorum not available") {
		return true
	}

	// Handle temporary overload conditions
	if strings.Contains(errStrLower, "overloaded") ||
		strings.Contains(errStrLower, "too many requests") ||
		strings.Contains(errStrLower, "rate limit") ||
		strings.Contains(errStrLower, "throttled") {
		return true
	}

	// Handle specific gocql error types that indicate retryable conditions
	switch typedErr := err.(type) {
	case *gocql.RequestErrUnavailable:
		// Requirement 3.2: Insufficient replicas available for consistency level
		return true
	case *gocql.RequestErrReadTimeout:
		// Requirement 3.1: Read timeout may indicate node issues but data might be available elsewhere
		return true
	case *gocql.RequestErrWriteTimeout:
		// Requirement 3.2: Write timeout may indicate consistency issues
		return true
	case *gocql.RequestErrReadFailure:
		// Requirement 3.1: Read failure may be temporary node issue
		return typedErr.Received >= typedErr.BlockFor/2 // Retry if we got some responses
	case *gocql.RequestErrWriteFailure:
		// Requirement 3.2: Write failure may be temporary consistency issue
		return typedErr.Received >= typedErr.BlockFor/2 // Retry if we got some responses
	}

	// Don't retry on permanent errors (syntax errors, authentication, schema issues, etc.)
	if strings.Contains(errStrLower, "syntax error") ||
		strings.Contains(errStrLower, "invalid query") ||
		strings.Contains(errStrLower, "authentication") ||
		strings.Contains(errStrLower, "authorization") ||
		strings.Contains(errStrLower, "keyspace") ||
		strings.Contains(errStrLower, "table") ||
		strings.Contains(errStrLower, "column") ||
		strings.Contains(errStrLower, "schema") {
		return false
	}

	// Default to not retrying unknown errors to avoid infinite loops
	return false
}

// QueryObserver implements gocql.QueryObserver for tracing and monitoring
type QueryObserver struct {
	logger *log.Logger
}

// ObserveQuery implements gocql.QueryObserver.ObserveQuery
func (qo *QueryObserver) ObserveQuery(ctx context.Context, q gocql.ObservedQuery) {
	if qo.logger == nil {
		return
	}

	latency := q.End.Sub(q.Start)
	host := q.Host.ConnectAddress().String()

	if q.Err != nil {
		qo.logger.Printf("ScyllaDB query failed: query=%s, latency=%v, attempts=%d, host=%s, error=%v",
			q.Statement, latency, q.Metrics.Attempts, host, q.Err)
	} else {
		qo.logger.Printf("ScyllaDB query completed: query=%s, latency=%v, attempts=%d, host=%s",
			q.Statement, latency, q.Metrics.Attempts, host)
	}

	// Log slow queries as warnings
	if latency > 1*time.Second {
		qo.logger.Printf("SLOW QUERY WARNING: query=%s, latency=%v, attempts=%d, host=%s",
			q.Statement, latency, q.Metrics.Attempts, host)
	}
}

// BatchObserver implements gocql.BatchObserver for batch operation monitoring
type BatchObserver struct {
	logger *log.Logger
}

// ObserveBatch implements gocql.BatchObserver.ObserveBatch
func (bo *BatchObserver) ObserveBatch(ctx context.Context, b gocql.ObservedBatch) {
	if bo.logger == nil {
		return
	}

	latency := b.End.Sub(b.Start)
	host := b.Host.ConnectAddress().String()

	if b.Err != nil {
		bo.logger.Printf("ScyllaDB batch failed: statements=%d, latency=%v, attempts=%d, host=%s, error=%v",
			len(b.Statements), latency, b.Metrics.Attempts, host, b.Err)
	} else {
		bo.logger.Printf("ScyllaDB batch completed: statements=%d, latency=%v, attempts=%d, host=%s",
			len(b.Statements), latency, b.Metrics.Attempts, host)
	}

	// Log slow batches as warnings
	if latency > 2*time.Second {
		bo.logger.Printf("SLOW BATCH WARNING: statements=%d, latency=%v, attempts=%d, host=%s",
			len(b.Statements), latency, b.Metrics.Attempts, host)
	}
}
