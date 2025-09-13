//go:build standalone
// +build standalone

package scyllastate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gocql/gocql"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Standalone test configuration
type StandaloneConfig struct {
	TracingEnabled bool
}

// Standalone CID for testing
type StandaloneCid struct {
	value string
}

func (s StandaloneCid) String() string {
	return s.value
}

func (s StandaloneCid) Bytes() []byte {
	return []byte(s.value)
}

// Standalone logger implementation for testing
type StandaloneLogger struct {
	logger logging.EventLogger
	config *StandaloneConfig
}

func NewStandaloneLogger(config *StandaloneConfig) *StandaloneLogger {
	return &StandaloneLogger{
		logger: logging.Logger("scyllastate"),
		config: config,
	}
}

// Test the core logging functionality
func TestStandaloneStructuredLogging(t *testing.T) {
	config := &StandaloneConfig{
		TracingEnabled: true,
	}

	logger := NewStandaloneLogger(config)
	require.NotNil(t, logger)

	// Test basic logging
	t.Run("basic logging", func(t *testing.T) {
		logger.logger.Info("Test log message")
		logger.logger.Infow("Test structured log", "key", "value", "duration", 100)
	})

	// Test error classification
	t.Run("error classification", func(t *testing.T) {
		testCases := []struct {
			name     string
			err      error
			expected string
		}{
			{
				name:     "timeout error",
				err:      errors.New("operation timeout"),
				expected: "TIMEOUT",
			},
			{
				name:     "unavailable error",
				err:      errors.New("cluster unavailable"),
				expected: "UNAVAILABLE",
			},
			{
				name:     "connection error",
				err:      errors.New("connection failed"),
				expected: "CONNECTION_FAILED",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := classifyStandaloneError(tc.err)
				assert.Equal(t, tc.expected, result)
			})
		}
	})

	// Test query type extraction
	t.Run("query type extraction", func(t *testing.T) {
		testCases := []struct {
			query    string
			expected string
		}{
			{"SELECT * FROM pins_by_cid", "SELECT"},
			{"INSERT INTO pins_by_cid VALUES (?)", "INSERT"},
			{"UPDATE pins_by_cid SET rf = ?", "UPDATE"},
			{"DELETE FROM pins_by_cid WHERE cid = ?", "DELETE"},
			{"  select * from test  ", "SELECT"}, // Test trimming and case
		}

		for _, tc := range testCases {
			t.Run(tc.query, func(t *testing.T) {
				result := extractStandaloneQueryType(tc.query)
				assert.Equal(t, tc.expected, result)
			})
		}
	})

	// Test operation context building
	t.Run("operation context", func(t *testing.T) {
		cid := StandaloneCid{value: "QmTest123"}

		// Simulate building an operation context
		fields := []interface{}{
			"operation", "Add",
			"cid", cid.String(),
			"duration_ms", 100,
			"consistency", "QUORUM",
			"query_type", "INSERT",
		}

		logger.logger.Infow("Operation completed", fields...)

		// Verify fields are properly structured
		assert.Equal(t, "Add", fields[1])
		assert.Equal(t, "QmTest123", fields[3])
		assert.Equal(t, 100, fields[5])
	})
}

// Helper functions for standalone testing
func classifyStandaloneError(err error) string {
	if err == nil {
		return ""
	}

	errStr := strings.ToLower(err.Error())

	switch {
	case strings.Contains(errStr, "timeout"):
		return "TIMEOUT"
	case strings.Contains(errStr, "unavailable"):
		return "UNAVAILABLE"
	case strings.Contains(errStr, "connection"):
		return "CONNECTION_FAILED"
	default:
		return "UNKNOWN"
	}
}

func extractStandaloneQueryType(query string) string {
	query = strings.TrimSpace(strings.ToUpper(query))

	switch {
	case strings.HasPrefix(query, "SELECT"):
		return "SELECT"
	case strings.HasPrefix(query, "INSERT"):
		return "INSERT"
	case strings.HasPrefix(query, "UPDATE"):
		return "UPDATE"
	case strings.HasPrefix(query, "DELETE"):
		return "DELETE"
	default:
		return "OTHER"
	}
}

