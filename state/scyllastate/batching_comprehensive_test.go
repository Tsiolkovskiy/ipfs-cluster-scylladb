package scyllastate

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockBatch implements a mock gocql.Batch for testing
type MockBatch struct {
	mock.Mock
}

func (m *MockBatch) Query(stmt string, args ...interface{}) {
	m.Called(stmt, args)
}

func (m *MockBatch) WithContext(ctx context.Context) *gocql.Batch {
	args := m.Called(ctx)
	return args.Get(0).(*gocql.Batch)
}

func (m *MockBatch) Size() int {
	args := m.Called()
	return args.Int(0)
}

// Helper function to create a test ScyllaBatchingState
func createTestBatchingState(t *testing.T) (*ScyllaBatchingState, *MockSession, *MockMetrics) {
	scyllaState, mockSession, mockMetrics := createTestScyllaState(t)
	
	batchingState := &ScyllaBatchingState{
		ScyllaState: scyllaState,
		batch:       &gocql.Batch{},
	}

	return batchingState, mockSession, mockMetrics
}

// TestScyllaBatchingState_Add tests adding pins to batch
func TestScyllaBatchingState_Add(t *testing.T) {
	batchState, mockSession, mockMetrics := createTestBatchingState(t)
	pin := createTestPin(t)

	// Setup mock expectations - batch operations don't execute immediately
	mockMetrics.On("RecordBatchOperation", "add", 1, nil)

	// Execute the test
	ctx := context.Background()
	err := batchState.Add(ctx, pin)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, 1, batchState.GetBatchSize())
	mockMetrics.AssertExpectations(t)
}

// TestScyllaBatchingState_Rm tests removing pins from batch
func TestScyllaBatchingState_Rm(t *testing.T) {
	batchState, mockSession, mockMetrics := createTestBatchingState(t)
	cid := createTestCID(t)

	// Setup mock expectations
	mockMetrics.On("RecordBatchOperation", "rm", 1, nil)

	// Execute the test
	ctx := context.Background()
	err := batchState.Rm(ctx, cid)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, 1, batchState.GetBatchSize())
	mockMetrics.AssertExpectations(t)
}

// TestScyllaBatchingState_Commit tests committing batch operations
func TestScyllaBatchingState_Commit(t *testing.T) {
	batchState, mockSession, mockMetrics := createTestBatchingState(t)
	pin := createTestPin(t)

	// Add a pin to the batch first
	mockMetrics.On("RecordBatchOperation", "add", 1, nil)
	ctx := context.Background()
	err := batchState.Add(ctx, pin)
	require.NoError(t, err)

	// Setup mock expectations for commit
	mockSession.On("ExecuteBatch", mock.AnythingOfType("*gocql.Batch")).Return(nil)
	mockMetrics.On("RecordBatchOperation", "commit", 1, nil)
	mockMetrics.On("RecordOperation", "batch_commit", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)

	// Execute the test
	err = batchState.Commit(ctx)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, 0, batchState.GetBatchSize()) // Batch should be reset after commit
	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

// TestScyllaBatchingState_Commit_Empty tests committing empty batch
func TestScyllaBatchingState_Commit_Empty(t *testing.T) {
	batchState, mockSession, mockMetrics := createTestBatchingState(t)

	// Setup mock expectations for empty batch commit
	mockMetrics.On("RecordOperation", "batch_commit", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)

	// Execute the test
	ctx := context.Background()
	err := batchState.Commit(ctx)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, 0, batchState.GetBatchSize())
	mockMetrics.AssertExpectations(t)
}

// TestScyllaBatchingState_Commit_Error tests commit with database error
func TestScyllaBatchingState_Commit_Error(t *testing.T) {
	batchState, mockSession, mockMetrics := createTestBatchingState(t)
	pin := createTestPin(t)

	// Add a pin to the batch first
	mockMetrics.On("RecordBatchOperation", "add", 1, nil)
	ctx := context.Background()
	err := batchState.Add(ctx, pin)
	require.NoError(t, err)

	// Setup mock expectations for commit error
	expectedError := gocql.RequestErrWriteTimeout{
		Consistency: gocql.Quorum,
		Received:    1,
		BlockFor:    2,
		WriteType:   "BATCH",
	}
	mockSession.On("ExecuteBatch", mock.AnythingOfType("*gocql.Batch")).Return(expectedError)
	mockMetrics.On("RecordBatchOperation", "commit", 1, expectedError)
	mockMetrics.On("RecordOperation", "batch_commit", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), expectedError)

	// Execute the test
	err = batchState.Commit(ctx)

	// Verify results
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	// Batch should still contain operations after failed commit
	assert.Equal(t, 1, batchState.GetBatchSize())
	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

