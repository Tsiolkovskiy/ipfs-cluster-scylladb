package scyllastate

import (
	"context"
	"errors"
	"log"
	"os"
	"testing"
	"time"

	"github.com/gocql/gocql"
)

// Simple test that validates graceful degradation logic without external dependencies

func TestGracefulDegradation_BasicFunctionality(t *testing.T) {
	// Test basic graceful degradation manager creation and lifecycle
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

	// Create a minimal mock session
	mockSession := &SimpleGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)
	if gdm == nil {
		t.Fatal("Failed to create GracefulDegradationManager")
	}

	// Test start and stop
	ctx := context.Background()
	err := gdm.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start graceful degradation manager: %v", err)
	}

	err = gdm.Stop()
	if err != nil {
		t.Fatalf("Failed to stop graceful degradation manager: %v", err)
	}
}

func TestGracefulDegradation_NodeHealthTracking(t *testing.T) {
	config := &Config{
		Hosts: []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &SimpleGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)

	// Test initial node health
	nodeHealth := gdm.GetNodeHealth()
	if len(nodeHealth) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(nodeHealth))
	}

	for host, health := range nodeHealth {
		if !health.IsHealthy {
			t.Errorf("Node %s should be initially healthy", host)
		}
		if health.ConsecutiveFailures != 0 {
			t.Errorf("Node %s should have 0 consecutive failures initially", host)
		}
	}

	// Test node failure simulation
	nodeFailureErr := errors.New("connection refused: 127.0.0.1:9042")
	gdm.updateNodeHealthFromError(nodeFailureErr)

	// Verify that error was processed (specific node health changes depend on error parsing)
	t.Logf("Node failure error processed: %v", nodeFailureErr)
}

func TestGracefulDegradation_ConsistencyFallback(t *testing.T) {
	config := &Config{
		Hosts:       []string{"127.0.0.1"},
		Consistency: "QUORUM",
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &SimpleGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)

	// Test initial consistency level
	initialConsistency := gdm.getCurrentConsistency()
	if initialConsistency != gocql.Quorum {
		t.Errorf("Expected initial consistency QUORUM, got %v", initialConsistency)
	}

	// Test consistency degradation
	unavailableErr := &gocql.RequestErrUnavailable{
		Consistency: gocql.Quorum,
		Required:    2,
		Alive:       1,
	}

	shouldDegrade := gdm.shouldDegradeConsistency(unavailableErr)
	if !shouldDegrade {
		t.Error("Should degrade consistency for unavailable error")
	}

	// Perform degradation
	newConsistency := gdm.degradeConsistency()
	if newConsistency == initialConsistency {
		t.Error("Consistency should have been degraded")
	}

	t.Logf("Consistency degraded from %v to %v", initialConsistency, newConsistency)
}

func TestGracefulDegradation_NetworkPartitionDetection(t *testing.T) {
	config := &Config{
		Hosts: []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &SimpleGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)

	// Initially no partition
	if gdm.IsPartitioned() {
		t.Error("Should not be partitioned initially")
	}

	// Simulate multiple node failures to trigger partition
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
	if !isPartition {
		t.Error("Should detect network partition with majority nodes unhealthy")
	}

	if !gdm.IsPartitioned() {
		t.Error("Should be in partitioned state")
	}

	t.Log("Network partition detected successfully")
}

func TestGracefulDegradation_CircuitBreaker(t *testing.T) {
	config := &Config{
		Hosts: []string{"127.0.0.1"},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &SimpleGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)

	host := "127.0.0.1"

	// Get circuit breaker
	gdm.breakerMutex.RLock()
	breaker := gdm.circuitBreakers[host]
	gdm.breakerMutex.RUnlock()

	if breaker == nil {
		t.Fatal("Circuit breaker should exist for host")
	}

	// Initially should be closed
	breaker.mutex.RLock()
	initialState := breaker.state
	breaker.mutex.RUnlock()

	if initialState != CircuitBreakerClosed {
		t.Errorf("Circuit breaker should be initially closed, got %v", initialState)
	}

	// Simulate multiple failures
	for i := 0; i < 6; i++ {
		err := errors.New("connection timeout")
		gdm.updateCircuitBreaker(host, err)
	}

	// Should be open now
	breaker.mutex.RLock()
	finalState := breaker.state
	failureCount := breaker.failureCount
	breaker.mutex.RUnlock()

	if finalState != CircuitBreakerOpen {
		t.Errorf("Circuit breaker should be open after failures, got %v", finalState)
	}

	if failureCount < 5 {
		t.Errorf("Expected at least 5 failures, got %d", failureCount)
	}

	t.Logf("Circuit breaker opened after %d failures", failureCount)
}

