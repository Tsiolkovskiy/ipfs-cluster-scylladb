# WriteOnly Interface Implementation

This document describes the implementation of the `state.WriteOnly` interface methods for ScyllaState, specifically task 4.3.

## Implemented Methods

### 1. Add(ctx context.Context, pin api.Pin) error

**Purpose**: Adds or updates a pin in the ScyllaDB state store with conditional operations to prevent race conditions.

**Key Features**:
- **Conditional Updates (Requirement 2.3)**: Uses `IF NOT EXISTS` for new pins and `IF updated_at = ?` for updates
- **Placement Management (Requirement 2.1)**: Initializes placement records for new pins
- **TTL Support**: Automatically adds pins with expiration to the TTL queue
- **Race Condition Prevention**: Prevents concurrent modifications through conditional CQL operations

**Implementation Details**:
```go
// For new pins
INSERT INTO pins_by_cid (...) VALUES (...) IF NOT EXISTS

// For existing pins  
UPDATE pins_by_cid SET ... WHERE ... IF updated_at = ?
```

**Error Handling**:
- Returns specific errors for concurrent modifications
- Continues operation even if placement/TTL operations fail (logs warnings)
- Maintains pin count accuracy

### 2. Rm(ctx context.Context, cid api.Cid) error

**Purpose**: Removes a pin and all related metadata with proper cleanup across all tables.

**Key Features**:
- **Conditional Delete (Requirement 2.3)**: Uses `IF updated_at = ?` to prevent race conditions
- **Complete Cleanup (Requirement 2.4)**: Removes data from all related tables:
  - `pins_by_cid` (main pin record)
  - `placements_by_cid` (placement information)
  - `pins_by_peer` (peer-specific records)
  - `pin_ttl_queue` (TTL records if applicable)

**Implementation Details**:
```go
// Conditional delete to prevent race conditions
DELETE FROM pins_by_cid WHERE mh_prefix = ? AND cid_bin = ? IF updated_at = ?
```

**Cleanup Process**:
1. Get existing pin details for conditional check
2. Perform conditional delete on main pin record
3. Clean up placement records
4. Clean up peer pin records
5. Remove from TTL queue if applicable
6. Update pin count

## Helper Methods

### initializePlacement()
- Creates initial placement record with empty desired/actual peer sets
- Called during pin addition to satisfy Requirement 2.1

### cleanupPlacement()
- Removes placement record during pin deletion
- Part of proper cleanup process (Requirement 2.4)

### cleanupPeerPins()
- Removes all peer-specific pin records
- Queries placement table to find affected peers
- Deletes from `pins_by_peer` for each peer

### getPeersForPin()
- Retrieves list of peers associated with a pin
- Used for comprehensive cleanup during deletion

### addToTTLQueue() / removeFromTTLQueue()
- Manages TTL queue entries for pins with expiration
- Uses hour-based bucketing for efficient querying

## Race Condition Prevention

The implementation prevents race conditions through:

1. **Conditional INSERT**: `IF NOT EXISTS` prevents duplicate creation
2. **Conditional UPDATE**: `IF updated_at = ?` ensures updates are based on current state
3. **Conditional DELETE**: `IF updated_at = ?` prevents deletion of modified pins

## Error Handling Strategy

- **Critical Errors**: Fail the operation (e.g., conditional operation failures)
- **Non-Critical Errors**: Log warnings but continue (e.g., placement cleanup failures)
- **Specific Error Messages**: Distinguish between different failure scenarios

## Requirements Compliance

✅ **Requirement 2.1**: Save pin metadata (CID, replication factor, timestamp, placements) in ScyllaDB
- Implemented through comprehensive pin storage and placement initialization

✅ **Requirement 2.3**: Use conditional updates to prevent race conditions
- Implemented through conditional CQL operations (IF NOT EXISTS, IF updated_at = ?)

✅ **Requirement 2.4**: Delete all related metadata with proper cleanup
- Implemented through multi-table cleanup process in Rm method

## Testing Strategy

The implementation includes:
- Interface compliance verification
- Conditional update logic testing (requires integration tests)
- Cleanup functionality testing (requires integration tests)
- Race condition prevention testing (requires concurrent test scenarios)

## Performance Considerations

- Uses prepared statements for optimal performance
- Minimizes round trips through batched cleanup operations
- Efficient partitioning through mh_prefix for scalability
- Non-blocking cleanup operations where possible

## Future Enhancements

1. **Batch Operations**: Could be enhanced with transaction-like batch operations
2. **Retry Logic**: Could add automatic retry for failed conditional operations
3. **Metrics**: Could add detailed metrics for conditional operation success/failure rates
4. **Optimization**: Could optimize peer lookup for cleanup operations