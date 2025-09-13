package scyllastate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockSession implements a mock gocql.Session for testing
type MockSession struct {
	mock.Mock
}

func (m *MockSession) Query(stmt string, values ...interface{}) *gocql.Query {
	args := m.Called(stmt, values)
	return args.Get(0).(*gocql.Query)
}

func (m *MockSession) Close() {
	m.Called()
}

func (m *MockSession) Closed() bool {
	args := m.Called()
	return args.Bool(0)
}

// MockQuery implements a mock gocql.Query for testing
type MockQuery struct {
	mock.Mock
}

func (m *MockQuery) Exec() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockQuery) Scan(dest ...interface{}) error {
	args := m.Called(dest)
	return args.Error(0)
}

func (m *MockQuery) Iter() *gocql.Iter {
	args := m.Called()
	return args.Get(0).(*gocql.Iter)
}

func (m *MockQuery) WithContext(ctx context.Context) *gocql.Query {
	args := m.Called(ctx)
	return args.Get(0).(*gocql.Query)
}

func (m *MockQuery) Consistency(c gocql.Consistency) *gocql.Query {
	args := m.Called(c)
	return args.Get(0).(*gocql.Query)
}

func (m *MockQuery) SerialConsistency(sc gocql.SerialConsistency) *gocql.Query {
	args := m.Called(sc)
	return args.Get(0).(*gocql.Query)
}

func (m *MockQuery) PageSize(n int) *gocql.Query {
	args := m.Called(n)
	return args.Get(0).(*gocql.Query)
}

func (m *MockQuery) Idempotent(value bool) *gocql.Query {
	args := m.Called(value)
	return args.Get(0).(*gocql.Query)
}

func (m *MockQuery) Release() {
	m.Called()
}

// MockIter implements a mock gocql.Iter for testing
type MockIter struct {
	mock.Mock
}

func (m *MockIter) Scan(dest ...interface{}) bool {
	args := m.Called(dest)
	return args.Bool(0)
}

func (m *MockIter) Close() error {
	args := m.Called()
	return args.Error(0)
}

// MockMetrics implements a mock Metrics interface for testing
type MockMetrics struct {
	mock.Mock
}

func (m *MockMetrics) RecordOperation(operation string, duration time.Duration, consistencyLevel string, err error) {
	m.Called(operation, duration, consistencyLevel, err)
}

func (m *MockMetrics) RecordQuery(queryType, keyspace string, duration time.Duration) {
	m.Called(queryType, keyspace, duration)
}

func (m *MockMetrics) RecordRetry(operation, reason string) {
	m.Called(operation, reason)
}

func (m *MockMetrics) RecordBatchOperation(batchType string, size int, err error) {
	m.Called(batchType, size, err)
}

func (m *MockMetrics) RecordPinOperation(operationType string) {
	m.Called(operationType)
}

func (m *MockMetrics) UpdateConnectionMetrics(activeConns int, poolStatus map[string]bool) {
	m.Called(activeConns, poolStatus)
}

func (m *MockMetrics) UpdateStateMetrics(totalPins int64) {
	m.Called(totalPins)
}

func (m *MockMetrics) UpdatePreparedStatements(count int) {
	m.Called(count)
}

func (m *MockMetrics) UpdateConsistencyLevel(operationType string, level string) {
	m.Called(operationType, level)
}

func (m *MockMetrics) UpdateDegradationMetrics(active bool, level int, degradationType string) {
	m.Called(active, level, degradationType)
}

func (m *MockMetrics) UpdateNodeHealth(host, datacenter, rack string, healthy bool) {
	m.Called(host, datacenter, rack, healthy)
}

func (m *MockMetrics) UpdatePartitionStatus(partitioned bool) {
	m.Called(partitioned)
}

func (m *MockMetrics) RecordConnectionError() {
	m.Called()
}

// Helper function to create a test ScyllaState with mocks
func createTestScyllaState(t *testing.T) (*ScyllaState, *MockSession, *MockMetrics) {
	mockSession := &MockSession{}
	mockMetrics := &MockMetrics{}

	config := &Config{}
	config.Default()

	state := &ScyllaState{
		session: mockSession,
		config:  config,
		metrics: mockMetrics,
		prepared: &PreparedStatements{
			insertPin:   &MockQuery{},
			selectPin:   &MockQuery{},
			deletePin:   &MockQuery{},
			checkExists: &MockQuery{},
			listAllPins: &MockQuery{},
		},
	}

	return state, mockSession, mockMetrics
}

