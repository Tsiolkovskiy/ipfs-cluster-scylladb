-- ScyllaDB schema for IPFS-Cluster pin metadata storage
-- This schema is optimized for trillion-scale pin management with optimized partitioning
-- Version: 1.1.0
-- 
-- Requirements addressed:
-- - 2.1: Store pin metadata (CID, replication factor, timestamp, placements)
-- - 2.2: Support consistent read semantics and conditional updates
-- - 7.3: Implement caching strategies for predictable query patterns
--
-- Partitioning strategy: mh_prefix (first 2 bytes of multihash digest)
-- This provides 65536 partitions for optimal distribution across cluster nodes

-- Create keyspace with NetworkTopologyStrategy for multi-DC deployment
-- Default configuration for single DC, can be modified for multi-DC setups
-- Supports requirement 6.1: Multi-DC replication strategies
CREATE KEYSPACE IF NOT EXISTS ipfs_pins
WITH replication = {
    'class': 'NetworkTopologyStrategy', 
    'datacenter1': '3'
} AND durable_writes = true;

-- 1) Main pin metadata table - source of truth for all pins
-- Partitioned by mh_prefix (first 2 bytes of digest) for optimal distribution across 65536 partitions
-- Requirement 2.1: Store pin metadata (CID, replication factor, timestamp, placements)
-- Requirement 2.2: Support conditional updates for race condition prevention
-- Requirement 7.3: Implement caching strategies for predictable query patterns
CREATE TABLE IF NOT EXISTS ipfs_pins.pins_by_cid (
    mh_prefix smallint,          -- First 2 bytes of digest (0..65535) for even partitioning
    cid_bin blob,                -- Binary multihash (complete CID in binary form)
    pin_type tinyint,            -- 0=direct, 1=recursive, 2=indirect
    rf tinyint,                  -- Required replication factor (1-255)
    owner text,                  -- Tenant/owner identifier for multi-tenancy
    tags set<text>,              -- Arbitrary tags for categorization and filtering
    ttl timestamp,               -- Planned auto-removal timestamp (NULL = permanent)
    metadata map<text, text>,    -- Additional key-value metadata
    created_at timestamp,        -- Pin creation time (for auditing)
    updated_at timestamp,        -- Last update time (for conflict resolution)
    size bigint,                 -- Pin size in bytes (for billing and monitoring)
    status tinyint,              -- Pin status: 0=pending, 1=active, 2=failed, 3=removing
    version int,                 -- Version counter for optimistic locking (conditional updates)
    checksum text,               -- Optional integrity checksum for validation
    priority tinyint,            -- Pin priority: 0=low, 1=normal, 2=high, 3=critical
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160',
    'tombstone_threshold': '0.2'
} AND gc_grace_seconds = 864000  -- 10 days for tombstone cleanup
  AND bloom_filter_fp_chance = 0.01  -- Optimized for read performance
  AND caching = {'keys': 'ALL', 'rows_per_partition': '1000'}  -- Cache frequently accessed data
  AND compression = {'class': 'LZ4Compressor'}  -- Fast compression for better I/O
  AND read_repair_chance = 0.1  -- Enable read repair for consistency
  AND dclocal_read_repair_chance = 0.1  -- Local DC read repair
  AND comment = 'Main pin metadata storage - source of truth for all pins';

-- 2) Pin placement tracking - desired vs actual peer assignments
-- Same partitioning as pins_by_cid for data co-location and join efficiency
-- Requirement 2.1: Store placement information for pin orchestration
-- Requirement 2.2: Support consistent read semantics for placement queries
CREATE TABLE IF NOT EXISTS ipfs_pins.placements_by_cid (
    mh_prefix smallint,          -- Same partitioning key as pins_by_cid
    cid_bin blob,                -- Binary CID for placement tracking
    desired set<text>,           -- List of peer_ids where pin should be placed
    actual set<text>,            -- List of peer_ids where pin is confirmed
    failed set<text>,            -- List of peer_ids where pin failed
    in_progress set<text>,       -- List of peer_ids currently pinning
    updated_at timestamp,        -- Last placement update timestamp
    reconcile_count int,         -- Number of reconciliation attempts
    last_reconcile timestamp,    -- Last reconciliation attempt timestamp
    drift_detected boolean,      -- Flag indicating desired != actual state
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160',
    'tombstone_threshold': '0.2'
} AND gc_grace_seconds = 864000  -- 10 days
  AND bloom_filter_fp_chance = 0.01
  AND caching = {'keys': 'ALL', 'rows_per_partition': '500'}
  AND compression = {'class': 'LZ4Compressor'}
  AND read_repair_chance = 0.1
  AND dclocal_read_repair_chance = 0.1
  AND comment = 'Pin placement tracking - desired vs actual peer assignments';

-- 3) Reverse index - pins by peer for efficient worker queries
-- Partitioned by peer_id for peer-specific queries
-- Requirement 2.1: Support efficient peer-based queries for worker agents
-- Requirement 7.3: Optimize for predictable worker query patterns
CREATE TABLE IF NOT EXISTS ipfs_pins.pins_by_peer (
    peer_id text,
    mh_prefix smallint,
    cid_bin blob,
    state tinyint,               -- 0=queued, 1=pinning, 2=pinned, 3=failed, 4=unpinned
    last_seen timestamp,         -- Last status update from peer
    assigned_at timestamp,       -- When pin was assigned to this peer
    completed_at timestamp,      -- When pin operation completed (success or failure)
    error_message text,          -- Error details if state=failed
    retry_count int,             -- Number of retry attempts
    pin_size bigint,             -- Size of pinned content (for monitoring)
    PRIMARY KEY ((peer_id), mh_prefix, cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160'
} AND gc_grace_seconds = 864000
  AND bloom_filter_fp_chance = 0.01
  AND caching = {'keys': 'ALL', 'rows_per_partition': '200'}  -- Cache for worker queries
  AND compression = {'class': 'LZ4Compressor'}
  AND comment = 'Reverse index - pins assigned to each peer';

-- 4) TTL queue for scheduled pin removal
-- Partitioned by hourly buckets for efficient TTL processing
CREATE TABLE IF NOT EXISTS ipfs_pins.pin_ttl_queue (
    ttl_bucket timestamp,        -- Hourly bucket (UTC, truncated to hour)
    cid_bin blob,
    owner text,
    ttl timestamp,               -- Exact TTL timestamp
    PRIMARY KEY ((ttl_bucket), ttl, cid_bin)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'DAYS', 
    'compaction_window_size': '1'
} AND default_time_to_live = 2592000  -- 30 days
  AND comment = 'TTL queue for scheduled pin removal';

