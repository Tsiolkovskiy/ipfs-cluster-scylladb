package scyllastate

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestScyllaState_Marshal tests the Marshal method
func TestScyllaState_Marshal(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)

	// Setup mock expectations for listing pins
	mockQuery := &MockQuery{}
	mockIter := &MockIter{}

	mockSession.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(mockQuery)
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery)
	mockQuery.On("Consistency", mock.Anything).Return(mockQuery)
	mockQuery.On("SerialConsistency", mock.Anything).Return(mockQuery)
	mockQuery.On("PageSize", mock.Anything).Return(mockQuery)
	mockQuery.On("Idempotent", mock.Anything).Return(mockQuery)
	mockQuery.On("Iter").Return(mockIter)
	mockQuery.On("Release").Return()

	// Mock iterator behavior - return false to indicate no more rows
	mockIter.On("Scan", mock.Anything).Return(false)
	mockIter.On("Close").Return(nil)

	mockMetrics.On("RecordOperation", "marshal", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)

	// Execute the test
	var buf bytes.Buffer
	err := state.Marshal(&buf)

	// Verify results
	assert.NoError(t, err)
	assert.NotEmpty(t, buf.Bytes())
	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

// TestScyllaState_Marshal_Closed tests Marshal when state is closed
func TestScyllaState_Marshal_Closed(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)

	// Close the state
	mockSession.On("Close").Return()
	mockMetrics.On("UpdateConnectionMetrics", 0, mock.AnythingOfType("map[string]bool"))
	state.Close()

	// Execute the test
	var buf bytes.Buffer
	err := state.Marshal(&buf)

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ScyllaState is closed")
}

// TestScyllaState_Unmarshal tests the Unmarshal method
func TestScyllaState_Unmarshal(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)

	// Create test data - empty JSON array for pins
	testData := `[]`
	reader := strings.NewReader(testData)

	mockMetrics.On("RecordOperation", "unmarshal", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)

	// Execute the test
	err := state.Unmarshal(reader)

	// Verify results
	assert.NoError(t, err)
	mockMetrics.AssertExpectations(t)
}

// TestScyllaState_Unmarshal_InvalidJSON tests Unmarshal with invalid JSON
func TestScyllaState_Unmarshal_InvalidJSON(t *testing.T) {
	state, _, mockMetrics := createTestScyllaState(t)

	// Create invalid JSON data
	testData := `{invalid json}`
	reader := strings.NewReader(testData)

	mockMetrics.On("RecordOperation", "unmarshal", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), mock.AnythingOfType("*errors.errorString"))

	// Execute the test
	err := state.Unmarshal(reader)

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error unmarshaling")
	mockMetrics.AssertExpectations(t)
}

// TestScyllaState_Migrate tests the Migrate method
func TestScyllaState_Migrate(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)

	// Create test migration data - empty for now
	testData := `[]`
	reader := strings.NewReader(testData)

	mockMetrics.On("RecordOperation", "migrate", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)

	// Execute the test
	ctx := context.Background()
	err := state.Migrate(ctx, reader)

	// Verify results
	assert.NoError(t, err)
	mockMetrics.AssertExpectations(t)
}

// TestScyllaState_Migrate_Closed tests Migrate when state is closed
func TestScyllaState_Migrate_Closed(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)

	// Close the state
	mockSession.On("Close").Return()
	mockMetrics.On("UpdateConnectionMetrics", 0, mock.AnythingOfType("map[string]bool"))
	state.Close()

	// Execute the test
	testData := `[]`
	reader := strings.NewReader(testData)
	ctx := context.Background()
	err := state.Migrate(ctx, reader)

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ScyllaState is closed")
}

// TestScyllaState_GetPinCount tests the GetPinCount method
func TestScyllaState_GetPinCount(t *testing.T) {
	state, _, _ := createTestScyllaState(t)

	// Initial count should be 0
	count := state.GetPinCount()
	assert.Equal(t, int64(0), count)

	// Test addPinCount
	newCount := state.addPinCount(5)
	assert.Equal(t, int64(5), newCount)

	// Verify GetPinCount returns updated value
	count = state.GetPinCount()
	assert.Equal(t, int64(5), count)
}

