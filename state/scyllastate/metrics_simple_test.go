package scyllastate

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMetricsInterface tests that PrometheusMetrics implements the Metrics interface
func TestMetricsInterface(t *testing.T) {
	var _ Metrics = (*PrometheusMetrics)(nil)

	// This test ensures that PrometheusMetrics implements all required methods
	// of the Metrics interface at compile time
	assert.True(t, true, "PrometheusMetrics implements Metrics interface")
}

// TestMetricsCreation tests basic metrics creation
func TestMetricsCreation(t *testing.T) {
	metrics := NewPrometheusMetrics()
	require.NotNil(t, metrics, "Metrics should be created successfully")

	// Test that we can call methods without panicking
	assert.NotPanics(t, func() {
		metrics.RecordOperation("test", time.Millisecond, "QUORUM", nil)
	})

	assert.NotPanics(t, func() {
		metrics.RecordQuery("SELECT", "test_keyspace", time.Millisecond)
	})

	assert.NotPanics(t, func() {
		metrics.UpdateStateMetrics(100)
	})
}

// TestErrorClassification tests error classification functions
func TestErrorClassification(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType string
	}{
		{
			name:         "timeout error",
			err:          errors.New("connection timeout"),
			expectedType: "timeout",
		},
		{
			name:         "unavailable error",
			err:          errors.New("service unavailable"),
			expectedType: "unavailable",
		},
		{
			name:         "connection error",
			err:          errors.New("connection refused"),
			expectedType: "connection",
		},
		{
			name:         "nil error",
			err:          nil,
			expectedType: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)
			assert.Equal(t, tt.expectedType, result)
		})
	}
}

// TestTimeoutErrorClassification tests timeout-specific error classification
func TestTimeoutErrorClassification(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedType string
	}{
		{
			name:         "read timeout",
			err:          errors.New("read timeout occurred"),
			expectedType: "read_timeout",
		},
		{
			name:         "write timeout",
			err:          errors.New("write timeout occurred"),
			expectedType: "write_timeout",
		},
		{
			name:         "general timeout",
			err:          errors.New("timeout"),
			expectedType: "general_timeout",
		},
		{
			name:         "nil error",
			err:          nil,
			expectedType: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyTimeoutError(tt.err)
			assert.Equal(t, tt.expectedType, result)
		})
	}
}

// TestConsistencyLevelEncoding tests consistency level encoding
func TestConsistencyLevelEncoding(t *testing.T) {
	tests := []struct {
		level    string
		expected int
	}{
		{"ANY", 0},
		{"ONE", 1},
		{"QUORUM", 4},
		{"ALL", 5},
		{"LOCAL_QUORUM", 6},
		{"UNKNOWN_LEVEL", -1},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			result := encodeConsistencyLevel(tt.level)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStringContains tests the contains helper function
func TestStringContains(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected bool
	}{
		{
			name:     "contains at start",
			s:        "timeout error",
			substr:   "timeout",
			expected: true,
		},
		{
			name:     "contains at end",
			s:        "connection timeout",
			substr:   "timeout",
			expected: true,
		},
		{
			name:     "contains in middle",
			s:        "read timeout error",
			substr:   "timeout",
			expected: true,
		},
		{
			name:     "does not contain",
			s:        "connection error",
			substr:   "timeout",
			expected: false,
		},
		{
			name:     "exact match",
			s:        "timeout",
			substr:   "timeout",
			expected: true,
		},
		{
			name:     "empty substring",
			s:        "any string",
			substr:   "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.s, tt.substr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMetricsMethodsDoNotPanic tests that all metrics methods handle nil gracefully
func TestMetricsMethodsDoNotPanic(t *testing.T) {
	metrics := NewPrometheusMetrics()

	// Test all methods don't panic with various inputs
	assert.NotPanics(t, func() {
		metrics.RecordOperation("test", time.Second, "QUORUM", errors.New("test error"))
	})

	assert.NotPanics(t, func() {
		metrics.RecordQuery("INSERT", "keyspace", time.Millisecond*100)
	})

	assert.NotPanics(t, func() {
		metrics.RecordRetry("operation", "timeout")
	})

	assert.NotPanics(t, func() {
		metrics.RecordBatchOperation("LOGGED", 50, nil)
	})

	assert.NotPanics(t, func() {
		metrics.RecordPinOperation("pin")
	})

	assert.NotPanics(t, func() {
		metrics.UpdateConnectionMetrics(10, map[string]bool{"host1": true})
	})

	assert.NotPanics(t, func() {
		metrics.UpdateStateMetrics(1000)
	})

	assert.NotPanics(t, func() {
		metrics.UpdatePreparedStatements(25)
	})

	assert.NotPanics(t, func() {
		metrics.UpdateConsistencyLevel("read", "QUORUM")
	})

	assert.NotPanics(t, func() {
		metrics.UpdateDegradationMetrics(true, 1, "consistency")
	})

	assert.NotPanics(t, func() {
		metrics.UpdateNodeHealth("host1", "dc1", "rack1", true)
	})

	assert.NotPanics(t, func() {
		metrics.UpdatePartitionStatus(false)
	})

	assert.NotPanics(t, func() {
		metrics.RecordConnectionError()
	})
}

// BenchmarkMetricsOperations benchmarks key metrics operations
func BenchmarkMetricsOperations(b *testing.B) {
	metrics := NewPrometheusMetrics()

	b.Run("RecordOperation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			metrics.RecordOperation("benchmark", time.Microsecond*100, "QUORUM", nil)
		}
	})

	b.Run("RecordQuery", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			metrics.RecordQuery("SELECT", "keyspace", time.Microsecond*50)
		}
	})

	b.Run("ClassifyError", func(b *testing.B) {
		err := errors.New("connection timeout")
		for i := 0; i < b.N; i++ {
			classifyError(err)
		}
	})
}
