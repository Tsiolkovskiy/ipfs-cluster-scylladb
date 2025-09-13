package scyllastate

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gocql/gocql"
)

// GracefulDegradationManager handles node failures and network partitions
// Implements requirements 3.1, 3.3, and 3.4 for graceful degradation
type GracefulDegradationManager struct {
	session     *gocql.Session
	config      *Config
	logger      *log.Logger
	retryPolicy *RetryPolicy

	// Node health tracking
	nodeHealth  map[string]*NodeHealthStatus
	healthMutex sync.RWMutex

	// Consistency level fallback
	consistencyFallback *ConsistencyFallback

	// Network partition detection
	partitionDetector *NetworkPartitionDetector

	// Circuit breaker for failed nodes
	circuitBreakers map[string]*CircuitBreaker
	breakerMutex    sync.RWMutex

	// Metrics
	degradationMetrics *DegradationMetrics
	prometheusMetrics  Metrics // Interface to Prometheus metrics

	// State
	isActive int32 // atomic flag
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NodeHealthStatus tracks the health of individual ScyllaDB nodes
type NodeHealthStatus struct {
	Host                string
	IsHealthy           bool
	LastSeen            time.Time
	ConsecutiveFailures int
	LastError           error
	ResponseTime        time.Duration

	// Connection state
	IsConnected      bool
	ConnectionErrors int

	mutex sync.RWMutex
}

// ConsistencyFallback manages automatic consistency level degradation
type ConsistencyFallback struct {
	originalConsistency gocql.Consistency
	currentConsistency  gocql.Consistency
	fallbackLevels      []gocql.Consistency
	degradationTime     time.Time

	mutex sync.RWMutex
}

// NetworkPartitionDetector detects and handles network partitions
type NetworkPartitionDetector struct {
	partitionThreshold int           // Minimum nodes to consider partition
	detectionWindow    time.Duration // Time window for partition detection
	recoveryWindow     time.Duration // Time to wait before declaring recovery

	isPartitioned      bool
	partitionStartTime time.Time
	availableNodes     []string

	mutex sync.RWMutex
}

// CircuitBreaker implements circuit breaker pattern for failed nodes
type CircuitBreaker struct {
	host            string
	state           CircuitBreakerState
	failureCount    int
	lastFailureTime time.Time
	nextRetryTime   time.Time

	// Configuration
	failureThreshold int
	timeout          time.Duration
	resetTimeout     time.Duration

	mutex sync.RWMutex
}

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int

const (
	CircuitBreakerClosed CircuitBreakerState = iota
	CircuitBreakerOpen
	CircuitBreakerHalfOpen
)

// DegradationMetrics tracks graceful degradation metrics
type DegradationMetrics struct {
	NodeFailures         int64
	ConsistencyFallbacks int64
	PartitionDetections  int64
	CircuitBreakerTrips  int64
	RecoveryEvents       int64

	// Current state
	HealthyNodes       int64
	UnhealthyNodes     int64
	CurrentConsistency string
	IsPartitioned      bool
}

// NewGracefulDegradationManager creates a new graceful degradation manager
func NewGracefulDegradationManager(session *gocql.Session, config *Config, logger *log.Logger, retryPolicy *RetryPolicy, metrics Metrics) *GracefulDegradationManager {
	gdm := &GracefulDegradationManager{
		session:           session,
		config:            config,
		logger:            logger,
		retryPolicy:       retryPolicy,
		nodeHealth:        make(map[string]*NodeHealthStatus),
		circuitBreakers:   make(map[string]*CircuitBreaker),
		stopCh:            make(chan struct{}),
		prometheusMetrics: metrics,
		degradationMetrics: &DegradationMetrics{
			CurrentConsistency: config.Consistency,
		},
	}

	// Initialize consistency fallback
	gdm.consistencyFallback = &ConsistencyFallback{
		originalConsistency: config.GetConsistency(),
		currentConsistency:  config.GetConsistency(),
		fallbackLevels:      gdm.createFallbackLevels(config.GetConsistency()),
	}

	// Initialize partition detector
	gdm.partitionDetector = &NetworkPartitionDetector{
		partitionThreshold: len(config.Hosts) / 2, // Majority threshold
		detectionWindow:    30 * time.Second,
		recoveryWindow:     60 * time.Second,
	}

	// Initialize node health for all configured hosts
	for _, host := range config.Hosts {
		gdm.nodeHealth[host] = &NodeHealthStatus{
			Host:        host,
			IsHealthy:   true,
			LastSeen:    time.Now(),
			IsConnected: true,
		}

		gdm.circuitBreakers[host] = &CircuitBreaker{
			host:             host,
			state:            CircuitBreakerClosed,
			failureThreshold: 5,
			timeout:          30 * time.Second,
			resetTimeout:     60 * time.Second,
		}
	}

	return gdm
}

// updateNodeHealthMetrics updates Prometheus metrics for node health
func (gdm *GracefulDegradationManager) updateNodeHealthMetrics() {
	if gdm.prometheusMetrics == nil {
		return
	}

	gdm.healthMutex.RLock()
	defer gdm.healthMutex.RUnlock()

	for host, health := range gdm.nodeHealth {
		health.mutex.RLock()
		// Extract datacenter and rack info (would need to be populated from cluster metadata)
		datacenter := "unknown"
		rack := "unknown"
		gdm.prometheusMetrics.UpdateNodeHealth(host, datacenter, rack, health.IsHealthy)
		health.mutex.RUnlock()
	}
}

// updateDegradationMetrics updates Prometheus metrics for degradation status
func (gdm *GracefulDegradationManager) updateDegradationMetrics() {
	if gdm.prometheusMetrics == nil {
		return
	}

	// Update degradation active status
	isActive := atomic.LoadInt32(&gdm.isActive) == 1
	gdm.prometheusMetrics.UpdateDegradationMetrics(isActive, 0, "general")

	// Update partition status
	gdm.partitionDetector.mutex.RLock()
	partitioned := gdm.partitionDetector.isPartitioned
	gdm.partitionDetector.mutex.RUnlock()
	gdm.prometheusMetrics.UpdatePartitionStatus(partitioned)

	// Update consistency level
	gdm.consistencyFallback.mutex.RLock()
	currentLevel := gdm.consistencyFallback.currentConsistency.String()
	gdm.consistencyFallback.mutex.RUnlock()
	gdm.prometheusMetrics.UpdateConsistencyLevel("read", currentLevel)
	gdm.prometheusMetrics.UpdateConsistencyLevel("write", currentLevel)
}

// Start begins the graceful degradation monitoring
func (gdm *GracefulDegradationManager) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&gdm.isActive, 0, 1) {
		return fmt.Errorf("graceful degradation manager already started")
	}

	gdm.logger.Printf("Starting graceful degradation manager")

	// Start health monitoring goroutine
	gdm.wg.Add(1)
	go gdm.healthMonitorLoop(ctx)

	// Start partition detection goroutine
	gdm.wg.Add(1)
	go gdm.partitionDetectionLoop(ctx)

	// Start recovery monitoring goroutine
	gdm.wg.Add(1)
	go gdm.recoveryMonitorLoop(ctx)

	return nil
}

