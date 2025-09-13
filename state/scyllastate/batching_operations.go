package scyllastate

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/gocql/gocql"
	"github.com/ipfs-cluster/ipfs-cluster/api"
)

// NewBatching creates a new batching state for efficient bulk operations
func (s *ScyllaState) NewBatching(ctx context.Context) (*ScyllaBatchingState, error) {
	if s.isClosed() {
		return nil, fmt.Errorf("ScyllaState is closed")
	}

	start := time.Now()

	// Create operation context for structured logging
	opCtx := NewOperationContext("NewBatching").
		WithConsistency(s.config.Consistency).
		WithQueryType("BATCH")

	// Create new batch
	batch := s.session.NewBatch(gocql.LoggedBatch)
	batch.Consistency(s.config.GetConsistency())

	batchingState := &ScyllaBatchingState{
		ScyllaState: s,
		batch:       batch,
	}

	// Log batch creation
	duration := time.Since(start)
	opCtx = opCtx.WithDuration(duration)
	s.structuredLogger.LogOperation(ctx, opCtx)

	return batchingState, nil
}

// Add adds a pin to the batch (implements state.WriteOnly)
func (bs *ScyllaBatchingState) Add(ctx context.Context, pin api.Pin) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.batch == nil {
		return fmt.Errorf("batch is nil or already committed")
	}

	start := time.Now()

	// Create operation context for structured logging
	opCtx := NewOperationContext("BatchAdd").
		WithCID(pin.Cid).
		WithConsistency(bs.config.Consistency).
		WithQueryType("BATCH_INSERT")

	// Serialize pin data
	pinData, err := bs.serializePin(pin)
	if err != nil {
		opCtx = opCtx.WithError(err).WithDuration(time.Since(start))
		bs.structuredLogger.LogOperation(ctx, opCtx)
		return fmt.Errorf("failed to serialize pin: %w", err)
	}

	// Calculate multihash prefix for partitioning
	cidBytes := pin.Cid.Bytes()
	mhPrefix := mhPrefix(cidBytes)

	// Add to batch
	bs.batch.Query(
		`INSERT INTO pins_by_cid (mh_prefix, cid_bin, pin_type, rf, owner, tags, ttl, metadata, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		mhPrefix,
		cidBytes,
		int(pin.Type),
		pin.ReplicationFactorMin,
		pin.UserAllocations, // owner field
		pin.Metadata,        // tags as metadata
		nil,                 // ttl - not implemented yet
		pinData,             // serialized metadata
		time.Now(),          // created_at
		time.Now(),          // updated_at
	)

	// Log batch add operation
	duration := time.Since(start)
	opCtx = opCtx.WithDuration(duration).
		WithBatchSize(len(bs.batch.Entries)).
		WithMetadata("mh_prefix", mhPrefix)

	bs.structuredLogger.LogOperation(ctx, opCtx)

	return nil
}

// Rm removes a pin from the batch (implements state.WriteOnly)
func (bs *ScyllaBatchingState) Rm(ctx context.Context, cid api.Cid) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.batch == nil {
		return fmt.Errorf("batch is nil or already committed")
	}

	start := time.Now()

	// Create operation context for structured logging
	opCtx := NewOperationContext("BatchRm").
		WithCID(cid).
		WithConsistency(bs.config.Consistency).
		WithQueryType("BATCH_DELETE")

	// Calculate multihash prefix for partitioning
	cidBytes := cid.Bytes()
	mhPrefix := mhPrefix(cidBytes)

	// Add delete to batch
	bs.batch.Query(
		`DELETE FROM pins_by_cid WHERE mh_prefix = ? AND cid_bin = ?`,
		mhPrefix,
		cidBytes,
	)

	// Log batch remove operation
	duration := time.Since(start)
	opCtx = opCtx.WithDuration(duration).
		WithBatchSize(len(bs.batch.Entries)).
		WithMetadata("mh_prefix", mhPrefix)

	bs.structuredLogger.LogOperation(ctx, opCtx)

	return nil
}

// Commit executes the batch and commits all operations (implements state.BatchingState)
func (bs *ScyllaBatchingState) Commit(ctx context.Context) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.batch == nil {
		return fmt.Errorf("batch is nil or already committed")
	}

	start := time.Now()
	batchSize := len(bs.batch.Entries)

	// Create operation context for structured logging
	opCtx := NewOperationContext("BatchCommit").
		WithConsistency(bs.config.Consistency).
		WithQueryType("BATCH_COMMIT").
		WithBatchSize(batchSize)

	// Check if batch is empty
	if batchSize == 0 {
		duration := time.Since(start)
		opCtx = opCtx.WithDuration(duration).
			WithMetadata("empty_batch", true)
		bs.structuredLogger.LogOperation(ctx, opCtx)
		return nil
	}

	// Execute batch with graceful degradation
	err := bs.executeWithGracefulDegradation(ctx, func(consistency gocql.Consistency) error {
		bs.batch.Consistency(consistency)

		queryStart := time.Now()
		err := bs.session.ExecuteBatch(bs.batch.WithContext(ctx))

		queryDuration := time.Since(queryStart)
		bs.structuredLogger.LogBatchOperation("LOGGED", batchSize, queryDuration, err)

		return err
	})

	// Record operation metrics and log
	duration := time.Since(start)
	opCtx = opCtx.WithDuration(duration)

	if err != nil {
		opCtx = opCtx.WithError(err)
		bs.recordConnectionError()
	} else {
		// Update metrics for successful batch
		bs.recordBatchOperation("LOGGED", batchSize, err)

		// Update pin count (approximate - we don't know exact add/remove ratio)
		atomic.AddInt64(&bs.totalPins, int64(batchSize/2)) // rough estimate
	}

	bs.structuredLogger.LogOperation(ctx, opCtx)
	bs.recordOperation("BatchCommit", duration, err)

	// Clear the batch after commit attempt
	bs.batch = nil

	return err
}

// Get delegates to the underlying ScyllaState (implements state.ReadOnly)
func (bs *ScyllaBatchingState) Get(ctx context.Context, cid api.Cid) (api.Pin, error) {
	return bs.ScyllaState.Get(ctx, cid)
}

// Has delegates to the underlying ScyllaState (implements state.ReadOnly)
func (bs *ScyllaBatchingState) Has(ctx context.Context, cid api.Cid) (bool, error) {
	return bs.ScyllaState.Has(ctx, cid)
}

// List delegates to the underlying ScyllaState (implements state.ReadOnly)
func (bs *ScyllaBatchingState) List(ctx context.Context, out chan<- api.Pin) error {
	return bs.ScyllaState.List(ctx, out)
}

// Marshal delegates to the underlying ScyllaState
func (bs *ScyllaBatchingState) Marshal(w io.Writer) error {
	// Commit any pending operations before marshaling
	if bs.GetBatchSize() > 0 {
		if err := bs.Commit(context.Background()); err != nil {
			return fmt.Errorf("failed to commit pending batch before marshal: %w", err)
		}
	}

	// Delegate to the underlying state
	return bs.ScyllaState.Marshal(w)
}

// Unmarshal delegates to the underlying ScyllaState
func (bs *ScyllaBatchingState) Unmarshal(r io.Reader) error {
	// Clear any existing batch before unmarshaling
	bs.mu.Lock()
	bs.batch = nil
	bs.mu.Unlock()

	// Delegate to the underlying state
	return bs.ScyllaState.Unmarshal(r)
}

// Migrate delegates to the underlying ScyllaState
func (bs *ScyllaBatchingState) Migrate(ctx context.Context, r io.Reader) error {
	// Clear any existing batch before migrating
	bs.mu.Lock()
	bs.batch = nil
	bs.mu.Unlock()

	// Delegate to the underlying state
	return bs.ScyllaState.Migrate(ctx, r)
}

// Close cleans up the batching state resources
func (bs *ScyllaBatchingState) Close() error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Clear the batch without committing
	bs.batch = nil

	// Log the close operation
	opCtx := NewOperationContext("BatchClose")
	bs.structuredLogger.LogOperation(context.Background(), opCtx)

	return nil
}

// GetBatchSize returns the current number of operations in the batch
func (bs *ScyllaBatchingState) GetBatchSize() int {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.batch == nil {
		return 0
	}

	return len(bs.batch.Entries)
}

// IsBatchFull checks if the batch has reached the configured maximum size
func (bs *ScyllaBatchingState) IsBatchFull() bool {
	return bs.GetBatchSize() >= bs.config.BatchSize
}

// ShouldAutoCommit checks if the batch should be automatically committed
// based on size or timeout
func (bs *ScyllaBatchingState) ShouldAutoCommit(batchStartTime time.Time) bool {
	return bs.IsBatchFull() || time.Since(batchStartTime) >= bs.config.BatchTimeout
}
