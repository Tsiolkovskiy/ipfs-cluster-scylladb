package scyllastate

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/state"
)

// Add implements state.WriteOnly interface - adds or updates a pin
func (s *ScyllaState) Add(ctx context.Context, pin api.Pin) error {
	if s.isClosed() {
		return fmt.Errorf("ScyllaState is closed")
	}

	start := time.Now()

	// Create operation context for structured logging
	opCtx := NewOperationContext("Add").
		WithCID(pin.Cid).
		WithConsistency(s.config.Consistency).
		WithQueryType("INSERT")

	// Check if pin already exists for metrics
	exists, _ := s.Has(ctx, pin.Cid)

	// Execute the add operation with graceful degradation
	err := s.executeAddOperation(ctx, pin)

	// Record operation metrics and log
	duration := time.Since(start)
	opCtx = opCtx.WithDuration(duration)

	if err != nil {
		opCtx = opCtx.WithError(err)
		s.recordConnectionError()
	} else {
		// Update pin count if this is a new pin
		if !exists {
			s.recordPinOperation("add")
		} else {
			s.recordPinOperation("update")
		}
	}

	s.structuredLogger.LogOperation(ctx, opCtx)
	s.recordOperation("Add", duration, err)

	return err
}

// executeAddOperation performs the actual add operation with retry logic
func (s *ScyllaState) executeAddOperation(ctx context.Context, pin api.Pin) error {
	// Serialize pin data
	pinData, err := s.serializePin(pin)
	if err != nil {
		return fmt.Errorf("failed to serialize pin: %w", err)
	}

	// Calculate multihash prefix for partitioning
	cidBytes := pin.Cid.Bytes()
	mhPrefix := mhPrefix(cidBytes)

	// Prepare metadata for logging
	metadata := map[string]interface{}{
		"pin_type":           pin.Type.String(),
		"replication_factor": pin.ReplicationFactorMin,
		"mh_prefix":          mhPrefix,
	}

	if len(pin.Allocations) > 0 {
		metadata["allocations_count"] = len(pin.Allocations)
	}

	// Execute with graceful degradation
	return s.executeWithGracefulDegradation(ctx, func(consistency gocql.Consistency) error {
		query := s.prepared.insertPin.WithContext(ctx)
		query.Consistency(consistency)

		// Log query execution
		queryStart := time.Now()
		err := query.Bind(
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
		).Exec()

		queryDuration := time.Since(queryStart)
		s.structuredLogger.LogQuery(ctx, s.prepared.insertPin.String(), nil, queryDuration, err)

		return err
	})
}

// Get implements state.ReadOnly interface - retrieves a pin by CID
func (s *ScyllaState) Get(ctx context.Context, cid api.Cid) (api.Pin, error) {
	if s.isClosed() {
		return api.Pin{}, fmt.Errorf("ScyllaState is closed")
	}

	start := time.Now()

	// Create operation context for structured logging
	opCtx := NewOperationContext("Get").
		WithCID(cid).
		WithConsistency(s.config.Consistency).
		WithQueryType("SELECT")

	// Execute the get operation
	pin, err := s.executeGetOperation(ctx, cid)

	// Record operation metrics and log
	duration := time.Since(start)
	opCtx = opCtx.WithDuration(duration)

	if err != nil {
		opCtx = opCtx.WithError(err)
		if !IsNotFound(err) {
			s.recordConnectionError()
		}
	}

	s.structuredLogger.LogOperation(ctx, opCtx)
	s.recordOperation("Get", duration, err)

	return pin, err
}

// executeGetOperation performs the actual get operation
func (s *ScyllaState) executeGetOperation(ctx context.Context, cid api.Cid) (api.Pin, error) {
	cidBytes := cid.Bytes()
	mhPrefix := mhPrefix(cidBytes)

	var pinType int
	var rf int
	var owner []string
	var metadata map[string]string
	var ttl *time.Time
	var pinData []byte
	var createdAt, updatedAt time.Time

	// Execute with graceful degradation
	err := s.executeWithGracefulDegradation(ctx, func(consistency gocql.Consistency) error {
		query := s.prepared.selectPin.WithContext(ctx)
		query.Consistency(consistency)

		queryStart := time.Now()
		err := query.Bind(mhPrefix, cidBytes).Scan(
			&pinType, &rf, &owner, &metadata, &ttl, &pinData, &createdAt, &updatedAt,
		)

		queryDuration := time.Since(queryStart)
		s.structuredLogger.LogQuery(ctx, s.prepared.selectPin.String(), nil, queryDuration, err)

		return err
	})

	if err != nil {
		if err == gocql.ErrNotFound {
			return api.Pin{}, state.ErrNotFound
		}
		return api.Pin{}, fmt.Errorf("failed to get pin: %w", err)
	}

	// Deserialize pin data
	pin, err := s.deserializePin(cid, pinData)
	if err != nil {
		return api.Pin{}, fmt.Errorf("failed to deserialize pin: %w", err)
	}

	return pin, nil
}

// Has implements state.ReadOnly interface - checks if a pin exists
func (s *ScyllaState) Has(ctx context.Context, cid api.Cid) (bool, error) {
	if s.isClosed() {
		return false, fmt.Errorf("ScyllaState is closed")
	}

	start := time.Now()

	// Create operation context for structured logging
	opCtx := NewOperationContext("Has").
		WithCID(cid).
		WithConsistency(s.config.Consistency).
		WithQueryType("SELECT")

	// Execute the has operation
	exists, err := s.executeHasOperation(ctx, cid)

	// Record operation metrics and log
	duration := time.Since(start)
	opCtx = opCtx.WithDuration(duration).
		WithMetadata("exists", exists)

	if err != nil {
		opCtx = opCtx.WithError(err)
		s.recordConnectionError()
	}

	s.structuredLogger.LogOperation(ctx, opCtx)
	s.recordOperation("Has", duration, err)

	return exists, err
}