// Helper function to create a test CID
func createTestCID(t *testing.T) api.Cid {
	cidStr := "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"
	cid, err := api.DecodeCid(cidStr)
	require.NoError(t, err)
	return cid
}

// Helper function to create a test Pin
func createTestPin(t *testing.T) api.Pin {
	cid := createTestCID(t)
	pin := api.Pin{
		Cid:  cid,
		Type: api.DataType,
		Allocations: []api.PeerID{
			api.PeerID("peer1"),
			api.PeerID("peer2"),
		},
		ReplicationFactorMin: 2,
		ReplicationFactorMax: 3,
		MaxDepth:             -1,
		PinOptions: api.PinOptions{
			Name: "test-pin",
		},
	}
	return pin
}

func TestScyllaState_Add_Success(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)
	pin := createTestPin(t)

	// Setup mock expectations
	mockQuery := &MockQuery{}
	mockSession.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(mockQuery)
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery)
	mockQuery.On("Consistency", mock.Anything).Return(mockQuery)
	mockQuery.On("SerialConsistency", mock.Anything).Return(mockQuery)
	mockQuery.On("PageSize", mock.Anything).Return(mockQuery)
	mockQuery.On("Idempotent", mock.Anything).Return(mockQuery)
	mockQuery.On("Exec").Return(nil)
	mockQuery.On("Release").Return()

	mockMetrics.On("RecordOperation", "add", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)
	mockMetrics.On("RecordPinOperation", "add")

	// Execute the test
	ctx := context.Background()
	err := state.Add(ctx, pin)

	// Verify results
	assert.NoError(t, err)
	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

func TestScyllaState_Add_Error(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)
	pin := createTestPin(t)

	// Setup mock expectations for error case
	mockQuery := &MockQuery{}
	expectedError := errors.New("database error")

	mockSession.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(mockQuery)
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery)
	mockQuery.On("Consistency", mock.Anything).Return(mockQuery)
	mockQuery.On("SerialConsistency", mock.Anything).Return(mockQuery)
	mockQuery.On("PageSize", mock.Anything).Return(mockQuery)
	mockQuery.On("Idempotent", mock.Anything).Return(mockQuery)
	mockQuery.On("Exec").Return(expectedError)
	mockQuery.On("Release").Return()

	mockMetrics.On("RecordOperation", "add", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), expectedError)

	// Execute the test
	ctx := context.Background()
	err := state.Add(ctx, pin)

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

func TestScyllaState_Get_Success(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)
	cid := createTestCID(t)

	// Setup mock expectations
	mockQuery := &MockQuery{}
	mockSession.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(mockQuery)
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery)
	mockQuery.On("Consistency", mock.Anything).Return(mockQuery)
	mockQuery.On("SerialConsistency", mock.Anything).Return(mockQuery)
	mockQuery.On("PageSize", mock.Anything).Return(mockQuery)
	mockQuery.On("Idempotent", mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Return(nil)
	mockQuery.On("Release").Return()

	mockMetrics.On("RecordOperation", "get", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)

	// Execute the test
	ctx := context.Background()
	pin, err := state.Get(ctx, cid)

	// Verify results
	assert.NoError(t, err)
	assert.NotNil(t, pin)
	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

func TestScyllaState_Get_NotFound(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)
	cid := createTestCID(t)

	// Setup mock expectations for not found case
	mockQuery := &MockQuery{}
	notFoundError := gocql.ErrNotFound

	mockSession.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(mockQuery)
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery)
	mockQuery.On("Consistency", mock.Anything).Return(mockQuery)
	mockQuery.On("SerialConsistency", mock.Anything).Return(mockQuery)
	mockQuery.On("PageSize", mock.Anything).Return(mockQuery)
	mockQuery.On("Idempotent", mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Return(notFoundError)
	mockQuery.On("Release").Return()

	mockMetrics.On("RecordOperation", "get", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), notFoundError)

	// Execute the test
	ctx := context.Background()
	pin, err := state.Get(ctx, cid)

	// Verify results
	assert.Error(t, err)
	assert.Nil(t, pin)
	assert.Equal(t, notFoundError, err)
	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

