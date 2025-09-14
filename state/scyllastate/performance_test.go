package scyllastate

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Performance test configuration
const (
	// Benchmark settings
	benchmarkPinCount     = 1000
	benchmarkBatchSize    = 100
	benchmarkConcurrency  = 10
	
	// Load test settings
	loadTestDuration      = 30 * time.Second
	loadTestPinCount      = 10000
	loadTestConcurrency   = 50
	
	// Stress test settings
	stressTestDuration    = 60 * time.Second
	stressTestConcurrency = 100
	stressTestPinCount    = 50000
)

// PerformanceTestSuite manages performance testing infrastructure
type PerformanceTestSuite struct {
	container *TestScyllaDBContainer
	cluster   *TestScyllaDBCluster
	state     *ScyllaState
	ctx       context.Context
}

// NewPerformanceTestSuite creates a new performance test suite
func NewPerformanceTestSuite(t *testing.T, useCluster bool) *PerformanceTestSuite {
	suite := &PerformanceTestSuite{
		ctx: context.Background(),
	}

	if useCluster {
		suite.cluster = StartScyllaDBCluster(t, 3)
		config := suite.cluster.GetConfig()
		state, err := New(suite.ctx, config)
		require.NoError(t, err)
		suite.state = state
	} else {
		suite.container = StartScyllaDBContainer(t)
		config := suite.container.GetConfig()
		state, err := New(suite.ctx, config)
		require.NoError(t, err)
		suite.state = state
	}

	return suite
}

// Cleanup cleans up the performance test suite
func (pts *PerformanceTestSuite) Cleanup() {
	if pts.state != nil {
		pts.state.Close()
	}
	if pts.container != nil {
		pts.container.Stop()
	}
	if pts.cluster != nil {
		pts.cluster.Cleanup()
	}
}

// generateBenchmarkPins creates a set of pins for benchmarking
func generateBenchmarkPins(t *testing.T, count int) []api.Pin {
	pins := make([]api.Pin, count)
	for i := 0; i < count; i++ {
		pin := createTestPin(t)
		pin.PinOptions.Name = fmt.Sprintf("benchmark-pin-%d", i)
		pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("bench-%d", i))
		pins[i] = pin
	}
	return pins
}

// BenchmarkScyllaState_Add benchmarks the Add operation
func BenchmarkScyllaState_Add(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	suite := NewPerformanceTestSuite(b, false)
	defer suite.Cleanup()

	pins := generateBenchmarkPins(b, b.N)

	b.ResetTimer()
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		err := suite.state.Add(suite.ctx, pins[i])
		if err != nil {
			b.Fatalf("Add failed: %v", err)
		}
	}

	b.StopTimer()

	// Report operations per second
	opsPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
}

// BenchmarkScyllaState_Get benchmarks the Get operation
func BenchmarkScyllaState_Get(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	suite := NewPerformanceTestSuite(b, false)
	defer suite.Cleanup()

	// Pre-populate with pins
	pins := generateBenchmarkPins(b, benchmarkPinCount)
	for _, pin := range pins {
		err := suite.state.Add(suite.ctx, pin)
		require.NoError(b, err)
	}

	b.ResetTimer()
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		pinIndex := i % len(pins)
		_, err := suite.state.Get(suite.ctx, pins[pinIndex].Cid)
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
	}

	b.StopTimer()

	opsPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
}

// BenchmarkScyllaState_Has benchmarks the Has operation
func BenchmarkScyllaState_Has(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	suite := NewPerformanceTestSuite(b, false)
	defer suite.Cleanup()

	// Pre-populate with pins
	pins := generateBenchmarkPins(b, benchmarkPinCount)
	for _, pin := range pins {
		err := suite.state.Add(suite.ctx, pin)
		require.NoError(b, err)
	}

	b.ResetTimer()
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		pinIndex := i % len(pins)
		_, err := suite.state.Has(suite.ctx, pins[pinIndex].Cid)
		if err != nil {
			b.Fatalf("Has failed: %v", err)
		}
	}

	b.StopTimer()

	opsPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
}

