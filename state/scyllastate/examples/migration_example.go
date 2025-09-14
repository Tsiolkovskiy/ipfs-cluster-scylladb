// Package examples demonstrates migration between different state backends
package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/datastore/inmem"
	"github.com/ipfs-cluster/ipfs-cluster/state"
	"github.com/ipfs-cluster/ipfs-cluster/state/dsstate"
	"github.com/ipfs-cluster/ipfs-cluster/state/scyllastate"
)

// MigrationExample demonstrates migrating from dsstate to ScyllaDB
func MigrationExample() {
	fmt.Println("=== Migration Example ===")

	ctx := context.Background()

	// Example 1: Simple migration from in-memory datastore
	fmt.Println("1. Simple migration from in-memory datastore...")
	simpleMigrationExample(ctx)

	// Example 2: Large dataset migration
	fmt.Println("2. Large dataset migration...")
	largeMigrationExample(ctx)

	// Example 3: Bidirectional migration
	fmt.Println("3. Bidirectional migration...")
	bidirectionalMigrationExample(ctx)

	// Example 4: Migration with validation
	fmt.Println("4. Migration with validation...")
	migrationWithValidationExample(ctx)

	fmt.Println("=== Migration Example Complete ===\n")
}

// simpleMigrationExample demonstrates basic migration
func simpleMigrationExample(ctx context.Context) {
	// Create source state (in-memory datastore)
	sourceDS := inmem.New()
	sourceState, err := dsstate.New(ctx, sourceDS, "test", nil)
	if err != nil {
		log.Fatal("Failed to create source state:", err)
	}
	defer sourceState.Close()

	// Add test data to source
	testPins := createMigrationTestPins(10)
	for i, pin := range testPins {
		err := sourceState.Add(ctx, pin)
		if err != nil {
			log.Printf("Failed to add pin %d to source: %v", i, err)
		}
	}
	fmt.Printf("  Added %d pins to source state\n", len(testPins))

	// Create destination state (ScyllaDB)
	destConfig := &scyllastate.Config{
		Hosts:       []string{"127.0.0.1"},
		Port:        9042,
		Keyspace:    "ipfs_pins_migration_simple",
		Consistency: "QUORUM",
		Timeout:     30 * time.Second,
	}
	destConfig.Default()

	destState, err := scyllastate.New(ctx, destConfig)
	if err != nil {
		log.Fatal("Failed to create destination state:", err)
	}
	defer destState.Close()

	// Perform migration
	fmt.Printf("  Starting migration...\n")
	start := time.Now()

	var buf bytes.Buffer
	
	// Export from source
	err = sourceState.Marshal(&buf)
	if err != nil {
		log.Fatal("Failed to marshal source state:", err)
	}
	
	// Import to destination
	err = destState.Unmarshal(&buf)
	if err != nil {
		log.Fatal("Failed to unmarshal to destination state:", err)
	}

	duration := time.Since(start)
	fmt.Printf("✓ Migration completed in %v\n", duration)

	// Verify migration
	verifyMigration(ctx, testPins, destState)

	// Cleanup
	for _, pin := range testPins {
		destState.Rm(ctx, pin.Cid)
	}
}