// Stop stops the graceful degradation monitoring
func (gdm *GracefulDegradationManager) Stop() error {
	if !atomic.CompareAndSwapInt32(&gdm.isActive, 1, 0) {
		return nil // Already stopped
	}

	gdm.logger.Printf("Stopping graceful degradation manager")

	close(gdm.stopCh)
	gdm.wg.Wait()

	return nil
}

// ExecuteWithDegradation executes an operation with graceful degradation support
// Implements requirement 3.1: Handle node unavailability with remaining available nodes
func (gdm *GracefulDegradationManager) ExecuteWithDegradation(ctx context.Context, operation func(consistency gocql.Consistency) error) error {
	// Get current consistency level (may be degraded)
	consistency := gdm.getCurrentConsistency()

	// Try with current consistency level
	err := operation(consistency)
	if err == nil {
		return nil
	}

	// Handle the error with graceful degradation
	return gdm.handleOperationError(ctx, err, operation)
}

// handleOperationError handles operation errors with graceful degradation strategies
func (gdm *GracefulDegradationManager) handleOperationError(ctx context.Context, err error, operation func(consistency gocql.Consistency) error) error {
	// Update node health based on error
	gdm.updateNodeHealthFromError(err)

	// Check if we should degrade consistency level
	if gdm.shouldDegradeConsistency(err) {
		newConsistency := gdm.degradeConsistency()
		if newConsistency != gdm.getCurrentConsistency() {
			gdm.logger.Printf("Degrading consistency level from %v to %v due to error: %v",
				gdm.getCurrentConsistency(), newConsistency, err)

			// Retry with degraded consistency
			retryErr := operation(newConsistency)
			if retryErr == nil {
				atomic.AddInt64(&gdm.degradationMetrics.ConsistencyFallbacks, 1)
				return nil
			}

			// If degraded consistency also fails, continue with original error handling
			err = retryErr
		}
	}

	// Check for network partition
	if gdm.detectNetworkPartition(err) {
		return gdm.handleNetworkPartition(ctx, err, operation)
	}

	// Use retry policy for other errors
	return gdm.retryPolicy.ExecuteWithRetry(ctx, func() error {
		return operation(gdm.getCurrentConsistency())
	})
}