// BenchmarkScyllaState_Rm benchmarks the Rm operation
func BenchmarkScyllaState_Rm(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	suite := NewPerformanceTestSuite(b, false)
	defer suite.Cleanup()

	pins := generateBenchmarkPins(b, b.N)

	// Pre-populate with pins
	for _, pin := range pins {
		err := suite.state.Add(suite.ctx, pin)
		require.NoError(b, err)
	}

	b.ResetTimer()
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		err := suite.state.Rm(suite.ctx, pins[i].Cid)
		if err != nil {
			b.Fatalf("Rm failed: %v", err)
		}
	}

	b.StopTimer()

	opsPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
}

// BenchmarkScyllaState_BatchAdd benchmarks batch Add operations
func BenchmarkScyllaState_BatchAdd(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	suite := NewPerformanceTestSuite(b, false)
	defer suite.Cleanup()

	pins := generateBenchmarkPins(b, b.N)
	batchState := suite.state.Batch()

	b.ResetTimer()
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		err := batchState.Add(suite.ctx, pins[i])
		if err != nil {
			b.Fatalf("Batch Add failed: %v", err)
		}

		// Commit every benchmarkBatchSize operations
		if (i+1)%benchmarkBatchSize == 0 || i == b.N-1 {
			err := batchState.Commit(suite.ctx)
			if err != nil {
				b.Fatalf("Batch Commit failed: %v", err)
			}
		}
	}

	b.StopTimer()

	opsPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
}

// BenchmarkScyllaState_List benchmarks the List operation
func BenchmarkScyllaState_List(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	suite := NewPerformanceTestSuite(b, false)
	defer suite.Cleanup()

	// Pre-populate with pins
	pins := generateBenchmarkPins(b, benchmarkPinCount)
	for _, pin := range pins {
		err := suite.state.Add(suite.ctx, pin)
		require.NoError(b, err)
	}

	b.ResetTimer()
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		pinChan := make(chan api.Pin, benchmarkPinCount+10)
		
		go func() {
			err := suite.state.List(suite.ctx, pinChan)
			if err != nil {
				b.Errorf("List failed: %v", err)
			}
		}()

		// Consume all pins
		count := 0
		for range pinChan {
			count++
		}
	}

	b.StopTimer()

	opsPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
}

// BenchmarkScyllaState_MixedOperations benchmarks mixed read/write operations
func BenchmarkScyllaState_MixedOperations(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	suite := NewPerformanceTestSuite(b, false)
	defer suite.Cleanup()

	// Pre-populate with some pins
	existingPins := generateBenchmarkPins(b, benchmarkPinCount/2)
	for _, pin := range existingPins {
		err := suite.state.Add(suite.ctx, pin)
		require.NoError(b, err)
	}

	newPins := generateBenchmarkPins(b, b.N)

	b.ResetTimer()
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		switch i % 4 {
		case 0: // Add new pin
			err := suite.state.Add(suite.ctx, newPins[i])
			if err != nil {
				b.Fatalf("Add failed: %v", err)
			}
		case 1: // Get existing pin
			if len(existingPins) > 0 {
				pinIndex := i % len(existingPins)
				_, err := suite.state.Get(suite.ctx, existingPins[pinIndex].Cid)
				if err != nil {
					b.Fatalf("Get failed: %v", err)
				}
			}
		case 2: // Has existing pin
			if len(existingPins) > 0 {
				pinIndex := i % len(existingPins)
				_, err := suite.state.Has(suite.ctx, existingPins[pinIndex].Cid)
				if err != nil {
					b.Fatalf("Has failed: %v", err)
				}
			}
		case 3: // Remove pin (if we have added some)
			if i > benchmarkPinCount/4 {
				removeIndex := (i - benchmarkPinCount/4) % (i/4)
				if removeIndex < len(newPins) {
					suite.state.Rm(suite.ctx, newPins[removeIndex].Cid)
				}
			}
		}
	}

	b.StopTimer()

	opsPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
}

// BenchmarkScyllaState_ConcurrentOperations benchmarks concurrent operations
func BenchmarkScyllaState_ConcurrentOperations(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	suite := NewPerformanceTestSuite(b, false)
	defer suite.Cleanup()

	pins := generateBenchmarkPins(b, b.N)
	
	b.ResetTimer()
	b.StartTimer()

	var wg sync.WaitGroup
	operationsPerWorker := b.N / benchmarkConcurrency
	
	for worker := 0; worker < benchmarkConcurrency; worker++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			start := workerID * operationsPerWorker
			end := start + operationsPerWorker
			if workerID == benchmarkConcurrency-1 {
				end = b.N // Handle remainder
			}
			
			for i := start; i < end; i++ {
				err := suite.state.Add(suite.ctx, pins[i])
				if err != nil {
					b.Errorf("Concurrent Add failed: %v", err)
				}
			}
		}(worker)
	}
	
	wg.Wait()
	b.StopTimer()

	opsPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
}

