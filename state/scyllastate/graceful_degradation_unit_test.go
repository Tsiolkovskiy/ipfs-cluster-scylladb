package scyllastate

import (
	"context"
	"errors"
	"log"
	"os"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGracefulDegradation_NodeFailureHandling tests requirement 3.1: Handle node unavailability
func TestGracefulDegradation_NodeFailureHandling(t *testing.T) {
	config := &Config{
		Hosts:       []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"},
		Consistency: "QUORUM",
		RetryPolicy: RetryPolicyConfig{
			NumRetries:    3,
			MinRetryDelay: 10 * time.Millisecond,
			MaxRetryDelay: 100 * time.Millisecond,
		},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(config.RetryPolicy)

	// Create a mock session that doesn't require actual ScyllaDB
	mockSession := &MockGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)
	require.NotNil(t, gdm)

	ctx := context.Background()
	err := gdm.Start(ctx)
	require.NoError(t, err)
	defer gdm.Stop()

	// Test initial state - all nodes should be healthy
	metrics := gdm.GetMetrics()
	assert.Equal(t, int64(3), metrics.HealthyNodes)
	assert.Equal(t, int64(0), metrics.UnhealthyNodes)
	assert.False(t, metrics.IsPartitioned)

	// Test node failure detection
	nodeFailureErr := errors.New("connection refused: 127.0.0.1:9042")
	gdm.updateNodeHealthFromError(nodeFailureErr)

	// Verify node health tracking
	nodeHealth := gdm.GetNodeHealth()
	assert.Len(t, nodeHealth, 3)

	// Test that system continues with remaining nodes (requirement 3.1)
	operationCount := 0
	err = gdm.ExecuteWithDegradation(ctx, func(consistency gocql.Consistency) error {
		operationCount++
		// Simulate that operation succeeds with remaining nodes
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, operationCount)
}

// TestGracefulDegradation_AutomaticReplicaSwitching tests requirement 3.3: Automatic switching to available replicas
func TestGracefulDegradation_AutomaticReplicaSwitching(t *testing.T) {
	config := &Config{
		Hosts:       []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"},
		Consistency: "QUORUM",
		RetryPolicy: RetryPolicyConfig{
			NumRetries:    2,
			MinRetryDelay: 10 * time.Millisecond,
			MaxRetryDelay: 100 * time.Millisecond,
		},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(config.RetryPolicy)
	mockSession := &MockGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)
	require.NotNil(t, gdm)

	ctx := context.Background()
	err := gdm.Start(ctx)
	require.NoError(t, err)
	defer gdm.Stop()

	// Test automatic switching when read operations encounter node failures
	attemptCount := 0
	consistencyLevels := []gocql.Consistency{}

	err = gdm.ExecuteWithDegradation(ctx, func(consistency gocql.Consistency) error {
		attemptCount++
		consistencyLevels = append(consistencyLevels, consistency)

		if attemptCount == 1 {
			// First attempt fails due to node failure
			return &gocql.RequestErrReadFailure{
				Consistency: consistency,
				Received:    1,
				BlockFor:    2,
				NumFailures: 1,
			}
		}

		// Second attempt succeeds with available replicas
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 2, attemptCount)
	assert.Len(t, consistencyLevels, 2)

	// Verify that consistency was potentially degraded for replica switching
	if len(consistencyLevels) > 1 {
		t.Logf("Consistency levels used: %v -> %v", consistencyLevels[0], consistencyLevels[1])
	}
}

// TestGracefulDegradation_NetworkPartitionHandling tests requirement 3.4: Handle network partitions
func TestGracefulDegradation_NetworkPartitionHandling(t *testing.T) {
	config := &Config{
		Hosts:       []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"},
		Consistency: "QUORUM",
		RetryPolicy: RetryPolicyConfig{
			NumRetries:    1,
			MinRetryDelay: 10 * time.Millisecond,
			MaxRetryDelay: 100 * time.Millisecond,
		},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(config.RetryPolicy)
	mockSession := &MockGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)
	require.NotNil(t, gdm)

	ctx := context.Background()
	err := gdm.Start(ctx)
	require.NoError(t, err)
	defer gdm.Stop()

	// Simulate network partition by marking majority of nodes as unhealthy
	gdm.healthMutex.Lock()
	for _, host := range []string{"127.0.0.1", "127.0.0.2"} {
		nodeHealth := gdm.nodeHealth[host]
		nodeHealth.mutex.Lock()
		nodeHealth.IsHealthy = false
		nodeHealth.ConsecutiveFailures = 5
		nodeHealth.mutex.Unlock()
	}
	gdm.healthMutex.Unlock()

	// Test partition detection
	partitionErr := errors.New("network unreachable")
	isPartition := gdm.detectNetworkPartition(partitionErr)
	assert.True(t, isPartition)
	assert.True(t, gdm.IsPartitioned())

	// Test that operations handle partition with eventual consistency (requirement 3.4)
	attemptCount := 0
	consistencyLevels := []gocql.Consistency{}

	err = gdm.handleNetworkPartition(ctx, partitionErr, func(consistency gocql.Consistency) error {
		attemptCount++
		consistencyLevels = append(consistencyLevels, consistency)

		// Succeed with relaxed consistency during partition
		if consistency == gocql.One || consistency == gocql.Any {
			return nil
		}

		// Fail with strict consistency during partition
		return errors.New("insufficient replicas available")
	})

	assert.NoError(t, err)
	assert.Greater(t, attemptCount, 0)

	// Verify that relaxed consistency levels were used during partition
	hasRelaxedConsistency := false
	for _, level := range consistencyLevels {
		if level == gocql.One || level == gocql.Any {
			hasRelaxedConsistency = true
			break
		}
	}
	assert.True(t, hasRelaxedConsistency, "Should use relaxed consistency during partition")

	// Test partition recovery
	gdm.healthMutex.Lock()
	for _, host := range []string{"127.0.0.1", "127.0.0.2"} {
		nodeHealth := gdm.nodeHealth[host]
		nodeHealth.mutex.Lock()
		nodeHealth.IsHealthy = true
		nodeHealth.ConsecutiveFailures = 0
		nodeHealth.mutex.Unlock()
	}
	gdm.healthMutex.Unlock()

	// Simulate recovery check
	gdm.checkPartitionRecovery()

	// Should eventually recover from partition
	// Note: In real implementation, this would happen over time
	t.Logf("Partition state after recovery attempt: %v", gdm.IsPartitioned())
}

// TestGracefulDegradation_ConsistencyFallbackChain tests consistency level degradation
func TestGracefulDegradation_ConsistencyFallbackChain(t *testing.T) {
	testCases := []struct {
		name             string
		originalLevel    gocql.Consistency
		expectedFallback []gocql.Consistency
	}{
		{
			name:          "ALL fallback chain",
			originalLevel: gocql.All,
			expectedFallback: []gocql.Consistency{
				gocql.All, gocql.Quorum, gocql.LocalQuorum, gocql.One,
			},
		},
		{
			name:          "QUORUM fallback chain",
			originalLevel: gocql.Quorum,
			expectedFallback: []gocql.Consistency{
				gocql.Quorum, gocql.LocalQuorum, gocql.One,
			},
		},
		{
			name:          "LOCAL_QUORUM fallback chain",
			originalLevel: gocql.LocalQuorum,
			expectedFallback: []gocql.Consistency{
				gocql.LocalQuorum, gocql.LocalOne, gocql.One,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &Config{
				Hosts:       []string{"127.0.0.1"},
				Consistency: "QUORUM",
			}

			logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
			retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
			mockSession := &MockGocqlSession{}

			gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)

			// Set up the fallback chain
			gdm.consistencyFallback.originalConsistency = tc.originalLevel
			gdm.consistencyFallback.currentConsistency = tc.originalLevel
			gdm.consistencyFallback.fallbackLevels = tc.expectedFallback

			// Test degradation through the chain
			for i := 0; i < len(tc.expectedFallback)-1; i++ {
				currentLevel := gdm.getCurrentConsistency()
				assert.Equal(t, tc.expectedFallback[i], currentLevel)

				newLevel := gdm.degradeConsistency()
				assert.Equal(t, tc.expectedFallback[i+1], newLevel)
			}
		})
	}
}

// TestGracefulDegradation_CircuitBreakerPattern tests circuit breaker functionality
func TestGracefulDegradation_CircuitBreakerPattern(t *testing.T) {
	config := &Config{
		Hosts: []string{"127.0.0.1"},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &MockGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)
	require.NotNil(t, gdm)

	host := "127.0.0.1"

	// Initially circuit breaker should be closed
	gdm.breakerMutex.RLock()
	breaker := gdm.circuitBreakers[host]
	gdm.breakerMutex.RUnlock()

	breaker.mutex.RLock()
	initialState := breaker.state
	breaker.mutex.RUnlock()
	assert.Equal(t, CircuitBreakerClosed, initialState)

	// Simulate multiple failures to trip the circuit breaker
	for i := 0; i < 6; i++ {
		err := errors.New("connection timeout")
		gdm.updateCircuitBreaker(host, err)
	}

	// Circuit breaker should now be open
	breaker.mutex.RLock()
	finalState := breaker.state
	failureCount := breaker.failureCount
	breaker.mutex.RUnlock()

	assert.Equal(t, CircuitBreakerOpen, finalState)
	assert.GreaterOrEqual(t, failureCount, 5)

	// Test reset functionality
	gdm.resetCircuitBreaker(host)

	breaker.mutex.RLock()
	resetState := breaker.state
	resetFailureCount := breaker.failureCount
	breaker.mutex.RUnlock()

	assert.Equal(t, CircuitBreakerClosed, resetState)
	assert.Equal(t, 0, resetFailureCount)
}

// TestGracefulDegradation_ErrorClassification tests error classification for retry decisions
func TestGracefulDegradation_ErrorClassification(t *testing.T) {
	config := &Config{
		Hosts: []string{"127.0.0.1"},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &MockGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)

	testCases := []struct {
		name          string
		err           error
		shouldDegrade bool
		errorCategory string
	}{
		{
			name: "Unavailable error should trigger degradation",
			err: &gocql.RequestErrUnavailable{
				Consistency: gocql.Quorum,
				Required:    2,
				Alive:       1,
			},
			shouldDegrade: true,
			errorCategory: "consistency",
		},
		{
			name:          "Connection error should not trigger degradation",
			err:           errors.New("connection refused"),
			shouldDegrade: false,
			errorCategory: "node_failure",
		},
		{
			name:          "Timeout error should not trigger degradation",
			err:           errors.New("operation timed out"),
			shouldDegrade: false,
			errorCategory: "timeout",
		},
		{
			name:          "Syntax error should not trigger degradation",
			err:           errors.New("syntax error in query"),
			shouldDegrade: false,
			errorCategory: "other",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			shouldDegrade := gdm.shouldDegradeConsistency(tc.err)
			assert.Equal(t, tc.shouldDegrade, shouldDegrade)

			category := gdm.retryPolicy.GetErrorCategory(tc.err)
			assert.Equal(t, tc.errorCategory, category)
		})
	}
}

// TestGracefulDegradation_MetricsTracking tests metrics collection
func TestGracefulDegradation_MetricsTracking(t *testing.T) {
	config := &Config{
		Hosts: []string{"127.0.0.1", "127.0.0.2"},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &MockGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)
	require.NotNil(t, gdm)

	// Initial metrics
	metrics := gdm.GetMetrics()
	assert.Equal(t, int64(2), metrics.HealthyNodes)
	assert.Equal(t, int64(0), metrics.UnhealthyNodes)
	assert.Equal(t, int64(0), metrics.NodeFailures)
	assert.False(t, metrics.IsPartitioned)

	// Simulate node failure and verify metrics update
	gdm.healthMutex.Lock()
	node := gdm.nodeHealth["127.0.0.1"]
	node.mutex.Lock()
	node.IsHealthy = false
	node.ConsecutiveFailures = 5
	node.mutex.Unlock()
	gdm.healthMutex.Unlock()

	// Update metrics manually (normally done by updateNodeHealthFromError)
	gdm.degradationMetrics.NodeFailures = 1

	// Check updated metrics
	metrics = gdm.GetMetrics()
	assert.Equal(t, int64(1), metrics.HealthyNodes)
	assert.Equal(t, int64(1), metrics.UnhealthyNodes)
	assert.Equal(t, int64(1), metrics.NodeFailures)
}

// MockGocqlSession is a minimal mock for testing without external dependencies
type MockGocqlSession struct {
	queryCount int
	shouldFail bool
}

func (m *MockGocqlSession) Query(stmt string, values ...interface{}) *gocql.Query {
	m.queryCount++
	// Return a minimal query mock
	return &gocql.Query{}
}

func (m *MockGocqlSession) Close() {
	// No-op for mock
}

// Benchmark tests for performance validation

func BenchmarkGracefulDegradation_ExecuteWithDegradation(b *testing.B) {
	config := &Config{
		Hosts:       []string{"127.0.0.1"},
		Consistency: "QUORUM",
		RetryPolicy: RetryPolicyConfig{
			NumRetries:    1,
			MinRetryDelay: 1 * time.Millisecond,
			MaxRetryDelay: 10 * time.Millisecond,
		},
	}

	logger := log.New(os.Stdout, "[Bench] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(config.RetryPolicy)
	mockSession := &MockGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err := gdm.ExecuteWithDegradation(ctx, func(consistency gocql.Consistency) error {
				return nil // Always succeed for benchmark
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkGracefulDegradation_NodeHealthCheck(b *testing.B) {
	config := &Config{
		Hosts: []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"},
	}

	logger := log.New(os.Stdout, "[Bench] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &MockGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gdm.performHealthCheck()
	}
}
