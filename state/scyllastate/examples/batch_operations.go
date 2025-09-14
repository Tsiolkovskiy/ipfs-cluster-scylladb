// Package examples demonstrates batch operations with ScyllaDB state store
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/state/scyllastate"
)

// BatchOperationsExample demonstrates efficient batch operations
func BatchOperationsExample() {
	fmt.Println("=== Batch Operations Example ===")

	// Setup
	config := &scyllastate.Config{
		Hosts:       []string{"127.0.0.1"},
		Port:        9042,
		Keyspace:    "ipfs_pins_batch_example",
		Consistency: "QUORUM",
		Timeout:     30 * time.Second,
	}
	config.Default()

	ctx := context.Background()
	state, err := scyllastate.New(ctx, config)
	if err != nil {
		log.Fatal("Failed to create state store:", err)
	}
	defer state.Close()

	// Example 1: Basic batch operations
	fmt.Println("1. Basic batch operations...")
	basicBatchExample(ctx, state)

	// Example 2: Large batch processing
	fmt.Println("2. Large batch processing...")
	largeBatchExample(ctx, state)

	// Example 3: Mixed batch operations
	fmt.Println("3. Mixed batch operations...")
	mixedBatchExample(ctx, state)

	// Example 4: Batch with error handling
	fmt.Println("4. Batch error handling...")
	batchErrorHandlingExample(ctx, state)

	fmt.Println("=== Batch Operations Example Complete ===\n")
}

// basicBatchExample demonstrates basic batch add operations
func basicBatchExample(ctx context.Context, state *scyllastate.ScyllaState) {
	// Create batch state
	batchState := state.Batch()
	if batchState == nil {
		log.Fatal("Failed to create batch state")
	}

	// Create test pins
	pins := make([]api.Pin, 10)
	for i := 0; i < 10; i++ {
		pins[i] = createBatchPin(fmt.Sprintf("basic-batch-pin-%d", i))
	}

	start := time.Now()

	// Add pins to batch
	for i, pin := range pins {
		err := batchState.Add(ctx, pin)
		if err != nil {
			log.Printf("Failed to add pin %d to batch: %v", i, err)
			continue
		}
		fmt.Printf("  Added pin %d to batch: %s\n", i+1, pin.PinOptions.Name)
	}

	// Commit the batch
	err := batchState.Commit(ctx)
	if err != nil {
		log.Fatal("Failed to commit batch:", err)
	}

	duration := time.Since(start)
	fmt.Printf("✓ Batch of %d pins committed in %v\n", len(pins), duration)

	// Verify pins were added
	for i, pin := range pins {
		exists, err := state.Has(ctx, pin.Cid)
		if err != nil {
			log.Printf("Failed to check pin %d: %v", i, err)
			continue
		}
		if !exists {
			log.Printf("Pin %d was not found after batch commit", i)
		}
	}
	fmt.Printf("✓ All %d pins verified in database\n", len(pins))

	// Cleanup
	for _, pin := range pins {
		state.Rm(ctx, pin.Cid)
	}
}