// updateNodeHealthFromError updates node health status based on operation errors
func (gdm *GracefulDegradationManager) updateNodeHealthFromError(err error) {
	if err == nil {
		return
	}

	// Extract host information from error if possible
	host := gdm.extractHostFromError(err)
	if host == "" {
		return
	}

	gdm.healthMutex.Lock()
	defer gdm.healthMutex.Unlock()

	nodeHealth, exists := gdm.nodeHealth[host]
	if !exists {
		return
	}

	nodeHealth.mutex.Lock()
	defer nodeHealth.mutex.Unlock()

	// Update failure count and health status
	nodeHealth.ConsecutiveFailures++
	nodeHealth.LastError = err

	// Mark as unhealthy if too many consecutive failures
	if nodeHealth.ConsecutiveFailures >= 3 {
		if nodeHealth.IsHealthy {
			gdm.logger.Printf("Marking node %s as unhealthy after %d consecutive failures",
				host, nodeHealth.ConsecutiveFailures)
			nodeHealth.IsHealthy = false
			atomic.AddInt64(&gdm.degradationMetrics.NodeFailures, 1)
			atomic.AddInt64(&gdm.degradationMetrics.UnhealthyNodes, 1)
			atomic.AddInt64(&gdm.degradationMetrics.HealthyNodes, -1)
		}
	}

	// Update circuit breaker
	gdm.updateCircuitBreaker(host, err)
}

// updateCircuitBreaker updates the circuit breaker state for a host
func (gdm *GracefulDegradationManager) updateCircuitBreaker(host string, err error) {
	gdm.breakerMutex.Lock()
	defer gdm.breakerMutex.Unlock()

	breaker, exists := gdm.circuitBreakers[host]
	if !exists {
		return
	}

	breaker.mutex.Lock()
	defer breaker.mutex.Unlock()

	now := time.Now()
	breaker.failureCount++
	breaker.lastFailureTime = now

	// Trip circuit breaker if failure threshold exceeded
	if breaker.state == CircuitBreakerClosed && breaker.failureCount >= breaker.failureThreshold {
		breaker.state = CircuitBreakerOpen
		breaker.nextRetryTime = now.Add(breaker.resetTimeout)

		gdm.logger.Printf("Circuit breaker opened for host %s after %d failures",
			host, breaker.failureCount)
		atomic.AddInt64(&gdm.degradationMetrics.CircuitBreakerTrips, 1)
	}
}

