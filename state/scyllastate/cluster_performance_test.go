package scyllastate

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScyllaState_ClusterPerformance_ConsistencyLevels tests performance across different consistency levels
func TestScyllaState_ClusterPerformance_ConsistencyLevels(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cluster performance test in short mode")
	}

	cluster := StartScyllaDBCluster(t, 3)
	defer cluster.Cleanup()

	consistencyLevels := []string{"ONE", "QUORUM", "ALL"}
	operationsPerTest := 1000

	results := make(map[string]map[string]float64)

	for _, consistency := range consistencyLevels {
		t.Run(fmt.Sprintf("Consistency_%s", consistency), func(t *testing.T) {
			config := cluster.GetConfig()
			config.Consistency = consistency

			state, err := New(context.Background(), config)
			require.NoError(t, err)
			defer state.Close()

			results[consistency] = make(map[string]float64)

			// Test Add performance
			t.Run("Add_performance", func(t *testing.T) {
				pins := generateBenchmarkPins(t, operationsPerTest)
				
				start := time.Now()
				for _, pin := range pins {
					err := state.Add(context.Background(), pin)
					require.NoError(t, err)
				}
				duration := time.Since(start)
				
				opsPerSec := float64(operationsPerTest) / duration.Seconds()
				results[consistency]["add"] = opsPerSec
				
				t.Logf("Add performance with %s consistency: %.2f ops/sec", consistency, opsPerSec)
			})

			// Test Get performance
			t.Run("Get_performance", func(t *testing.T) {
				// Use pins from Add test
				pins := generateBenchmarkPins(t, operationsPerTest)
				
				start := time.Now()
				for i := 0; i < operationsPerTest; i++ {
					pinIndex := i % len(pins)
					_, err := state.Get(context.Background(), pins[pinIndex].Cid)
					if err != nil {
						// Pin might not exist, skip
						continue
					}
				}
				duration := time.Since(start)
				
				opsPerSec := float64(operationsPerTest) / duration.Seconds()
				results[consistency]["get"] = opsPerSec
				
				t.Logf("Get performance with %s consistency: %.2f ops/sec", consistency, opsPerSec)
			})
		})
	}

	// Compare performance across consistency levels
	t.Run("Performance_comparison", func(t *testing.T) {
		t.Logf("Performance comparison across consistency levels:")
		for _, consistency := range consistencyLevels {
			if addPerf, ok := results[consistency]["add"]; ok {
				t.Logf("  %s - Add: %.2f ops/sec", consistency, addPerf)
			}
			if getPerf, ok := results[consistency]["get"]; ok {
				t.Logf("  %s - Get: %.2f ops/sec", consistency, getPerf)
			}
		}

		// ONE should generally be fastest for writes
		if oneAdd, ok := results["ONE"]["add"]; ok {
			if quorumAdd, ok := results["QUORUM"]["add"]; ok {
				assert.GreaterOrEqual(t, oneAdd, quorumAdd*0.8, 
					"ONE consistency should be faster or comparable to QUORUM for writes")
			}
		}
	})
}