// TestScyllaState_GetDegradationMetrics tests graceful degradation metrics
func TestScyllaState_GetDegradationMetrics(t *testing.T) {
	state, _, _ := createTestScyllaState(t)

	// Test when degradation manager is nil
	metrics := state.GetDegradationMetrics()
	assert.NotNil(t, metrics)
}

// TestScyllaState_GetNodeHealth tests node health status
func TestScyllaState_GetNodeHealth(t *testing.T) {
	state, _, _ := createTestScyllaState(t)

	// Test when degradation manager is nil
	health := state.GetNodeHealth()
	assert.NotNil(t, health)
	assert.Empty(t, health)
}

// TestScyllaState_IsPartitioned tests partition detection
func TestScyllaState_IsPartitioned(t *testing.T) {
	state, _, _ := createTestScyllaState(t)

	// Test when degradation manager is nil
	partitioned := state.IsPartitioned()
	assert.False(t, partitioned)
}

// TestScyllaState_GetCurrentConsistencyLevel tests consistency level retrieval
func TestScyllaState_GetCurrentConsistencyLevel(t *testing.T) {
	state, _, _ := createTestScyllaState(t)

	// Test when degradation manager is nil
	level := state.GetCurrentConsistencyLevel()
	assert.Equal(t, state.config.GetConsistency(), level)
}

// TestScyllaState_ValidateSerializedPin tests pin validation
func TestScyllaState_ValidateSerializedPin(t *testing.T) {
	state, _, _ := createTestScyllaState(t)
	pin := createTestPin(t)

	// Serialize a valid pin
	data, err := state.serializePin(pin)
	require.NoError(t, err)

	// Test validation with valid data
	err = state.validateSerializedPin(pin.Cid, data)
	assert.NoError(t, err)

	// Test validation with invalid data
	err = state.validateSerializedPin(pin.Cid, []byte("invalid"))
	assert.Error(t, err)
}

// TestScyllaState_MigrateSerializedPin tests pin migration
func TestScyllaState_MigrateSerializedPin(t *testing.T) {
	state, _, _ := createTestScyllaState(t)
	pin := createTestPin(t)

	// Serialize a pin (this will be our "old" format)
	oldData, err := state.serializePin(pin)
	require.NoError(t, err)

	// Test migration
	newData, err := state.migrateSerializedPin(pin.Cid, oldData)
	assert.NoError(t, err)
	assert.NotEmpty(t, newData)

	// Verify the migrated data can be deserialized
	migratedPin, err := state.deserializePin(pin.Cid, newData)
	assert.NoError(t, err)
	assert.Equal(t, pin.Cid, migratedPin.Cid)
}

// TestScyllaState_GetPreparedQuery tests prepared query caching
func TestScyllaState_GetPreparedQuery(t *testing.T) {
	state, mockSession, _ := createTestScyllaState(t)

	// Test when cache is nil
	state.preparedCache = nil
	_, err := state.GetPreparedQuery("SELECT * FROM test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prepared statement cache not initialized")

	// Test with initialized cache
	state.preparedCache = NewPreparedStatementCache(mockSession, 10)
	
	// Mock the session.Prepare call
	mockQuery := &MockQuery{}
	mockSession.On("Prepare", "SELECT * FROM test").Return(mockQuery, nil)

	query, err := state.GetPreparedQuery("SELECT * FROM test")
	assert.NoError(t, err)
	assert.NotNil(t, query)
}