// largeMigrationExample demonstrates migration of large datasets
func largeMigrationExample(ctx context.Context) {
	const datasetSize = 1000

	// Create source state with large dataset
	sourceDS := inmem.New()
	sourceState, err := dsstate.New(ctx, sourceDS, "large_test", nil)
	if err != nil {
		log.Fatal("Failed to create source state:", err)
	}
	defer sourceState.Close()

	// Generate large dataset
	fmt.Printf("  Generating %d pins for large migration...\n", datasetSize)
	testPins := createMigrationTestPins(datasetSize)

	// Add pins to source in batches for efficiency
	batchSize := 100
	for i := 0; i < len(testPins); i += batchSize {
		end := i + batchSize
		if end > len(testPins) {
			end = len(testPins)
		}

		for j := i; j < end; j++ {
			err := sourceState.Add(ctx, testPins[j])
			if err != nil {
				log.Printf("Failed to add pin %d: %v", j, err)
			}
		}

		if (i+batchSize)%500 == 0 || end == len(testPins) {
			fmt.Printf("    Added %d/%d pins to source\n", end, len(testPins))
		}
	}

	// Create destination state
	destConfig := &scyllastate.Config{
		Hosts:       []string{"127.0.0.1"},
		Port:        9042,
		Keyspace:    "ipfs_pins_migration_large",
		Consistency: "QUORUM",
		Timeout:     60 * time.Second,
	}
	destConfig.Default()

	destState, err := scyllastate.New(ctx, destConfig)
	if err != nil {
		log.Fatal("Failed to create destination state:", err)
	}
	defer destState.Close()

	// Perform large migration
	fmt.Printf("  Starting large migration (%d pins)...\n", datasetSize)
	start := time.Now()

	var buf bytes.Buffer
	
	// Export from source
	exportStart := time.Now()
	err = sourceState.Marshal(&buf)
	if err != nil {
		log.Fatal("Failed to marshal source state:", err)
	}
	exportDuration := time.Since(exportStart)
	
	// Import to destination
	importStart := time.Now()
	err = destState.Unmarshal(&buf)
	if err != nil {
		log.Fatal("Failed to unmarshal to destination state:", err)
	}
	importDuration := time.Since(importStart)

	totalDuration := time.Since(start)
	
	fmt.Printf("✓ Large migration completed:\n")
	fmt.Printf("    Export: %v\n", exportDuration)
	fmt.Printf("    Import: %v\n", importDuration)
	fmt.Printf("    Total: %v\n", totalDuration)
	fmt.Printf("    Rate: %.2f pins/sec\n", float64(datasetSize)/totalDuration.Seconds())

	// Verify sample of migrated data
	sampleSize := 50
	samplePins := testPins[:sampleSize]
	fmt.Printf("  Verifying sample of %d pins...\n", sampleSize)
	verifyMigration(ctx, samplePins, destState)

	// Cleanup (in batches for efficiency)
	fmt.Printf("  Cleaning up %d pins...\n", datasetSize)
	batchState := destState.Batch()
	for i, pin := range testPins {
		batchState.Rm(ctx, pin.Cid)
		
		if (i+1)%batchSize == 0 || i == len(testPins)-1 {
			err := batchState.Commit(ctx)
			if err != nil {
				log.Printf("Failed to commit cleanup batch: %v", err)
			}
		}
	}
}

// bidirectionalMigrationExample demonstrates migration in both directions
func bidirectionalMigrationExample(ctx context.Context) {
	// Create ScyllaDB state as source
	scyllaConfig := &scyllastate.Config{
		Hosts:       []string{"127.0.0.1"},
		Port:        9042,
		Keyspace:    "ipfs_pins_migration_bidir",
		Consistency: "QUORUM",
		Timeout:     30 * time.Second,
	}
	scyllaConfig.Default()

	scyllaState, err := scyllastate.New(ctx, scyllaConfig)
	if err != nil {
		log.Fatal("Failed to create ScyllaDB state:", err)
	}
	defer scyllaState.Close()

	// Add test data to ScyllaDB
	testPins := createMigrationTestPins(25)
	for _, pin := range testPins {
		err := scyllaState.Add(ctx, pin)
		if err != nil {
			log.Printf("Failed to add pin to ScyllaDB: %v", err)
		}
	}
	fmt.Printf("  Added %d pins to ScyllaDB state\n", len(testPins))

	// Create in-memory datastore as destination
	destDS := inmem.New()
	destState, err := dsstate.New(ctx, destDS, "bidir_test", nil)
	if err != nil {
		log.Fatal("Failed to create destination state:", err)
	}
	defer destState.Close()

	// Migrate from ScyllaDB to in-memory
	fmt.Printf("  Migrating from ScyllaDB to in-memory datastore...\n")
	start := time.Now()

	var buf bytes.Buffer
	
	err = scyllaState.Marshal(&buf)
	if err != nil {
		log.Fatal("Failed to marshal ScyllaDB state:", err)
	}
	
	err = destState.Unmarshal(&buf)
	if err != nil {
		log.Fatal("Failed to unmarshal to in-memory state:", err)
	}

	duration := time.Since(start)
	fmt.Printf("✓ ScyllaDB → In-memory migration completed in %v\n", duration)

	// Verify migration
	verifyMigration(ctx, testPins, destState)

	// Now migrate back from in-memory to ScyllaDB
	fmt.Printf("  Migrating back from in-memory to ScyllaDB...\n")
	
	// Create new ScyllaDB state for return migration
	returnConfig := &scyllastate.Config{
		Hosts:       []string{"127.0.0.1"},
		Port:        9042,
		Keyspace:    "ipfs_pins_migration_return",
		Consistency: "QUORUM",
		Timeout:     30 * time.Second,
	}
	returnConfig.Default()

	returnState, err := scyllastate.New(ctx, returnConfig)
	if err != nil {
		log.Fatal("Failed to create return ScyllaDB state:", err)
	}
	defer returnState.Close()

	start = time.Now()
	buf.Reset()
	
	err = destState.Marshal(&buf)
	if err != nil {
		log.Fatal("Failed to marshal in-memory state:", err)
	}
	
	err = returnState.Unmarshal(&buf)
	if err != nil {
		log.Fatal("Failed to unmarshal to return ScyllaDB state:", err)
	}

	duration = time.Since(start)
	fmt.Printf("✓ In-memory → ScyllaDB migration completed in %v\n", duration)

	// Verify return migration
	verifyMigration(ctx, testPins, returnState)

	// Cleanup
	for _, pin := range testPins {
		scyllaState.Rm(ctx, pin.Cid)
		returnState.Rm(ctx, pin.Cid)
	}
}