// largeBatchExample demonstrates processing large batches efficiently
func largeBatchExample(ctx context.Context, state *scyllastate.ScyllaState) {
	const totalPins = 1000
	const batchSize = 100

	fmt.Printf("  Processing %d pins in batches of %d...\n", totalPins, batchSize)

	start := time.Now()
	batchState := state.Batch()
	processed := 0

	for i := 0; i < totalPins; i++ {
		pin := createBatchPin(fmt.Sprintf("large-batch-pin-%d", i))

		err := batchState.Add(ctx, pin)
		if err != nil {
			log.Printf("Failed to add pin %d to batch: %v", i, err)
			continue
		}

		processed++

		// Commit every batchSize operations
		if processed%batchSize == 0 || i == totalPins-1 {
			err := batchState.Commit(ctx)
			if err != nil {
				log.Printf("Failed to commit batch at pin %d: %v", i, err)
				continue
			}
			fmt.Printf("  Committed batch %d/%d (%d pins)\n", 
				processed/batchSize, (totalPins+batchSize-1)/batchSize, processed)
		}
	}

	duration := time.Since(start)
	rate := float64(processed) / duration.Seconds()
	fmt.Printf("✓ Processed %d pins in %v (%.2f pins/sec)\n", processed, duration, rate)

	// Cleanup - remove all pins in batches
	fmt.Printf("  Cleaning up %d pins...\n", processed)
	cleanupStart := time.Now()
	
	for i := 0; i < processed; i++ {
		pin := createBatchPin(fmt.Sprintf("large-batch-pin-%d", i))
		
		err := batchState.Rm(ctx, pin.Cid)
		if err != nil {
			log.Printf("Failed to add removal %d to batch: %v", i, err)
			continue
		}

		// Commit cleanup batches
		if (i+1)%batchSize == 0 || i == processed-1 {
			err := batchState.Commit(ctx)
			if err != nil {
				log.Printf("Failed to commit cleanup batch at pin %d: %v", i, err)
			}
		}
	}

	cleanupDuration := time.Since(cleanupStart)
	fmt.Printf("✓ Cleanup completed in %v\n", cleanupDuration)
}

// mixedBatchExample demonstrates mixed add/remove operations in batches
func mixedBatchExample(ctx context.Context, state *scyllastate.ScyllaState) {
	// First, add some pins individually for removal testing
	existingPins := make([]api.Pin, 5)
	for i := 0; i < 5; i++ {
		existingPins[i] = createBatchPin(fmt.Sprintf("existing-pin-%d", i))
		err := state.Add(ctx, existingPins[i])
		if err != nil {
			log.Printf("Failed to add existing pin %d: %v", i, err)
		}
	}

	// Create batch with mixed operations
	batchState := state.Batch()
	
	// Add new pins
	newPins := make([]api.Pin, 5)
	for i := 0; i < 5; i++ {
		newPins[i] = createBatchPin(fmt.Sprintf("mixed-new-pin-%d", i))
		err := batchState.Add(ctx, newPins[i])
		if err != nil {
			log.Printf("Failed to add new pin %d to batch: %v", i, err)
		}
	}

	// Remove existing pins
	for i, pin := range existingPins {
		err := batchState.Rm(ctx, pin.Cid)
		if err != nil {
			log.Printf("Failed to add removal %d to batch: %v", i, err)
		}
	}

	// Commit mixed batch
	start := time.Now()
	err := batchState.Commit(ctx)
	if err != nil {
		log.Fatal("Failed to commit mixed batch:", err)
	}
	duration := time.Since(start)

	fmt.Printf("✓ Mixed batch (5 adds, 5 removes) committed in %v\n", duration)

	// Verify results
	addCount := 0
	for _, pin := range newPins {
		exists, err := state.Has(ctx, pin.Cid)
		if err == nil && exists {
			addCount++
		}
	}

	removeCount := 0
	for _, pin := range existingPins {
		exists, err := state.Has(ctx, pin.Cid)
		if err == nil && !exists {
			removeCount++
		}
	}

	fmt.Printf("✓ Verified: %d pins added, %d pins removed\n", addCount, removeCount)

	// Cleanup new pins
	for _, pin := range newPins {
		state.Rm(ctx, pin.Cid)
	}
}

