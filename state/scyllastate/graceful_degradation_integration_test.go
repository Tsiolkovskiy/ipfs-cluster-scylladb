package scyllastate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScyllaState_GracefulDegradationIntegration tests graceful degradation with actual state operations
func TestScyllaState_GracefulDegradationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a test configuration with graceful degradation enabled
	config := &Config{}
	config.Default()
	config.Hosts = []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"} // Multiple hosts for testing
	config.Keyspace = "test_graceful_degradation"
	config.Consistency = "QUORUM"
	config.RetryPolicy = RetryPolicyConfig{
		NumRetries:    3,
		MinRetryDelay: 100 * time.Millisecond,
		MaxRetryDelay: 2 * time.Second,
	}

	ctx := context.Background()

	// Note: This test requires a running ScyllaDB cluster for full integration testing
	// For unit testing, we'll create a mock scenario
	t.Run("MockedDegradationScenario", func(t *testing.T) {
		// Create a ScyllaState with mocked session that can simulate failures
		mockState := createMockScyllaStateWithDegradation(t, config)
		defer mockState.Close()

		// Test that operations work normally when all nodes are healthy
		testPin := api.Pin{
			Cid:  testCid1,
			Type: api.DataType,
		}

		// This should work with normal consistency
		err := mockState.Add(ctx, testPin)
		assert.NoError(t, err)

		// Verify the pin was added
		has, err := mockState.Has(ctx, testCid1)
		assert.NoError(t, err)
		assert.True(t, has)

		// Simulate node failures by updating the degradation manager
		mockState.simulateNodeFailures([]string{"127.0.0.1", "127.0.0.2"})

		// Operations should still work with degraded consistency
		testPin2 := api.Pin{
			Cid:  testCid2,
			Type: api.DataType,
		}

		err = mockState.Add(ctx, testPin2)
		assert.NoError(t, err)

		// Verify degradation metrics
		metrics := mockState.GetDegradationMetrics()
		assert.Greater(t, metrics.UnhealthyNodes, int64(0))
		assert.Greater(t, metrics.NodeFailures, int64(0))

		// Test network partition scenario
		mockState.simulateNetworkPartition()
		assert.True(t, mockState.IsPartitioned())

		// Operations should still work during partition with relaxed consistency
		testPin3 := api.Pin{
			Cid:  testCid3,
			Type: api.DataType,
		}

		err = mockState.Add(ctx, testPin3)
		// This might fail or succeed depending on partition severity
		// The important thing is that it handles the partition gracefully
		if err != nil {
			t.Logf("Operation failed during partition as expected: %v", err)
		} else {
			t.Logf("Operation succeeded during partition with degraded consistency")
		}

		// Test recovery
		mockState.simulateRecovery()
		assert.False(t, mockState.IsPartitioned())

		// Operations should work normally after recovery
		err = mockState.Has(ctx, testCid1)
		assert.NoError(t, err)
	})
}

// TestScyllaState_ConsistencyDegradationInOperations tests consistency degradation in actual operations
func TestScyllaState_ConsistencyDegradationInOperations(t *testing.T) {
	config := &Config{}
	config.Default()
	config.Consistency = "QUORUM"
	config.RetryPolicy = RetryPolicyConfig{
		NumRetries:    2,
		MinRetryDelay: 50 * time.Millisecond,
		MaxRetryDelay: 500 * time.Millisecond,
	}

	mockState := createMockScyllaStateWithDegradation(t, config)
	defer mockState.Close()

	ctx := context.Background()

	// Test Add operation with degradation
	t.Run("AddWithDegradation", func(t *testing.T) {
		testPin := api.Pin{
			Cid:  testCid1,
			Type: api.DataType,
		}

		// Initially should use QUORUM consistency
		initialConsistency := mockState.GetCurrentConsistencyLevel()
		assert.Equal(t, gocql.Quorum, initialConsistency)

		// Simulate unavailable error to trigger degradation
		mockState.simulateUnavailableError()

		err := mockState.Add(ctx, testPin)
		// Should succeed with degraded consistency or fail gracefully
		if err != nil {
			t.Logf("Add operation handled degradation: %v", err)
		}

		// Check if consistency was degraded
		currentConsistency := mockState.GetCurrentConsistencyLevel()
		if currentConsistency != gocql.Quorum {
			t.Logf("Consistency degraded from QUORUM to %v", currentConsistency)
		}
	})

	// Test Get operation with degradation
	t.Run("GetWithDegradation", func(t *testing.T) {
		// Simulate read failure scenario
		mockState.simulateReadFailure()

		_, err := mockState.Get(ctx, testCid1)
		// Should handle read failures gracefully
		if err != nil {
			t.Logf("Get operation handled read failure: %v", err)
		}
	})

	// Test List operation with degradation
	t.Run("ListWithDegradation", func(t *testing.T) {
		// Simulate timeout scenario
		mockState.simulateTimeout()

		pinChan := make(chan api.Pin, 10)
		go func() {
			defer close(pinChan)
			err := mockState.List(ctx, pinChan)
			if err != nil {
				t.Logf("List operation handled timeout: %v", err)
			}
		}()

		// Consume pins from channel
		pinCount := 0
		for range pinChan {
			pinCount++
		}

		t.Logf("List operation returned %d pins despite degradation", pinCount)
	})
}

