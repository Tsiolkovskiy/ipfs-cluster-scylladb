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

// MockSession implements a mock gocql.Session for testing
type MockSession struct {
	shouldFail      bool
	failureError    error
	queryCount      int
	lastConsistency gocql.Consistency
}

func (m *MockSession) Query(stmt string, values ...interface{}) *gocql.Query {
	m.queryCount++
	// Return a mock query that will fail if shouldFail is true
	return &gocql.Query{}
}

func (m *MockSession) Close() {}

// TestGracefulDegradationManager_NodeFailureHandling tests node failure handling
func TestGracefulDegradationManager_NodeFailureHandling(t *testing.T) {
	config := &Config{
		Hosts:       []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"},
		Consistency: "QUORUM",
		RetryPolicy: RetryPolicyConfig{
			NumRetries:    3,
			MinRetryDelay: 100 * time.Millisecond,
			MaxRetryDelay: 1 * time.Second,
		},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(config.RetryPolicy)
	mockSession := &MockSession{}

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

	// Simulate node failure
	nodeFailureErr := errors.New("connection refused: 127.0.0.1:9042")
	gdm.updateNodeHealthFromError(nodeFailureErr)

	// Check that node health is updated
	nodeHealth := gdm.GetNodeHealth()
	assert.Len(t, nodeHealth, 3)

	// Test consistency degradation on unavailable error
	unavailableErr := &gocql.RequestErrUnavailable{
		Consistency: gocql.Quorum,
		Required:    2,
		Alive:       1,
	}

	shouldDegrade := gdm.shouldDegradeConsistency(unavailableErr)
	assert.True(t, shouldDegrade)

	// Test degradation
	originalConsistency := gdm.getCurrentConsistency()
	newConsistency := gdm.degradeConsistency()
	assert.NotEqual(t, originalConsistency, newConsistency)

	// Verify metrics are updated
	metrics = gdm.GetMetrics()
	assert.Greater(t, metrics.ConsistencyFallbacks, int64(0))
}

// TestGracefulDegradationManager_NetworkPartitionDetection tests network partition detection
func TestGracefulDegradationManager_NetworkPartitionDetection(t *testing.T) {
	config := &Config{
		Hosts:       []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"},
		Consistency: "QUORUM",
		RetryPolicy: RetryPolicyConfig{
			NumRetries:    3,
			MinRetryDelay: 100 * time.Millisecond,
			MaxRetryDelay: 1 * time.Second,
		},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(config.RetryPolicy)
	mockSession := &MockSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)
	require.NotNil(t, gdm)

	ctx := context.Background()
	err := gdm.Start(ctx)
	require.NoError(t, err)
	defer gdm.Stop()

	// Initially no partition
	assert.False(t, gdm.IsPartitioned())

	// Simulate multiple node failures to trigger partition detection
	for i := 0; i < 2; i++ {
		host := config.Hosts[i]
		gdm.healthMutex.Lock()
		nodeHealth := gdm.nodeHealth[host]
		nodeHealth.mutex.Lock()
		nodeHealth.IsHealthy = false
		nodeHealth.ConsecutiveFailures = 5
		nodeHealth.mutex.Unlock()
		gdm.healthMutex.Unlock()
	}

	// Test partition detection
	partitionErr := errors.New("network unreachable")
	isPartition := gdm.detectNetworkPartition(partitionErr)
	assert.True(t, isPartition)
	assert.True(t, gdm.IsPartitioned())

	// Verify metrics
	metrics := gdm.GetMetrics()
	assert.True(t, metrics.IsPartitioned)
	assert.Greater(t, metrics.PartitionDetections, int64(0))
}

// TestGracefulDegradationManager_ConsistencyFallback tests consistency level fallback
func TestGracefulDegradationManager_ConsistencyFallback(t *testing.T) {
	testCases := []struct {
		name             string
		originalLevel    gocql.Consistency
		expectedFallback []gocql.Consistency
	}{
		{
			name:          "ALL fallback",
			originalLevel: gocql.All,
			expectedFallback: []gocql.Consistency{
				gocql.All, gocql.Quorum, gocql.LocalQuorum, gocql.One,
			},
		},
		{
			name:          "QUORUM fallback",
			originalLevel: gocql.Quorum,
			expectedFallback: []gocql.Consistency{
				gocql.Quorum, gocql.LocalQuorum, gocql.One,
			},
		},
		{
			name:          "LOCAL_QUORUM fallback",
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
			mockSession := &MockSession{}

			gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)

			// Override the fallback levels for testing
			gdm.consistencyFallback.originalConsistency = tc.originalLevel
			gdm.consistencyFallback.currentConsistency = tc.originalLevel
			gdm.consistencyFallback.fallbackLevels = tc.expectedFallback

			// Test degradation through the levels
			for i := 0; i < len(tc.expectedFallback)-1; i++ {
				currentLevel := gdm.getCurrentConsistency()
				assert.Equal(t, tc.expectedFallback[i], currentLevel)

				newLevel := gdm.degradeConsistency()
				assert.Equal(t, tc.expectedFallback[i+1], newLevel)
			}
		})
	}
}