// BenchmarkScyllaState_MultiNodeCluster benchmarks operations on multi-node cluster
func BenchmarkScyllaState_MultiNodeCluster(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	suite := NewPerformanceTestSuite(b, true) // Use cluster
	defer suite.Cleanup()

	pins := generateBenchmarkPins(b, b.N)

	b.ResetTimer()
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		err := suite.state.Add(suite.ctx, pins[i])
		if err != nil {
			b.Fatalf("Cluster Add failed: %v", err)
		}
	}

	b.StopTimer()

	opsPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
}

// TestScyllaState_LoadTest_HighThroughput tests high throughput scenarios
func TestScyllaState_LoadTest_HighThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	suite := NewPerformanceTestSuite(t, false)
	defer suite.Cleanup()

	t.Run("Sustained write load", func(t *testing.T) {
		const targetOpsPerSec = 1000
		const testDuration = 30 * time.Second
		
		start := time.Now()
		var opsCompleted int64
		var wg sync.WaitGroup
		
		// Start multiple workers
		for worker := 0; worker < loadTestConcurrency; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				opCount := 0
				for time.Since(start) < testDuration {
					pin := createTestPin(t)
					pin.PinOptions.Name = fmt.Sprintf("load-test-pin-%d-%d", workerID, opCount)
					pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("load-%d-%d", workerID, opCount))
					
					err := suite.state.Add(suite.ctx, pin)
					assert.NoError(t, err)
					
					opCount++
					opsCompleted++
				}
			}(worker)
		}
		
		wg.Wait()
		
		actualDuration := time.Since(start)
		actualOpsPerSec := float64(opsCompleted) / actualDuration.Seconds()
		
		t.Logf("Load test completed: %d operations in %v (%.2f ops/sec)", 
			opsCompleted, actualDuration, actualOpsPerSec)
		
		// We should achieve reasonable throughput
		assert.Greater(t, actualOpsPerSec, float64(targetOpsPerSec/2), 
			"Should achieve at least half the target throughput")
	})

	t.Run("Mixed read/write load", func(t *testing.T) {
		// Pre-populate with pins for read operations
		existingPins := generateBenchmarkPins(t, 1000)
		for _, pin := range existingPins {
			err := suite.state.Add(suite.ctx, pin)
			require.NoError(t, err)
		}

		const testDuration = 20 * time.Second
		start := time.Now()
		
		var readOps, writeOps int64
		var wg sync.WaitGroup
		
		// Start read workers
		for worker := 0; worker < loadTestConcurrency/2; worker++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				
				for time.Since(start) < testDuration {
					pinIndex := rand.Intn(len(existingPins))
					_, err := suite.state.Get(suite.ctx, existingPins[pinIndex].Cid)
					assert.NoError(t, err)
					readOps++
				}
			}()
		}
		
		// Start write workers
		for worker := 0; worker < loadTestConcurrency/2; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				opCount := 0
				for time.Since(start) < testDuration {
					pin := createTestPin(t)
					pin.PinOptions.Name = fmt.Sprintf("mixed-load-pin-%d-%d", workerID, opCount)
					pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("mixed-%d-%d", workerID, opCount))
					
					err := suite.state.Add(suite.ctx, pin)
					assert.NoError(t, err)
					
					opCount++
					writeOps++
				}
			}(worker)
		}
		
		wg.Wait()
		
		actualDuration := time.Since(start)
		totalOps := readOps + writeOps
		totalOpsPerSec := float64(totalOps) / actualDuration.Seconds()
		
		t.Logf("Mixed load test: %d reads, %d writes in %v (%.2f total ops/sec)", 
			readOps, writeOps, actualDuration, totalOpsPerSec)
		
		assert.Greater(t, totalOpsPerSec, 500.0, "Should achieve reasonable mixed throughput")
	})
}