// migrationWithValidationExample demonstrates migration with comprehensive validation
func migrationWithValidationExample(ctx context.Context) {
	// Create source state
	sourceDS := inmem.New()
	sourceState, err := dsstate.New(ctx, sourceDS, "validation_test", nil)
	if err != nil {
		log.Fatal("Failed to create source state:", err)
	}
	defer sourceState.Close()

	// Create test data with various pin types
	testPins := createDiverseTestPins()
	
	// Add pins to source
	for _, pin := range testPins {
		err := sourceState.Add(ctx, pin)
		if err != nil {
			log.Printf("Failed to add pin to source: %v", err)
		}
	}
	fmt.Printf("  Added %d diverse pins to source state\n", len(testPins))

	// Create destination state
	destConfig := &scyllastate.Config{
		Hosts:       []string{"127.0.0.1"},
		Port:        9042,
		Keyspace:    "ipfs_pins_migration_validation",
		Consistency: "QUORUM",
		Timeout:     30 * time.Second,
	}
	destConfig.Default()

	destState, err := scyllastate.New(ctx, destConfig)
	if err != nil {
		log.Fatal("Failed to create destination state:", err)
	}
	defer destState.Close()

	// Pre-migration validation
	fmt.Printf("  Pre-migration validation...\n")
	sourceCount := countPins(ctx, sourceState)
	fmt.Printf("    Source state has %d pins\n", sourceCount)

	// Perform migration
	fmt.Printf("  Performing migration with validation...\n")
	start := time.Now()

	var buf bytes.Buffer
	
	err = sourceState.Marshal(&buf)
	if err != nil {
		log.Fatal("Failed to marshal source state:", err)
	}
	
	err = destState.Unmarshal(&buf)
	if err != nil {
		log.Fatal("Failed to unmarshal to destination state:", err)
	}

	duration := time.Since(start)
	fmt.Printf("✓ Migration completed in %v\n", duration)

	// Post-migration validation
	fmt.Printf("  Post-migration validation...\n")
	destCount := countPins(ctx, destState)
	fmt.Printf("    Destination state has %d pins\n", destCount)

	if sourceCount == destCount {
		fmt.Printf("✓ Pin count matches: %d pins\n", destCount)
	} else {
		fmt.Printf("✗ Pin count mismatch: source=%d, dest=%d\n", sourceCount, destCount)
	}

	// Detailed validation
	fmt.Printf("  Detailed validation...\n")
	validationErrors := 0
	
	for i, expectedPin := range testPins {
		// Check existence
		exists, err := destState.Has(ctx, expectedPin.Cid)
		if err != nil {
			fmt.Printf("    ✗ Error checking pin %d: %v\n", i, err)
			validationErrors++
			continue
		}
		
		if !exists {
			fmt.Printf("    ✗ Pin %d missing: %s\n", i, expectedPin.Cid.String())
			validationErrors++
			continue
		}

		// Check pin data
		actualPin, err := destState.Get(ctx, expectedPin.Cid)
		if err != nil {
			fmt.Printf("    ✗ Error retrieving pin %d: %v\n", i, err)
			validationErrors++
			continue
		}

		// Validate pin fields
		if !actualPin.Cid.Equals(expectedPin.Cid) {
			fmt.Printf("    ✗ Pin %d CID mismatch\n", i)
			validationErrors++
		}
		
		if actualPin.PinOptions.Name != expectedPin.PinOptions.Name {
			fmt.Printf("    ✗ Pin %d name mismatch: expected=%s, actual=%s\n", 
				i, expectedPin.PinOptions.Name, actualPin.PinOptions.Name)
			validationErrors++
		}
		
		if actualPin.Type != expectedPin.Type {
			fmt.Printf("    ✗ Pin %d type mismatch\n", i)
			validationErrors++
		}
	}

	if validationErrors == 0 {
		fmt.Printf("✓ All %d pins validated successfully\n", len(testPins))
	} else {
		fmt.Printf("✗ Validation failed: %d errors found\n", validationErrors)
	}

	// Cleanup
	for _, pin := range testPins {
		destState.Rm(ctx, pin.Cid)
	}
}