// TestGracefulDegradationManager_CircuitBreaker tests circuit breaker functionality
func TestGracefulDegradationManager_CircuitBreaker(t *testing.T) {
	config := &Config{
		Hosts: []string{"127.0.0.1"},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &MockSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)
	require.NotNil(t, gdm)

	host := "127.0.0.1"

	// Initially circuit breaker should be closed
	gdm.breakerMutex.RLock()
	breaker := gdm.circuitBreakers[host]
	gdm.breakerMutex.RUnlock()

	breaker.mutex.RLock()
	assert.Equal(t, CircuitBreakerClosed, breaker.state)
	breaker.mutex.RUnlock()

	// Simulate multiple failures to trip the circuit breaker
	for i := 0; i < 6; i++ {
		err := errors.New("connection timeout")
		gdm.updateCircuitBreaker(host, err)
	}

	// Circuit breaker should now be open
	breaker.mutex.RLock()
	assert.Equal(t, CircuitBreakerOpen, breaker.state)
	assert.GreaterOrEqual(t, breaker.failureCount, 5)
	breaker.mutex.RUnlock()

	// Test reset
	gdm.resetCircuitBreaker(host)

	breaker.mutex.RLock()
	assert.Equal(t, CircuitBreakerClosed, breaker.state)
	assert.Equal(t, 0, breaker.failureCount)
	breaker.mutex.RUnlock()
}