// TestScyllaState_IsIdempotentQuery tests query idempotency detection
func TestScyllaState_IsIdempotentQuery(t *testing.T) {
	state, _, _ := createTestScyllaState(t)

	tests := []struct {
		name     string
		cql      string
		expected bool
	}{
		{
			name:     "SELECT query",
			cql:      "SELECT * FROM pins_by_cid WHERE mh_prefix = ? AND cid_bin = ?",
			expected: true,
		},
		{
			name:     "DELETE with WHERE",
			cql:      "DELETE FROM pins_by_cid WHERE mh_prefix = ? AND cid_bin = ?",
			expected: true,
		},
		{
			name:     "INSERT with IF NOT EXISTS",
			cql:      "INSERT INTO pins_by_cid (...) VALUES (...) IF NOT EXISTS",
			expected: true,
		},
		{
			name:     "UPDATE with WHERE",
			cql:      "UPDATE pins_by_cid SET ... WHERE mh_prefix = ? AND cid_bin = ?",
			expected: true,
		},
		{
			name:     "INSERT without IF NOT EXISTS",
			cql:      "INSERT INTO pins_by_cid (...) VALUES (...)",
			expected: false,
		},
		{
			name:     "DELETE without WHERE",
			cql:      "DELETE FROM pins_by_cid",
			expected: false,
		},
		{
			name:     "TRUNCATE",
			cql:      "TRUNCATE pins_by_cid",
			expected: false,
		},
		{
			name:     "DROP",
			cql:      "DROP TABLE pins_by_cid",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := state.isIdempotentQuery(tt.cql)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestScyllaState_RecordOperations tests metric recording methods
func TestScyllaState_RecordOperations(t *testing.T) {
	state, _, mockMetrics := createTestScyllaState(t)

	// Test recordOperation
	mockMetrics.On("RecordOperation", "test", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)
	state.recordOperation("test", time.Millisecond, nil)

	// Test recordQuery
	mockMetrics.On("RecordQuery", "SELECT", "test_keyspace", mock.AnythingOfType("time.Duration"))
	state.recordQuery("SELECT", time.Millisecond)

	// Test recordRetry
	mockMetrics.On("RecordRetry", "add", "timeout")
	state.recordRetry("add", "timeout")

	// Test recordBatchOperation
	mockMetrics.On("RecordBatchOperation", "LOGGED", 5, nil)
	state.recordBatchOperation("LOGGED", 5, nil)

	// Test recordPinOperation
	mockMetrics.On("RecordPinOperation", "add")
	state.recordPinOperation("add")

	// Test recordConnectionError
	mockMetrics.On("RecordConnectionError")
	state.recordConnectionError()

	mockMetrics.AssertExpectations(t)
}

// TestScyllaState_RecordOperations_NilMetrics tests metric recording with nil metrics
func TestScyllaState_RecordOperations_NilMetrics(t *testing.T) {
	state, _, _ := createTestScyllaState(t)
	state.metrics = nil

	// These should not panic when metrics is nil
	state.recordOperation("test", time.Millisecond, nil)
	state.recordQuery("SELECT", time.Millisecond)
	state.recordRetry("add", "timeout")
	state.recordBatchOperation("LOGGED", 5, nil)
	state.recordPinOperation("add")
	state.recordConnectionError()
	state.updatePreparedStatementsMetric()
}

// TestScyllaState_LogOperation tests structured logging
func TestScyllaState_LogOperation(t *testing.T) {
	state, _, _ := createTestScyllaState(t)
	cid := createTestCID(t)

	// Test successful operation logging
	ctx := context.Background()
	state.logOperation(ctx, "test", cid, time.Millisecond, nil)

	// Test error operation logging
	testErr := errors.New("test error")
	state.logOperation(ctx, "test", cid, time.Millisecond, testErr)

	// These should not panic
	assert.True(t, true)
}

// TestScyllaState_ErrorHandling tests various error conditions
func TestScyllaState_ErrorHandling(t *testing.T) {
	t.Run("operations on closed state", func(t *testing.T) {
		state, mockSession, mockMetrics := createTestScyllaState(t)
		pin := createTestPin(t)
		cid := createTestCID(t)

		// Close the state
		mockSession.On("Close").Return()
		mockMetrics.On("UpdateConnectionMetrics", 0, mock.AnythingOfType("map[string]bool"))
		state.Close()

		ctx := context.Background()

		// Test Add on closed state
		err := state.Add(ctx, pin)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ScyllaState is closed")

		// Test Get on closed state
		_, err = state.Get(ctx, cid)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ScyllaState is closed")

		// Test Has on closed state
		_, err = state.Has(ctx, cid)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ScyllaState is closed")

		// Test Rm on closed state
		err = state.Rm(ctx, cid)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ScyllaState is closed")

		// Test List on closed state
		pinChan := make(chan api.Pin, 1)
		err = state.List(ctx, pinChan)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ScyllaState is closed")
	})

	t.Run("timeout errors", func(t *testing.T) {
		state, mockSession, mockMetrics := createTestScyllaState(t)
		pin := createTestPin(t)

		// Setup mock expectations for timeout error
		mockQuery := &MockQuery{}
		timeoutError := gocql.RequestErrWriteTimeout{
			Consistency: gocql.Quorum,
			Received:    1,
			BlockFor:    2,
			WriteType:   "SIMPLE",
		}

		mockSession.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(mockQuery)
		mockQuery.On("WithContext", mock.Anything).Return(mockQuery)
		mockQuery.On("Consistency", mock.Anything).Return(mockQuery)
		mockQuery.On("SerialConsistency", mock.Anything).Return(mockQuery)
		mockQuery.On("PageSize", mock.Anything).Return(mockQuery)
		mockQuery.On("Idempotent", mock.Anything).Return(mockQuery)
		mockQuery.On("Exec").Return(timeoutError)
		mockQuery.On("Release").Return()

		mockMetrics.On("RecordOperation", "add", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), timeoutError)

		// Execute the test
		ctx := context.Background()
		err := state.Add(ctx, pin)

		// Verify results
		assert.Error(t, err)
		mockSession.AssertExpectations(t)
		mockMetrics.AssertExpectations(t)
	})
}

// TestScyllaState_ConcurrentOperations tests thread safety
func TestScyllaState_ConcurrentOperations(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)

	// Setup mock expectations for concurrent operations
	mockQuery := &MockQuery{}
	mockSession.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(mockQuery)
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery)
	mockQuery.On("Consistency", mock.Anything).Return(mockQuery)
	mockQuery.On("SerialConsistency", mock.Anything).Return(mockQuery)
	mockQuery.On("PageSize", mock.Anything).Return(mockQuery)
	mockQuery.On("Idempotent", mock.Anything).Return(mockQuery)
	mockQuery.On("Exec").Return(nil)
	mockQuery.On("Release").Return()

	mockMetrics.On("RecordOperation", mock.AnythingOfType("string"), mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil).Maybe()
	mockMetrics.On("RecordPinOperation", mock.AnythingOfType("string")).Maybe()

	// Run concurrent operations
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			pin := createTestPin(t)
			pin.PinOptions.Name = fmt.Sprintf("test-pin-%d", id)

			ctx := context.Background()
			err := state.Add(ctx, pin)
			assert.NoError(t, err)

			// Test concurrent pin count updates
			state.addPinCount(1)
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify final pin count
	finalCount := state.GetPinCount()
	assert.Equal(t, int64(numGoroutines), finalCount)
}