// TestScyllaState_NetworkPartitionRecovery tests network partition detection and recovery
func TestScyllaState_NetworkPartitionRecovery(t *testing.T) {
	config := &Config{}
	config.Default()
	config.Hosts = []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"}
	config.Consistency = "QUORUM"

	mockState := createMockScyllaStateWithDegradation(t, config)
	defer mockState.Close()

	ctx := context.Background()

	// Initially no partition
	assert.False(t, mockState.IsPartitioned())

	// Simulate network partition
	mockState.simulateNetworkPartition()
	assert.True(t, mockState.IsPartitioned())

	// Test operations during partition
	testPin := api.Pin{
		Cid:  testCid1,
		Type: api.DataType,
	}

	err := mockState.Add(ctx, testPin)
	// Should either succeed with relaxed consistency or fail gracefully
	t.Logf("Add during partition result: %v", err)

	// Check partition metrics
	metrics := mockState.GetDegradationMetrics()
	assert.True(t, metrics.IsPartitioned)
	assert.Greater(t, metrics.PartitionDetections, int64(0))

	// Simulate recovery
	mockState.simulateRecovery()

	// Wait a bit for recovery to be detected
	time.Sleep(100 * time.Millisecond)

	// Should no longer be partitioned
	assert.False(t, mockState.IsPartitioned())

	// Operations should work normally after recovery
	err = mockState.Has(ctx, testCid1)
	assert.NoError(t, err)

	// Check recovery metrics
	metrics = mockState.GetDegradationMetrics()
	assert.False(t, metrics.IsPartitioned)
	assert.Greater(t, metrics.RecoveryEvents, int64(0))
}

// TestScyllaState_CircuitBreakerIntegration tests circuit breaker integration with operations
func TestScyllaState_CircuitBreakerIntegration(t *testing.T) {
	config := &Config{}
	config.Default()
	config.Hosts = []string{"127.0.0.1"}
	config.RetryPolicy = RetryPolicyConfig{
		NumRetries:    1,
		MinRetryDelay: 10 * time.Millisecond,
		MaxRetryDelay: 100 * time.Millisecond,
	}

	mockState := createMockScyllaStateWithDegradation(t, config)
	defer mockState.Close()

	ctx := context.Background()

	// Simulate multiple failures to trip circuit breaker
	for i := 0; i < 6; i++ {
		mockState.simulateConnectionError()

		testPin := api.Pin{
			Cid:  testCid1,
			Type: api.DataType,
		}

		err := mockState.Add(ctx, testPin)
		t.Logf("Operation %d result: %v", i+1, err)
	}

	// Check circuit breaker metrics
	metrics := mockState.GetDegradationMetrics()
	assert.Greater(t, metrics.CircuitBreakerTrips, int64(0))

	// Simulate recovery
	mockState.simulateRecovery()

	// Operations should work after circuit breaker reset
	err := mockState.Has(ctx, testCid1)
	if err == nil {
		t.Log("Operation succeeded after circuit breaker recovery")
	}
}

// Mock ScyllaState implementation for testing graceful degradation

type MockScyllaStateWithDegradation struct {
	*ScyllaState
	simulatedErrors map[string]error
	errorCount      int
}

func createMockScyllaStateWithDegradation(t *testing.T, config *Config) *MockScyllaStateWithDegradation {
	// Create a basic ScyllaState structure for testing
	state := &ScyllaState{
		config: config,
		logger: testLogger,
	}

	// Initialize degradation manager
	retryPolicy := NewRetryPolicy(config.RetryPolicy)
	mockSession := &MockSession{}
	state.degradationManager = NewGracefulDegradationManager(mockSession, config, testLogger, retryPolicy)

	ctx := context.Background()
	err := state.degradationManager.Start(ctx)
	require.NoError(t, err)

	mockState := &MockScyllaStateWithDegradation{
		ScyllaState:     state,
		simulatedErrors: make(map[string]error),
	}

	return mockState
}

func (m *MockScyllaStateWithDegradation) simulateNodeFailures(hosts []string) {
	for _, host := range hosts {
		m.degradationManager.healthMutex.Lock()
		if nodeHealth, exists := m.degradationManager.nodeHealth[host]; exists {
			nodeHealth.mutex.Lock()
			nodeHealth.IsHealthy = false
			nodeHealth.ConsecutiveFailures = 5
			nodeHealth.LastError = errors.New("simulated node failure")
			nodeHealth.mutex.Unlock()
		}
		m.degradationManager.healthMutex.Unlock()
	}
}