// shouldDegradeConsistency determines if consistency level should be degraded
func (gdm *GracefulDegradationManager) shouldDegradeConsistency(err error) bool {
	if err == nil {
		return false
	}

	// Check for consistency-related errors
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "unavailable") ||
		strings.Contains(errStr, "not enough replicas") ||
		strings.Contains(errStr, "cannot achieve consistency") ||
		strings.Contains(errStr, "quorum not available")
}

// degradeConsistency degrades the consistency level to the next fallback level
func (gdm *GracefulDegradationManager) degradeConsistency() gocql.Consistency {
	gdm.consistencyFallback.mutex.Lock()
	defer gdm.consistencyFallback.mutex.Unlock()

	// Find current position in fallback levels
	currentIndex := -1
	for i, level := range gdm.consistencyFallback.fallbackLevels {
		if level == gdm.consistencyFallback.currentConsistency {
			currentIndex = i
			break
		}
	}

	// Move to next fallback level if available
	if currentIndex >= 0 && currentIndex < len(gdm.consistencyFallback.fallbackLevels)-1 {
		gdm.consistencyFallback.currentConsistency = gdm.consistencyFallback.fallbackLevels[currentIndex+1]
		gdm.consistencyFallback.degradationTime = time.Now()

		// Update metrics
		gdm.degradationMetrics.CurrentConsistency = gdm.consistencyToString(gdm.consistencyFallback.currentConsistency)
	}

	return gdm.consistencyFallback.currentConsistency
}

// getCurrentConsistency returns the current (possibly degraded) consistency level
func (gdm *GracefulDegradationManager) getCurrentConsistency() gocql.Consistency {
	gdm.consistencyFallback.mutex.RLock()
	defer gdm.consistencyFallback.mutex.RUnlock()
	return gdm.consistencyFallback.currentConsistency
}

// detectNetworkPartition detects if a network partition has occurred
func (gdm *GracefulDegradationManager) detectNetworkPartition(err error) bool {
	if err == nil {
		return false
	}

	// Count unhealthy nodes
	unhealthyCount := gdm.countUnhealthyNodes()
	totalNodes := len(gdm.config.Hosts)

	// Consider it a partition if more than half the nodes are unhealthy
	isPartition := unhealthyCount > totalNodes/2

	gdm.partitionDetector.mutex.Lock()
	defer gdm.partitionDetector.mutex.Unlock()

	if isPartition && !gdm.partitionDetector.isPartitioned {
		gdm.partitionDetector.isPartitioned = true
		gdm.partitionDetector.partitionStartTime = time.Now()

		gdm.logger.Printf("Network partition detected: %d/%d nodes unhealthy",
			unhealthyCount, totalNodes)
		atomic.AddInt64(&gdm.degradationMetrics.PartitionDetections, 1)
		gdm.degradationMetrics.IsPartitioned = true

		return true
	}

	return gdm.partitionDetector.isPartitioned
}

// handleNetworkPartition handles operations during network partitions
// Implements requirement 3.4: Handle network partitions with eventual consistency
func (gdm *GracefulDegradationManager) handleNetworkPartition(ctx context.Context, err error, operation func(consistency gocql.Consistency) error) error {
	gdm.logger.Printf("Handling operation during network partition")

	// During partition, use the most relaxed consistency level available
	partitionConsistency := gocql.One // Use ONE for maximum availability

	// Try to execute with relaxed consistency
	partitionErr := operation(partitionConsistency)
	if partitionErr == nil {
		gdm.logger.Printf("Operation succeeded during partition with consistency ONE")
		return nil
	}

	// If even ONE consistency fails, try with ANY (write-only)
	anyErr := operation(gocql.Any)
	if anyErr == nil {
		gdm.logger.Printf("Operation succeeded during partition with consistency ANY")
		return nil
	}

	// If all consistency levels fail, return the original error
	gdm.logger.Printf("All consistency levels failed during partition: %v", err)
	return fmt.Errorf("operation failed during network partition: %w", err)
}

