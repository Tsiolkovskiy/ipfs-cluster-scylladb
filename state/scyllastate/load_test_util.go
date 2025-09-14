package scyllastate

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
)

// LoadTestConfig defines configuration for load testing
type LoadTestConfig struct {
	Duration          time.Duration
	Concurrency       int
	OperationsPerSec  int
	ReadWriteRatio    float64 // 0.0 = all writes, 1.0 = all reads
	BatchSize         int
	DatasetSize       int
	ReportInterval    time.Duration
}

// DefaultLoadTestConfig returns a default load test configuration
func DefaultLoadTestConfig() *LoadTestConfig {
	return &LoadTestConfig{
		Duration:          60 * time.Second,
		Concurrency:       10,
		OperationsPerSec:  1000,
		ReadWriteRatio:    0.7, // 70% reads, 30% writes
		BatchSize:         100,
		DatasetSize:       10000,
		ReportInterval:    10 * time.Second,
	}
}

// LoadTestMetrics tracks metrics during load testing
type LoadTestMetrics struct {
	TotalOperations   int64
	SuccessfulOps     int64
	FailedOps         int64
	ReadOps           int64
	WriteOps          int64
	BatchOps          int64
	
	TotalLatency      time.Duration
	MinLatency        time.Duration
	MaxLatency        time.Duration
	
	StartTime         time.Time
	EndTime           time.Time
	
	mu                sync.RWMutex
	latencies         []time.Duration
}

// NewLoadTestMetrics creates a new metrics tracker
func NewLoadTestMetrics() *LoadTestMetrics {
	return &LoadTestMetrics{
		MinLatency: time.Hour, // Initialize to high value
		latencies:  make([]time.Duration, 0, 10000),
		StartTime:  time.Now(),
	}
}

// RecordOperation records an operation result
func (m *LoadTestMetrics) RecordOperation(opType string, latency time.Duration, success bool) {
	atomic.AddInt64(&m.TotalOperations, 1)
	
	if success {
		atomic.AddInt64(&m.SuccessfulOps, 1)
	} else {
		atomic.AddInt64(&m.FailedOps, 1)
	}
	
	switch opType {
	case "read", "get", "has", "list":
		atomic.AddInt64(&m.ReadOps, 1)
	case "write", "add", "rm":
		atomic.AddInt64(&m.WriteOps, 1)
	case "batch":
		atomic.AddInt64(&m.BatchOps, 1)
	}
	
	// Update latency metrics
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.TotalLatency += latency
	if latency < m.MinLatency {
		m.MinLatency = latency
	}
	if latency > m.MaxLatency {
		m.MaxLatency = latency
	}
	
	// Store latency for percentile calculations (sample to avoid memory issues)
	if len(m.latencies) < cap(m.latencies) {
		m.latencies = append(m.latencies, latency)
	} else if rand.Intn(100) < 10 { // 10% sampling
		idx := rand.Intn(len(m.latencies))
		m.latencies[idx] = latency
	}
}

// GetSnapshot returns a snapshot of current metrics
func (m *LoadTestMetrics) GetSnapshot() LoadTestMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	snapshot := *m
	snapshot.latencies = make([]time.Duration, len(m.latencies))
	copy(snapshot.latencies, m.latencies)
	
	return snapshot
}

// CalculatePercentile calculates the given percentile of latencies
func (m *LoadTestMetrics) CalculatePercentile(percentile float64) time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if len(m.latencies) == 0 {
		return 0
	}
	
	// Simple percentile calculation (not perfectly accurate but good enough for testing)
	sorted := make([]time.Duration, len(m.latencies))
	copy(sorted, m.latencies)
	
	// Simple bubble sort for small datasets
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	
	index := int(float64(len(sorted)) * percentile / 100.0)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	
	return sorted[index]
}