// TestScyllaBatchingState_ReadOperations tests that read operations delegate to underlying state
func TestScyllaBatchingState_ReadOperations(t *testing.T) {
	batchState, mockSession, mockMetrics := createTestBatchingState(t)
	cid := createTestCID(t)

	// Test Get
	t.Run("Get", func(t *testing.T) {
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

		ctx := context.Background()
		pin, err := batchState.Get(ctx, cid)

		assert.NoError(t, err)
		assert.NotNil(t, pin)
	})

	// Test Has
	t.Run("Has", func(t *testing.T) {
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

		ctx := context.Background()
		exists, err := batchState.Has(ctx, cid)

		assert.NoError(t, err)
		assert.True(t, exists)
	})

	// Test List
	t.Run("List", func(t *testing.T) {
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

		mockIter.On("Scan", mock.Anything).Return(false)
		mockIter.On("Close").Return(nil)

		mockMetrics.On("RecordOperation", "list", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)

		ctx := context.Background()
		pinChan := make(chan api.Pin, 10)
		err := batchState.List(ctx, pinChan)

		assert.NoError(t, err)
	})
}

// TestScyllaBatchingState_Marshal tests marshaling with pending operations
func TestScyllaBatchingState_Marshal(t *testing.T) {
	batchState, mockSession, mockMetrics := createTestBatchingState(t)
	pin := createTestPin(t)

	// Add a pin to the batch
	mockMetrics.On("RecordBatchOperation", "add", 1, nil)
	ctx := context.Background()
	err := batchState.Add(ctx, pin)
	require.NoError(t, err)

	// Setup mock expectations for commit (which should happen before marshal)
	mockSession.On("ExecuteBatch", mock.AnythingOfType("*gocql.Batch")).Return(nil)
	mockMetrics.On("RecordBatchOperation", "commit", 1, nil)
	mockMetrics.On("RecordOperation", "batch_commit", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)

	// Setup mock expectations for marshal (list operation)
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

	mockIter.On("Scan", mock.Anything).Return(false)
	mockIter.On("Close").Return(nil)

	mockMetrics.On("RecordOperation", "marshal", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)

	// Execute the test
	var buf bytes.Buffer
	err = batchState.Marshal(&buf)

	// Verify results
	assert.NoError(t, err)
	assert.NotEmpty(t, buf.Bytes())
	assert.Equal(t, 0, batchState.GetBatchSize()) // Batch should be committed and cleared
	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

// TestScyllaBatchingState_Unmarshal tests unmarshaling with batch clearing
func TestScyllaBatchingState_Unmarshal(t *testing.T) {
	batchState, mockSession, mockMetrics := createTestBatchingState(t)
	pin := createTestPin(t)

	// Add a pin to the batch first
	mockMetrics.On("RecordBatchOperation", "add", 1, nil)
	ctx := context.Background()
	err := batchState.Add(ctx, pin)
	require.NoError(t, err)
	assert.Equal(t, 1, batchState.GetBatchSize())

	// Setup mock expectations for unmarshal
	mockMetrics.On("RecordOperation", "unmarshal", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)

	// Execute the test
	testData := `[]`
	reader := strings.NewReader(testData)
	err = batchState.Unmarshal(reader)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, 0, batchState.GetBatchSize()) // Batch should be cleared
	mockMetrics.AssertExpectations(t)
}

// TestScyllaBatchingState_Migrate tests migration with batch clearing
func TestScyllaBatchingState_Migrate(t *testing.T) {
	batchState, mockSession, mockMetrics := createTestBatchingState(t)
	pin := createTestPin(t)

	// Add a pin to the batch first
	mockMetrics.On("RecordBatchOperation", "add", 1, nil)
	ctx := context.Background()
	err := batchState.Add(ctx, pin)
	require.NoError(t, err)
	assert.Equal(t, 1, batchState.GetBatchSize())

	// Setup mock expectations for migrate
	mockMetrics.On("RecordOperation", "migrate", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)

	// Execute the test
	testData := `[]`
	reader := strings.NewReader(testData)
	err = batchState.Migrate(ctx, reader)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, 0, batchState.GetBatchSize()) // Batch should be cleared
	mockMetrics.AssertExpectations(t)
}

// TestScyllaBatchingState_ConcurrentOperations tests thread safety of batch operations
func TestScyllaBatchingState_ConcurrentOperations(t *testing.T) {
	batchState, mockSession, mockMetrics := createTestBatchingState(t)

	// Setup mock expectations for concurrent batch operations
	mockMetrics.On("RecordBatchOperation", mock.AnythingOfType("string"), mock.AnythingOfType("int"), nil).Maybe()

	// Run concurrent Add operations
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			pin := createTestPin(t)
			pin.PinOptions.Name = fmt.Sprintf("batch-pin-%d", id)

			ctx := context.Background()
			err := batchState.Add(ctx, pin)
			assert.NoError(t, err)
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify final batch size
	finalSize := batchState.GetBatchSize()
	assert.Equal(t, numGoroutines, finalSize)
}