// createFallbackLevels creates a list of fallback consistency levels
func (gdm *GracefulDegradationManager) createFallbackLevels(original gocql.Consistency) []gocql.Consistency {
	// Create fallback chain based on original consistency level
	switch original {
	case gocql.All:
		return []gocql.Consistency{gocql.All, gocql.Quorum, gocql.LocalQuorum, gocql.One}
	case gocql.Quorum:
		return []gocql.Consistency{gocql.Quorum, gocql.LocalQuorum, gocql.One}
	case gocql.LocalQuorum:
		return []gocql.Consistency{gocql.LocalQuorum, gocql.LocalOne, gocql.One}
	case gocql.EachQuorum:
		return []gocql.Consistency{gocql.EachQuorum, gocql.LocalQuorum, gocql.One}
	default:
		return []gocql.Consistency{original, gocql.One}
	}
}

// healthMonitorLoop continuously monitors node health
func (gdm *GracefulDegradationManager) healthMonitorLoop(ctx context.Context) {
	defer gdm.wg.Done()

	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-gdm.stopCh:
			return
		case <-ticker.C:
			gdm.performHealthCheck()
		}
	}
}

// performHealthCheck performs health checks on all nodes
func (gdm *GracefulDegradationManager) performHealthCheck() {
	gdm.healthMutex.RLock()
	hosts := make([]string, 0, len(gdm.nodeHealth))
	for host := range gdm.nodeHealth {
		hosts = append(hosts, host)
	}
	gdm.healthMutex.RUnlock()

	for _, host := range hosts {
		gdm.checkNodeHealth(host)
	}
}

// checkNodeHealth checks the health of a specific node
func (gdm *GracefulDegradationManager) checkNodeHealth(host string) {
	start := time.Now()

	// Simple health check query
	query := gdm.session.Query("SELECT now() FROM system.local")
	var now time.Time
	err := query.Scan(&now)
	query.Release()

	responseTime := time.Since(start)

	gdm.healthMutex.Lock()
	nodeHealth, exists := gdm.nodeHealth[host]
	if !exists {
		gdm.healthMutex.Unlock()
		return
	}
	gdm.healthMutex.Unlock()

	nodeHealth.mutex.Lock()
	defer nodeHealth.mutex.Unlock()

	if err == nil {
		// Node is healthy
		if !nodeHealth.IsHealthy {
			gdm.logger.Printf("Node %s recovered and is now healthy", host)
			atomic.AddInt64(&gdm.degradationMetrics.RecoveryEvents, 1)
			atomic.AddInt64(&gdm.degradationMetrics.HealthyNodes, 1)
			atomic.AddInt64(&gdm.degradationMetrics.UnhealthyNodes, -1)
		}

		nodeHealth.IsHealthy = true
		nodeHealth.LastSeen = time.Now()
		nodeHealth.ConsecutiveFailures = 0
		nodeHealth.LastError = nil
		nodeHealth.ResponseTime = responseTime
		nodeHealth.IsConnected = true

		// Reset circuit breaker on successful health check
		gdm.resetCircuitBreaker(host)
	} else {
		// Node is unhealthy
		nodeHealth.ConsecutiveFailures++
		nodeHealth.LastError = err
		nodeHealth.ResponseTime = responseTime

		if nodeHealth.ConsecutiveFailures >= 3 && nodeHealth.IsHealthy {
			gdm.logger.Printf("Node %s marked as unhealthy after health check failure: %v", host, err)
			nodeHealth.IsHealthy = false
			atomic.AddInt64(&gdm.degradationMetrics.NodeFailures, 1)
			atomic.AddInt64(&gdm.degradationMetrics.UnhealthyNodes, 1)
			atomic.AddInt64(&gdm.degradationMetrics.HealthyNodes, -1)
		}
	}
}

