package scyllastate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/gocql/gocql"
	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/state"
)

// Add implements state.WriteOnly.Add
func (s *ScyllaState) Add(ctx context.Context, pin api.Pin) error {
	start := time.Now()
	var err error
	defer func() {
		duration := time.Since(start)
		s.recordOperation("add", duration, err)
		s.recordPinOperation("pin")
		s.logOperation(ctx, "add", pin.Cid, duration, err)

		// Update prepared statements cache metrics
		s.updatePreparedStatementsMetric()
	}()

	if s.isClosed() {
		err = fmt.Errorf("ScyllaState is closed")
		return err
	}

	// Convert api.Pin to internal format
	cidBin := pin.Cid.Cid.Bytes()
	prefix, _ := cidToPartitionedKey(cidBin)

	now := time.Now()
	var ttlPtr *int64
	if !pin.ExpireAt.IsZero() {
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

	// Check if pin already exists for conditional update
	existingPin, existsErr := s.Get(ctx, pin.Cid)
	isUpdate := existsErr == nil

	// Use conditional update to prevent race conditions (Requirement 2.3)
	var query *gocql.Query
	if isUpdate {
		// Update existing pin with conditional check on updated_at
		updateCQL := `UPDATE pins_by_cid SET pin_type = ?, rf = ?, owner = ?, tags = ?, ttl = ?, metadata = ?, updated_at = ?
			WHERE mh_prefix = ? AND cid_bin = ? IF updated_at = ?`
		query = s.session.Query(updateCQL).WithContext(ctx).Bind(
			uint8(pin.Type),
			uint8(pin.ReplicationFactorMin),
			"", // owner - will be set by higher level components
			tags,
			ttlPtr,
			metadata,
			now,
			prefix,
			cidBin,
			existingPin.Timestamp, // Conditional check on existing timestamp
		)
	} else {
		// Insert new pin with IF NOT EXISTS to prevent race conditions
		insertCQL := `INSERT INTO pins_by_cid (mh_prefix, cid_bin, pin_type, rf, owner, tags, ttl, metadata, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) IF NOT EXISTS`
		query = s.session.Query(insertCQL).WithContext(ctx).Bind(
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
	}
	defer query.Release()

	// Execute conditional operation
	if isUpdate {
		// For conditional UPDATE, check if it was applied
		var applied bool
		if err = query.Scan(&applied); err != nil {
			err = fmt.Errorf("failed to execute conditional update: %w", err)
			return err
		}
		if !applied {
			err = fmt.Errorf("pin update failed due to concurrent modification")
			return err
		}
	} else {
		// For conditional INSERT, check if it was applied
		var applied bool
		if err = query.Scan(&applied); err != nil {
			err = fmt.Errorf("failed to execute conditional insert: %w", err)
			return err
		}
		if !applied {
			err = fmt.Errorf("pin creation failed due to concurrent creation")
			return err
		}
	}

	// Initialize placement data (Requirement 2.1)
	if err = s.initializePlacement(ctx, pin.Cid, prefix, cidBin, now); err != nil {
		s.logger.Printf("Failed to initialize placement for CID %s: %v", pin.Cid.String(), err)
		// Don't fail the operation if placement initialization fails
	}

	// Add to TTL queue if TTL is set
	if ttlPtr != nil {
		if err = s.addToTTLQueue(ctx, pin.Cid, cidBin, *ttlPtr, ""); err != nil {
			s.logger.Printf("Failed to add CID %s to TTL queue: %v", pin.Cid.String(), err)
			// Don't fail the operation if TTL queue addition fails
		}
	}

	// Update pin count only for new pins
	if !isUpdate {
		s.incrementPinCount()
	}

	return nil
}

// Get implements state.ReadOnly.Get
func (s *ScyllaState) Get(ctx context.Context, cid api.Cid) (api.Pin, error) {
	start := time.Now()
	var err error
	defer func() {
		duration := time.Since(start)
		s.recordOperation("get", duration, err)
		s.recordQuery("SELECT", duration)
		s.logOperation(ctx, "get", cid, duration, err)
	}()

	if s.isClosed() {
		err = fmt.Errorf("ScyllaState is closed")
		return api.Pin{}, err
	}

	cidBin := cid.Cid.Bytes()
	prefix, _ := cidToPartitionedKey(cidBin)

	query := s.prepared.selectPin.WithContext(ctx).Bind(prefix, cidBin)
	defer query.Release()

	var pinType, rf uint8
	var owner string
	var tags []string
	var ttlPtr *int64
	var metadata map[string]string
	var createdAt, updatedAt time.Time

	if err = query.Scan(&pinType, &rf, &owner, &tags, &ttlPtr, &metadata, &createdAt, &updatedAt); err != nil {
		if err == gocql.ErrNotFound {
			err = state.ErrNotFound
			return api.Pin{}, err
		}
		err = fmt.Errorf("failed to get pin: %w", err)
		return api.Pin{}, err
	}

	// Convert back to api.Pin
	pin := api.Pin{
		Cid:       cid,
		Type:      api.PinType(pinType),
		MaxDepth:  -1,
		Timestamp: createdAt,
	}

	// Set PinOptions fields
	pin.ReplicationFactorMin = int(rf)
	pin.ReplicationFactorMax = int(rf)

	// Set expiration if TTL is set
	if ttlPtr != nil {
		pin.ExpireAt = time.UnixMilli(*ttlPtr)
	}

	// Convert tags back to origins
	for _, tag := range tags {
		if origin, err := api.NewMultiaddr(tag); err == nil {
			pin.Origins = append(pin.Origins, origin)
		}
	}

	// Set metadata
	if name, ok := metadata["name"]; ok {
		pin.Name = name
	}
	if ref, ok := metadata["reference"]; ok {
		if refCid, err := api.DecodeCid(ref); err == nil {
			pin.Reference = &refCid
		}
	}

	return pin, nil
}

// Has implements state.ReadOnly.Has
func (s *ScyllaState) Has(ctx context.Context, cid api.Cid) (bool, error) {
	start := time.Now()
	var err error
	defer func() {
		duration := time.Since(start)
		s.recordOperation("has", duration, err)
		s.logOperation(ctx, "has", cid, duration, err)
	}()

	if s.isClosed() {
		err = fmt.Errorf("ScyllaState is closed")
		return false, err
	}

	cidBin := cid.Cid.Bytes()
	prefix, _ := cidToPartitionedKey(cidBin)

	query := s.prepared.checkExists.WithContext(ctx).Bind(prefix, cidBin)
	defer query.Release()

	var existingCid []byte
	if err = query.Scan(&existingCid); err != nil {
		if err == gocql.ErrNotFound {
			return false, nil
		}
		err = fmt.Errorf("failed to check pin existence: %w", err)
		return false, err
	}

	return true, nil
}

// List implements state.ReadOnly.List
func (s *ScyllaState) List(ctx context.Context, out chan<- api.Pin) error {
	start := time.Now()
	var err error
	defer func() {
		duration := time.Since(start)
		s.recordOperation("list", duration, err)
		s.logOperation(ctx, "list", api.CidUndef, duration, err)
		close(out)
	}()

	if s.isClosed() {
		err = fmt.Errorf("ScyllaState is closed")
		return err
	}

	query := s.prepared.listAllPins.WithContext(ctx)
	defer query.Release()

	iter := query.Iter()
	defer iter.Close()

	var prefix int16
	var cidBin []byte
	var pinType, rf uint8
	var owner string
	var tags []string
	var ttlPtr *int64
	var metadata map[string]string
	var createdAt, updatedAt time.Time

	for iter.Scan(&prefix, &cidBin, &pinType, &rf, &owner, &tags, &ttlPtr, &metadata, &createdAt, &updatedAt) {
		// Convert CID
		cid, cidErr := api.CastCid(cidBin)
		if cidErr != nil {
			s.logger.Printf("Failed to decode CID during list: %v, cid_bin: %x", cidErr, cidBin)
			continue
		}

		// Convert to api.Pin
		pin := api.Pin{
			Cid:       cid,
			Type:      api.PinType(pinType),
			MaxDepth:  -1,
			Timestamp: createdAt,
		}

		// Set PinOptions fields
		pin.ReplicationFactorMin = int(rf)
		pin.ReplicationFactorMax = int(rf)

		// Set expiration if TTL is set
		if ttlPtr != nil {
			pin.ExpireAt = time.UnixMilli(*ttlPtr)
		}

		// Convert tags back to origins
		for _, tag := range tags {
			if origin, err := api.NewMultiaddr(tag); err == nil {
				pin.Origins = append(pin.Origins, origin)
			}
		}

		// Set metadata
		if name, ok := metadata["name"]; ok {
			pin.Name = name
		}
		if ref, ok := metadata["reference"]; ok {
			if refCid, err := api.DecodeCid(ref); err == nil {
				pin.Reference = &refCid
			}
		}

		select {
		case out <- pin:
		case <-ctx.Done():
			err = ctx.Err()
			return err
		}
	}

	if err = iter.Close(); err != nil {
		err = fmt.Errorf("failed to iterate pins: %w", err)
		return err
	}

	return nil
}

// Rm implements state.WriteOnly.Rm
func (s *ScyllaState) Rm(ctx context.Context, cid api.Cid) error {
	start := time.Now()
	var err error
	defer func() {
		duration := time.Since(start)
		s.recordOperation("rm", duration, err)
		s.logOperation(ctx, "rm", cid, duration, err)
	}()

	if s.isClosed() {
		err = fmt.Errorf("ScyllaState is closed")
		return err
	}

	cidBin := cid.Cid.Bytes()
	prefix, _ := cidToPartitionedKey(cidBin)

	// Get pin details before deletion for proper cleanup
	existingPin, err := s.Get(ctx, cid)
	if err != nil {
		if err == state.ErrNotFound {
			return err
		}
		err = fmt.Errorf("failed to get pin details for deletion: %w", err)
		return err
	}

	// Use conditional delete to prevent race conditions (Requirement 2.3)
	deleteCQL := `DELETE FROM pins_by_cid WHERE mh_prefix = ? AND cid_bin = ? IF updated_at = ?`
	query := s.session.Query(deleteCQL).WithContext(ctx).Bind(
		prefix,
		cidBin,
		existingPin.Timestamp,
	)
	defer query.Release()

	applied := false
	if err = query.Scan(&applied); err != nil {
		err = fmt.Errorf("failed to execute conditional delete: %w", err)
		return err
	}

	if !applied {
		err = fmt.Errorf("pin deletion failed due to concurrent modification")
		return err
	}

	// Proper cleanup of all related metadata (Requirement 2.4)

	// 1. Delete from placements_by_cid
	if err = s.cleanupPlacement(ctx, prefix, cidBin); err != nil {
		s.logger.Printf("Failed to cleanup placement for CID %s: %v", cid.String(), err)
		// Continue with other cleanup operations
	}

	// 2. Delete from pins_by_peer for all peers
	if err = s.cleanupPeerPins(ctx, prefix, cidBin); err != nil {
		s.logger.Printf("Failed to cleanup peer pins for CID %s: %v", cid.String(), err)
		// Continue with other cleanup operations
	}

	// 3. Remove from TTL queue if it has TTL
	if !existingPin.ExpireAt.IsZero() {
		if err = s.removeFromTTLQueue(ctx, cid, cidBin, existingPin.ExpireAt.UnixMilli(), ""); err != nil {
			s.logger.Printf("Failed to remove CID %s from TTL queue: %v", cid.String(), err)
			// Continue - this is not critical
		}
	}

	// Update pin count
	s.decrementPinCount()

	return nil
}

// Marshal implements state.State.Marshal - exports the entire state to a writer
func (s *ScyllaState) Marshal(w io.Writer) error {
	if s.isClosed() {
		return fmt.Errorf("ScyllaState is closed")
	}

	start := time.Now()
	ctx := context.Background()

	// Create operation context for structured logging
	opCtx := NewOperationContext("Marshal").
		WithConsistency(s.config.Consistency).
		WithQueryType("EXPORT")

	// Use JSON encoder for header and length-prefixed format for pins
	enc := json.NewEncoder(w)

	// Create a channel to receive pins
	pinChan := make(chan api.Pin, s.config.BatchSize)

	// Start listing pins in a goroutine
	var listErr error
	go func() {
		defer close(pinChan)
		if err := s.List(ctx, pinChan); err != nil {
			listErr = err
			s.logger.Printf("Failed to list pins during marshal: %v", err)
		}
	}()

	// Write state format version header
	stateHeader := StateExportHeader{
		Version:   CurrentStateVersion,
		Timestamp: time.Now(),
		Keyspace:  s.config.Keyspace,
	}

	if err := enc.Encode(stateHeader); err != nil {
		return fmt.Errorf("failed to write state header: %w", err)
	}

	// Write pins in length-prefixed format (compatible with existing implementations)
	count := 0
	for pin := range pinChan {
		// Serialize pin data using existing protobuf method
		pinData, err := s.serializePin(pin)
		if err != nil {
			return fmt.Errorf("failed to serialize pin %s: %w", pin.Cid.String(), err)
		}

		// Write length prefix and data (compatible with dsstate format)
		if _, err := fmt.Fprintf(w, "%d\n", len(pinData)); err != nil {
			return fmt.Errorf("failed to write pin length: %w", err)
		}

		if _, err := w.Write(pinData); err != nil {
			return fmt.Errorf("failed to write pin data: %w", err)
		}

		if _, err := w.Write([]byte("\n")); err != nil {
			return fmt.Errorf("failed to write newline: %w", err)
		}

		count++

		// Log progress for large exports
		if count > 0 && count%10000 == 0 {
			s.logger.Printf("Marshal progress: %d pins exported", count)
		}
	}

	// Check for list errors
	if listErr != nil {
		return fmt.Errorf("failed to list pins during marshal: %w", listErr)
	}

	// Record operation metrics and log
	duration := time.Since(start)
	opCtx = opCtx.WithDuration(duration).
		WithRecordCount(int64(count)).
		WithMetadata("export_format", "length_prefixed_protobuf")

	s.structuredLogger.LogOperation(ctx, opCtx)
	s.recordOperation("Marshal", duration, nil)

	s.logger.Printf("State marshaled successfully: pin_count=%d, duration=%v", count, duration)
	return nil
}

// Unmarshal implements state.State.Unmarshal - imports state from a reader
func (s *ScyllaState) Unmarshal(r io.Reader) error {
	if s.isClosed() {
		return fmt.Errorf("ScyllaState is closed")
	}

	start := time.Now()
	ctx := context.Background()

	// Create operation context for structured logging
	opCtx := NewOperationContext("Unmarshal").
		WithConsistency(s.config.Consistency).
		WithQueryType("IMPORT")

	// Read pins from length-prefixed format (compatible with dsstate and legacy formats)
	count := 0

	// Create batching state for efficient bulk import
	batchingState, err := s.NewBatching(ctx)
	if err != nil {
		return fmt.Errorf("failed to create batching state: %w", err)
	}
	defer batchingState.Close()

	batchCount := 0

	// Try to read header first (JSON format)
	dec := json.NewDecoder(r)
	var header StateExportHeader
	headerErr := dec.Decode(&header)

	if headerErr == nil {
		// Header found, validate version
		if header.Version > CurrentStateVersion {
			return fmt.Errorf("unsupported state version %d (current: %d)",
				header.Version, CurrentStateVersion)
		}
		s.logger.Printf("Unmarshaling state: version=%d, keyspace=%s, exported_at=%v",
			header.Version, header.Keyspace, header.Timestamp)
	} else {
		// No header found, assume legacy format
		s.logger.Printf("No header found, processing as legacy format")
		// Reset reader if possible
		if seeker, ok := r.(io.Seeker); ok {
			seeker.Seek(0, io.SeekStart)
		}
	}

	// Read pins in length-prefixed format
	for {
		var length int
		if _, err := fmt.Fscanf(r, "%d\n", &length); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read pin length: %w", err)
		}

		// Read pin data
		data := make([]byte, length)
		if _, err := io.ReadFull(r, data); err != nil {
			return fmt.Errorf("failed to read pin data: %w", err)
		}

		// Read trailing newline
		var newline [1]byte
		if _, err := r.Read(newline[:]); err != nil {
			return fmt.Errorf("failed to read newline: %w", err)
		}

		// Unmarshal pin using protobuf
		var pin api.Pin
		if err := pin.ProtoUnmarshal(data); err != nil {
			s.logger.Printf("Failed to unmarshal pin, skipping: %v", err)
			continue
		}

		// Add to batch
		if err := batchingState.Add(ctx, pin); err != nil {
			return fmt.Errorf("failed to add pin to batch during unmarshal: %w", err)
		}

		count++
		batchCount++

		// Commit batch when it reaches the configured size
		if batchCount >= s.config.BatchSize {
			if err := batchingState.Commit(ctx); err != nil {
				return fmt.Errorf("failed to commit batch during unmarshal: %w", err)
			}

			// Create new batch for next set of operations
			batchingState.Close()
			batchingState, err = s.NewBatching(ctx)
			if err != nil {
				return fmt.Errorf("failed to create new batching state: %w", err)
			}
			batchCount = 0

			// Log progress
			if count%10000 == 0 {
				s.logger.Printf("Unmarshal progress: %d pins imported", count)
			}
		}
	}

	// Commit any remaining pins in the batch
	if batchCount > 0 {
		if err := batchingState.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit final batch during unmarshal: %w", err)
		}
	}

	// Record operation metrics and log
	duration := time.Since(start)
	opCtx = opCtx.WithDuration(duration).
		WithRecordCount(int64(count)).
		WithMetadata("import_format", "length_prefixed_protobuf")

	s.structuredLogger.LogOperation(ctx, opCtx)
	s.recordOperation("Unmarshal", duration, nil)

	s.logger.Printf("State unmarshaled successfully: pin_count=%d, duration=%v", count, duration)
	return nil
}