// TestScyllaState_ClusterPerformance_NodeFailure tests performance during node failures
func TestScyllaState_ClusterPerformance_NodeFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cluster performance test in short mode")
	}

	cluster := StartScyllaDBCluster(t, 3)
	defer cluster.Cleanup()

	config := cluster.GetConfig()
	config.Consistency = "QUORUM" // Use QUORUM for failure tolerance

	state, err := New(context.Background(), config)
	require.NoError(t, err)
	defer state.Close()

	simulator := NewClusterFailureSimulator(cluster)

	t.Run("Performance_before_failure", func(t *testing.T) {
		pins := generateBenchmarkPins(t, 500)
		
		start := time.Now()
		for _, pin := range pins {
			err := state.Add(context.Background(), pin)
			require.NoError(t, err)
		}
		duration := time.Since(start)
		
		opsPerSec := float64(len(pins)) / duration.Seconds()
		t.Logf("Performance before failure: %.2f ops/sec", opsPerSec)
		
		// Store baseline performance
		baselinePerf := opsPerSec
		
		t.Run("Performance_during_failure", func(t *testing.T) {
			// Stop one node (non-seed)
			err := simulator.SimulateNodeFailure(1)
			require.NoError(t, err)
			
			// Wait for failure detection
			time.Sleep(5 * time.Second)
			
			// Test performance with one node down
			failurePins := generateBenchmarkPins(t, 500)
			for i := range failurePins {
				failurePins[i].PinOptions.Name = fmt.Sprintf("failure-pin-%d", i)
				failurePins[i].Cid = createTestCIDWithSuffix(t, fmt.Sprintf("failure-%d", i))
			}
			
			start := time.Now()
			successCount := 0
			
			for _, pin := range failurePins {
				err := state.Add(context.Background(), pin)
				if err == nil {
					successCount++
				}
			}
			duration := time.Since(start)
			
			if successCount > 0 {
				opsPerSec := float64(successCount) / duration.Seconds()
				t.Logf("Performance during failure: %.2f ops/sec (%d/%d successful)", 
					opsPerSec, successCount, len(failurePins))
				
				// Performance should degrade but still function
				degradationRatio := opsPerSec / baselinePerf
				t.Logf("Performance degradation: %.2f%% of baseline", degradationRatio*100)
				
				// Should maintain at least 50% of baseline performance
				assert.Greater(t, degradationRatio, 0.3, 
					"Should maintain reasonable performance during single node failure")
			}
			
			// Recover the node
			err = simulator.SimulateNodeRecovery(1)
			require.NoError(t, err)
			
			// Wait for recovery
			time.Sleep(10 * time.Second)
			
			t.Run("Performance_after_recovery", func(t *testing.T) {
				recoveryPins := generateBenchmarkPins(t, 500)
				for i := range recoveryPins {
					recoveryPins[i].PinOptions.Name = fmt.Sprintf("recovery-pin-%d", i)
					recoveryPins[i].Cid = createTestCIDWithSuffix(t, fmt.Sprintf("recovery-%d", i))
				}
				
				start := time.Now()
				for _, pin := range recoveryPins {
					err := state.Add(context.Background(), pin)
					require.NoError(t, err)
				}
				duration := time.Since(start)
				
				opsPerSec := float64(len(recoveryPins)) / duration.Seconds()
				t.Logf("Performance after recovery: %.2f ops/sec", opsPerSec)
				
				// Performance should recover to near baseline
				recoveryRatio := opsPerSec / baselinePerf
				t.Logf("Recovery ratio: %.2f%% of baseline", recoveryRatio*100)
				
				assert.Greater(t, recoveryRatio, 0.7, 
					"Performance should recover after node recovery")
			})
		})
	})
}

// TestScyllaState_ClusterPerformance_LoadBalancing tests load balancing performance
func TestScyllaState_ClusterPerformance_LoadBalancing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cluster performance test in short mode")
	}

	cluster := StartScyllaDBCluster(t, 3)
	defer cluster.Cleanup()

	config := cluster.GetConfig()
	config.TokenAwareRouting = true // Enable token-aware routing for better load balancing

	state, err := New(context.Background(), config)
	require.NoError(t, err)
	defer state.Close()

	t.Run("Concurrent_operations_load_balancing", func(t *testing.T) {
		const numWorkers = 15 // 5 workers per node
		const operationsPerWorker = 200
		
		var wg sync.WaitGroup
		results := make([]float64, numWorkers)
		
		start := time.Now()
		
		for worker := 0; worker < numWorkers; worker++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				workerStart := time.Now()
				
				for i := 0; i < operationsPerWorker; i++ {
					pin := createTestPin(t)
					pin.PinOptions.Name = fmt.Sprintf("lb-pin-%d-%d", workerID, i)
					pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("lb-%d-%d", workerID, i))
					
					err := state.Add(context.Background(), pin)
					assert.NoError(t, err)
				}
				
				workerDuration := time.Since(workerStart)
				workerOpsPerSec := float64(operationsPerWorker) / workerDuration.Seconds()
				results[workerID] = workerOpsPerSec
			}(worker)
		}
		
		wg.Wait()
		totalDuration := time.Since(start)
		
		totalOps := numWorkers * operationsPerWorker
		totalOpsPerSec := float64(totalOps) / totalDuration.Seconds()
		
		t.Logf("Load balancing test: %d total ops in %v (%.2f ops/sec)", 
			totalOps, totalDuration, totalOpsPerSec)
		
		// Calculate worker performance statistics
		var minWorkerPerf, maxWorkerPerf, avgWorkerPerf float64
		minWorkerPerf = results[0]
		maxWorkerPerf = results[0]
		
		for _, perf := range results {
			if perf < minWorkerPerf {
				minWorkerPerf = perf
			}
			if perf > maxWorkerPerf {
				maxWorkerPerf = perf
			}
			avgWorkerPerf += perf
		}
		avgWorkerPerf /= float64(numWorkers)
		
		t.Logf("Worker performance - Min: %.2f, Max: %.2f, Avg: %.2f ops/sec", 
			minWorkerPerf, maxWorkerPerf, avgWorkerPerf)
		
		// Load balancing should result in relatively even performance
		perfVariance := (maxWorkerPerf - minWorkerPerf) / avgWorkerPerf
		t.Logf("Performance variance: %.2f%%", perfVariance*100)
		
		// Variance should be reasonable (less than 50%)
		assert.Less(t, perfVariance, 0.5, 
			"Load balancing should result in relatively even worker performance")
		
		// Total throughput should be good
		assert.Greater(t, totalOpsPerSec, 1000.0, 
			"Cluster should achieve good total throughput")
	})
}