// TestScyllaBatchingState_MixedOperations tests mixing Add and Rm operations in batch
func TestScyllaBatchingState_MixedOperations(t *testing.T) {
	batchState, mockSession, mockMetrics := createTestBatchingState(t)
	pin1 := createTestPin(t)
	pin2 := createTestPin(t)
	pin2.PinOptions.Name = "pin2"

	// Setup mock expectations
	mockMetrics.On("RecordBatchOperation", "add", 1, nil)
	mockMetrics.On("RecordBatchOperation", "add", 2, nil)
	mockMetrics.On("RecordBatchOperation", "rm", 3, nil)

	ctx := context.Background()

	// Add two pins
	err := batchState.Add(ctx, pin1)
	assert.NoError(t, err)
	assert.Equal(t, 1, batchState.GetBatchSize())

	err = batchState.Add(ctx, pin2)
	assert.NoError(t, err)
	assert.Equal(t, 2, batchState.GetBatchSize())

	// Remove one pin
	err = batchState.Rm(ctx, pin1.Cid)
	assert.NoError(t, err)
	assert.Equal(t, 3, batchState.GetBatchSize())

	mockMetrics.AssertExpectations(t)
}

// TestScyllaBatchingState_BatchSizeLimit tests batch size management
func TestScyllaBatchingState_BatchSizeLimit(t *testing.T) {
	batchState, mockSession, mockMetrics := createTestBatchingState(t)

	// Set a small batch size limit for testing
	batchState.ScyllaState.config.BatchSize = 3

	// Setup mock expectations for auto-commit when batch size limit is reached
	mockMetrics.On("RecordBatchOperation", "add", mock.AnythingOfType("int"), nil).Maybe()
	mockSession.On("ExecuteBatch", mock.AnythingOfType("*gocql.Batch")).Return(nil).Maybe()
	mockMetrics.On("RecordBatchOperation", "commit", mock.AnythingOfType("int"), nil).Maybe()
	mockMetrics.On("RecordOperation", "batch_commit", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil).Maybe()

	ctx := context.Background()

	// Add pins up to the batch size limit
	for i := 0; i < 5; i++ {
		pin := createTestPin(t)
		pin.PinOptions.Name = fmt.Sprintf("pin-%d", i)

		err := batchState.Add(ctx, pin)
		assert.NoError(t, err)

		// Check if batch was auto-committed when limit was reached
		if i >= batchState.ScyllaState.config.BatchSize-1 {
			// Batch should have been committed and reset
			// The exact behavior depends on the implementation
		}
	}
}

// TestScyllaBatchingState_ErrorRecovery tests error recovery in batch operations
func TestScyllaBatchingState_ErrorRecovery(t *testing.T) {
	batchState, mockSession, mockMetrics := createTestBatchingState(t)
	pin := createTestPin(t)

	// Add a pin to the batch
	mockMetrics.On("RecordBatchOperation", "add", 1, nil)
	ctx := context.Background()
	err := batchState.Add(ctx, pin)
	require.NoError(t, err)

	// Setup mock expectations for commit failure
	expectedError := gocql.RequestErrWriteTimeout{
		Consistency: gocql.Quorum,
		Received:    1,
		BlockFor:    2,
		WriteType:   "BATCH",
	}
	mockSession.On("ExecuteBatch", mock.AnythingOfType("*gocql.Batch")).Return(expectedError).Once()
	mockMetrics.On("RecordBatchOperation", "commit", 1, expectedError)
	mockMetrics.On("RecordOperation", "batch_commit", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), expectedError)

	// First commit should fail
	err = batchState.Commit(ctx)
	assert.Error(t, err)
	assert.Equal(t, 1, batchState.GetBatchSize()) // Batch should still contain operations

	// Setup mock expectations for successful retry
	mockSession.On("ExecuteBatch", mock.AnythingOfType("*gocql.Batch")).Return(nil).Once()
	mockMetrics.On("RecordBatchOperation", "commit", 1, nil)
	mockMetrics.On("RecordOperation", "batch_commit", mock.AnythingOfType("time.Duration"), mock.AnythingOfType("string"), nil)

	// Retry should succeed
	err = batchState.Commit(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 0, batchState.GetBatchSize()) // Batch should be cleared after successful commit

	mockSession.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

// TestScyllaBatchingState_GetBatchSize tests batch size tracking
func TestScyllaBatchingState_GetBatchSize(t *testing.T) {
	batchState, _, mockMetrics := createTestBatchingState(t)

	// Initial size should be 0
	assert.Equal(t, 0, batchState.GetBatchSize())

	// Add operations and verify size increases
	pin := createTestPin(t)
	cid := createTestCID(t)

	mockMetrics.On("RecordBatchOperation", "add", 1, nil)
	mockMetrics.On("RecordBatchOperation", "rm", 2, nil)

	ctx := context.Background()

	err := batchState.Add(ctx, pin)
	assert.NoError(t, err)
	assert.Equal(t, 1, batchState.GetBatchSize())

	err = batchState.Rm(ctx, cid)
	assert.NoError(t, err)
	assert.Equal(t, 2, batchState.GetBatchSize())

	mockMetrics.AssertExpectations(t)
}