func TestScyllaState_Has_Success(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)
	cid := createTestCID(t)

	// Setup mock expectations
	mockQuery := &MockQuery{}
	mockSession.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(mockQuery)
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery)
	mockQuery.On("Consistency", mock.Anything).Return(mockQuery)
	mockQuery.On("SerialConsistency", mock.Anything).Return(mockQuery)
	mockQuery.On("PageSize", mock.Anything).Return(mockQuery)
	mockQuery.On("Idempotent", mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Return(nil)
	mockQuery.On("Release").Return()

	mockMetrics.On("RecordOperation", "has", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)

	// Execute the test
	ctx := context.Background()
	exists, err := state.Has(ctx, cid)

	// Verify results
	assert.NoError(t, err)
	assert.True(t, exists)
	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

func TestScyllaState_Has_NotFound(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)
	cid := createTestCID(t)

	// Setup mock expectations for not found case
	mockQuery := &MockQuery{}
	notFoundError := gocql.ErrNotFound

	mockSession.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(mockQuery)
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery)
	mockQuery.On("Consistency", mock.Anything).Return(mockQuery)
	mockQuery.On("SerialConsistency", mock.Anything).Return(mockQuery)
	mockQuery.On("PageSize", mock.Anything).Return(mockQuery)
	mockQuery.On("Idempotent", mock.Anything).Return(mockQuery)
	mockQuery.On("Scan", mock.Anything).Return(notFoundError)
	mockQuery.On("Release").Return()

	mockMetrics.On("RecordOperation", "has", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), notFoundError)

	// Execute the test
	ctx := context.Background()
	exists, err := state.Has(ctx, cid)

	// Verify results
	assert.NoError(t, err)
	assert.False(t, exists)
	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

func TestScyllaState_Rm_Success(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)
	cid := createTestCID(t)

	// Setup mock expectations
	mockQuery := &MockQuery{}
	mockSession.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(mockQuery)
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery)
	mockQuery.On("Consistency", mock.Anything).Return(mockQuery)
	mockQuery.On("SerialConsistency", mock.Anything).Return(mockQuery)
	mockQuery.On("PageSize", mock.Anything).Return(mockQuery)
	mockQuery.On("Idempotent", mock.Anything).Return(mockQuery)
	mockQuery.On("Exec").Return(nil)
	mockQuery.On("Release").Return()

	mockMetrics.On("RecordOperation", "rm", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)
	mockMetrics.On("RecordPinOperation", "rm")

	// Execute the test
	ctx := context.Background()
	err := state.Rm(ctx, cid)

	// Verify results
	assert.NoError(t, err)
	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

func TestScyllaState_List_Success(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)

	// Setup mock expectations
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

	mockMetrics.On("RecordOperation", "list", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)

	// Execute the test
	ctx := context.Background()
	pinChan := make(chan api.Pin, 10)

	err := state.List(ctx, pinChan)

	// Verify results
	assert.NoError(t, err)
	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

func TestScyllaState_SerializePin_Success(t *testing.T) {
	state, _, _ := createTestScyllaState(t)
	pin := createTestPin(t)

	// Execute the test
	data, err := state.serializePin(pin)

	// Verify results
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	// Verify we can deserialize it back
	deserializedPin, err := state.deserializePin(pin.Cid, data)
	assert.NoError(t, err)
	assert.Equal(t, pin.Cid, deserializedPin.Cid)
}

func TestScyllaState_DeserializePin_Success(t *testing.T) {
	state, _, _ := createTestScyllaState(t)
	pin := createTestPin(t)

	// First serialize a pin
	data, err := state.serializePin(pin)
	require.NoError(t, err)

	// Execute the test
	deserializedPin, err := state.deserializePin(pin.Cid, data)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, pin.Cid, deserializedPin.Cid)
	assert.Equal(t, pin.Type, deserializedPin.Type)
}

func TestScyllaState_DeserializePin_InvalidData(t *testing.T) {
	state, _, _ := createTestScyllaState(t)
	cid := createTestCID(t)

	// Execute the test with invalid data
	_, err := state.deserializePin(cid, []byte("invalid data"))

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal")
}

func TestScyllaState_DeserializePin_EmptyData(t *testing.T) {
	state, _, _ := createTestScyllaState(t)
	cid := createTestCID(t)

	// Execute the test with empty data
	_, err := state.deserializePin(cid, []byte{})

	// Verify results
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty pin data")
}

