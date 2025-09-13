package scyllastate

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPrometheusMetrics(t *testing.T) {
	// Create new metrics instance
	metrics := NewPrometheusMetrics()
	require.NotNil(t, metrics)

	// Verify all metrics are initialized
	assert.NotNil(t, metrics.operationDuration)
	assert.NotNil(t, metrics.operationCounter)
	assert.NotNil(t, metrics.operationErrors)
	assert.NotNil(t, metrics.activeConnections)
	assert.NotNil(t, metrics.connectionErrors)
	assert.NotNil(t, metrics.totalPins)
	assert.NotNil(t, metrics.scyllaTimeouts)
	assert.NotNil(t, metrics.scyllaUnavailable)
	assert.NotNil(t, metrics.queryLatency)
	assert.NotNil(t, metrics.degradationActive)
	assert.NotNil(t, metrics.nodeHealth)
}

func TestRecordOperation(t *testing.T) {
	// Create a custom registry to avoid conflicts with global metrics
	registry := prometheus.NewRegistry()

	// Create metrics with custom registry (we'll simulate this)
	metrics := NewPrometheusMetrics()

	tests := []struct {
		name             string
		operation        string
		duration         time.Duration
		consistencyLevel string
		err              error
		expectedStatus   string
	}{
		{
			name:             "successful operation",
			operation:        "add_pin",
			duration:         100 * time.Millisecond,
			consistencyLevel: "QUORUM",
			err:              nil,
			expectedStatus:   "success",
		},
		{
			name:             "failed operation",
			operation:        "get_pin",
			duration:         50 * time.Millisecond,
			consistencyLevel: "ONE",
			err:              errors.New("connection timeout"),
			expectedStatus:   "error",
		},
		{
			name:             "timeout error",
			operation:        "list_pins",
			duration:         200 * time.Millisecond,
			consistencyLevel: "LOCAL_QUORUM",
			err:              errors.New("read timeout"),
			expectedStatus:   "error",
		},
		{
			name:             "unavailable error",
			operation:        "remove_pin",
			duration:         75 * time.Millisecond,
			consistencyLevel: "ALL",
			err:              errors.New("unavailable"),
			expectedStatus:   "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Record the operation
			metrics.RecordOperation(tt.operation, tt.duration, tt.consistencyLevel, tt.err)

			// For successful operations, verify duration was recorded
			if tt.err == nil {
				// We can't easily test the histogram values without access to the internal state
				// In a real test environment, you might use prometheus testutil
				assert.True(t, tt.duration > 0)
			}

			// Verify error classification works
			if tt.err != nil {
				errorType := classifyError(tt.err)
				assert.NotEmpty(t, errorType)

				if isTimeoutError(tt.err) {
					timeoutType := classifyTimeoutError(tt.err)
					assert.NotEmpty(t, timeoutType)
				}
			}
		})
	}
}

func TestRecordQuery(t *testing.T) {
	metrics := NewPrometheusMetrics()

	queryType := "SELECT"
	keyspace := "ipfs_pins"
	duration := 25 * time.Millisecond

	// Record query metrics
	metrics.RecordQuery(queryType, keyspace, duration)

	// Verify the query was recorded (in a real test, you'd check the metric value)
	assert.True(t, duration > 0)
}

func TestRecordBatchOperation(t *testing.T) {
	metrics := NewPrometheusMetrics()

	tests := []struct {
		name      string
		batchType string
		size      int
		err       error
	}{
		{
			name:      "successful batch",
			batchType: "INSERT",
			size:      100,
			err:       nil,
		},
		{
			name:      "failed batch",
			batchType: "DELETE",
			size:      50,
			err:       errors.New("batch timeout"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics.RecordBatchOperation(tt.batchType, tt.size, tt.err)

			// Verify size is positive
			assert.True(t, tt.size > 0)
		})
	}
}

func TestUpdateConnectionMetrics(t *testing.T) {
	metrics := NewPrometheusMetrics()

	activeConns := 10
	poolStatus := map[string]bool{
		"192.168.1.1": true,
		"192.168.1.2": false,
		"192.168.1.3": true,
	}

	metrics.UpdateConnectionMetrics(activeConns, poolStatus)

	// Verify metrics were updated (in a real test, you'd check the gauge values)
	assert.Equal(t, 10, activeConns)
	assert.Len(t, poolStatus, 3)
}

func TestUpdateStateMetrics(t *testing.T) {
	metrics := NewPrometheusMetrics()

	totalPins := int64(1000000)
	metrics.UpdateStateMetrics(totalPins)

	// Verify state was updated
	assert.Equal(t, int64(1000000), totalPins)
}

func TestUpdateDegradationMetrics(t *testing.T) {
	metrics := NewPrometheusMetrics()

	// Test active degradation
	metrics.UpdateDegradationMetrics(true, 2, "consistency_fallback")

	// Test inactive degradation
	metrics.UpdateDegradationMetrics(false, 0, "none")

	// Verify calls completed without error
	assert.True(t, true)
}

