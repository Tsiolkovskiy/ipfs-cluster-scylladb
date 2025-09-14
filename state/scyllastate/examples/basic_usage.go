// Package examples demonstrates basic usage of the ScyllaDB state store
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/state/scyllastate"
)

// BasicUsageExample demonstrates basic CRUD operations with ScyllaDB state store
func BasicUsageExample() {
	fmt.Println("=== Basic Usage Example ===")

	// 1. Create configuration
	config := &scyllastate.Config{
		Hosts:       []string{"127.0.0.1"},
		Port:        9042,
		Keyspace:    "ipfs_pins_example",
		Consistency: "QUORUM",
		Timeout:     10 * time.Second,
	}
	
	// Apply default values
	config.Default()

	// 2. Create state store instance
	ctx := context.Background()
	state, err := scyllastate.New(ctx, config)
	if err != nil {
		log.Fatal("Failed to create state store:", err)
	}
	defer state.Close()

	// 3. Create a test pin
	cid, err := api.DecodeCid("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	if err != nil {
		log.Fatal("Failed to decode CID:", err)
	}

	pin := api.Pin{
		Cid:  cid,
		Type: api.DataType,
		Allocations: []api.PeerID{
			api.PeerID("12D3KooWBhMbKvZso7VbJGqJNjKgJkGGN8yKWBGTGaHFdMzqk1mC"),
			api.PeerID("12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo"),
		},
		ReplicationFactorMin: 2,
		ReplicationFactorMax: 3,
		MaxDepth:             -1,
		PinOptions: api.PinOptions{
			Name:        "example-document",
			UserDefined: map[string]string{
				"category": "documents",
				"priority": "high",
			},
		},
	}

	// 4. Add the pin
	fmt.Println("Adding pin...")
	err = state.Add(ctx, pin)
	if err != nil {
		log.Fatal("Failed to add pin:", err)
	}
	fmt.Printf("✓ Pin added successfully: %s\n", pin.Cid.String())

	// 5. Check if pin exists
	fmt.Println("Checking if pin exists...")
	exists, err := state.Has(ctx, cid)
	if err != nil {
		log.Fatal("Failed to check pin existence:", err)
	}
	fmt.Printf("✓ Pin exists: %t\n", exists)

	// 6. Retrieve the pin
	fmt.Println("Retrieving pin...")
	retrievedPin, err := state.Get(ctx, cid)
	if err != nil {
		log.Fatal("Failed to get pin:", err)
	}
	fmt.Printf("✓ Retrieved pin: %s (name: %s)\n", 
		retrievedPin.Cid.String(), retrievedPin.PinOptions.Name)

	// 7. List all pins
	fmt.Println("Listing all pins...")
	pinChan := make(chan api.Pin, 100)
	
	go func() {
		defer close(pinChan)
		err := state.List(ctx, pinChan)
		if err != nil {
			log.Printf("Failed to list pins: %v", err)
		}
	}()

	count := 0
	for pin := range pinChan {
		count++
		fmt.Printf("  - Pin %d: %s (%s)\n", count, pin.Cid.String(), pin.PinOptions.Name)
	}
	fmt.Printf("✓ Listed %d pins total\n", count)

	// 8. Remove the pin
	fmt.Println("Removing pin...")
	err = state.Rm(ctx, cid)
	if err != nil {
		log.Fatal("Failed to remove pin:", err)
	}
	fmt.Printf("✓ Pin removed successfully: %s\n", cid.String())

	// 9. Verify removal
	exists, err = state.Has(ctx, cid)
	if err != nil {
		log.Fatal("Failed to check pin existence after removal:", err)
	}
	fmt.Printf("✓ Pin exists after removal: %t\n", exists)

	fmt.Println("=== Basic Usage Example Complete ===\n")
}