// resetCircuitBreaker resets the circuit breaker for a host
func (gdm *GracefulDegradationManager) resetCircuitBreaker(host string) {
	gdm.breakerMutex.Lock()
	defer gdm.breakerMutex.Unlock()

	breaker, exists := gdm.circuitBreakers[host]
	if !exists {
		return
	}

	breaker.mutex.Lock()
	defer breaker.mutex.Unlock()

	if breaker.state != CircuitBreakerClosed {
		gdm.logger.Printf("Circuit breaker reset for host %s", host)
		breaker.state = CircuitBreakerClosed
		breaker.failureCount = 0
	}
}

// partitionDetectionLoop continuously monitors for network partitions
func (gdm *GracefulDegradationManager) partitionDetectionLoop(ctx context.Context) {
	defer gdm.wg.Done()

	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-gdm.stopCh:
			return
		case <-ticker.C:
			gdm.checkPartitionRecovery()
		}
	}
}

// checkPartitionRecovery checks if the network partition has been resolved
func (gdm *GracefulDegradationManager) checkPartitionRecovery() {
	gdm.partitionDetector.mutex.Lock()
	defer gdm.partitionDetector.mutex.Unlock()

	if !gdm.partitionDetector.isPartitioned {
		return
	}

	// Check if enough nodes are healthy again
	healthyCount := gdm.countHealthyNodes()
	totalNodes := len(gdm.config.Hosts)

	// Consider partition resolved if majority of nodes are healthy
	if healthyCount > totalNodes/2 {
		// Wait for recovery window before declaring recovery
		if time.Since(gdm.partitionDetector.partitionStartTime) > gdm.partitionDetector.recoveryWindow {
			gdm.partitionDetector.isPartitioned = false
			gdm.degradationMetrics.IsPartitioned = false

			gdm.logger.Printf("Network partition resolved: %d/%d nodes healthy",
				healthyCount, totalNodes)
			atomic.AddInt64(&gdm.degradationMetrics.RecoveryEvents, 1)
		}
	}
}

// recoveryMonitorLoop monitors for consistency level recovery
func (gdm *GracefulDegradationManager) recoveryMonitorLoop(ctx context.Context) {
	defer gdm.wg.Done()

	ticker := time.NewTicker(60 * time.Second) // Check every minute
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-gdm.stopCh:
			return
		case <-ticker.C:
			gdm.attemptConsistencyRecovery()
		}
	}
}

// attemptConsistencyRecovery attempts to recover to higher consistency levels
func (gdm *GracefulDegradationManager) attemptConsistencyRecovery() {
	gdm.consistencyFallback.mutex.Lock()
	defer gdm.consistencyFallback.mutex.Unlock()

	// Only attempt recovery if we're degraded and enough time has passed
	if gdm.consistencyFallback.currentConsistency == gdm.consistencyFallback.originalConsistency {
		return
	}

	if time.Since(gdm.consistencyFallback.degradationTime) < 5*time.Minute {
		return
	}

	// Check if we can recover to a higher consistency level
	healthyCount := gdm.countHealthyNodes()
	totalNodes := len(gdm.config.Hosts)

	// Attempt to recover based on healthy node count
	var targetConsistency gocql.Consistency

	if healthyCount >= totalNodes {
		targetConsistency = gdm.consistencyFallback.originalConsistency
	} else if healthyCount > totalNodes/2 {
		targetConsistency = gocql.Quorum
	} else if healthyCount > 1 {
		targetConsistency = gocql.LocalQuorum
	} else {
		return // Not enough nodes for recovery
	}

	// Test if the target consistency level works
	if gdm.testConsistencyLevel(targetConsistency) {
		gdm.consistencyFallback.currentConsistency = targetConsistency
		gdm.degradationMetrics.CurrentConsistency = gdm.consistencyToString(targetConsistency)

		gdm.logger.Printf("Consistency level recovered to %v with %d/%d healthy nodes",
			targetConsistency, healthyCount, totalNodes)
		atomic.AddInt64(&gdm.degradationMetrics.RecoveryEvents, 1)
	}
}

