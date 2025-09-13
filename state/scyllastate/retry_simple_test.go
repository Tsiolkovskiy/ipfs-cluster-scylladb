package scyllastate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gocql/gocql"
)

// Simple test that doesn't depend on external config structures
func TestRetryPolicy_Basic(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      10 * time.Millisecond,
		maxDelay:      100 * time.Millisecond,
		backoffFactor: 2.0,
	}

	// Test successful operation
	callCount := 0
	operation := func() error {
		callCount++
		return nil
	}

	err := rp.ExecuteWithRetry(context.Background(), operation)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestRetryPolicy_RetryableError(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    2,
		minDelay:      1 * time.Millisecond,
		maxDelay:      10 * time.Millisecond,
		backoffFactor: 2.0,
	}

	callCount := 0
	operation := func() error {
		callCount++
		if callCount < 3 {
			return errors.New("timeout error") // Retryable
		}
		return nil
	}

	err := rp.ExecuteWithRetry(context.Background(), operation)
	if err != nil {
		t.Errorf("Expected no error after retries, got %v", err)
	}
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}

func TestRetryPolicy_NonRetryableError(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      1 * time.Millisecond,
		maxDelay:      10 * time.Millisecond,
		backoffFactor: 2.0,
	}

	callCount := 0
	operation := func() error {
		callCount++
		return errors.New("syntax error") // Non-retryable
	}

	err := rp.ExecuteWithRetry(context.Background(), operation)
	if err == nil {
		t.Error("Expected error for non-retryable error")
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call for non-retryable error, got %d", callCount)
	}
}

func TestRetryPolicy_ContextCancellation(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    5,
		minDelay:      50 * time.Millisecond,
		maxDelay:      200 * time.Millisecond,
		backoffFactor: 2.0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	callCount := 0
	operation := func() error {
		callCount++
		return errors.New("timeout error") // Retryable
	}

	err := rp.ExecuteWithRetry(ctx, operation)
	if err == nil {
		t.Error("Expected error due to context cancellation")
	}
	if !isContextError(err) {
		t.Errorf("Expected context cancellation error, got %v", err)
	}
}

func TestRetryPolicy_DelayCalculation(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      100 * time.Millisecond,
		maxDelay:      1 * time.Second,
		backoffFactor: 2.0,
	}

	// Test delay calculation
	delay0 := rp.calculateDelay(0)
	if delay0 != 0 {
		t.Errorf("Expected 0 delay for attempt 0, got %v", delay0)
	}

	delay1 := rp.calculateDelay(1)
	if delay1 < rp.minDelay || delay1 > rp.maxDelay {
		t.Errorf("Delay1 %v not within bounds [%v, %v]", delay1, rp.minDelay, rp.maxDelay)
	}

	delay2 := rp.calculateDelay(2)
	if delay2 < rp.minDelay || delay2 > rp.maxDelay {
		t.Errorf("Delay2 %v not within bounds [%v, %v]", delay2, rp.minDelay, rp.maxDelay)
	}
}

func TestRetryPolicy_ErrorClassification(t *testing.T) {
	rp := &RetryPolicy{}

	tests := []struct {
		err       error
		retryable bool
	}{
		{nil, false},
		{errors.New("timeout"), true},
		{errors.New("timed out"), true},
		{errors.New("unavailable"), true},
		{errors.New("connection refused"), true},
		{errors.New("overloaded"), true},
		{errors.New("syntax error"), false},
		{errors.New("authentication failed"), false},
	}

	for _, test := range tests {
		result := rp.isRetryableError(test.err)
		if result != test.retryable {
			t.Errorf("Error %v: expected retryable=%v, got %v", test.err, test.retryable, result)
		}
	}
}

// Helper function to check if error is context-related
func isContextError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "cancelled") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "context")
}
func TestRetryPolicy_GocqlInterface(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      10 * time.Millisecond,
		maxDelay:      100 * time.Millisecond,
		backoffFactor: 2.0,
	}

	// Test Attempt method
	mockQuery := &mockRetryableQuery{attempts: 0}

	// Should allow retries up to numRetries
	if !rp.Attempt(mockQuery) {
		t.Error("Expected Attempt to return true for 0 attempts")
	}

	mockQuery.attempts = 3
	if !rp.Attempt(mockQuery) {
		t.Error("Expected Attempt to return true for 3 attempts")
	}

	mockQuery.attempts = 4
	if rp.Attempt(mockQuery) {
		t.Error("Expected Attempt to return false for 4 attempts (exceeds numRetries)")
	}
}