-- 5) Operation deduplication for idempotency
-- Short-lived table to prevent duplicate operations
CREATE TABLE IF NOT EXISTS ipfs_pins.op_dedup (
    op_id text,                  -- ULID/UUID from client
    ts timestamp,                -- Operation timestamp
    PRIMARY KEY (op_id)
) WITH default_time_to_live = 604800  -- 7 days
  AND comment = 'Operation deduplication for idempotency';

-- 6) Pin statistics and counters
-- For monitoring and analytics
CREATE TABLE IF NOT EXISTS ipfs_pins.pin_stats (
    stat_key text,               -- Statistic identifier (e.g., 'total_pins', 'peer:QmXXX:pinned')
    stat_value counter,          -- Counter value
    PRIMARY KEY (stat_key)
) WITH comment = 'Pin statistics and counters';

-- 7) Pin events log for audit and debugging
-- Time-series data with automatic cleanup
CREATE TABLE IF NOT EXISTS ipfs_pins.pin_events (
    mh_prefix smallint,
    cid_bin blob,
    ts timeuuid,                 -- Time-ordered UUID for precise ordering
    peer_id text,                -- Peer that generated the event
    event_type text,             -- Event type (pinned, unpinned, failed, etc.)
    details text,                -- JSON details or error message
    PRIMARY KEY ((mh_prefix, cid_bin), ts)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'DAYS',
    'compaction_window_size': '7'
} AND default_time_to_live = 2592000  -- 30 days
  AND comment = 'Pin events log for audit and debugging';

-- Indexes for common query patterns (use sparingly in ScyllaDB)
-- Note: Secondary indexes should be avoided for high-scale scenarios
-- Instead, use denormalized tables above

-- 8) Partition statistics for load balancing and hot partition detection
-- Requirement 7.3: Monitor partition distribution for performance optimization
CREATE TABLE IF NOT EXISTS ipfs_pins.partition_stats (
    mh_prefix smallint,
    stat_type text,              -- 'pin_count', 'total_size', 'operations_per_hour'
    stat_value bigint,
    updated_at timestamp,
    PRIMARY KEY (mh_prefix, stat_type)
) WITH compaction = {'class': 'LeveledCompactionStrategy'}
  AND default_time_to_live = 86400  -- 24 hours
  AND comment = 'Partition statistics for load balancing';

-- 9) Performance metrics table for monitoring query patterns
-- Requirement 7.3: Track performance metrics for optimization
CREATE TABLE IF NOT EXISTS ipfs_pins.performance_metrics (
    metric_name text,
    time_bucket timestamp,       -- Hourly buckets for time-series data
    metric_value double,
    tags map<text, text>,        -- Additional metric dimensions
    PRIMARY KEY (metric_name, time_bucket)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'HOURS',
    'compaction_window_size': '24'
} AND default_time_to_live = 604800  -- 7 days
  AND comment = 'Performance metrics for monitoring and optimization';

-- 10) Batch operation tracking for performance optimization
-- Requirement 7.3: Support efficient batch operations
CREATE TABLE IF NOT EXISTS ipfs_pins.batch_operations (
    batch_id uuid,
    operation_type text,         -- 'pin', 'unpin', 'update'
    cid_list list<blob>,         -- List of CIDs in batch
    status text,                 -- 'pending', 'processing', 'completed', 'failed'
    created_at timestamp,
    completed_at timestamp,
    error_details text,
    PRIMARY KEY (batch_id)
) WITH compaction = {'class': 'LeveledCompactionStrategy'}
  AND default_time_to_live = 2592000  -- 30 days
  AND comment = 'Batch operation tracking for performance optimization';

-- Indexes and materialized views (use sparingly in ScyllaDB)
-- Note: Secondary indexes should be avoided for high-scale scenarios
-- Instead, use denormalized tables above for query patterns

-- Optional materialized view for pins by owner (commented out by default)
-- Only enable if owner-based queries are frequent and performance critical
-- CREATE MATERIALIZED VIEW IF NOT EXISTS ipfs_pins.pins_by_owner AS
--     SELECT owner, mh_prefix, cid_bin, pin_type, rf, tags, ttl, created_at, status
--     FROM ipfs_pins.pins_by_cid
--     WHERE owner IS NOT NULL AND mh_prefix IS NOT NULL AND cid_bin IS NOT NULL
--     PRIMARY KEY (owner, mh_prefix, cid_bin);

-- Optional materialized view for pins by status (commented out by default)
-- CREATE MATERIALIZED VIEW IF NOT EXISTS ipfs_pins.pins_by_status AS
--     SELECT status, mh_prefix, cid_bin, owner, created_at, updated_at
--     FROM ipfs_pins.pins_by_cid
--     WHERE status IS NOT NULL AND mh_prefix IS NOT NULL AND cid_bin IS NOT NULL
--     PRIMARY KEY (status, mh_prefix, cid_bin);