// testConsistencyLevel tests if a consistency level is achievable
func (gdm *GracefulDegradationManager) testConsistencyLevel(consistency gocql.Consistency) bool {
	query := gdm.session.Query("SELECT now() FROM system.local").Consistency(consistency)
	defer query.Release()

	var now time.Time
	err := query.Scan(&now)
	return err == nil
}

// Helper methods

// countHealthyNodes returns the number of healthy nodes
func (gdm *GracefulDegradationManager) countHealthyNodes() int {
	gdm.healthMutex.RLock()
	defer gdm.healthMutex.RUnlock()

	count := 0
	for _, nodeHealth := range gdm.nodeHealth {
		nodeHealth.mutex.RLock()
		if nodeHealth.IsHealthy {
			count++
		}
		nodeHealth.mutex.RUnlock()
	}
	return count
}

// countUnhealthyNodes returns the number of unhealthy nodes
func (gdm *GracefulDegradationManager) countUnhealthyNodes() int {
	return len(gdm.config.Hosts) - gdm.countHealthyNodes()
}

// extractHostFromError attempts to extract host information from an error
func (gdm *GracefulDegradationManager) extractHostFromError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// Try to extract host from common gocql error patterns
	for _, host := range gdm.config.Hosts {
		if strings.Contains(errStr, host) {
			return host
		}
	}

	return ""
}

// consistencyToString converts gocql.Consistency to string
func (gdm *GracefulDegradationManager) consistencyToString(consistency gocql.Consistency) string {
	switch consistency {
	case gocql.Any:
		return "ANY"
	case gocql.One:
		return "ONE"
	case gocql.Two:
		return "TWO"
	case gocql.Three:
		return "THREE"
	case gocql.Quorum:
		return "QUORUM"
	case gocql.All:
		return "ALL"
	case gocql.LocalQuorum:
		return "LOCAL_QUORUM"
	case gocql.EachQuorum:
		return "EACH_QUORUM"
	case gocql.LocalOne:
		return "LOCAL_ONE"
	default:
		return "UNKNOWN"
	}
}

// GetMetrics returns current degradation metrics
func (gdm *GracefulDegradationManager) GetMetrics() *DegradationMetrics {
	// Update current node counts
	gdm.degradationMetrics.HealthyNodes = int64(gdm.countHealthyNodes())
	gdm.degradationMetrics.UnhealthyNodes = int64(gdm.countUnhealthyNodes())

	return gdm.degradationMetrics
}

// GetNodeHealth returns the health status of all nodes
func (gdm *GracefulDegradationManager) GetNodeHealth() map[string]*NodeHealthStatus {
	gdm.healthMutex.RLock()
	defer gdm.healthMutex.RUnlock()

	result := make(map[string]*NodeHealthStatus)
	for host, health := range gdm.nodeHealth {
		health.mutex.RLock()
		result[host] = &NodeHealthStatus{
			Host:                health.Host,
			IsHealthy:           health.IsHealthy,
			LastSeen:            health.LastSeen,
			ConsecutiveFailures: health.ConsecutiveFailures,
			LastError:           health.LastError,
			ResponseTime:        health.ResponseTime,
			IsConnected:         health.IsConnected,
			ConnectionErrors:    health.ConnectionErrors,
		}
		health.mutex.RUnlock()
	}

	return result
}

// IsPartitioned returns true if a network partition is detected
func (gdm *GracefulDegradationManager) IsPartitioned() bool {
	gdm.partitionDetector.mutex.RLock()
	defer gdm.partitionDetector.mutex.RUnlock()
	return gdm.partitionDetector.isPartitioned
}

// GetCurrentConsistencyLevel returns the current consistency level
func (gdm *GracefulDegradationManager) GetCurrentConsistencyLevel() gocql.Consistency {
	return gdm.getCurrentConsistency()
}