// Report generates a metrics report
func (m *LoadTestMetrics) Report() string {
	snapshot := m.GetSnapshot()
	
	duration := snapshot.EndTime.Sub(snapshot.StartTime)
	if duration == 0 {
		duration = time.Since(snapshot.StartTime)
	}
	
	opsPerSec := float64(snapshot.TotalOperations) / duration.Seconds()
	successRate := float64(snapshot.SuccessfulOps) / float64(snapshot.TotalOperations) * 100
	
	avgLatency := time.Duration(0)
	if snapshot.TotalOperations > 0 {
		avgLatency = time.Duration(int64(snapshot.TotalLatency) / snapshot.TotalOperations)
	}
	
	p50 := snapshot.CalculatePercentile(50)
	p95 := snapshot.CalculatePercentile(95)
	p99 := snapshot.CalculatePercentile(99)
	
	return fmt.Sprintf(`Load Test Results:
  Duration: %v
  Total Operations: %d
  Successful: %d (%.2f%%)
  Failed: %d
  Operations/sec: %.2f
  
  Operation Breakdown:
    Read Operations: %d
    Write Operations: %d
    Batch Operations: %d
  
  Latency Statistics:
    Average: %v
    Min: %v
    Max: %v
    P50: %v
    P95: %v
    P99: %v`,
		duration,
		snapshot.TotalOperations,
		snapshot.SuccessfulOps, successRate,
		snapshot.FailedOps,
		opsPerSec,
		snapshot.ReadOps,
		snapshot.WriteOps,
		snapshot.BatchOps,
		avgLatency,
		snapshot.MinLatency,
		snapshot.MaxLatency,
		p50, p95, p99)
}

// LoadTestRunner executes load tests against ScyllaState
type LoadTestRunner struct {
	state   *ScyllaState
	config  *LoadTestConfig
	metrics *LoadTestMetrics
	dataset []api.Pin
	ctx     context.Context
}

// NewLoadTestRunner creates a new load test runner
func NewLoadTestRunner(state *ScyllaState, config *LoadTestConfig) *LoadTestRunner {
	return &LoadTestRunner{
		state:   state,
		config:  config,
		metrics: NewLoadTestMetrics(),
		ctx:     context.Background(),
	}
}

// PrepareDataset prepares the dataset for load testing
func (ltr *LoadTestRunner) PrepareDataset() error {
	ltr.dataset = make([]api.Pin, ltr.config.DatasetSize)
	
	for i := 0; i < ltr.config.DatasetSize; i++ {
		pin := api.Pin{
			Cid:  createTestCIDWithSuffix(nil, fmt.Sprintf("load-%d", i)),
			Type: api.DataType,
			Allocations: []api.PeerID{
				api.PeerID("peer1"),
				api.PeerID("peer2"),
			},
			ReplicationFactorMin: 2,
			ReplicationFactorMax: 3,
			MaxDepth:             -1,
			PinOptions: api.PinOptions{
				Name: fmt.Sprintf("load-test-pin-%d", i),
			},
		}
		ltr.dataset[i] = pin
	}
	
	// Pre-populate some data for read operations
	populateCount := int(float64(ltr.config.DatasetSize) * 0.5) // Pre-populate 50%
	
	for i := 0; i < populateCount; i++ {
		err := ltr.state.Add(ltr.ctx, ltr.dataset[i])
		if err != nil {
			return fmt.Errorf("failed to pre-populate dataset: %w", err)
		}
	}
	
	return nil
}

// RunLoadTest executes the load test
func (ltr *LoadTestRunner) RunLoadTest() error {
	if err := ltr.PrepareDataset(); err != nil {
		return err
	}
	
	var wg sync.WaitGroup
	stopChan := make(chan struct{})
	
	// Start progress reporter
	go ltr.reportProgress(stopChan)
	
	// Start workers
	for worker := 0; worker < ltr.config.Concurrency; worker++ {
		wg.Add(1)
		go ltr.worker(worker, &wg, stopChan)
	}
	
	// Run for specified duration
	time.Sleep(ltr.config.Duration)
	close(stopChan)
	
	wg.Wait()
	ltr.metrics.EndTime = time.Now()
	
	return nil
}

// worker executes operations for load testing
func (ltr *LoadTestRunner) worker(workerID int, wg *sync.WaitGroup, stopChan <-chan struct{}) {
	defer wg.Done()
	
	// Calculate operation interval to achieve target ops/sec
	opsPerWorker := ltr.config.OperationsPerSec / ltr.config.Concurrency
	interval := time.Second / time.Duration(opsPerWorker)
	
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			ltr.executeOperation(workerID)
		}
	}
}

// executeOperation executes a single operation
func (ltr *LoadTestRunner) executeOperation(workerID int) {
	start := time.Now()
	
	// Decide operation type based on read/write ratio
	isRead := rand.Float64() < ltr.config.ReadWriteRatio
	
	var err error
	var opType string
	
	if isRead {
		// Perform read operation
		opType = ltr.executeReadOperation()
	} else {
		// Perform write operation
		opType, err = ltr.executeWriteOperation(workerID)
	}
	
	latency := time.Since(start)
	ltr.metrics.RecordOperation(opType, latency, err == nil)
}