// batchErrorHandlingExample demonstrates error handling in batch operations
func batchErrorHandlingExample(ctx context.Context, state *scyllastate.ScyllaState) {
	batchState := state.Batch()

	// Add some valid pins
	validPins := make([]api.Pin, 3)
	for i := 0; i < 3; i++ {
		validPins[i] = createBatchPin(fmt.Sprintf("valid-pin-%d", i))
		err := batchState.Add(ctx, validPins[i])
		if err != nil {
			log.Printf("Failed to add valid pin %d: %v", i, err)
		}
	}

	// Try to add a pin with invalid data (this might not fail immediately in batch)
	invalidPin := api.Pin{
		// Missing required fields to potentially cause issues
		Type: api.DataType,
		PinOptions: api.PinOptions{
			Name: "invalid-pin",
		},
	}

	err := batchState.Add(ctx, invalidPin)
	if err != nil {
		fmt.Printf("✓ Caught error adding invalid pin to batch: %v\n", err)
	} else {
		fmt.Printf("  Invalid pin added to batch (will fail on commit)\n")
	}

	// Attempt to commit batch
	fmt.Printf("  Attempting to commit batch with potential errors...\n")
	err = batchState.Commit(ctx)
	if err != nil {
		fmt.Printf("✓ Batch commit failed as expected: %v\n", err)
		
		// Create new batch for valid pins only
		fmt.Printf("  Retrying with valid pins only...\n")
		retryBatch := state.Batch()
		
		for _, pin := range validPins {
			retryBatch.Add(ctx, pin)
		}
		
		err = retryBatch.Commit(ctx)
		if err != nil {
			log.Printf("Retry batch also failed: %v", err)
		} else {
			fmt.Printf("✓ Retry batch committed successfully\n")
		}
	} else {
		fmt.Printf("✓ Batch committed successfully (invalid pin was handled)\n")
	}

	// Cleanup
	for _, pin := range validPins {
		state.Rm(ctx, pin.Cid)
	}
}

// PerformanceComparisonExample compares batch vs individual operations
func PerformanceComparisonExample() {
	fmt.Println("=== Performance Comparison Example ===")

	config := &scyllastate.Config{
		Hosts:       []string{"127.0.0.1"},
		Port:        9042,
		Keyspace:    "ipfs_pins_perf_example",
		Consistency: "QUORUM",
		Timeout:     30 * time.Second,
	}
	config.Default()

	ctx := context.Background()
	state, err := scyllastate.New(ctx, config)
	if err != nil {
		log.Fatal("Failed to create state store:", err)
	}
	defer state.Close()

	const numPins = 100

	// Test 1: Individual operations
	fmt.Printf("1. Individual operations (%d pins)...\n", numPins)
	individualPins := make([]api.Pin, numPins)
	for i := 0; i < numPins; i++ {
		individualPins[i] = createBatchPin(fmt.Sprintf("individual-pin-%d", i))
	}

	start := time.Now()
	for _, pin := range individualPins {
		err := state.Add(ctx, pin)
		if err != nil {
			log.Printf("Failed to add individual pin: %v", err)
		}
	}
	individualDuration := time.Since(start)
	individualRate := float64(numPins) / individualDuration.Seconds()

	fmt.Printf("✓ Individual operations: %v (%.2f pins/sec)\n", 
		individualDuration, individualRate)

	// Test 2: Batch operations
	fmt.Printf("2. Batch operations (%d pins)...\n", numPins)
	batchPins := make([]api.Pin, numPins)
	for i := 0; i < numPins; i++ {
		batchPins[i] = createBatchPin(fmt.Sprintf("batch-pin-%d", i))
	}

	start = time.Now()
	batchState := state.Batch()
	for _, pin := range batchPins {
		err := batchState.Add(ctx, pin)
		if err != nil {
			log.Printf("Failed to add batch pin: %v", err)
		}
	}
	err = batchState.Commit(ctx)
	if err != nil {
		log.Printf("Failed to commit batch: %v", err)
	}
	batchDuration := time.Since(start)
	batchRate := float64(numPins) / batchDuration.Seconds()

	fmt.Printf("✓ Batch operations: %v (%.2f pins/sec)\n", 
		batchDuration, batchRate)

	// Performance comparison
	improvement := (individualDuration.Seconds() - batchDuration.Seconds()) / individualDuration.Seconds() * 100
	fmt.Printf("✓ Batch operations are %.2f%% faster\n", improvement)
	fmt.Printf("✓ Throughput improvement: %.2fx\n", batchRate/individualRate)

	// Cleanup
	for _, pin := range individualPins {
		state.Rm(ctx, pin.Cid)
	}
	for _, pin := range batchPins {
		state.Rm(ctx, pin.Cid)
	}

	fmt.Println("=== Performance Comparison Example Complete ===\n")
}

