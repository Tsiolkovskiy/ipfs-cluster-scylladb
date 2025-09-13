package scyllastate

import (
	"context"
	"time"
)

// PinStore defines the interface for ScyllaDB-based pin storage
// This is the core interface for managing pin metadata at scale
type PinStore interface {
	// UpsertPin creates or updates a pin specification (idempotent)
	// opID ensures idempotency - same opID will not create duplicate entries
	UpsertPin(ctx context.Context, opID string, spec PinSpec) error

	// SetDesiredPlacement sets the desired peer placement for a CID
	// This is used by the scheduler to assign pins to specific peers
	SetDesiredPlacement(ctx context.Context, cidBin []byte, desired []string) error

	// AckPlacement acknowledges a placement state change from a peer
	// This updates both peer-specific state and global actual placement
	AckPlacement(ctx context.Context, cidBin []byte, peerID string, newState PinState) error

	// GetPin retrieves pin specification by CID
	GetPin(ctx context.Context, cidBin []byte) (*PinSpec, error)

	// GetPlacement retrieves current placement state for a CID
	GetPlacement(ctx context.Context, cidBin []byte) (*PinPlacement, error)

	// ListPinsByPeer returns pins assigned to a specific peer
	// Used by workers to get their assigned work
	ListPinsByPeer(ctx context.Context, peerID string, limit int) ([][]byte, error)

	// GetRF returns the required replication factor for a CID
	GetRF(ctx context.Context, cidBin []byte) (uint8, error)

	// ListExpiredPins returns pins that have exceeded their TTL
	// Used by TTL cleanup workers
	ListExpiredPins(ctx context.Context, bucket time.Time, limit int) ([]*TTLEntry, error)

	// RemovePin removes a pin and all associated metadata
	RemovePin(ctx context.Context, cidBin []byte) error

	// GetStats returns basic statistics about the pin store
	GetStats(ctx context.Context) (*StoreStats, error)
}

// StoreStats provides basic statistics about the pin store
type StoreStats struct {
	TotalPins   int64 // Total number of pins
	PinnedPins  int64 // Successfully pinned pins
	PendingPins int64 // Pins in queued/pinning state
	FailedPins  int64 // Pins in failed state
	ExpiredPins int64 // Pins past their TTL
	TotalPeers  int64 // Number of active peers
}

// BatchPinStore extends PinStore with batching capabilities
// for high-throughput scenarios
type BatchPinStore interface {
	PinStore

	// BeginBatch starts a new batch operation
	BeginBatch(ctx context.Context) (Batch, error)
}

// Batch represents a batch of operations that can be committed atomically
type Batch interface {
	// UpsertPin adds a pin upsert to the batch
	UpsertPin(opID string, spec PinSpec) error

	// SetDesiredPlacement adds a placement update to the batch
	SetDesiredPlacement(cidBin []byte, desired []string) error

	// AckPlacement adds a placement acknowledgment to the batch
	AckPlacement(cidBin []byte, peerID string, newState PinState) error

	// Commit executes all batched operations
	Commit(ctx context.Context) error

	// Rollback discards all batched operations
	Rollback() error
}