// StreamingMigrationExample demonstrates streaming migration for very large datasets
func StreamingMigrationExample() {
	fmt.Println("=== Streaming Migration Example ===")

	ctx := context.Background()

	// Create source state with large dataset
	sourceDS := inmem.New()
	sourceState, err := dsstate.New(ctx, sourceDS, "streaming_test", nil)
	if err != nil {
		log.Fatal("Failed to create source state:", err)
	}
	defer sourceState.Close()

	// Create destination state
	destConfig := &scyllastate.Config{
		Hosts:       []string{"127.0.0.1"},
		Port:        9042,
		Keyspace:    "ipfs_pins_migration_streaming",
		Consistency: "QUORUM",
		Timeout:     60 * time.Second,
	}
	destConfig.Default()

	destState, err := scyllastate.New(ctx, destConfig)
	if err != nil {
		log.Fatal("Failed to create destination state:", err)
	}
	defer destState.Close()

	// Add test data
	const datasetSize = 2000
	fmt.Printf("Preparing %d pins for streaming migration...\n", datasetSize)
	
	testPins := createMigrationTestPins(datasetSize)
	for i, pin := range testPins {
		err := sourceState.Add(ctx, pin)
		if err != nil {
			log.Printf("Failed to add pin %d: %v", i, err)
		}
		
		if (i+1)%500 == 0 {
			fmt.Printf("  Added %d/%d pins\n", i+1, datasetSize)
		}
	}

	// Perform streaming migration
	fmt.Printf("Starting streaming migration...\n")
	start := time.Now()

	pinChan := make(chan api.Pin, 100)
	
	// Start reading from source
	go func() {
		defer close(pinChan)
		err := sourceState.List(ctx, pinChan)
		if err != nil {
			log.Printf("Failed to list source pins: %v", err)
		}
	}()

	// Stream to destination using batches
	batchState := destState.Batch()
	batchSize := 200
	processed := 0

	for pin := range pinChan {
		err := batchState.Add(ctx, pin)
		if err != nil {
			log.Printf("Failed to add pin to batch: %v", err)
			continue
		}
		
		processed++

		// Commit batch when full
		if processed%batchSize == 0 {
			err := batchState.Commit(ctx)
			if err != nil {
				log.Printf("Failed to commit batch: %v", err)
			}
			fmt.Printf("  Migrated %d/%d pins\n", processed, datasetSize)
		}
	}

	// Commit final batch
	if processed%batchSize != 0 {
		err := batchState.Commit(ctx)
		if err != nil {
			log.Printf("Failed to commit final batch: %v", err)
		}
	}

	duration := time.Since(start)
	rate := float64(processed) / duration.Seconds()
	
	fmt.Printf("✓ Streaming migration completed:\n")
	fmt.Printf("    Processed: %d pins\n", processed)
	fmt.Printf("    Duration: %v\n", duration)
	fmt.Printf("    Rate: %.2f pins/sec\n", rate)

	// Verify migration
	destCount := countPins(ctx, destState)
	fmt.Printf("    Verification: %d pins in destination\n", destCount)

	if destCount == processed {
		fmt.Printf("✓ Streaming migration successful\n")
	} else {
		fmt.Printf("✗ Migration incomplete: expected=%d, actual=%d\n", processed, destCount)
	}

	// Cleanup
	fmt.Printf("Cleaning up %d pins...\n", processed)
	cleanupBatch := destState.Batch()
	for i, pin := range testPins {
		cleanupBatch.Rm(ctx, pin.Cid)
		
		if (i+1)%batchSize == 0 || i == len(testPins)-1 {
			cleanupBatch.Commit(ctx)
		}
	}

	fmt.Println("=== Streaming Migration Example Complete ===\n")
}