func TestGracefulDegradation_ErrorClassification(t *testing.T) {
	config := &Config{
		Hosts: []string{"127.0.0.1"},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &SimpleGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)

	testCases := []struct {
		name          string
		err           error
		shouldDegrade bool
		category      string
	}{
		{
			name: "Unavailable error",
			err: &gocql.RequestErrUnavailable{
				Consistency: gocql.Quorum,
				Required:    2,
				Alive:       1,
			},
			shouldDegrade: true,
			category:      "consistency",
		},
		{
			name:          "Connection error",
			err:           errors.New("connection refused"),
			shouldDegrade: false,
			category:      "node_failure",
		},
		{
			name:          "Timeout error",
			err:           errors.New("operation timed out"),
			shouldDegrade: false,
			category:      "timeout",
		},
		{
			name:          "Syntax error",
			err:           errors.New("syntax error in query"),
			shouldDegrade: false,
			category:      "other",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			shouldDegrade := gdm.shouldDegradeConsistency(tc.err)
			if shouldDegrade != tc.shouldDegrade {
				t.Errorf("Expected shouldDegrade=%v, got %v for error: %v",
					tc.shouldDegrade, shouldDegrade, tc.err)
			}

			category := gdm.retryPolicy.GetErrorCategory(tc.err)
			if category != tc.category {
				t.Errorf("Expected category=%s, got %s for error: %v",
					tc.category, category, tc.err)
			}
		})
	}
}

func TestGracefulDegradation_ExecuteWithDegradation(t *testing.T) {
	config := &Config{
		Hosts:       []string{"127.0.0.1", "127.0.0.2"},
		Consistency: "QUORUM",
		RetryPolicy: RetryPolicyConfig{
			NumRetries:    2,
			MinRetryDelay: 1 * time.Millisecond,
			MaxRetryDelay: 10 * time.Millisecond,
		},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(config.RetryPolicy)
	mockSession := &SimpleGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)

	ctx := context.Background()
	err := gdm.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start degradation manager: %v", err)
	}
	defer gdm.Stop()

	// Test successful operation
	callCount := 0
	err = gdm.ExecuteWithDegradation(ctx, func(consistency gocql.Consistency) error {
		callCount++
		if consistency != gocql.Quorum {
			t.Errorf("Expected QUORUM consistency, got %v", consistency)
		}
		return nil
	})

	if err != nil {
		t.Errorf("Expected successful operation, got error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}

	// Test operation with degradation
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
		// Second call succeeds
		return nil
	})

	if err != nil {
		t.Errorf("Expected operation to succeed after degradation, got error: %v", err)
	}

	if callCount != 2 {
		t.Errorf("Expected 2 calls (original + degraded), got %d", callCount)
	}

	if len(consistencyLevels) != 2 {
		t.Errorf("Expected 2 consistency levels recorded, got %d", len(consistencyLevels))
	}

	t.Logf("Operation succeeded with consistency levels: %v", consistencyLevels)
}

func TestGracefulDegradation_Metrics(t *testing.T) {
	config := &Config{
		Hosts: []string{"127.0.0.1", "127.0.0.2"},
	}

	logger := log.New(os.Stdout, "[Test] ", log.LstdFlags)
	retryPolicy := NewRetryPolicy(RetryPolicyConfig{})
	mockSession := &SimpleGocqlSession{}

	gdm := NewGracefulDegradationManager(mockSession, config, logger, retryPolicy)

	// Test initial metrics
	metrics := gdm.GetMetrics()
	if metrics.HealthyNodes != 2 {
		t.Errorf("Expected 2 healthy nodes, got %d", metrics.HealthyNodes)
	}
	if metrics.UnhealthyNodes != 0 {
		t.Errorf("Expected 0 unhealthy nodes, got %d", metrics.UnhealthyNodes)
	}
	if metrics.NodeFailures != 0 {
		t.Errorf("Expected 0 node failures, got %d", metrics.NodeFailures)
	}
	if metrics.IsPartitioned {
		t.Error("Should not be partitioned initially")
	}

	t.Logf("Initial metrics: healthy=%d, unhealthy=%d, failures=%d, partitioned=%v",
		metrics.HealthyNodes, metrics.UnhealthyNodes, metrics.NodeFailures, metrics.IsPartitioned)
}

// SimpleGocqlSession is a minimal mock for testing
type SimpleGocqlSession struct{}

func (s *SimpleGocqlSession) Query(stmt string, values ...interface{}) *gocql.Query {
	// Return a minimal query mock
	return &gocql.Query{}
}

func (s *SimpleGocqlSession) Close() {
	// No-op for mock
}

// Benchmark for performance validation
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
	mockSession := &SimpleGocqlSession{}

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