// Migrate implements state.State.Migrate - migrates state from older formats
func (s *ScyllaState) Migrate(ctx context.Context, r io.Reader) error {
	if s.isClosed() {
		return fmt.Errorf("ScyllaState is closed")
	}

	start := time.Now()

	// Create operation context for structured logging
	opCtx := NewOperationContext("Migrate").
		WithConsistency(s.config.Consistency).
		WithQueryType("MIGRATE")

	// Try to detect the format by reading the first few bytes
	buf := make([]byte, 1024)
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read migration data header: %w", err)
	}

	// Create a new reader that includes the read bytes
	fullReader := io.MultiReader(bytes.NewReader(buf[:n]), r)

	// Try to detect format and migrate accordingly
	var migrationErr error
	var count int64

	// First, try to unmarshal as current format (msgpack with header)
	if err := s.tryUnmarshalCurrentFormat(fullReader); err == nil {
		duration := time.Since(start)
		opCtx = opCtx.WithDuration(duration).
			WithMetadata("migration_type", "current_format")
		s.structuredLogger.LogOperation(ctx, opCtx)
		s.recordOperation("Migrate", duration, nil)
		return nil
	}

	// Reset reader for next attempt
	fullReader = io.MultiReader(bytes.NewReader(buf[:n]), r)

	// Try to migrate from legacy format (simple length-prefixed)
	count, migrationErr = s.migrateLegacyFormat(ctx, fullReader)
	if migrationErr == nil {
		duration := time.Since(start)
		opCtx = opCtx.WithDuration(duration).
			WithRecordCount(count).
			WithMetadata("migration_type", "legacy_format")
		s.structuredLogger.LogOperation(ctx, opCtx)
		s.recordOperation("Migrate", duration, nil)
		s.logger.Printf("Successfully migrated from legacy format: pin_count=%d", count)
		return nil
	}

	// Reset reader for next attempt
	fullReader = io.MultiReader(bytes.NewReader(buf[:n]), r)

	// Try to migrate from dsstate format (msgpack without header)
	count, migrationErr = s.migrateDSStateFormat(ctx, fullReader)
	if migrationErr == nil {
		duration := time.Since(start)
		opCtx = opCtx.WithDuration(duration).
			WithRecordCount(count).
			WithMetadata("migration_type", "dsstate_format")
		s.structuredLogger.LogOperation(ctx, opCtx)
		s.recordOperation("Migrate", duration, nil)
		s.logger.Printf("Successfully migrated from dsstate format: pin_count=%d", count)
		return nil
	}

	// If all migration attempts failed, return the last error
	duration := time.Since(start)
	opCtx = opCtx.WithDuration(duration).WithError(migrationErr)
	s.structuredLogger.LogOperation(ctx, opCtx)
	s.recordOperation("Migrate", duration, migrationErr)

	return fmt.Errorf("failed to migrate state from any known format: %w", migrationErr)
}