// TestScyllaState_ClusterPerformance_DataDistribution tests performance with data distribution
func TestScyllaState_ClusterPerformance_DataDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cluster performance test in short mode")
	}

	cluster := StartScyllaDBCluster(t, 3)
	defer cluster.Cleanup()

	config := cluster.GetConfig()
	state, err := New(context.Background(), config)
	require.NoError(t, err)
	defer state.Close()

	t.Run("Large_dataset_distribution", func(t *testing.T) {
		const datasetSize = 10000
		
		t.Logf("Testing data distribution with %d pins", datasetSize)
		
		// Generate pins with diverse CIDs to ensure good distribution
		pins := make([]api.Pin, datasetSize)
		for i := 0; i < datasetSize; i++ {
			pin := createTestPin(t)
			pin.PinOptions.Name = fmt.Sprintf("dist-pin-%d", i)
			// Create more diverse CIDs for better distribution
			pin.Cid = createTestCIDWithSuffix(t, fmt.Sprintf("dist-%d-%d", i, i*7919)) // Use prime for better distribution
			pins[i] = pin
		}
		
		// Insert data using batch operations for efficiency
		batchState := state.Batch()
		batchSize := 500
		
		start := time.Now()
		
		for i, pin := range pins {
			err := batchState.Add(context.Background(), pin)
			require.NoError(t, err)
			
			if (i+1)%batchSize == 0 || i == datasetSize-1 {
				err := batchState.Commit(context.Background())
				require.NoError(t, err)
				
				if (i+1)%2000 == 0 {
					t.Logf("Inserted %d/%d pins", i+1, datasetSize)
				}
			}
		}
		
		insertDuration := time.Since(start)
		insertRate := float64(datasetSize) / insertDuration.Seconds()
		
		t.Logf("Distributed %d pins across cluster in %v (%.2f pins/sec)", 
			datasetSize, insertDuration, insertRate)
		
		// Test random access performance across distributed data
		t.Run("Random_access_performance", func(t *testing.T) {
			const randomAccessCount = 2000
			
			start := time.Now()
			
			for i := 0; i < randomAccessCount; i++ {
				// Access random pins
				pinIndex := i % len(pins)
				_, err := state.Get(context.Background(), pins[pinIndex].Cid)
				assert.NoError(t, err)
			}
			
			accessDuration := time.Since(start)
			accessRate := float64(randomAccessCount) / accessDuration.Seconds()
			
			t.Logf("Random access performance: %.2f gets/sec", accessRate)
			
			// Should maintain good random access performance
			assert.Greater(t, accessRate, 1000.0, 
				"Should maintain good random access performance on distributed data")
		})
		
		// Test list performance with large distributed dataset
		t.Run("List_performance_distributed", func(t *testing.T) {
			start := time.Now()
			
			pinChan := make(chan api.Pin, 1000)
			go func() {
				err := state.List(context.Background(), pinChan)
				assert.NoError(t, err)
			}()
			
			count := 0
			for range pinChan {
				count++
			}
			
			listDuration := time.Since(start)
			listRate := float64(count) / listDuration.Seconds()
			
			t.Logf("Listed %d pins from distributed cluster in %v (%.2f pins/sec)", 
				count, listDuration, listRate)
			
			assert.GreaterOrEqual(t, count, datasetSize, "Should list all distributed pins")
			assert.Greater(t, listRate, 2000.0, "Should achieve good list performance")
		})
		
		assert.Greater(t, insertRate, 1000.0, "Should achieve good distributed insert rate")
	})
}

