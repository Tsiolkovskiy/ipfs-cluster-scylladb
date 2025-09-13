package scyllastate

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gocql/gocql"
	"github.com/ipfs-cluster/ipfs-cluster/api"
)

// NewBatching creates a new ScyllaBatchingState from an existing ScyllaState.
// This allows for batching multiple pin operations together for better performance.
// The batch uses LoggedBatch type for consistency and atomicity.
func (s *ScyllaState) NewBatching() (*ScyllaBatchingState, error) {
	if s.isClosed() {
		return nil, fmt.Errorf("ScyllaState is closed")
	}

	batch := s.session.NewBatch(gocql.LoggedBatch)
	batch.SetConsistency(s.config.GetConsistency())

	return &ScyllaBatchingState{
		ScyllaState: s,
		batch:       batch,
	}, nil
}

// Add implements state.WriteOnly.Add for batched operations
func (bs *ScyllaBatchingState) Add(ctx context.Context, pin api.Pin) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.batch == nil {
		return fmt.Errorf("batch is not initialized")
	}

	// Convert api.Pin to internal format
	cidBin := pin.Cid.Cid.Bytes()
	prefix, _ := cidToPartitionedKey(cidBin)

	now := time.Now()
	var ttlPtr *int64
	if pin.ExpireAt.After(time.Time{}) {
		ttl := pin.ExpireAt.UnixMilli()
		ttlPtr = &ttl
	}

	// Convert origins to string slice for tags
	tags := make([]string, 0, len(pin.Origins))
	for _, origin := range pin.Origins {
		tags = append(tags, origin.String())
	}

	// Convert metadata
	metadata := make(map[string]string)
	if pin.Name != "" {
		metadata["name"] = pin.Name
	}
	if pin.Reference != nil {
		metadata["reference"] = pin.Reference.String()
	}

	// Add to batch
	bs.batch.Query(
		`INSERT INTO pins_by_cid (mh_prefix, cid_bin, pin_type, rf, owner, tags, ttl, metadata, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		prefix,
		cidBin,
		uint8(pin.Type),
		uint8(pin.ReplicationFactorMin),
		"", // owner - will be set by higher level components
		tags,
		ttlPtr,
		metadata,
		now,
		now,
	)

	bs.ScyllaState.logger.Printf("Added pin to batch: cid=%s, batch_size=%d",
		pin.Cid.String(), len(bs.batch.Entries))

	return nil
}

// Rm implements state.WriteOnly.Rm for batched operations
func (bs *ScyllaBatchingState) Rm(ctx context.Context, cid api.Cid) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.batch == nil {
		return fmt.Errorf("batch is not initialized")
	}

	cidBin := cid.Cid.Bytes()
	prefix, _ := cidToPartitionedKey(cidBin)

	// Add deletion to batch
	bs.batch.Query(
		`DELETE FROM pins_by_cid WHERE mh_prefix = ? AND cid_bin = ?`,
		prefix,
		cidBin,
	)

	// Also delete from placements
	bs.batch.Query(
		`DELETE FROM placements_by_cid WHERE mh_prefix = ? AND cid_bin = ?`,
		prefix,
		cidBin,
	)

	bs.ScyllaState.logger.Printf("Added pin removal to batch: cid=%s, batch_size=%d",
		cid.String(), len(bs.batch.Entries))

	return nil
}

// Commit implements state.BatchingState.Commit.
// Executes all batched operations atomically and resets the batch for reuse.
// This provides efficient bulk operations for high-throughput scenarios.
func (bs *ScyllaBatchingState) Commit(ctx context.Context) error {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		bs.ScyllaState.recordOperation("batch_commit", duration, nil)
	}()

	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.batch == nil {
		return fmt.Errorf("batch is not initialized")
	}

	if len(bs.batch.Entries) == 0 {
		bs.ScyllaState.logger.Printf("No operations in batch, skipping commit")
		return nil
	}

	batchSize := len(bs.batch.Entries)
	bs.ScyllaState.logger.Printf("Committing batch: batch_size=%d", batchSize)

	// Execute batch with context
	if err := bs.ScyllaState.session.ExecuteBatch(bs.batch); err != nil {
		bs.ScyllaState.recordOperation("batch_commit", time.Since(start), err)
		bs.ScyllaState.logger.Printf("Failed to commit batch: batch_size=%d, error=%v", batchSize, err)
		return fmt.Errorf("failed to commit batch: %w", err)
	}

	// Reset batch for reuse
	bs.batch = bs.ScyllaState.session.NewBatch(gocql.LoggedBatch)
	bs.batch.SetConsistency(bs.ScyllaState.config.GetConsistency())

	bs.ScyllaState.logger.Printf("Batch committed successfully: batch_size=%d, duration=%v",
		batchSize, time.Since(start))

	return nil
}

// BatchSize returns the current number of operations in the batch
func (bs *ScyllaBatchingState) BatchSize() int {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.batch == nil {
		return 0
	}

	return len(bs.batch.Entries)
}

// Clear clears all operations from the batch without committing
func (bs *ScyllaBatchingState) Clear() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.batch != nil {
		bs.batch = bs.ScyllaState.session.NewBatch(gocql.LoggedBatch)
		bs.batch.SetConsistency(bs.ScyllaState.config.GetConsistency())
	}

	bs.ScyllaState.logger.Printf("Batch cleared")
}

// Close closes the batching state and releases resources
func (bs *ScyllaBatchingState) Close() error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Commit any pending operations
	if bs.batch != nil && len(bs.batch.Entries) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := bs.Commit(ctx); err != nil {
			bs.ScyllaState.logger.Printf("Failed to commit batch during close: %v", err)
		}
	}

	bs.batch = nil
	return nil
}

// AutoCommitBatcher provides automatic batching with size and time-based commits.
// This implements efficient batching logic that groups operations into optimal
// batches based on configurable size and time thresholds (Requirement 7.2).
type AutoCommitBatcher struct {
	*ScyllaBatchingState
	maxBatchSize int
	maxBatchTime time.Duration
	lastCommit   time.Time
	mu           sync.Mutex
}

// NewAutoCommitBatcher creates a new auto-committing batcher
func (s *ScyllaState) NewAutoCommitBatcher(maxBatchSize int, maxBatchTime time.Duration) (*AutoCommitBatcher, error) {
	batchingState, err := s.NewBatching()
	if err != nil {
		return nil, err
	}

	return &AutoCommitBatcher{
		ScyllaBatchingState: batchingState,
		maxBatchSize:        maxBatchSize,
		maxBatchTime:        maxBatchTime,
		lastCommit:          time.Now(),
	}, nil
}

// Add implements state.WriteOnly.Add with auto-commit logic
func (acb *AutoCommitBatcher) Add(ctx context.Context, pin api.Pin) error {
	acb.mu.Lock()
	defer acb.mu.Unlock()

	// Add to batch
	if err := acb.ScyllaBatchingState.Add(ctx, pin); err != nil {
		return err
	}

	// Check if we should auto-commit
	return acb.checkAutoCommit(ctx)
}

// Rm implements state.WriteOnly.Rm with auto-commit logic
func (acb *AutoCommitBatcher) Rm(ctx context.Context, cid api.Cid) error {
	acb.mu.Lock()
	defer acb.mu.Unlock()

	// Add to batch
	if err := acb.ScyllaBatchingState.Rm(ctx, cid); err != nil {
		return err
	}

	// Check if we should auto-commit
	return acb.checkAutoCommit(ctx)
}

// checkAutoCommit checks if the batch should be auto-committed based on size or time
func (acb *AutoCommitBatcher) checkAutoCommit(ctx context.Context) error {
	batchSize := acb.ScyllaBatchingState.BatchSize()
	timeSinceLastCommit := time.Since(acb.lastCommit)

	shouldCommit := false

	// Check size threshold
	if batchSize >= acb.maxBatchSize {
		shouldCommit = true
		acb.ScyllaState.logger.Printf("Auto-commit triggered by batch size: batch_size=%d, max_batch_size=%d",
			batchSize, acb.maxBatchSize)
	}

	// Check time threshold
	if timeSinceLastCommit >= acb.maxBatchTime {
		shouldCommit = true
		acb.ScyllaState.logger.Printf("Auto-commit triggered by time: time_since_last_commit=%v, max_batch_time=%v",
			timeSinceLastCommit, acb.maxBatchTime)
	}

	if shouldCommit && batchSize > 0 {
		if err := acb.ScyllaBatchingState.Commit(ctx); err != nil {
			return err
		}
		acb.lastCommit = time.Now()
	}

	return nil
}

// ForceCommit forces a commit regardless of batch size or time
func (acb *AutoCommitBatcher) ForceCommit(ctx context.Context) error {
	acb.mu.Lock()
	defer acb.mu.Unlock()

	if err := acb.ScyllaBatchingState.Commit(ctx); err != nil {
		return err
	}
	acb.lastCommit = time.Now()
	return nil
}

// Close closes the auto-commit batcher
func (acb *AutoCommitBatcher) Close() error {
	acb.mu.Lock()
	defer acb.mu.Unlock()

	return acb.ScyllaBatchingState.Close()
}