// incrementPinCount atomically increments the pin count
func (s *ScyllaState) incrementPinCount() {
	newCount := s.addPinCount(1)
	if s.metrics != nil {
		s.metrics.totalPins = newCount
		s.metrics.pinOperationsRate++
	}
}

// decrementPinCount atomically decrements the pin count
func (s *ScyllaState) decrementPinCount() {
	newCount := s.addPinCount(-1)
	if s.metrics != nil {
		s.metrics.totalPins = newCount
		s.metrics.pinOperationsRate++
	}
}

// addPinCount atomically adds delta to the pin count and returns the new value
func (s *ScyllaState) addPinCount(delta int64) int64 {
	return atomic.AddInt64(&s.totalPins, delta)
}

// GetPinCount returns the current pin count
func (s *ScyllaState) GetPinCount() int64 {
	return atomic.LoadInt64(&s.totalPins)
}

// initializePlacement creates initial placement record for a new pin
func (s *ScyllaState) initializePlacement(ctx context.Context, cid api.Cid, prefix int16, cidBin []byte, timestamp time.Time) error {
	// Initialize with empty desired and actual sets
	// These will be populated by the cluster orchestrator
	desired := make([]string, 0)
	actual := make([]string, 0)

	query := s.prepared.insertPlacement.WithContext(ctx).Bind(
		prefix,
		cidBin,
		desired,
		actual,
		timestamp,
	)
	defer query.Release()

	if err := query.Exec(); err != nil {
		return fmt.Errorf("failed to initialize placement: %w", err)
	}

	return nil
}