// ErrorHandlingExample demonstrates proper error handling
func ErrorHandlingExample() {
	fmt.Println("=== Error Handling Example ===")

	config := &scyllastate.Config{
		Hosts:          []string{"127.0.0.1"},
		Port:           9042,
		Keyspace:       "ipfs_pins_example",
		Consistency:    "QUORUM",
		Timeout:        5 * time.Second,
		ConnectTimeout: 3 * time.Second,
	}
	config.Default()

	ctx := context.Background()
	state, err := scyllastate.New(ctx, config)
	if err != nil {
		log.Fatal("Failed to create state store:", err)
	}
	defer state.Close()

	// Example 1: Handle non-existent pin
	fmt.Println("1. Handling non-existent pin...")
	nonExistentCid, _ := api.DecodeCid("QmNonExistentCidForTestingPurposes123456789")
	
	_, err = state.Get(ctx, nonExistentCid)
	if err != nil {
		if err.Error() == "not found" {
			fmt.Printf("✓ Correctly handled non-existent pin: %v\n", err)
		} else {
			fmt.Printf("✗ Unexpected error: %v\n", err)
		}
	}

	// Example 2: Handle timeout scenarios
	fmt.Println("2. Handling timeout scenarios...")
	shortCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()

	testPin := createExamplePin("timeout-test")
	err = state.Add(shortCtx, testPin)
	if err != nil {
		fmt.Printf("✓ Correctly handled timeout: %v\n", err)
	}

	// Example 3: Handle connection issues gracefully
	fmt.Println("3. Demonstrating graceful degradation...")
	
	// This would normally fail if ScyllaDB is unavailable
	// The retry policy will handle temporary failures
	testPin2 := createExamplePin("resilience-test")
	err = state.Add(ctx, testPin2)
	if err != nil {
		fmt.Printf("Operation failed (expected if ScyllaDB unavailable): %v\n", err)
	} else {
		fmt.Printf("✓ Operation succeeded with retry policy\n")
		// Clean up
		state.Rm(ctx, testPin2.Cid)
	}

	fmt.Println("=== Error Handling Example Complete ===\n")
}

// ConfigurationExample demonstrates different configuration options
func ConfigurationExample() {
	fmt.Println("=== Configuration Example ===")

	// Example 1: Development configuration
	fmt.Println("1. Development configuration...")
	devConfig := &scyllastate.Config{
		Hosts:       []string{"localhost"},
		Port:        9042,
		Keyspace:    "ipfs_pins_dev",
		Consistency: "ONE", // Relaxed consistency for development
		Timeout:     30 * time.Second,
	}
	devConfig.Default()
	fmt.Printf("✓ Dev config: %d hosts, %s consistency\n", 
		len(devConfig.Hosts), devConfig.Consistency)

	// Example 2: Production configuration
	fmt.Println("2. Production configuration...")
	prodConfig := &scyllastate.Config{
		Hosts: []string{
			"scylla-node1.prod.example.com",
			"scylla-node2.prod.example.com",
			"scylla-node3.prod.example.com",
		},
		Port:              9042,
		Keyspace:          "ipfs_pins_prod",
		Username:          "ipfs_cluster",
		Password:          "secure_password",
		Consistency:       "QUORUM",
		TokenAwareRouting: true,
		NumConns:          8,
		Timeout:           15 * time.Second,
		ConnectTimeout:    10 * time.Second,
	}
	prodConfig.Default()
	fmt.Printf("✓ Prod config: %d hosts, %d connections per host\n", 
		len(prodConfig.Hosts), prodConfig.NumConns)

	// Example 3: High-performance configuration
	fmt.Println("3. High-performance configuration...")
	perfConfig := &scyllastate.Config{
		Hosts:                      []string{"127.0.0.1"},
		Port:                       9042,
		Keyspace:                   "ipfs_pins_perf",
		Consistency:                "ONE",
		TokenAwareRouting:          true,
		NumConns:                   16,
		PageSize:                   10000,
		PreparedStatementCacheSize: 5000,
		Timeout:                    5 * time.Second,
	}
	perfConfig.Default()
	fmt.Printf("✓ Perf config: %d page size, %d statement cache\n", 
		perfConfig.PageSize, perfConfig.PreparedStatementCacheSize)

	fmt.Println("=== Configuration Example Complete ===\n")
}

// createExamplePin creates a test pin with the given name
func createExamplePin(name string) api.Pin {
	// Generate a unique CID based on the name
	cidStr := fmt.Sprintf("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPb%02x", 
		len(name)) // Simple way to create different CIDs
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
				"example": "true",
			},
		},
	}
}

func main() {
	fmt.Println("ScyllaDB State Store - Basic Usage Examples")
	fmt.Println("==========================================")

	// Run examples
	BasicUsageExample()
	ErrorHandlingExample()
	ConfigurationExample()

	fmt.Println("All examples completed successfully!")
}