func TestScyllaState_MhPrefix(t *testing.T) {
	tests := []struct {
		name     string
		cidBin   []byte
		expected int16
	}{
		{
			name:     "empty cid",
			cidBin:   []byte{},
			expected: 0,
		},
		{
			name:     "single byte",
			cidBin:   []byte{0x12},
			expected: 0,
		},
		{
			name:     "two bytes",
			cidBin:   []byte{0x12, 0x34},
			expected: 0x1234,
		},
		{
			name:     "more than two bytes",
			cidBin:   []byte{0x12, 0x34, 0x56, 0x78},
			expected: 0x1234,
		},
		{
			name:     "zero bytes",
			cidBin:   []byte{0x00, 0x00},
			expected: 0x0000,
		},
		{
			name:     "max bytes",
			cidBin:   []byte{0xFF, 0xFF},
			expected: -1, // 0xFFFF as int16
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mhPrefix(tt.cidBin)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestScyllaState_CidToPartitionedKey(t *testing.T) {
	cidBin := []byte{0x12, 0x34, 0x56, 0x78}
	prefix, key := cidToPartitionedKey(cidBin)

	assert.Equal(t, int16(0x1234), prefix)
	assert.Equal(t, cidBin, key)
}

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := state.isIdempotentQuery(tt.cql)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestScyllaState_Close(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)

	// Setup mock expectations
	mockSession.On("Close").Return()
	mockMetrics.On("UpdateConnectionMetrics", 0, mock.AnythingOfType("map[string]bool"))

	// Execute the test
	err := state.Close()

	// Verify results
	assert.NoError(t, err)
	assert.True(t, state.isClosed())
	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)

	// Test double close
	err = state.Close()
	assert.NoError(t, err) // Should not error on double close
}

func TestScyllaState_IsClosed(t *testing.T) {
	state, _, _ := createTestScyllaState(t)

	// Initially should not be closed
	assert.False(t, state.isClosed())

	// After closing should be closed
	state.Close()
	assert.True(t, state.isClosed())
}

// Benchmark tests
func BenchmarkScyllaState_MhPrefix(b *testing.B) {
	cidBin := []byte{0x12, 0x34, 0x56, 0x78}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mhPrefix(cidBin)
	}
}

func BenchmarkScyllaState_SerializePin(b *testing.B) {
	state, _, _ := createTestScyllaState(&testing.T{})
	pin := createTestPin(&testing.T{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = state.serializePin(pin)
	}
}

func BenchmarkScyllaState_DeserializePin(b *testing.B) {
	state, _, _ := createTestScyllaState(&testing.T{})
	pin := createTestPin(&testing.T{})
	data, _ := state.serializePin(pin)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = state.deserializePin(pin.Cid, data)
	}
}

// Test error handling and edge cases
func TestScyllaState_ErrorHandling(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		ctx := context.Background()
		_, err := New(ctx, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "config cannot be nil")
	})

	t.Run("invalid config", func(t *testing.T) {
		ctx := context.Background()
		config := &Config{
			Hosts: []string{}, // Invalid: empty hosts
		}
		_, err := New(ctx, config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid configuration")
	})
}

func TestScyllaState_ContextCancellation(t *testing.T) {
	state, mockSession, mockMetrics := createTestScyllaState(t)
	pin := createTestPin(t)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Setup mock expectations
	mockQuery := &MockQuery{}
	mockSession.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(mockQuery)
	mockQuery.On("WithContext", ctx).Return(mockQuery)
	mockQuery.On("Consistency", mock.Anything).Return(mockQuery)
	mockQuery.On("SerialConsistency", mock.Anything).Return(mockQuery)
	mockQuery.On("PageSize", mock.Anything).Return(mockQuery)
	mockQuery.On("Idempotent", mock.Anything).Return(mockQuery)
	mockQuery.On("Exec").Return(context.Canceled)
	mockQuery.On("Release").Return()

	mockMetrics.On("RecordOperation", "add", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), context.Canceled)

	// Execute the test
	err := state.Add(ctx, pin)

	// Verify results
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

func TestScyllaState_PreparedStatementCache(t *testing.T) {
	// Test prepared statement cache functionality
	mockSession := &MockSession{}
	cache := NewPreparedStatementCache(mockSession, 5)

	assert.NotNil(t, cache)
	assert.Equal(t, 0, cache.Size())

	// Test cache operations would require more complex mocking
	// This is a basic structure test
	cache.Clear()
	assert.Equal(t, 0, cache.Size())
}