// executeHasOperation performs the actual has operation
func (s *ScyllaState) executeHasOperation(ctx context.Context, cid api.Cid) (bool, error) {
	cidBytes := cid.Bytes()
	mhPrefix := mhPrefix(cidBytes)

	var foundCid []byte

	// Execute with graceful degradation
	err := s.executeWithGracefulDegradation(ctx, func(consistency gocql.Consistency) error {
		query := s.prepared.checkExists.WithContext(ctx)
		query.Consistency(consistency)

		queryStart := time.Now()
		err := query.Bind(mhPrefix, cidBytes).Scan(&foundCid)

		queryDuration := time.Since(queryStart)
		s.structuredLogger.LogQuery(ctx, s.prepared.checkExists.String(), nil, queryDuration, err)

		return err
	})

	if err != nil {
		if err == gocql.ErrNotFound {
			return false, nil
		}
		return false, fmt.Errorf("failed to check pin existence: %w", err)
	}

	return true, nil
}

// Rm implements state.WriteOnly interface - removes a pin
func (s *ScyllaState) Rm(ctx context.Context, cid api.Cid) error {
	if s.isClosed() {
		return fmt.Errorf("ScyllaState is closed")
	}

	start := time.Now()

	// Create operation context for structured logging
	opCtx := NewOperationContext("Rm").
		WithCID(cid).
		WithConsistency(s.config.Consistency).
		WithQueryType("DELETE")

	// Check if pin exists before removal for metrics
	exists, _ := s.Has(ctx, cid)

	// Execute the remove operation
	err := s.executeRemoveOperation(ctx, cid)

	// Record operation metrics and log
	duration := time.Since(start)
	opCtx = opCtx.WithDuration(duration).
		WithMetadata("existed", exists)

	if err != nil {
		opCtx = opCtx.WithError(err)
		s.recordConnectionError()
	} else if exists {
		s.recordPinOperation("remove")
	}

	s.structuredLogger.LogOperation(ctx, opCtx)
	s.recordOperation("Rm", duration, err)

	return err
}

// executeRemoveOperation performs the actual remove operation
func (s *ScyllaState) executeRemoveOperation(ctx context.Context, cid api.Cid) error {
	cidBytes := cid.Bytes()
	mhPrefix := mhPrefix(cidBytes)

	// Execute with graceful degradation
	return s.executeWithGracefulDegradation(ctx, func(consistency gocql.Consistency) error {
		query := s.prepared.deletePin.WithContext(ctx)
		query.Consistency(consistency)

		queryStart := time.Now()
		err := query.Bind(mhPrefix, cidBytes).Exec()

		queryDuration := time.Since(queryStart)
		s.structuredLogger.LogQuery(ctx, s.prepared.deletePin.String(), nil, queryDuration, err)

		return err
	})
}

// List implements state.ReadOnly interface - lists all pins
func (s *ScyllaState) List(ctx context.Context, out chan<- api.Pin) error {
	defer close(out)

	if s.isClosed() {
		return fmt.Errorf("ScyllaState is closed")
	}

	start := time.Now()

	// Create operation context for structured logging
	opCtx := NewOperationContext("List").
		WithConsistency(s.config.Consistency).
		WithQueryType("SELECT")

	// Execute the list operation
	count, err := s.executeListOperation(ctx, out)

	// Record operation metrics and log
	duration := time.Since(start)
	opCtx = opCtx.WithDuration(duration).
		WithRecordCount(count).
		WithMetadata("page_size", s.config.PageSize)

	if err != nil {
		opCtx = opCtx.WithError(err)
		s.recordConnectionError()
	}

	s.structuredLogger.LogOperation(ctx, opCtx)
	s.recordOperation("List", duration, err)

	return err
}

// executeListOperation performs the actual list operation
func (s *ScyllaState) executeListOperation(ctx context.Context, out chan<- api.Pin) (int64, error) {
	var count int64

	// Execute with graceful degradation
	err := s.iterQueryWithDegradation(ctx, s.prepared.listAllPins, func(iter *gocql.Iter) error {
		var mhPrefix int16
		var cidBin []byte
		var pinType int
		var rf int
		var owner []string
		var metadata map[string]string
		var ttl *time.Time
		var pinData []byte
		var createdAt, updatedAt time.Time

		for iter.Scan(&mhPrefix, &cidBin, &pinType, &rf, &owner, &metadata, &ttl, &pinData, &createdAt, &updatedAt) {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Convert binary CID back to api.Cid
			cid, err := api.CastCid(cidBin)
			if err != nil {
				logger.Warnf("Invalid CID in database, skipping: %v", err)
				continue
			}

			// Deserialize pin
			pin, err := s.deserializePin(cid, pinData)
			if err != nil {
				logger.Warnf("Failed to deserialize pin %s, skipping: %v", cid.String(), err)
				continue
			}

			// Send pin to output channel
			select {
			case out <- pin:
				count++

				// Log progress for large datasets
				if count > 0 && count%100000 == 0 {
					s.structuredLogger.LogPerformanceMetrics(map[string]interface{}{
						"operation":     "List",
						"pins_streamed": count,
						"duration_ms":   time.Since(time.Now()).Milliseconds(),
					})
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		return nil
	})

	return count, err
}

// Note: serializePin and deserializePin methods are implemented in scyllastate.go
// with versioning support for better compatibility and future extensibility

// mhPrefix extracts the multihash prefix for partitioning
func mhPrefix(cidBin []byte) int16 {
	if len(cidBin) < 2 {
		return 0
	}
	return int16(cidBin[0])<<8 | int16(cidBin[1])
}