// TestScyllaState_EdgeCases tests various edge cases
func TestScyllaState_EdgeCases(t *testing.T) {
	t.Run("empty CID handling", func(t *testing.T) {
		// Test mhPrefix with empty CID
		result := mhPrefix([]byte{})
		assert.Equal(t, int16(0), result)

		// Test mhPrefix with single byte
		result = mhPrefix([]byte{0x12})
		assert.Equal(t, int16(0), result)
	})

	t.Run("serialization edge cases", func(t *testing.T) {
		state, _, _ := createTestScyllaState(t)
		cid := createTestCID(t)

		// Test deserializePin with empty data
		_, err := state.deserializePin(cid, []byte{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty pin data")

		// Test deserializePin with invalid data
		_, err = state.deserializePin(cid, []byte("invalid"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal")
	})

	t.Run("double close", func(t *testing.T) {
		state, mockSession, mockMetrics := createTestScyllaState(t)

		// Setup mock expectations
		mockSession.On("Close").Return()
		mockMetrics.On("UpdateConnectionMetrics", 0, mock.AnythingOfType("map[string]bool"))

		// First close should succeed
		err := state.Close()
		assert.NoError(t, err)
		assert.True(t, state.isClosed())

		// Second close should also succeed (no-op)
		err = state.Close()
		assert.NoError(t, err)
		assert.True(t, state.isClosed())

		mockSession.AssertExpectations(t)
		mockMetrics.AssertExpectations(t)
	})
}