// TestScyllaState_LoadTest_LargeDataVolume tests operations with large data volumes
func TestScyllaState_LoadTest_LargeDataVolume(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	suite := NewPerformanceTestSuite(t, false)
	defer suite.Cleanup()

	t.Run("Large dataset operations", func(t *testing.T) {
		const largeDatasetSize = 50000
		
		t.Logf("Creating %d pins for large dataset test", largeDatasetSize)
		
		// Use batch operations for efficient insertion
		batchState := suite.state.Batch()
		batchSize := 1000
		
		start := time.Now()
		
		for i := 0; i < largeDatasetSize; i++ {
			pin := createTestPin(t)
			pin.PinOptions.Name = fmt.Sprintf("large-dataset-pin-%d", i)
			pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("large-%d", i))
			
			err := batchState.Add(suite.ctx, pin)
			require.NoError(t, err)
			
			// Commit batch periodically
			if (i+1)%batchSize == 0 || i == largeDatasetSize-1 {
				err := batchState.Commit(suite.ctx)
				require.NoError(t, err)
				
				if (i+1)%10000 == 0 {
					t.Logf("Inserted %d/%d pins", i+1, largeDatasetSize)
				}
			}
		}
		
		insertDuration := time.Since(start)
		insertRate := float64(largeDatasetSize) / insertDuration.Seconds()
		
		t.Logf("Inserted %d pins in %v (%.2f pins/sec)", 
			largeDatasetSize, insertDuration, insertRate)
		
		// Test list operation with large dataset
		t.Log("Testing list operation with large dataset")
		listStart := time.Now()
		
		pinChan := make(chan api.Pin, 1000)
		go func() {
			err := suite.state.List(suite.ctx, pinChan)
			assert.NoError(t, err)
		}()
		
		count := 0
		for range pinChan {
			count++
		}
		
		listDuration := time.Since(listStart)
		listRate := float64(count) / listDuration.Seconds()
		
		t.Logf("Listed %d pins in %v (%.2f pins/sec)", 
			count, listDuration, listRate)
		
		assert.GreaterOrEqual(t, count, largeDatasetSize, "Should list all inserted pins")
		assert.Greater(t, insertRate, 1000.0, "Should achieve reasonable insert rate")
		assert.Greater(t, listRate, 5000.0, "Should achieve reasonable list rate")
	})
}

// TestScyllaState_StressTest_MemoryUsage tests memory usage under stress
func TestScyllaState_StressTest_MemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	suite := NewPerformanceTestSuite(t, false)
	defer suite.Cleanup()

	t.Run("Memory usage under load", func(t *testing.T) {
		// Force garbage collection and get baseline memory
		runtime.GC()
		var memBefore runtime.MemStats
		runtime.ReadMemStats(&memBefore)
		
		const stressOperations = 10000
		pins := generateBenchmarkPins(t, stressOperations)
		
		// Perform operations
		for i, pin := range pins {
			err := suite.state.Add(suite.ctx, pin)
			require.NoError(t, err)
			
			// Periodically check memory usage
			if i%1000 == 0 && i > 0 {
				var memCurrent runtime.MemStats
				runtime.ReadMemStats(&memCurrent)
				
				memUsedMB := float64(memCurrent.Alloc-memBefore.Alloc) / 1024 / 1024
				t.Logf("After %d operations: Memory used: %.2f MB", i, memUsedMB)
				
				// Memory usage should be reasonable (less than 100MB for 10k operations)
				assert.Less(t, memUsedMB, 100.0, "Memory usage should be reasonable")
			}
		}
		
		// Force garbage collection and check final memory
		runtime.GC()
		var memAfter runtime.MemStats
		runtime.ReadMemStats(&memAfter)
		
		finalMemUsedMB := float64(memAfter.Alloc-memBefore.Alloc) / 1024 / 1024
		t.Logf("Final memory usage: %.2f MB", finalMemUsedMB)
		
		// Final memory usage should be reasonable
		assert.Less(t, finalMemUsedMB, 50.0, "Final memory usage should be reasonable after GC")
	})
}