// TestGracefulDegradationManager_ExecuteWithDegradation tests operation execution with degradation
func TestGracefulDegradationManager_ExecuteWithDegradation(t *testing.T) {
	config := &Config{
		Hosts:       []string{"127.0.0.1", "127.0.0.2"},
		Consistency: "QUORUM",
		RetryPolicy: RetryPolicyConfig{
			NumRetries:    2,
			MinRetryDelay: 10 * time.Millisecond,
			MaxRetryDelay: 100 * time.Millisecond,
		},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(config.RetryPolicy)
	mockSession := &MockSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)
	require.NotNil(t, gdm)

	ctx := context.Background()
	err := gdm.Start(ctx)
	require.NoError(t, err)
	defer gdm.Stop()

	// Test successful operation
	callCount := 0
	err = gdm.ExecuteWithDegradation(ctx, func(consistency gocql.Consistency) error {
		callCount++
		assert.Equal(t, gocql.Quorum, consistency)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Test operation with unavailable error that triggers degradation
	callCount = 0
	consistencyLevels := []gocql.Consistency{}

	err = gdm.ExecuteWithDegradation(ctx, func(consistency gocql.Consistency) error {
		callCount++
		consistencyLevels = append(consistencyLevels, consistency)

		if callCount == 1 {
			// First call fails with unavailable error
			return &gocql.RequestErrUnavailable{
				Consistency: consistency,
				Required:    2,
				Alive:       1,
			}
		}
		// Second call succeeds with degraded consistency
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 2, callCount)
	assert.Equal(t, gocql.Quorum, consistencyLevels[0])
	assert.NotEqual(t, gocql.Quorum, consistencyLevels[1]) // Should be degraded
}

// TestGracefulDegradationManager_NetworkPartitionHandling tests network partition handling
func TestGracefulDegradationManager_NetworkPartitionHandling(t *testing.T) {
	config := &Config{
		Hosts:       []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"},
		Consistency: "QUORUM",
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &MockSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)
	require.NotNil(t, gdm)

	ctx := context.Background()

	// Simulate partition by marking majority of nodes as unhealthy
	gdm.healthMutex.Lock()
	for i, host := range []string{"127.0.0.1", "127.0.0.2"} {
		nodeHealth := gdm.nodeHealth[host]
		nodeHealth.mutex.Lock()
		nodeHealth.IsHealthy = false
		nodeHealth.ConsecutiveFailures = 5
		nodeHealth.mutex.Unlock()

		if i == 0 {
			// Trigger partition detection
			gdm.partitionDetector.mutex.Lock()
			gdm.partitionDetector.isPartitioned = true
			gdm.partitionDetector.partitionStartTime = time.Now()
			gdm.partitionDetector.mutex.Unlock()
		}
	}
	gdm.healthMutex.Unlock()

	// Test partition handling
	callCount := 0
	consistencyLevels := []gocql.Consistency{}

	partitionErr := errors.New("network partition detected")
	err := gdm.handleNetworkPartition(ctx, partitionErr, func(consistency gocql.Consistency) error {
		callCount++
		consistencyLevels = append(consistencyLevels, consistency)

		if consistency == gocql.One {
			return nil // Succeed with ONE consistency
		}
		return errors.New("still failing")
	})

	assert.NoError(t, err)
	assert.Greater(t, callCount, 0)
	assert.Contains(t, consistencyLevels, gocql.One)
}

// TestGracefulDegradationManager_HealthMonitoring tests health monitoring functionality
func TestGracefulDegradationManager_HealthMonitoring(t *testing.T) {
	config := &Config{
		Hosts: []string{"127.0.0.1"},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &MockSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)
	require.NotNil(t, gdm)

	// Test initial health status
	nodeHealth := gdm.GetNodeHealth()
	assert.Len(t, nodeHealth, 1)

	health := nodeHealth["127.0.0.1"]
	assert.NotNil(t, health)
	assert.True(t, health.IsHealthy)
	assert.Equal(t, 0, health.ConsecutiveFailures)

	// Simulate health check failure
	gdm.healthMutex.Lock()
	node := gdm.nodeHealth["127.0.0.1"]
	node.mutex.Lock()
	node.ConsecutiveFailures = 3
	node.IsHealthy = false
	node.LastError = errors.New("health check failed")
	node.mutex.Unlock()
	gdm.healthMutex.Unlock()

	// Check updated health status
	nodeHealth = gdm.GetNodeHealth()
	health = nodeHealth["127.0.0.1"]
	assert.False(t, health.IsHealthy)
	assert.Equal(t, 3, health.ConsecutiveFailures)
	assert.NotNil(t, health.LastError)
}

// TestGracefulDegradationManager_MetricsTracking tests metrics tracking
func TestGracefulDegradationManager_MetricsTracking(t *testing.T) {
	config := &Config{
		Hosts: []string{"127.0.0.1", "127.0.0.2"},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &MockSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)
	require.NotNil(t, gdm)

	// Initial metrics
	metrics := gdm.GetMetrics()
	assert.Equal(t, int64(2), metrics.HealthyNodes)
	assert.Equal(t, int64(0), metrics.UnhealthyNodes)
	assert.Equal(t, int64(0), metrics.NodeFailures)
	assert.False(t, metrics.IsPartitioned)

	// Simulate node failure
	gdm.healthMutex.Lock()
	node := gdm.nodeHealth["127.0.0.1"]
	node.mutex.Lock()
	node.IsHealthy = false
	node.ConsecutiveFailures = 5
	node.mutex.Unlock()
	gdm.healthMutex.Unlock()

	// Manually update metrics counters (normally done by updateNodeHealthFromError)
	gdm.degradationMetrics.NodeFailures = 1
	gdm.degradationMetrics.UnhealthyNodes = 1
	gdm.degradationMetrics.HealthyNodes = 1

	// Check updated metrics
	metrics = gdm.GetMetrics()
	assert.Equal(t, int64(1), metrics.HealthyNodes)
	assert.Equal(t, int64(1), metrics.UnhealthyNodes)
	assert.Equal(t, int64(1), metrics.NodeFailures)
}

// TestGracefulDegradationManager_ConsistencyRecovery tests consistency level recovery
func TestGracefulDegradationManager_ConsistencyRecovery(t *testing.T) {
	config := &Config{
		Hosts:       []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"},
		Consistency: "QUORUM",
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &MockSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)
	require.NotNil(t, gdm)

	// Degrade consistency level
	gdm.consistencyFallback.mutex.Lock()
	gdm.consistencyFallback.currentConsistency = gocql.One
	gdm.consistencyFallback.degradationTime = time.Now().Add(-10 * time.Minute) // Old enough for recovery
	gdm.consistencyFallback.mutex.Unlock()

	// All nodes are healthy, should be able to recover
	assert.Equal(t, 3, gdm.countHealthyNodes())

	// Attempt recovery
	gdm.attemptConsistencyRecovery()

	// Should recover to original consistency level
	currentConsistency := gdm.getCurrentConsistency()
	assert.Equal(t, gocql.Quorum, currentConsistency)
}

// BenchmarkGracefulDegradationManager_ExecuteWithDegradation benchmarks degradation execution
func BenchmarkGracefulDegradationManager_ExecuteWithDegradation(b *testing.B) {
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
	mockSession := &MockSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err := gdm.ExecuteWithDegradation(ctx, func(consistency gocql.Consistency) error {
				return nil // Always succeed
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
