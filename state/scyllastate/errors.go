package scyllastate

import (
	"errors"
	"fmt"
)

// Common errors for ScyllaDB state store operations
var (
	// ErrNotFound indicates a pin was not found
	ErrNotFound = errors.New("pin not found")

	// ErrInvalidCID indicates an invalid CID was provided
	ErrInvalidCID = errors.New("invalid CID")

	// ErrInvalidPeerID indicates an invalid peer ID was provided
	ErrInvalidPeerID = errors.New("invalid peer ID")

	// ErrInvalidOpID indicates an invalid operation ID was provided
	ErrInvalidOpID = errors.New("invalid operation ID")

	// ErrInvalidReplicationFactor indicates an invalid replication factor
	ErrInvalidReplicationFactor = errors.New("invalid replication factor")

	// ErrConnectionFailed indicates ScyllaDB connection failed
	ErrConnectionFailed = errors.New("ScyllaDB connection failed")

	// ErrTimeout indicates an operation timed out
	ErrTimeout = errors.New("operation timed out")

	// ErrUnavailable indicates ScyllaDB cluster is unavailable
	ErrUnavailable = errors.New("ScyllaDB cluster unavailable")

	// ErrConsistency indicates a consistency level could not be achieved
	ErrConsistency = errors.New("consistency level not achieved")

	// ErrBatchTooLarge indicates a batch operation is too large
	ErrBatchTooLarge = errors.New("batch operation too large")

	// ErrInvalidConfig indicates invalid configuration
	ErrInvalidConfig = errors.New("invalid configuration")
)

// ScyllaError wraps ScyllaDB-specific errors with additional context
type ScyllaError struct {
	Op      string // Operation that failed
	CID     string // CID involved (if applicable)
	PeerID  string // Peer ID involved (if applicable)
	Err     error  // Underlying error
	Retries int    // Number of retries attempted
}

func (e *ScyllaError) Error() string {
	if e.CID != "" && e.PeerID != "" {
		return fmt.Sprintf("scylla %s failed for CID %s on peer %s after %d retries: %v",
			e.Op, e.CID, e.PeerID, e.Retries, e.Err)
	} else if e.CID != "" {
		return fmt.Sprintf("scylla %s failed for CID %s after %d retries: %v",
			e.Op, e.CID, e.Retries, e.Err)
	} else if e.PeerID != "" {
		return fmt.Sprintf("scylla %s failed for peer %s after %d retries: %v",
			e.Op, e.PeerID, e.Retries, e.Err)
	}
	return fmt.Sprintf("scylla %s failed after %d retries: %v",
		e.Op, e.Retries, e.Err)
}

func (e *ScyllaError) Unwrap() error {
	return e.Err
}

// IsRetryable determines if an error is retryable
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for specific retryable errors
	switch {
	case errors.Is(err, ErrTimeout):
		return true
	case errors.Is(err, ErrUnavailable):
		return true
	case errors.Is(err, ErrConnectionFailed):
		return true
	default:
		return false
	}
}

// IsNotFound checks if an error indicates a not found condition
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// NewScyllaError creates a new ScyllaError with context
func NewScyllaError(op string, err error) *ScyllaError {
	return &ScyllaError{
		Op:  op,
		Err: err,
	}
}

// WithCID adds CID context to a ScyllaError
func (e *ScyllaError) WithCID(cid string) *ScyllaError {
	e.CID = cid
	return e
}

// WithPeerID adds peer ID context to a ScyllaError
func (e *ScyllaError) WithPeerID(peerID string) *ScyllaError {
	e.PeerID = peerID
	return e
}

// WithRetries adds retry count to a ScyllaError
func (e *ScyllaError) WithRetries(retries int) *ScyllaError {
	e.Retries = retries
	return e
}