// executeReadOperation executes a read operation
func (ltr *LoadTestRunner) executeReadOperation() string {
	// Randomly choose read operation type
	switch rand.Intn(3) {
	case 0: // Get
		if len(ltr.dataset) > 0 {
			idx := rand.Intn(len(ltr.dataset))
			ltr.state.Get(ltr.ctx, ltr.dataset[idx].Cid)
		}
		return "get"
	case 1: // Has
		if len(ltr.dataset) > 0 {
			idx := rand.Intn(len(ltr.dataset))
			ltr.state.Has(ltr.ctx, ltr.dataset[idx].Cid)
		}
		return "has"
	case 2: // List (sample)
		pinChan := make(chan api.Pin, 100)
		go func() {
			ltr.state.List(ltr.ctx, pinChan)
		}()
		// Consume a few pins and stop
		count := 0
		for range pinChan {
			count++
			if count >= 10 {
				break
			}
		}
		return "list"
	}
	return "read"
}

// executeWriteOperation executes a write operation
func (ltr *LoadTestRunner) executeWriteOperation(workerID int) (string, error) {
	// Randomly choose write operation type
	switch rand.Intn(3) {
	case 0: // Add
		pin := api.Pin{
			Cid:  createTestCIDWithSuffix(nil, fmt.Sprintf("worker-%d-%d", workerID, time.Now().UnixNano())),
			Type: api.DataType,
			Allocations: []api.PeerID{
				api.PeerID("peer1"),
			},
			ReplicationFactorMin: 1,
			ReplicationFactorMax: 2,
			MaxDepth:             -1,
			PinOptions: api.PinOptions{
				Name: fmt.Sprintf("load-pin-%d", workerID),
			},
		}
		err := ltr.state.Add(ltr.ctx, pin)
		return "add", err
	case 1: // Remove
		if len(ltr.dataset) > 0 {
			idx := rand.Intn(len(ltr.dataset))
			err := ltr.state.Rm(ltr.ctx, ltr.dataset[idx].Cid)
			return "rm", err
		}
		return "rm", nil
	case 2: // Batch operation
		batchState := ltr.state.Batch()
		for i := 0; i < ltr.config.BatchSize && i < 10; i++ {
			pin := api.Pin{
				Cid:  createTestCIDWithSuffix(nil, fmt.Sprintf("batch-%d-%d-%d", workerID, time.Now().UnixNano(), i)),
				Type: api.DataType,
				Allocations: []api.PeerID{
					api.PeerID("peer1"),
				},
				ReplicationFactorMin: 1,
				ReplicationFactorMax: 2,
				MaxDepth:             -1,
				PinOptions: api.PinOptions{
					Name: fmt.Sprintf("batch-pin-%d-%d", workerID, i),
				},
			}
			batchState.Add(ltr.ctx, pin)
		}
		err := batchState.Commit(ltr.ctx)
		return "batch", err
	}
	return "write", nil
}

// reportProgress reports progress during load testing
func (ltr *LoadTestRunner) reportProgress(stopChan <-chan struct{}) {
	ticker := time.NewTicker(ltr.config.ReportInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			snapshot := ltr.metrics.GetSnapshot()
			duration := time.Since(snapshot.StartTime)
			opsPerSec := float64(snapshot.TotalOperations) / duration.Seconds()
			
			fmt.Printf("[%v] Operations: %d, Ops/sec: %.2f, Success: %d, Failed: %d\n",
				duration.Truncate(time.Second),
				snapshot.TotalOperations,
				opsPerSec,
				snapshot.SuccessfulOps,
				snapshot.FailedOps)
		}
	}
}

// GetMetrics returns the current metrics
func (ltr *LoadTestRunner) GetMetrics() *LoadTestMetrics {
	return ltr.metrics
}

// createTestCIDWithSuffix creates a test CID (helper function for load testing)
func createTestCIDWithSuffix(t interface{}, suffix string) api.Cid {
	// Simple CID generation for load testing
	baseCidStr := "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbd"
	
	// Create a hash of the suffix for uniqueness
	hash := fmt.Sprintf("%x", []byte(suffix))
	if len(hash) > 10 {
		hash = hash[:10]
	}
	
	finalCidStr := baseCidStr + hash
	
	cid, err := api.DecodeCid(finalCidStr)
	if err != nil {
		// Fallback to a default CID
		cid, _ = api.DecodeCid("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	}
	
	return cid
}