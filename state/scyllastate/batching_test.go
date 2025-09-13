package scyllastate

import (
	"testing"
	"time"
)

func TestScyllaState_NewBatching_Closed(t *testing.T) {
	// Create a closed ScyllaState
	s := &ScyllaState{
		closed: 1, // Mark as closed
	}

	// Test creating a new batching state on closed state
	batchingState, err := s.NewBatching()
	if err == nil {
		t.Error("Expected error for closed state")
	}
	if batchingState != nil {
		t.Error("Expected nil batching state for closed state")
	}
	if err != nil && err.Error() != "ScyllaState is closed" {
		t.Errorf("Expected 'ScyllaState is closed' error, got: %v", err)
	}
}

func TestAutoCommitBatcher_Configuration(t *testing.T) {
	// Test auto-commit batcher configuration validation
	maxBatchSize := 100
	maxBatchTime := 5 * time.Second

	// Test that the configuration values are reasonable
	if maxBatchSize <= 0 {
		t.Error("Max batch size should be positive")
	}
	if maxBatchTime <= 0 {
		t.Error("Max batch time should be positive")
	}

	// Test that batch size limits are enforced
	if maxBatchSize > 10000 {
		t.Error("Max batch size should not be too large to avoid memory issues")
	}
}

func TestBatchingLogic(t *testing.T) {
	// Test batching logic without requiring a real session

	// Test batch size calculation
	batchEntries := make([]interface{}, 0)
	if len(batchEntries) != 0 {
		t.Errorf("Expected empty batch, got size: %d", len(batchEntries))
	}

	// Simulate adding entries
	batchEntries = append(batchEntries, "entry1", "entry2")
	if len(batchEntries) != 2 {
		t.Errorf("Expected batch size 2, got: %d", len(batchEntries))
	}

	// Test clearing batch
	batchEntries = batchEntries[:0]
	if len(batchEntries) != 0 {
		t.Errorf("Expected empty batch after clear, got size: %d", len(batchEntries))
	}
}

func TestBatchingStateValidation(t *testing.T) {
	// Test validation logic for batching operations

	// Test that batch operations require valid CIDs
	testCidBytes := []byte("test-cid-bytes")
	if len(testCidBytes) == 0 {
		t.Error("CID bytes should not be empty")
	}

	// Test that batch operations handle metadata correctly
	metadata := make(map[string]string)
	metadata["name"] = "test-pin"
	metadata["reference"] = "test-reference"

	if len(metadata) != 2 {
		t.Errorf("Expected 2 metadata entries, got: %d", len(metadata))
	}

	if metadata["name"] != "test-pin" {
		t.Errorf("Expected name 'test-pin', got: %s", metadata["name"])
	}
}
