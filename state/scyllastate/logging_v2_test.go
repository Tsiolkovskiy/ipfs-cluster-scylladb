package scyllastate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockCid for testing
type MockCid struct {
	value string
}

func (m MockCid) String() string {
	return m.value
}

func (m MockCid) Bytes() []byte {
	return []byte(m.value)
}

func TestStructuredLogger_V2_Creation(t *testing.T) {
	config := &LoggingConfig{
		TracingEnabled: true,
	}

	logger := NewStructuredLogger(config)
	require.NotNil(t, logger)
	assert.Equal(t, config, logger.config)
}

func TestStructuredLogger_V2_LogOperation_Success(t *testing.T) {
	config := &LoggingConfig{
		TracingEnabled: true,
	}

	logger := NewStructuredLogger(config)
	ctx := context.Background()
	cid := MockCid{value: "QmTest123"}

	opCtx := NewOperationContext("Add").
		WithCID(cid).
		WithDuration(100*time.Millisecond).
		WithConsistency("QUORUM").
		WithQueryType("INSERT").
		WithMetadata("test_key", "test_value")

	// This should not panic and should log at appropriate level
	logger.LogOperation(ctx, opCtx)
}

func TestStructuredLogger_V2_LogOperation_Error(t *testing.T) {
	config := &LoggingConfig{
		TracingEnabled: true,
	}

	logger := NewStructuredLogger(config)
	ctx := context.Background()
	cid := MockCid{value: "QmTest456"}
	testErr := errors.New("timeout error")

	opCtx := NewOperationContext("Get").
		WithCID(cid).
		WithDuration(5 * time.Second).
		WithError(testErr).
		WithRetries(3).
		WithConsistency("LOCAL_QUORUM")

	// This should not panic and should log error
	logger.LogOperation(ctx, opCtx)
}

func TestStructuredLogger_V2_LogQuery(t *testing.T) {
	config := &LoggingConfig{
		TracingEnabled: true,
	}

	logger := NewStructuredLogger(config)
	ctx := context.Background()

	// Test successful query logging
	t.Run("successful query", func(t *testing.T) {
		query := "SELECT * FROM pins_by_cid WHERE mh_prefix = ? AND cid_bin = ?"
		args := []interface{}{int16(123), []byte("test")}
		duration := 50 * time.Millisecond

		logger.LogQuery(ctx, query, args, duration, nil)
	})

	// Test failed query logging
	t.Run("failed query", func(t *testing.T) {
		query := "INSERT INTO pins_by_cid VALUES (?, ?, ?)"
		args := []interface{}{int16(456), []byte("test2")}
		duration := 2 * time.Second
		err := errors.New("write timeout")

		logger.LogQuery(ctx, query, args, duration, err)
	})
}