// TestScyllaState_PerformanceRegression tests for performance regressions
func TestScyllaState_PerformanceRegression(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance regression test in short mode")
	}

	suite := NewPerformanceTestSuite(t, false)
	defer suite.Cleanup()

	// Define performance baselines (operations per second)
	baselines := map[string]float64{
		"add":   500,  // Should achieve at least 500 adds/sec
		"get":   2000, // Should achieve at least 2000 gets/sec
		"has":   2500, // Should achieve at least 2500 has/sec
		"batch": 2000, // Should achieve at least 2000 batch ops/sec
	}

	t.Run("Add operation performance", func(t *testing.T) {
		pins := generateBenchmarkPins(t, 1000)
		
		start := time.Now()
		for _, pin := range pins {
			err := suite.state.Add(suite.ctx, pin)
			require.NoError(t, err)
		}
		duration := time.Since(start)
		
		opsPerSec := float64(len(pins)) / duration.Seconds()
		t.Logf("Add performance: %.2f ops/sec", opsPerSec)
		
		assert.GreaterOrEqual(t, opsPerSec, baselines["add"], 
			"Add performance should meet baseline")
	})

	t.Run("Get operation performance", func(t *testing.T) {
		// Pre-populate
		pins := generateBenchmarkPins(t, 1000)
		for _, pin := range pins {
			err := suite.state.Add(suite.ctx, pin)
			require.NoError(t, err)
		}
		
		// Benchmark gets
		start := time.Now()
		for i := 0; i < 2000; i++ {
			pinIndex := i % len(pins)
			_, err := suite.state.Get(suite.ctx, pins[pinIndex].Cid)
			require.NoError(t, err)
		}
		duration := time.Since(start)
		
		opsPerSec := 2000.0 / duration.Seconds()
		t.Logf("Get performance: %.2f ops/sec", opsPerSec)
		
		assert.GreaterOrEqual(t, opsPerSec, baselines["get"], 
			"Get performance should meet baseline")
	})

	t.Run("Batch operation performance", func(t *testing.T) {
		pins := generateBenchmarkPins(t, 2000)
		batchState := suite.state.Batch()
		
		start := time.Now()
		for i, pin := range pins {
			err := batchState.Add(suite.ctx, pin)
			require.NoError(t, err)
			
			// Commit every 100 operations
			if (i+1)%100 == 0 {
				err := batchState.Commit(suite.ctx)
				require.NoError(t, err)
			}
		}
		duration := time.Since(start)
		
		opsPerSec := float64(len(pins)) / duration.Seconds()
		t.Logf("Batch performance: %.2f ops/sec", opsPerSec)
		
		assert.GreaterOrEqual(t, opsPerSec, baselines["batch"], 
			"Batch performance should meet baseline")
	})
}