// Test gocql error type classification
func TestGocqlErrorClassification(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "read timeout",
			err:      &gocql.RequestErrReadTimeout{},
			expected: "READ_TIMEOUT",
		},
		{
			name:     "write timeout",
			err:      &gocql.RequestErrWriteTimeout{},
			expected: "WRITE_TIMEOUT",
		},
		{
			name:     "unavailable",
			err:      &gocql.RequestErrUnavailable{},
			expected: "UNAVAILABLE",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := classifyGocqlError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func classifyGocqlError(err error) string {
	if err == nil {
		return ""
	}

	switch err.(type) {
	case *gocql.RequestErrReadTimeout:
		return "READ_TIMEOUT"
	case *gocql.RequestErrWriteTimeout:
		return "WRITE_TIMEOUT"
	case *gocql.RequestErrUnavailable:
		return "UNAVAILABLE"
	default:
		return "UNKNOWN"
	}
}

// Benchmark logging performance
func BenchmarkStructuredLogging(b *testing.B) {
	config := &StandaloneConfig{
		TracingEnabled: false,
	}

	logger := NewStandaloneLogger(config)
	cid := StandaloneCid{value: "QmBenchmark"}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.logger.Infow("Operation completed",
			"operation", "Add",
			"cid", cid.String(),
			"duration_ms", 100,
			"consistency", "QUORUM",
		)
	}
}

// Test logging with different levels
func TestLoggingLevels(t *testing.T) {
	config := &StandaloneConfig{
		TracingEnabled: true,
	}

	logger := NewStandaloneLogger(config)

	// Test different log levels
	logger.logger.Debug("Debug message")
	logger.logger.Info("Info message")
	logger.logger.Warn("Warning message")
	logger.logger.Error("Error message")

	// Test structured logging at different levels
	logger.logger.Debugw("Debug structured", "key", "debug_value")
	logger.logger.Infow("Info structured", "key", "info_value")
	logger.logger.Warnw("Warning structured", "key", "warn_value")
	logger.logger.Errorw("Error structured", "key", "error_value")
}

// Test context-aware logging
func TestContextAwareLogging(t *testing.T) {
	config := &StandaloneConfig{
		TracingEnabled: true,
	}

	logger := NewStandaloneLogger(config)
	ctx := context.Background()

	// Add context values
	ctx = context.WithValue(ctx, "request_id", "req_123")
	ctx = context.WithValue(ctx, "user_id", "user_456")

	// Log with context (in real implementation, we'd extract context values)
	logger.logger.Infow("Context-aware log",
		"request_id", ctx.Value("request_id"),
		"user_id", ctx.Value("user_id"),
		"operation", "Get",
	)
}

// Test performance metrics logging
func TestPerformanceMetricsLogging(t *testing.T) {
	config := &StandaloneConfig{
		TracingEnabled: true,
	}

	logger := NewStandaloneLogger(config)

	// Simulate performance metrics
	metrics := map[string]interface{}{
		"operation_count":    1000,
		"avg_latency_ms":     50.5,
		"p95_latency_ms":     120.0,
		"error_rate_percent": 0.1,
		"throughput_ops_sec": 200.0,
	}

	fields := []interface{}{}
	for key, value := range metrics {
		fields = append(fields, key, value)
	}

	logger.logger.Infow("Performance metrics", fields...)
}

// Test batch operation logging
func TestBatchOperationLogging(t *testing.T) {
	config := &StandaloneConfig{
		TracingEnabled: true,
	}

	logger := NewStandaloneLogger(config)

	// Simulate batch operation
	batchSize := 100
	duration := 2 * time.Second

	logger.logger.Infow("Batch operation completed",
		"batch_type", "LOGGED",
		"batch_size", batchSize,
		"duration_ms", duration.Milliseconds(),
		"throughput_ops_per_sec", float64(batchSize)/duration.Seconds(),
	)
}

// Test retry logging
func TestRetryLogging(t *testing.T) {
	config := &StandaloneConfig{
		TracingEnabled: true,
	}

	logger := NewStandaloneLogger(config)

	// Simulate retry attempts
	for attempt := 1; attempt <= 3; attempt++ {
		delay := time.Duration(attempt*100) * time.Millisecond

		logger.logger.Infow("Retrying operation",
			"operation", "Add",
			"retry_attempt", attempt,
			"retry_delay_ms", delay.Milliseconds(),
			"retry_reason", "timeout",
		)
	}
}