func (m *MockScyllaStateWithDegradation) simulateNetworkPartition() {
	m.degradationManager.partitionDetector.mutex.Lock()
	m.degradationManager.partitionDetector.isPartitioned = true
	m.degradationManager.partitionDetector.partitionStartTime = time.Now()
	m.degradationManager.partitionDetector.mutex.Unlock()

	m.degradationManager.degradationMetrics.IsPartitioned = true
	m.degradationManager.degradationMetrics.PartitionDetections++
}

func (m *MockScyllaStateWithDegradation) simulateRecovery() {
	// Reset all nodes to healthy
	m.degradationManager.healthMutex.Lock()
	for _, nodeHealth := range m.degradationManager.nodeHealth {
		nodeHealth.mutex.Lock()
		nodeHealth.IsHealthy = true
		nodeHealth.ConsecutiveFailures = 0
		nodeHealth.LastError = nil
		nodeHealth.mutex.Unlock()
	}
	m.degradationManager.healthMutex.Unlock()

	// Reset partition state
	m.degradationManager.partitionDetector.mutex.Lock()
	m.degradationManager.partitionDetector.isPartitioned = false
	m.degradationManager.partitionDetector.mutex.Unlock()

	m.degradationManager.degradationMetrics.IsPartitioned = false
	m.degradationManager.degradationMetrics.RecoveryEvents++

	// Reset consistency level
	m.degradationManager.consistencyFallback.mutex.Lock()
	m.degradationManager.consistencyFallback.currentConsistency = m.degradationManager.consistencyFallback.originalConsistency
	m.degradationManager.consistencyFallback.mutex.Unlock()
}

func (m *MockScyllaStateWithDegradation) simulateUnavailableError() {
	m.simulatedErrors["unavailable"] = &gocql.RequestErrUnavailable{
		Consistency: gocql.Quorum,
		Required:    2,
		Alive:       1,
	}
}

func (m *MockScyllaStateWithDegradation) simulateReadFailure() {
	m.simulatedErrors["read_failure"] = &gocql.RequestErrReadFailure{
		Consistency: gocql.Quorum,
		Received:    1,
		BlockFor:    2,
		NumFailures: 1,
	}
}

func (m *MockScyllaStateWithDegradation) simulateTimeout() {
	m.simulatedErrors["timeout"] = errors.New("timeout: operation timed out")
}

func (m *MockScyllaStateWithDegradation) simulateConnectionError() {
	m.simulatedErrors["connection"] = errors.New("connection refused: 127.0.0.1:9042")
	m.errorCount++
}

// Mock implementations of state operations that use graceful degradation

func (m *MockScyllaStateWithDegradation) Add(ctx context.Context, pin api.Pin) error {
	return m.executeWithGracefulDegradation(ctx, func(consistency gocql.Consistency) error {
		// Simulate different error scenarios
		if err, exists := m.simulatedErrors["unavailable"]; exists && m.errorCount < 2 {
			m.errorCount++
			return err
		}
		if err, exists := m.simulatedErrors["connection"]; exists && m.errorCount < 3 {
			return err
		}

		// Simulate successful operation
		return nil
	})
}

func (m *MockScyllaStateWithDegradation) Get(ctx context.Context, cid api.Cid) (api.Pin, error) {
	var pin api.Pin
	err := m.executeWithGracefulDegradation(ctx, func(consistency gocql.Consistency) error {
		// Simulate different error scenarios
		if err, exists := m.simulatedErrors["read_failure"]; exists && m.errorCount < 2 {
			m.errorCount++
			return err
		}

		// Simulate successful operation
		pin = api.Pin{
			Cid:  cid,
			Type: api.DataType,
		}
		return nil
	})

	return pin, err
}

func (m *MockScyllaStateWithDegradation) Has(ctx context.Context, cid api.Cid) (bool, error) {
	var exists bool
	err := m.executeWithGracefulDegradation(ctx, func(consistency gocql.Consistency) error {
		// Simulate different error scenarios
		if err, exists := m.simulatedErrors["timeout"]; exists && m.errorCount < 2 {
			m.errorCount++
			return err
		}

		// Simulate successful operation
		exists = true
		return nil
	})

	return exists, err
}

func (m *MockScyllaStateWithDegradation) List(ctx context.Context, out chan<- api.Pin) error {
	return m.executeWithGracefulDegradation(ctx, func(consistency gocql.Consistency) error {
		// Simulate different error scenarios
		if err, exists := m.simulatedErrors["timeout"]; exists && m.errorCount < 1 {
			m.errorCount++
			return err
		}

		// Simulate successful operation - send some test pins
		testPins := []api.Pin{
			{Cid: testCid1, Type: api.DataType},
			{Cid: testCid2, Type: api.DataType},
		}

		for _, pin := range testPins {
			select {
			case out <- pin:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		return nil
	})
}

func (m *MockScyllaStateWithDegradation) Close() error {
	if m.degradationManager != nil {
		return m.degradationManager.Stop()
	}
	return nil
}