// TestScyllaState_ScalabilityTest tests scalability with increasing load
func TestScyllaState_ScalabilityTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping scalability test in short mode")
	}

	suite := NewPerformanceTestSuite(t, true) // Use cluster for scalability
	defer suite.Cleanup()

	concurrencyLevels := []int{1, 5, 10, 20, 50}
	operationsPerLevel := 1000

	t.Run("Scalability with increasing concurrency", func(t *testing.T) {
		results := make(map[int]float64)
		
		for _, concurrency := range concurrencyLevels {
			t.Logf("Testing with concurrency level: %d", concurrency)
			
			pins := generateBenchmarkPins(t, operationsPerLevel)
			
			start := time.Now()
			var wg sync.WaitGroup
			
			operationsPerWorker := operationsPerLevel / concurrency
			
			for worker := 0; worker < concurrency; worker++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					
					startIdx := workerID * operationsPerWorker
					endIdx := startIdx + operationsPerWorker
					if workerID == concurrency-1 {
						endIdx = operationsPerLevel // Handle remainder
					}
					
					for i := startIdx; i < endIdx; i++ {
						err := suite.state.Add(suite.ctx, pins[i])
						assert.NoError(t, err)
					}
				}(worker)
			}
			
			wg.Wait()
			duration := time.Since(start)
			
			opsPerSec := float64(operationsPerLevel) / duration.Seconds()
			results[concurrency] = opsPerSec
			
			t.Logf("Concurrency %d: %.2f ops/sec", concurrency, opsPerSec)
		}
		
		// Verify that performance scales reasonably with concurrency
		// Performance should improve with concurrency up to a point
		assert.Greater(t, results[5], results[1]*0.8, 
			"Performance should improve with moderate concurrency")
		
		// High concurrency should still maintain reasonable performance
		assert.Greater(t, results[50], results[1]*0.5, 
			"High concurrency should maintain reasonable performance")
	})
}// T
estScyllaState_ExtendedLoadTest runs extended load tests using the load test utility
func TestScyllaState_ExtendedLoadTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping extended load test in short mode")
	}

	suite := NewPerformanceTestSuite(t, false)
	defer suite.Cleanup()

	t.Run("Standard_load_test", func(t *testing.T) {
		config := DefaultLoadTestConfig()
		config.Duration = 30 * time.Second
		config.Concurrency = 10
		config.OperationsPerSec = 500
		config.ReadWriteRatio = 0.7

		runner := NewLoadTestRunner(suite.state, config)
		
		t.Logf("Starting load test: %d workers, %d ops/sec, %.0f%% reads, %v duration",
			config.Concurrency, config.OperationsPerSec, config.ReadWriteRatio*100, config.Duration)
		
		err := runner.RunLoadTest()
		require.NoError(t, err)

		metrics := runner.GetMetrics()
		report := metrics.Report()
		t.Log(report)

		// Validate results
		assert.Greater(t, metrics.TotalOperations, int64(100), "Should complete reasonable number of operations")
		
		successRate := float64(metrics.SuccessfulOps) / float64(metrics.TotalOperations)
		assert.Greater(t, successRate, 0.95, "Should have high success rate")
		
		duration := metrics.EndTime.Sub(metrics.StartTime)
		actualOpsPerSec := float64(metrics.TotalOperations) / duration.Seconds()
		
		// Should achieve at least 80% of target throughput
		expectedMinOps := float64(config.OperationsPerSec) * 0.8
		assert.Greater(t, actualOpsPerSec, expectedMinOps, 
			"Should achieve reasonable fraction of target throughput")
	})

	t.Run("High_concurrency_load_test", func(t *testing.T) {
		config := DefaultLoadTestConfig()
		config.Duration = 20 * time.Second
		config.Concurrency = 50
		config.OperationsPerSec = 2000
		config.ReadWriteRatio = 0.8

		runner := NewLoadTestRunner(suite.state, config)
		
		t.Logf("Starting high concurrency test: %d workers, %d ops/sec target",
			config.Concurrency, config.OperationsPerSec)
		
		err := runner.RunLoadTest()
		require.NoError(t, err)

		metrics := runner.GetMetrics()
		t.Log(metrics.Report())

		// High concurrency should still maintain reasonable performance
		successRate := float64(metrics.SuccessfulOps) / float64(metrics.TotalOperations)
		assert.Greater(t, successRate, 0.90, "Should maintain good success rate under high concurrency")
		
		// Latency should be reasonable
		p95Latency := metrics.CalculatePercentile(95)
		assert.Less(t, p95Latency, 5*time.Second, "P95 latency should be reasonable")
	})

	t.Run("Write_heavy_load_test", func(t *testing.T) {
		config := DefaultLoadTestConfig()
		config.Duration = 15 * time.Second
		config.Concurrency = 20
		config.OperationsPerSec = 800
		config.ReadWriteRatio = 0.2 // 80% writes

		runner := NewLoadTestRunner(suite.state, config)
		
		t.Logf("Starting write-heavy test: %.0f%% writes", (1-config.ReadWriteRatio)*100)
		
		err := runner.RunLoadTest()
		require.NoError(t, err)

		metrics := runner.GetMetrics()
		t.Log(metrics.Report())

		// Write-heavy workload should still perform reasonably
		assert.Greater(t, metrics.WriteOps, metrics.ReadOps, "Should have more writes than reads")
		
		successRate := float64(metrics.SuccessfulOps) / float64(metrics.TotalOperations)
		assert.Greater(t, successRate, 0.90, "Should handle write-heavy workload well")
	})
}

