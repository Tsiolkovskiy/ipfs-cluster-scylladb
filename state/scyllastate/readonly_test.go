package scyllastate

import (
	"context"
	"testing"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/state"
)

// TestReadOnlyInterface verifies that ScyllaState implements state.ReadOnly interface
func TestReadOnlyInterface(t *testing.T) {
	// This test verifies that ScyllaState implements the ReadOnly interface
	var _ state.ReadOnly = (*ScyllaState)(nil)
}

// TestReadOnlyMethods tests the ReadOnly methods with mock data
func TestReadOnlyMethods(t *testing.T) {
	// Create a mock ScyllaState for testing
	s := &ScyllaState{
		closed: 0, // not closed
	}

	ctx := context.Background()

	// Test that methods exist and have correct signatures
	t.Run("Get method signature", func(t *testing.T) {
		// This will fail if the method signature is wrong
		_, err := s.Get(ctx, api.CidUndef)
		// We expect an error since we don't have a real connection
		if err == nil {
			t.Error("Expected error from Get with mock state")
		}
	})

	t.Run("Has method signature", func(t *testing.T) {
		// This will fail if the method signature is wrong
		_, err := s.Has(ctx, api.CidUndef)
		// We expect an error since we don't have a real connection
		if err == nil {
			t.Error("Expected error from Has with mock state")
		}
	})

	t.Run("List method signature", func(t *testing.T) {
		// This will fail if the method signature is wrong
		out := make(chan api.Pin, 1)
		err := s.List(ctx, out)
		// We expect an error since we don't have a real connection
		if err == nil {
			t.Error("Expected error from List with mock state")
		}
		// Drain the channel
		select {
		case <-out:
		default:
		}
	})
}

// TestReadOnlyMethodsClosedState tests that methods return appropriate errors when state is closed
func TestReadOnlyMethodsClosedState(t *testing.T) {
	s := &ScyllaState{
		closed: 1, // closed
	}

	ctx := context.Background()

	t.Run("Get returns error when closed", func(t *testing.T) {
		_, err := s.Get(ctx, api.CidUndef)
		if err == nil {
			t.Error("Expected error from Get when state is closed")
		}
		if err.Error() != "ScyllaState is closed" {
			t.Errorf("Expected 'ScyllaState is closed' error, got: %v", err)
		}
	})

	t.Run("Has returns error when closed", func(t *testing.T) {
		_, err := s.Has(ctx, api.CidUndef)
		if err == nil {
			t.Error("Expected error from Has when state is closed")
		}
		if err.Error() != "ScyllaState is closed" {
			t.Errorf("Expected 'ScyllaState is closed' error, got: %v", err)
		}
	})

	t.Run("List returns error when closed", func(t *testing.T) {
		out := make(chan api.Pin, 1)
		err := s.List(ctx, out)
		if err == nil {
			t.Error("Expected error from List when state is closed")
		}
		if err.Error() != "ScyllaState is closed" {
			t.Errorf("Expected 'ScyllaState is closed' error, got: %v", err)
		}
		// Drain the channel
		select {
		case <-out:
		default:
		}
	})
}