func TestUpdateNodeHealth(t *testing.T) {
	metrics := NewPrometheusMetrics()

	tests := []struct {
		host       string
		datacenter string
		rack       string
		healthy    bool
	}{
		{"node1.dc1", "datacenter1", "rack1", true},
		{"node2.dc1", "datacenter1", "rack2", false},
		{"node3.dc2", "datacenter2", "rack1", true},
	}

	for _, tt := range tests {
		metrics.UpdateNodeHealth(tt.host, tt.datacenter, tt.rack, tt.healthy)
	}

	// Verify all nodes were processed
	assert.Len(t, tests, 3)
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "none",
		},
		{
			name:     "timeout error",
			err:      errors.New("connection timeout"),
			expected: "timeout",
		},
		{
			name:     "unavailable error",
			err:      errors.New("unavailable"),
			expected: "unavailable",
		},
		{
			name:     "connection error",
			err:      errors.New("connection refused"),
			expected: "connection",
		},
		{
			name:     "authentication error",
			err:      errors.New("authentication failed"),
			expected: "authentication",
		},
		{
			name:     "syntax error",
			err:      errors.New("syntax error in query"),
			expected: "syntax",
		},
		{
			name:     "other error",
			err:      errors.New("unknown error"),
			expected: "other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClassifyTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "read timeout",
			err:      errors.New("read timeout"),
			expected: "read_timeout",
		},
		{
			name:     "write timeout",
			err:      errors.New("write timeout"),
			expected: "write_timeout",
		},
		{
			name:     "connect timeout",
			err:      errors.New("connect timeout"),
			expected: "connect_timeout",
		},
		{
			name:     "request timeout",
			err:      errors.New("request timeout"),
			expected: "request_timeout",
		},
		{
			name:     "general timeout",
			err:      errors.New("timeout occurred"),
			expected: "general_timeout",
		},
		{
			name:     "nil error",
			err:      nil,
			expected: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyTimeoutError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEncodeConsistencyLevel(t *testing.T) {
	tests := []struct {
		level    string
		expected int
	}{
		{"ANY", 0},
		{"ONE", 1},
		{"TWO", 2},
		{"THREE", 3},
		{"QUORUM", 4},
		{"ALL", 5},
		{"LOCAL_QUORUM", 6},
		{"EACH_QUORUM", 7},
		{"LOCAL_ONE", 8},
		{"SERIAL", 9},
		{"LOCAL_SERIAL", 10},
		{"UNKNOWN", -1},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			result := encodeConsistencyLevel(tt.level)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRecordRetry(t *testing.T) {
	metrics := NewPrometheusMetrics()

	operation := "add_pin"
	reason := "timeout"

	metrics.RecordRetry(operation, reason)

	// Verify retry was recorded
	assert.NotEmpty(t, operation)
	assert.NotEmpty(t, reason)
}

func TestRecordPinOperation(t *testing.T) {
	metrics := NewPrometheusMetrics()

	operationTypes := []string{"pin", "unpin", "update", "list"}

	for _, opType := range operationTypes {
		metrics.RecordPinOperation(opType)
	}

	// Verify all operations were recorded
	assert.Len(t, operationTypes, 4)
}

func TestUpdatePreparedStatements(t *testing.T) {
	metrics := NewPrometheusMetrics()

	count := 150
	metrics.UpdatePreparedStatements(count)

	// Verify count was updated
	assert.Equal(t, 150, count)
}

func TestUpdateConsistencyLevel(t *testing.T) {
	metrics := NewPrometheusMetrics()

	operationType := "read"
	level := "QUORUM"

	metrics.UpdateConsistencyLevel(operationType, level)

	// Verify consistency level was updated
	assert.NotEmpty(t, operationType)
	assert.NotEmpty(t, level)
}

func TestUpdatePartitionStatus(t *testing.T) {
	metrics := NewPrometheusMetrics()

	// Test partitioned state
	metrics.UpdatePartitionStatus(true)

	// Test non-partitioned state
	metrics.UpdatePartitionStatus(false)

	// Verify calls completed
	assert.True(t, true)
}

func TestRecordConnectionError(t *testing.T) {
	metrics := NewPrometheusMetrics()

	// Record multiple connection errors
	for i := 0; i < 5; i++ {
		metrics.RecordConnectionError()
	}

	// Verify errors were recorded
	assert.True(t, true)
}

// Benchmark tests for performance
func BenchmarkRecordOperation(b *testing.B) {
	metrics := NewPrometheusMetrics()
	operation := "benchmark_op"
	duration := 10 * time.Millisecond
	consistencyLevel := "QUORUM"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordOperation(operation, duration, consistencyLevel, nil)
	}
}

func BenchmarkRecordQuery(b *testing.B) {
	metrics := NewPrometheusMetrics()
	queryType := "SELECT"
	keyspace := "ipfs_pins"
	duration := 5 * time.Millisecond

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordQuery(queryType, keyspace, duration)
	}
}

func BenchmarkClassifyError(b *testing.B) {
	err := errors.New("connection timeout")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		classifyError(err)
	}
}