// TestScyllaState_ClusterExtendedLoadTest runs extended load tests on cluster
func TestScyllaState_ClusterExtendedLoadTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping extended cluster load test in short mode")
	}

	suite := NewPerformanceTestSuite(t, true) // Use cluster
	defer suite.Cleanup()

	t.Run("Cluster_sustained_load", func(t *testing.T) {
		config := DefaultLoadTestConfig()
		config.Duration = 45 * time.Second
		config.Concurrency = 30
		config.OperationsPerSec = 1500
		config.ReadWriteRatio = 0.6

		runner := NewLoadTestRunner(suite.state, config)
		
		t.Logf("Starting cluster sustained load test: %d workers, %d ops/sec target",
			config.Concurrency, config.OperationsPerSec)
		
		err := runner.RunLoadTest()
		require.NoError(t, err)

		metrics := runner.GetMetrics()
		t.Log(metrics.Report())

		// Cluster should handle sustained load well
		successRate := float64(metrics.SuccessfulOps) / float64(metrics.TotalOperations)
		assert.Greater(t, successRate, 0.95, "Cluster should handle sustained load with high success rate")
		
		duration := metrics.EndTime.Sub(metrics.StartTime)
		actualOpsPerSec := float64(metrics.TotalOperations) / duration.Seconds()
		
		// Cluster should achieve better throughput than single node
		assert.Greater(t, actualOpsPerSec, 1000.0, "Cluster should achieve good throughput")
		
		// Latency should be reasonable
		avgLatency := time.Duration(int64(metrics.TotalLatency) / metrics.TotalOperations)
		assert.Less(t, avgLatency, 100*time.Millisecond, "Average latency should be reasonable")
	})
}

// TestScyllaState_PerformanceComparison compares performance across different configurations
func TestScyllaState_PerformanceComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance comparison test in short mode")
	}

	// Test different configurations
	configs := []struct {
		name        string
		useCluster  bool
		consistency string
		batchSize   int
	}{
		{"single_node_one", false, "ONE", 100},
		{"single_node_quorum", false, "QUORUM", 100},
		{"cluster_one", true, "ONE", 100},
		{"cluster_quorum", true, "QUORUM", 100},
		{"cluster_large_batch", true, "QUORUM", 500},
	}

	results := make(map[string]float64)

	for _, config := range configs {
		t.Run(config.name, func(t *testing.T) {
			var suite *PerformanceTestSuite
			if config.useCluster {
				suite = NewPerformanceTestSuite(t, true)
			} else {
				suite = NewPerformanceTestSuite(t, false)
			}
			defer suite.Cleanup()

			// Configure consistency
			if config.useCluster {
				suite.cluster.GetConfig().Consistency = config.consistency
			} else {
				suite.container.GetConfig().Consistency = config.consistency
			}

			loadConfig := DefaultLoadTestConfig()
			loadConfig.Duration = 20 * time.Second
			loadConfig.Concurrency = 15
			loadConfig.OperationsPerSec = 1000
			loadConfig.BatchSize = config.batchSize

			runner := NewLoadTestRunner(suite.state, loadConfig)
			
			err := runner.RunLoadTest()
			require.NoError(t, err)

			metrics := runner.GetMetrics()
			duration := metrics.EndTime.Sub(metrics.StartTime)
			opsPerSec := float64(metrics.TotalOperations) / duration.Seconds()
			
			results[config.name] = opsPerSec
			
			t.Logf("%s: %.2f ops/sec", config.name, opsPerSec)
		})
	}

	// Compare results
	t.Run("Performance_analysis", func(t *testing.T) {
		t.Log("Performance Comparison Results:")
		for name, opsPerSec := range results {
			t.Logf("  %s: %.2f ops/sec", name, opsPerSec)
		}

		// Cluster should generally outperform single node
		if clusterQuorum, ok := results["cluster_quorum"]; ok {
			if singleQuorum, ok := results["single_node_quorum"]; ok {
				improvement := (clusterQuorum - singleQuorum) / singleQuorum * 100
				t.Logf("Cluster improvement over single node: %.2f%%", improvement)
				
				// Cluster should provide some improvement (at least not be significantly worse)
				assert.Greater(t, clusterQuorum, singleQuorum*0.8, 
					"Cluster should not be significantly slower than single node")
			}
		}

		// ONE consistency should generally be faster than QUORUM
		if singleOne, ok := results["single_node_one"]; ok {
			if singleQuorum, ok := results["single_node_quorum"]; ok {
				assert.GreaterOrEqual(t, singleOne, singleQuorum*0.9, 
					"ONE consistency should be faster or comparable to QUORUM")
			}
		}
	})
}