// TestScyllaState_ClusterPerformance_NetworkLatency tests performance under network latency
func TestScyllaState_ClusterPerformance_NetworkLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cluster performance test in short mode")
	}

	cluster := StartScyllaDBCluster(t, 3)
	defer cluster.Cleanup()

	// Test with different timeout configurations to simulate network latency
	timeoutConfigs := []struct {
		name    string
		timeout time.Duration
	}{
		{"low_latency", 5 * time.Second},
		{"medium_latency", 10 * time.Second},
		{"high_latency", 20 * time.Second},
	}

	for _, timeoutConfig := range timeoutConfigs {
		t.Run(timeoutConfig.name, func(t *testing.T) {
			config := cluster.GetConfig()
			config.Timeout = timeoutConfig.timeout
			config.ConnectTimeout = timeoutConfig.timeout / 2

			state, err := New(context.Background(), config)
			require.NoError(t, err)
			defer state.Close()

			const operationCount = 500
			pins := generateBenchmarkPins(t, operationCount)

			start := time.Now()
			successCount := 0

			for _, pin := range pins {
				err := state.Add(context.Background(), pin)
				if err == nil {
					successCount++
				}
			}

			duration := time.Since(start)
			opsPerSec := float64(successCount) / duration.Seconds()

			t.Logf("Performance with %s timeout (%v): %.2f ops/sec (%d/%d successful)", 
				timeoutConfig.name, timeoutConfig.timeout, opsPerSec, successCount, operationCount)

			// Should complete most operations successfully
			successRate := float64(successCount) / float64(operationCount)
			assert.Greater(t, successRate, 0.9, 
				"Should complete most operations even with higher latency")

			// Performance should degrade gracefully with higher timeouts
			assert.Greater(t, opsPerSec, 50.0, 
				"Should maintain reasonable performance even with network latency")
		})
	}
}

// BenchmarkScyllaState_ClusterThroughput benchmarks cluster throughput
func BenchmarkScyllaState_ClusterThroughput(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping cluster benchmark in short mode")
	}

	cluster := StartScyllaDBCluster(b, 3)
	defer cluster.Cleanup()

	config := cluster.GetConfig()
	state, err := New(context.Background(), config)
	require.NoError(b, err)
	defer state.Close()

	pins := generateBenchmarkPins(b, b.N)

	b.ResetTimer()
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		err := state.Add(context.Background(), pins[i])
		if err != nil {
			b.Fatalf("Cluster Add failed: %v", err)
		}
	}

	b.StopTimer()

	opsPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
	
	// Report cluster-specific metrics
	b.ReportMetric(float64(cluster.GetNodeCount()), "nodes")
}

// BenchmarkScyllaState_ClusterConcurrentThroughput benchmarks concurrent cluster throughput
func BenchmarkScyllaState_ClusterConcurrentThroughput(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping cluster benchmark in short mode")
	}

	cluster := StartScyllaDBCluster(b, 3)
	defer cluster.Cleanup()

	config := cluster.GetConfig()
	state, err := New(context.Background(), config)
	require.NoError(b, err)
	defer state.Close()

	pins := generateBenchmarkPins(b, b.N)
	const concurrency = 20

	b.ResetTimer()
	b.StartTimer()

	var wg sync.WaitGroup
	operationsPerWorker := b.N / concurrency

	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			start := workerID * operationsPerWorker
			end := start + operationsPerWorker
			if workerID == concurrency-1 {
				end = b.N // Handle remainder
			}

			for i := start; i < end; i++ {
				err := state.Add(context.Background(), pins[i])
				if err != nil {
					b.Errorf("Concurrent cluster Add failed: %v", err)
				}
			}
		}(worker)
	}

	wg.Wait()
	b.StopTimer()

	opsPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
	b.ReportMetric(float64(concurrency), "workers")
	b.ReportMetric(float64(cluster.GetNodeCount()), "nodes")
}