// cleanupPlacement removes placement record for a pin
func (s *ScyllaState) cleanupPlacement(ctx context.Context, prefix int16, cidBin []byte) error {
	query := s.prepared.deletePlacement.WithContext(ctx).Bind(prefix, cidBin)
	defer query.Release()

	if err := query.Exec(); err != nil {
		return fmt.Errorf("failed to delete placement: %w", err)
	}

	return nil
}

// cleanupPeerPins removes all peer pin records for a CID
func (s *ScyllaState) cleanupPeerPins(ctx context.Context, prefix int16, cidBin []byte) error {
	// First, get all peers that have this pin
	peers, err := s.getPeersForPin(ctx, prefix, cidBin)
	if err != nil {
		return fmt.Errorf("failed to get peers for pin cleanup: %w", err)
	}

	// Delete from pins_by_peer for each peer
	for _, peerID := range peers {
		query := s.prepared.deletePeerPin.WithContext(ctx).Bind(peerID, prefix, cidBin)
		if err := query.Exec(); err != nil {
			s.logger.Printf("Failed to delete peer pin for peer %s: %v", peerID, err)
			// Continue with other peers
		}
		query.Release()
	}

	return nil
}

// getPeersForPin retrieves all peers that have a specific pin
func (s *ScyllaState) getPeersForPin(ctx context.Context, prefix int16, cidBin []byte) ([]string, error) {
	// This would require a reverse lookup or maintaining an index
	// For now, we'll use a simple approach that may not be fully efficient
	// In production, you might want to maintain a separate index table

	// Query placements to get the list of peers
	query := s.prepared.selectPlacement.WithContext(ctx).Bind(prefix, cidBin)
	defer query.Release()

	var desired, actual []string
	var updatedAt time.Time

	if err := query.Scan(&desired, &actual, &updatedAt); err != nil {
		if err == gocql.ErrNotFound {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to get placement for peer cleanup: %w", err)
	}

	// Combine desired and actual peers (remove duplicates)
	peerSet := make(map[string]bool)
	for _, peer := range desired {
		peerSet[peer] = true
	}
	for _, peer := range actual {
		peerSet[peer] = true
	}

	peers := make([]string, 0, len(peerSet))
	for peer := range peerSet {
		peers = append(peers, peer)
	}

	return peers, nil
}

// addToTTLQueue adds a pin to the TTL queue for scheduled deletion
func (s *ScyllaState) addToTTLQueue(ctx context.Context, cid api.Cid, cidBin []byte, ttlMillis int64, owner string) error {
	ttlTime := time.UnixMilli(ttlMillis)

	// Create TTL bucket (truncate to hour for efficient querying)
	ttlBucket := ttlTime.Truncate(time.Hour)

	query := s.prepared.insertTTL.WithContext(ctx).Bind(
		ttlBucket,
		cidBin,
		owner,
		ttlTime,
	)
	defer query.Release()

	if err := query.Exec(); err != nil {
		return fmt.Errorf("failed to add to TTL queue: %w", err)
	}

	return nil
}

// removeFromTTLQueue removes a pin from the TTL queue
func (s *ScyllaState) removeFromTTLQueue(ctx context.Context, cid api.Cid, cidBin []byte, ttlMillis int64, owner string) error {
	ttlTime := time.UnixMilli(ttlMillis)
	ttlBucket := ttlTime.Truncate(time.Hour)

	query := s.prepared.deleteTTL.WithContext(ctx).Bind(
		ttlBucket,
		ttlTime,
		cidBin,
	)
	defer query.Release()

	if err := query.Exec(); err != nil {
		return fmt.Errorf("failed to remove from TTL queue: %w", err)
	}

	return nil
}

// State export/import data structures

// CurrentStateVersion is the current version of the state export format
const CurrentStateVersion = 1

// StateExportHeader contains metadata about the exported state
type StateExportHeader struct {
	Version   int       `codec:"version"`
	Timestamp time.Time `codec:"timestamp"`
	Keyspace  string    `codec:"keyspace"`
}

// StateEntry represents a single pin entry in the exported state
type StateEntry struct {
	CID  []byte `codec:"cid"`
	Data []byte `codec:"data"`
}

// Migration helper methods

// tryUnmarshalCurrentFormat attempts to unmarshal using the current format
func (s *ScyllaState) tryUnmarshalCurrentFormat(r io.Reader) error {
	return s.Unmarshal(r)
}

// migrateLegacyFormat migrates from the legacy length-prefixed format
func (s *ScyllaState) migrateLegacyFormat(ctx context.Context, r io.Reader) (int64, error) {
	count := int64(0)

	// Create batching state for efficient bulk import
	batchingState, err := s.NewBatching(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to create batching state: %w", err)
	}
	defer batchingState.Close()

	batchCount := 0

	// Read pins from the legacy format
	for {
		var length int
		if _, err := fmt.Fscanf(r, "%d\n", &length); err != nil {
			if err == io.EOF {
				break
			}
			return count, fmt.Errorf("failed to read pin length: %w", err)
		}

		// Read pin data
		data := make([]byte, length)
		if _, err := io.ReadFull(r, data); err != nil {
			return count, fmt.Errorf("failed to read pin data: %w", err)
		}

		// Read trailing newline
		var newline [1]byte
		if _, err := r.Read(newline[:]); err != nil {
			return count, fmt.Errorf("failed to read newline: %w", err)
		}

		// Unmarshal pin using protobuf
		var pin api.Pin
		if err := pin.ProtoUnmarshal(data); err != nil {
			s.logger.Printf("Failed to unmarshal legacy pin, skipping: %v", err)
			continue
		}

		// Add to batch
		if err := batchingState.Add(ctx, pin); err != nil {
			return count, fmt.Errorf("failed to add pin to batch during migration: %w", err)
		}

		count++
		batchCount++

		// Commit batch when it reaches the configured size
		if batchCount >= s.config.BatchSize {
			if err := batchingState.Commit(ctx); err != nil {
				return count, fmt.Errorf("failed to commit batch during migration: %w", err)
			}

			// Create new batch for next set of operations
			batchingState.Close()
			batchingState, err = s.NewBatching(ctx)
			if err != nil {
				return count, fmt.Errorf("failed to create new batching state: %w", err)
			}
			batchCount = 0
		}
	}

	// Commit any remaining pins in the batch
	if batchCount > 0 {
		if err := batchingState.Commit(ctx); err != nil {
			return count, fmt.Errorf("failed to commit final batch during migration: %w", err)
		}
	}

	return count, nil
}

// migrateDSStateFormat migrates from dsstate msgpack format
func (s *ScyllaState) migrateDSStateFormat(ctx context.Context, r io.Reader) (int64, error) {
	count := int64(0)

	// Process dsstate format (this is a simplified version - real dsstate uses msgpack)
	// For now, we'll treat it as legacy format since we don't have msgpack codec available

	// Create batching state for efficient bulk import
	batchingState, err := s.NewBatching(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to create batching state: %w", err)
	}
	defer batchingState.Close()

	batchCount := 0

	// For now, treat dsstate format as legacy format since we don't have msgpack codec
	// In a real implementation, this would parse the msgpack format
	// For this implementation, we'll delegate to legacy format migration
	return s.migrateLegacyFormat(ctx, r)

	return count, nil
}

// unmarshalLegacyFormat is a helper method to unmarshal legacy format data
func (s *ScyllaState) unmarshalLegacyFormat(ctx context.Context, r io.Reader) error {
	count, err := s.migrateLegacyFormat(ctx, r)
	if err != nil {
		return fmt.Errorf("failed to unmarshal legacy format: %w", err)
	}

	s.logger.Printf("Successfully unmarshaled legacy format: pin_count=%d", count)
	return nil
}