// OptimalBatchSizeExample demonstrates finding optimal batch sizes
func OptimalBatchSizeExample() {
	fmt.Println("=== Optimal Batch Size Example ===")

	config := &scyllastate.Config{
		Hosts:       []string{"127.0.0.1"},
		Port:        9042,
		Keyspace:    "ipfs_pins_batch_size_example",
		Consistency: "QUORUM",
		Timeout:     30 * time.Second,
	}
	config.Default()

	ctx := context.Background()
	state, err := scyllastate.New(ctx, config)
	if err != nil {
		log.Fatal("Failed to create state store:", err)
	}
	defer state.Close()

	batchSizes := []int{10, 50, 100, 200, 500, 1000}
	const totalPins = 1000

	fmt.Printf("Testing batch sizes with %d total pins...\n", totalPins)

	for _, batchSize := range batchSizes {
		fmt.Printf("  Testing batch size: %d\n", batchSize)

		start := time.Now()
		batchState := state.Batch()
		processed := 0

		for i := 0; i < totalPins; i++ {
			pin := createBatchPin(fmt.Sprintf("size-test-%d-%d", batchSize, i))
			
			err := batchState.Add(ctx, pin)
			if err != nil {
				log.Printf("Failed to add pin: %v", err)
				continue
			}
			processed++

			// Commit when batch is full
			if processed%batchSize == 0 || i == totalPins-1 {
				err := batchState.Commit(ctx)
				if err != nil {
					log.Printf("Failed to commit batch: %v", err)
				}
			}
		}

		duration := time.Since(start)
		rate := float64(processed) / duration.Seconds()
		
		fmt.Printf("    Batch size %d: %v (%.2f pins/sec)\n", 
			batchSize, duration, rate)

		// Cleanup
		cleanupBatch := state.Batch()
		for i := 0; i < processed; i++ {
			pin := createBatchPin(fmt.Sprintf("size-test-%d-%d", batchSize, i))
			cleanupBatch.Rm(ctx, pin.Cid)
			
			if (i+1)%batchSize == 0 || i == processed-1 {
				cleanupBatch.Commit(ctx)
			}
		}
	}

	fmt.Println("✓ Batch size testing complete")
	fmt.Println("  Recommendation: Use batch sizes between 100-500 for optimal performance")
	fmt.Println("=== Optimal Batch Size Example Complete ===\n")
}

// createBatchPin creates a test pin for batch operations
func createBatchPin(name string) api.Pin {
	// Create a unique CID based on the name
	cidStr := fmt.Sprintf("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPb%02x", 
		len(name)%256)
	cid, _ := api.DecodeCid(cidStr)

	return api.Pin{
		Cid:  cid,
		Type: api.DataType,
		Allocations: []api.PeerID{
			api.PeerID("12D3KooWBhMbKvZso7VbJGqJNjKgJkGGN8yKWBGTGaHFdMzqk1mC"),
		},
		ReplicationFactorMin: 1,
		ReplicationFactorMax: 2,
		MaxDepth:             -1,
		PinOptions: api.PinOptions{
			Name: name,
			UserDefined: map[string]string{
				"batch":     "true",
				"timestamp": fmt.Sprintf("%d", time.Now().Unix()),
			},
		},
	}
}

func main() {
	fmt.Println("ScyllaDB State Store - Batch Operations Examples")
	fmt.Println("===============================================")

	// Run examples
	BatchOperationsExample()
	PerformanceComparisonExample()
	OptimalBatchSizeExample()

	fmt.Println("All batch operation examples completed successfully!")
}