func TestStructuredLogger_V2_ClassifyError(t *testing.T) {
	config := &LoggingConfig{}
	logger := NewStructuredLogger(config)

	testCases := []struct {
		name     string
		err      error
		expected ScyllaErrorCode
	}{
		{
			name:     "timeout error",
			err:      errors.New("operation timeout"),
			expected: ErrorCodeTimeout,
		},
		{
			name:     "unavailable error",
			err:      errors.New("cluster unavailable"),
			expected: ErrorCodeUnavailable,
		},
		{
			name:     "connection error",
			err:      errors.New("connection failed"),
			expected: ErrorCodeConnectionFailed,
		},
		{
			name:     "unauthorized error",
			err:      errors.New("authentication failed"),
			expected: ErrorCodeUnauthorized,
		},
		{
			name:     "syntax error",
			err:      errors.New("invalid syntax"),
			expected: ErrorCodeSyntax,
		},
		{
			name:     "unknown error",
			err:      errors.New("some random error"),
			expected: ErrorCodeUnknown,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := logger.classifyError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestStructuredLogger_V2_ExtractQueryType(t *testing.T) {
	config := &LoggingConfig{}
	logger := NewStructuredLogger(config)

	testCases := []struct {
		query    string
		expected string
	}{
		{"SELECT * FROM pins_by_cid", "SELECT"},
		{"INSERT INTO pins_by_cid VALUES (?)", "INSERT"},
		{"UPDATE pins_by_cid SET rf = ?", "UPDATE"},
		{"DELETE FROM pins_by_cid WHERE cid = ?", "DELETE"},
		{"CREATE TABLE test (id int)", "CREATE"},
		{"DROP TABLE test", "DROP"},
		{"ALTER TABLE test ADD COLUMN", "ALTER"},
		{"TRUNCATE TABLE test", "TRUNCATE"},
		{"BEGIN BATCH", "BATCH"},
		{"  select * from test  ", "SELECT"}, // Test trimming and case
		{"UNKNOWN QUERY TYPE", "OTHER"},
	}

	for _, tc := range testCases {
		t.Run(tc.query, func(t *testing.T) {
			result := logger.extractQueryType(tc.query)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestStructuredLogger_V2_GocqlErrorClassification(t *testing.T) {
	config := &LoggingConfig{}
	logger := NewStructuredLogger(config)

	// Test gocql-specific error types
	testCases := []struct {
		name     string
		err      error
		expected ScyllaErrorCode
	}{
		{
			name:     "read timeout",
			err:      &gocql.RequestErrReadTimeout{},
			expected: ErrorCodeReadTimeout,
		},
		{
			name:     "write timeout",
			err:      &gocql.RequestErrWriteTimeout{},
			expected: ErrorCodeWriteTimeout,
		},
		{
			name:     "unavailable",
			err:      &gocql.RequestErrUnavailable{},
			expected: ErrorCodeUnavailable,
		},
		{
			name:     "already exists",
			err:      &gocql.RequestErrAlreadyExists{},
			expected: ErrorCodeAlreadyExists,
		},
		{
			name:     "unprepared",
			err:      &gocql.RequestErrUnprepared{},
			expected: ErrorCodeUnprepared,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := logger.classifyError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestOperationContext_V2_Builders(t *testing.T) {
	cid := MockCid{value: "QmTestBuilder"}

	opCtx := NewOperationContext("TestOp").
		WithCID(cid).
		WithPeerID("peer123").
		WithDuration(100*time.Millisecond).
		WithError(errors.New("test error")).
		WithRetries(3).
		WithConsistency("QUORUM").
		WithQueryType("SELECT").
		WithBatchSize(50).
		WithRecordCount(1000).
		WithMetadata("custom_key", "custom_value")

	assert.Equal(t, "TestOp", opCtx.Operation)
	assert.Equal(t, cid, opCtx.CID)
	assert.Equal(t, "peer123", opCtx.PeerID)
	assert.Equal(t, 100*time.Millisecond, opCtx.Duration)
	assert.Equal(t, "test error", opCtx.Error.Error())
	assert.Equal(t, 3, opCtx.Retries)
	assert.Equal(t, "QUORUM", opCtx.Consistency)
	assert.Equal(t, "SELECT", opCtx.QueryType)
	assert.Equal(t, 50, opCtx.BatchSize)
	assert.Equal(t, int64(1000), opCtx.RecordCount)
	assert.Equal(t, "custom_value", opCtx.Metadata["custom_key"])
}

func TestStructuredLogger_V2_LogRetry(t *testing.T) {
	config := &LoggingConfig{}
	logger := NewStructuredLogger(config)

	lastError := errors.New("connection failed")

	logger.LogRetry("Add", 2, 500*time.Millisecond, lastError, "connection_timeout")
}

func TestStructuredLogger_V2_LogBatchOperation(t *testing.T) {
	config := &LoggingConfig{}
	logger := NewStructuredLogger(config)

	// Test successful batch
	t.Run("successful batch", func(t *testing.T) {
		logger.LogBatchOperation("LOGGED", 100, 2*time.Second, nil)
	})

	// Test failed batch
	t.Run("failed batch", func(t *testing.T) {
		err := errors.New("batch too large")
		logger.LogBatchOperation("UNLOGGED", 50, 1*time.Second, err)
	})
}

func TestStructuredLogger_V2_LogConnectionEvent(t *testing.T) {
	config := &LoggingConfig{}
	logger := NewStructuredLogger(config)

	details := map[string]interface{}{
		"duration_ms": 100,
		"retry_count": 2,
	}

	// Test different event types
	testCases := []string{
		"connection_established",
		"connection_lost",
		"connection_failed",
		"node_down",
	}

	for _, eventType := range testCases {
		t.Run(eventType, func(t *testing.T) {
			logger.LogConnectionEvent(eventType, "192.168.1.100", "datacenter1", details)
		})
	}
}

func TestStructuredLogger_V2_FormatFields(t *testing.T) {
	config := &LoggingConfig{}
	logger := NewStructuredLogger(config)

	fields := map[string]interface{}{
		"operation":   "Add",
		"duration_ms": 100,
		"cid":         "QmTest123",
		"retries":     3,
	}

	result := logger.formatFields(fields)

	// Check that all fields are present in the result
	assert.Contains(t, result, "operation=Add")
	assert.Contains(t, result, "duration_ms=100")
	assert.Contains(t, result, "cid=QmTest123")
	assert.Contains(t, result, "retries=3")
}

func TestStructuredLogger_V2_SanitizeQuery(t *testing.T) {
	config := &LoggingConfig{}
	logger := NewStructuredLogger(config)

	testCases := []struct {
		name     string
		query    string
		expected string
	}{
		{
			name:     "simple query",
			query:    "SELECT * FROM pins_by_cid",
			expected: "SELECT * FROM pins_by_cid",
		},
		{
			name:     "query with string literals",
			query:    "SELECT * FROM pins WHERE owner = 'user123'",
			expected: "SELECT * FROM pins WHERE owner = '***'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := logger.sanitizeQuery(tc.query)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestStructuredLogger_V2_SanitizeArgs(t *testing.T) {
	config := &LoggingConfig{}
	logger := NewStructuredLogger(config)

	args := []interface{}{
		"short",
		"this_is_a_long_string_value",
		[]byte{1, 2, 3, 4, 5},
		123,
		true,
		nil,
	}

	result := logger.sanitizeArgs(args)

	assert.Equal(t, "***", result[0])           // short string
	assert.Equal(t, "thi***lue", result[1])     // long string
	assert.Equal(t, "[]byte{len=5}", result[2]) // byte slice
	assert.Equal(t, 123, result[3])             // int (unchanged)
	assert.Equal(t, true, result[4])            // bool (unchanged)
	assert.Equal(t, nil, result[5])             // nil (unchanged)
}