// Helper functions

func createMigrationTestPins(count int) []api.Pin {
	pins := make([]api.Pin, count)
	for i := 0; i < count; i++ {
		cidStr := fmt.Sprintf("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPb%04x", i)
		cid, _ := api.DecodeCid(cidStr)

		pins[i] = api.Pin{
			Cid:  cid,
			Type: api.DataType,
			Allocations: []api.PeerID{
				api.PeerID("12D3KooWBhMbKvZso7VbJGqJNjKgJkGGN8yKWBGTGaHFdMzqk1mC"),
			},
			ReplicationFactorMin: 1,
			ReplicationFactorMax: 2,
			MaxDepth:             -1,
			PinOptions: api.PinOptions{
				Name: fmt.Sprintf("migration-test-pin-%d", i),
				UserDefined: map[string]string{
					"migration": "true",
					"index":     fmt.Sprintf("%d", i),
				},
			},
		}
	}
	return pins
}

func createDiverseTestPins() []api.Pin {
	pins := make([]api.Pin, 0)

	// Different pin types and configurations
	pinConfigs := []struct {
		name     string
		pinType  api.PinType
		replMin  int
		replMax  int
		maxDepth int
		metadata map[string]string
	}{
		{"data-pin", api.DataType, 1, 3, -1, map[string]string{"type": "data"}},
		{"meta-pin", api.MetaType, 2, 4, 2, map[string]string{"type": "meta"}},
		{"cluster-pin", api.ClusterDAGType, 1, 2, 5, map[string]string{"type": "cluster"}},
		{"shard-pin", api.ShardType, 3, 5, -1, map[string]string{"type": "shard"}},
	}

	for i, config := range pinConfigs {
		cidStr := fmt.Sprintf("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPb%04x", i+1000)
		cid, _ := api.DecodeCid(cidStr)

		pin := api.Pin{
			Cid:                  cid,
			Type:                 config.pinType,
			ReplicationFactorMin: config.replMin,
			ReplicationFactorMax: config.replMax,
			MaxDepth:             config.maxDepth,
			PinOptions: api.PinOptions{
				Name:        config.name,
				UserDefined: config.metadata,
			},
		}

		// Add different numbers of allocations
		for j := 0; j < config.replMax; j++ {
			peerID := api.PeerID(fmt.Sprintf("12D3KooWBhMbKvZso7VbJGqJNjKgJkGGN8yKWBGTGaHFdMzqk%02d", j))
			pin.Allocations = append(pin.Allocations, peerID)
		}

		pins = append(pins, pin)
	}

	return pins
}

func verifyMigration(ctx context.Context, expectedPins []api.Pin, destState state.State) {
	errors := 0
	
	for i, expectedPin := range expectedPins {
		exists, err := destState.Has(ctx, expectedPin.Cid)
		if err != nil {
			fmt.Printf("    ✗ Error checking pin %d: %v\n", i, err)
			errors++
			continue
		}
		
		if !exists {
			fmt.Printf("    ✗ Pin %d missing after migration\n", i)
			errors++
			continue
		}

		actualPin, err := destState.Get(ctx, expectedPin.Cid)
		if err != nil {
			fmt.Printf("    ✗ Error retrieving pin %d: %v\n", i, err)
			errors++
			continue
		}

		if actualPin.PinOptions.Name != expectedPin.PinOptions.Name {
			fmt.Printf("    ✗ Pin %d name mismatch\n", i)
			errors++
		}
	}

	if errors == 0 {
		fmt.Printf("✓ Migration verification passed (%d pins)\n", len(expectedPins))
	} else {
		fmt.Printf("✗ Migration verification failed (%d errors)\n", errors)
	}
}

func countPins(ctx context.Context, state state.State) int {
	pinChan := make(chan api.Pin, 100)
	
	go func() {
		defer close(pinChan)
		state.List(ctx, pinChan)
	}()

	count := 0
	for range pinChan {
		count++
	}
	
	return count
}

func main() {
	fmt.Println("ScyllaDB State Store - Migration Examples")
	fmt.Println("========================================")

	// Run examples
	MigrationExample()
	StreamingMigrationExample()

	fmt.Println("All migration examples completed successfully!")
}