func TestRetryPolicy_GetRetryType(t *testing.T) {
	rp := &RetryPolicy{}

	// Test retryable errors
	timeoutErr := errors.New("timeout error")
	if rp.GetRetryType(timeoutErr) != gocql.Retry {
		t.Error("Expected Retry for timeout error")
	}

	// Test non-retryable errors
	syntaxErr := errors.New("syntax error")
	if rp.GetRetryType(syntaxErr) != gocql.Rethrow {
		t.Error("Expected Rethrow for syntax error")
	}
}

// Test requirement 3.1: Node failure handling
func TestRetryPolicy_NodeFailureHandling(t *testing.T) {
	rp := &RetryPolicy{}

	nodeFailureErrors := []error{
		errors.New("host down"),
		errors.New("connection refused"),
		errors.New("no hosts available in the pool"),
		errors.New("host unreachable"),
		errors.New("network unreachable"),
	}

	for _, err := range nodeFailureErrors {
		if !rp.IsNodeFailureError(err) {
			t.Errorf("Expected %v to be classified as node failure", err)
		}
		if !rp.isRetryableError(err) {
			t.Errorf("Expected %v to be retryable", err)
		}
		if rp.GetErrorCategory(err) != "node_failure" {
			t.Errorf("Expected %v to be categorized as node_failure", err)
		}
	}
}

// Test requirement 3.2: Consistency error handling
func TestRetryPolicy_ConsistencyErrorHandling(t *testing.T) {
	rp := &RetryPolicy{}

	consistencyErrors := []error{
		errors.New("not enough replicas available"),
		errors.New("cannot achieve consistency level"),
		errors.New("quorum not available"),
		errors.New("insufficient replicas"),
		&gocql.RequestErrUnavailable{},
	}

	for _, err := range consistencyErrors {
		if !rp.IsConsistencyError(err) {
			t.Errorf("Expected %v to be classified as consistency error", err)
		}
		if !rp.isRetryableError(err) {
			t.Errorf("Expected %v to be retryable", err)
		}
		if rp.GetErrorCategory(err) != "consistency" {
			t.Errorf("Expected %v to be categorized as consistency", err)
		}
	}
}

// Test adaptive delay calculation
func TestRetryPolicy_AdaptiveDelayCalculation(t *testing.T) {
	rp := &RetryPolicy{
		numRetries:    3,
		minDelay:      100 * time.Millisecond,
		maxDelay:      1 * time.Second,
		backoffFactor: 2.0,
	}

	baseDelay := rp.calculateDelay(2)

	// Node failure should have longer delay
	nodeFailureErr := errors.New("host down")
	nodeDelay := rp.calculateDelayForError(2, nodeFailureErr)
	if nodeDelay <= baseDelay {
		t.Error("Expected longer delay for node failure errors")
	}

	// Consistency error should have shorter delay
	consistencyErr := errors.New("not enough replicas")
	consistencyDelay := rp.calculateDelayForError(2, consistencyErr)
	if consistencyDelay >= baseDelay {
		t.Error("Expected shorter delay for consistency errors")
	}
}

// Test non-retryable errors
func TestRetryPolicy_NonRetryableErrors(t *testing.T) {
	rp := &RetryPolicy{}

	nonRetryableErrors := []error{
		errors.New("syntax error in CQL"),
		errors.New("authentication failed"),
		errors.New("invalid query"),
		errors.New("keyspace does not exist"),
		errors.New("table not found"),
		errors.New("column does not exist"),
	}

	for _, err := range nonRetryableErrors {
		if rp.isRetryableError(err) {
			t.Errorf("Expected %v to be non-retryable", err)
		}
	}
}

// Mock implementation of gocql.RetryableQuery for testing
type mockRetryableQuery struct {
	attempts int
}

func (m *mockRetryableQuery) Attempts() int {
	return m.attempts
}

func (m *mockRetryableQuery) GetConsistency() gocql.Consistency {
	return gocql.Quorum // Mock consistency level
}
