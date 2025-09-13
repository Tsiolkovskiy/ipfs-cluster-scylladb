package scyllastate

// getInitialSchemaScript returns the initial schema creation script (v1.0.0)
func getInitialSchemaScript() string {
	return `
-- Initial ScyllaDB schema for IPFS-Cluster pin metadata storage (v1.0.0)

-- Create keyspace with NetworkTopologyStrategy for multi-DC deployment
CREATE KEYSPACE IF NOT EXISTS ipfs_pins
WITH replication = {
    'class': 'NetworkTopologyStrategy', 
    'datacenter1': '3'
} AND durable_writes = true;

-- 1) Main pin metadata table - source of truth for all pins
CREATE TABLE IF NOT EXISTS ipfs_pins.pins_by_cid (
    mh_prefix smallint,          -- First 2 bytes of digest for partitioning
    cid_bin blob,                -- Binary multihash (complete CID)
    pin_type tinyint,            -- 0=direct, 1=recursive, 2=indirect
    rf tinyint,                  -- Required replication factor
    owner text,                  -- Tenant/owner identifier
    tags set<text>,              -- Arbitrary tags for categorization
    ttl timestamp,               -- Planned auto-removal timestamp (NULL = permanent)
    metadata map<text, text>,    -- Additional key-value metadata
    created_at timestamp,        -- Pin creation time
    updated_at timestamp,        -- Last update time
    size bigint,                 -- Pin size in bytes
    status tinyint,              -- Pin status: 0=pending, 1=active, 2=failed, 3=removing
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160',
    'tombstone_threshold': '0.2'
} AND gc_grace_seconds = 864000
  AND bloom_filter_fp_chance = 0.01
  AND caching = {'keys': 'ALL', 'rows_per_partition': '1000'}
  AND compression = {'class': 'LZ4Compressor'}
  AND comment = 'Main pin metadata storage - source of truth for all pins';

-- 2) Pin placement tracking - desired vs actual peer assignments
CREATE TABLE IF NOT EXISTS ipfs_pins.placements_by_cid (
    mh_prefix smallint,          -- Same partitioning key as pins_by_cid
    cid_bin blob,                -- Binary CID for placement tracking
    desired set<text>,           -- List of peer_ids where pin should be placed
    actual set<text>,            -- List of peer_ids where pin is confirmed
    failed set<text>,            -- List of peer_ids where pin failed
    updated_at timestamp,        -- Last placement update timestamp
    reconcile_count int,         -- Number of reconciliation attempts
    drift_detected boolean,      -- Flag indicating desired != actual state
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160',
    'tombstone_threshold': '0.2'
} AND gc_grace_seconds = 864000
  AND bloom_filter_fp_chance = 0.01
  AND caching = {'keys': 'ALL', 'rows_per_partition': '500'}
  AND compression = {'class': 'LZ4Compressor'}
  AND comment = 'Pin placement tracking - desired vs actual peer assignments';

-- 3) Reverse index - pins by peer for efficient worker queries
CREATE TABLE IF NOT EXISTS ipfs_pins.pins_by_peer (
    peer_id text,
    mh_prefix smallint,
    cid_bin blob,
    state tinyint,               -- 0=queued, 1=pinning, 2=pinned, 3=failed, 4=unpinned
    last_seen timestamp,         -- Last status update from peer
    retry_count int,             -- Number of retry attempts
    pin_size bigint,             -- Size of pinned content
    PRIMARY KEY ((peer_id), mh_prefix, cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160'
} AND gc_grace_seconds = 864000
  AND bloom_filter_fp_chance = 0.01
  AND caching = {'keys': 'ALL', 'rows_per_partition': '200'}
  AND compression = {'class': 'LZ4Compressor'}
  AND comment = 'Reverse index - pins assigned to each peer';

-- 4) TTL queue for scheduled pin removal
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
} AND default_time_to_live = 2592000
  AND comment = 'TTL queue for scheduled pin removal';

-- 5) Operation deduplication for idempotency
CREATE TABLE IF NOT EXISTS ipfs_pins.op_dedup (
    op_id text,                  -- ULID/UUID from client
    ts timestamp,                -- Operation timestamp
    PRIMARY KEY (op_id)
) WITH default_time_to_live = 604800
  AND comment = 'Operation deduplication for idempotency';

-- 6) Pin statistics and counters
CREATE TABLE IF NOT EXISTS ipfs_pins.pin_stats (
    stat_key text,               -- Statistic identifier
    stat_value counter,          -- Counter value
    PRIMARY KEY (stat_key)
) WITH comment = 'Pin statistics and counters';

-- 7) Pin events log for audit and debugging
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
} AND default_time_to_live = 2592000
  AND comment = 'Pin events log for audit and debugging';
`
}

// getV110MigrationScript returns the migration script from v1.0.0 to v1.1.0
func getV110MigrationScript() string {
	return `
-- Migration from version 1.0.0 to 1.1.0
-- Adds enhanced pin metadata and performance optimization tables

-- Add new columns to pins_by_cid table for enhanced functionality
ALTER TABLE ipfs_pins.pins_by_cid ADD version int;
ALTER TABLE ipfs_pins.pins_by_cid ADD checksum text;
ALTER TABLE ipfs_pins.pins_by_cid ADD priority tinyint;

-- Add new columns to placements_by_cid table for better tracking
ALTER TABLE ipfs_pins.placements_by_cid ADD in_progress set<text>;
ALTER TABLE ipfs_pins.placements_by_cid ADD last_reconcile timestamp;

-- Add new columns to pins_by_peer table for enhanced monitoring
ALTER TABLE ipfs_pins.pins_by_peer ADD assigned_at timestamp;
ALTER TABLE ipfs_pins.pins_by_peer ADD completed_at timestamp;
ALTER TABLE ipfs_pins.pins_by_peer ADD error_message text;

-- Create partition statistics table for load balancing
CREATE TABLE IF NOT EXISTS ipfs_pins.partition_stats (
    mh_prefix smallint,
    stat_type text,              -- 'pin_count', 'total_size', 'operations_per_hour'
    stat_value bigint,
    updated_at timestamp,
    PRIMARY KEY (mh_prefix, stat_type)
) WITH compaction = {'class': 'LeveledCompactionStrategy'}
  AND default_time_to_live = 86400
  AND comment = 'Partition statistics for load balancing';

-- Create performance metrics table for monitoring
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
} AND default_time_to_live = 604800
  AND comment = 'Performance metrics for monitoring and optimization';

-- Create batch operations table for performance optimization
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
  AND default_time_to_live = 2592000
  AND comment = 'Batch operation tracking for performance optimization';

-- Update table properties for better performance (these are safe to run multiple times)
-- Note: Some of these may already be set, but ALTER TABLE is idempotent for properties

-- Update compression and repair settings for pins_by_cid
ALTER TABLE ipfs_pins.pins_by_cid WITH read_repair_chance = 0.1;
ALTER TABLE ipfs_pins.pins_by_cid WITH dclocal_read_repair_chance = 0.1;

-- Update compression and repair settings for placements_by_cid  
ALTER TABLE ipfs_pins.placements_by_cid WITH read_repair_chance = 0.1;
ALTER TABLE ipfs_pins.placements_by_cid WITH dclocal_read_repair_chance = 0.1;
`
}

// getV120MigrationScript returns the migration script from v1.1.0 to v1.2.0 (future)
// This is a placeholder for future migrations
func getV120MigrationScript() string {
	return `
-- Migration from version 1.1.0 to 1.2.0 (placeholder for future enhancements)
-- This migration would add new features like:
-- - Enhanced security features
-- - Additional indexing for performance
-- - New monitoring capabilities

-- Example future migration (commented out):
-- ALTER TABLE ipfs_pins.pins_by_cid ADD security_level tinyint;
-- ALTER TABLE ipfs_pins.pins_by_cid ADD encryption_key text;

-- CREATE TABLE IF NOT EXISTS ipfs_pins.security_audit (
--     audit_id uuid,
--     operation text,
--     user_id text,
--     cid_bin blob,
--     timestamp timestamp,
--     details text,
--     PRIMARY KEY (audit_id)
-- ) WITH default_time_to_live = 7776000; -- 90 days
`
}

// getRollbackScript returns a rollback script for a specific version
// Note: Rollbacks are generally not recommended for ScyllaDB due to the nature of the data
func getRollbackScript(fromVersion, toVersion string) string {
	switch fromVersion + "->" + toVersion {
	case "1.1.0->1.0.0":
		return `
-- Rollback from v1.1.0 to v1.0.0
-- WARNING: This will remove data and cannot be undone!

-- Drop new tables added in v1.1.0
DROP TABLE IF EXISTS ipfs_pins.partition_stats;
DROP TABLE IF EXISTS ipfs_pins.performance_metrics;
DROP TABLE IF EXISTS ipfs_pins.batch_operations;

-- Note: Cannot drop columns in ScyllaDB, so new columns will remain but be unused
-- The application should ignore: version, checksum, priority, in_progress, last_reconcile,
-- assigned_at, completed_at, error_message columns
`
	default:
		return "-- No rollback script available for this version combination"
	}
}

// getSchemaValidationScript returns a script to validate schema integrity
func getSchemaValidationScript() string {
	return `
-- Schema validation queries
-- These queries check that the schema is properly configured

-- Check keyspace replication
SELECT keyspace_name, replication FROM system_schema.keyspaces WHERE keyspace_name = 'ipfs_pins';

-- Check all required tables exist
SELECT table_name, compaction, compression, caching 
FROM system_schema.tables 
WHERE keyspace_name = 'ipfs_pins' 
ORDER BY table_name;

-- Check column structure for main tables
SELECT table_name, column_name, type, kind 
FROM system_schema.columns 
WHERE keyspace_name = 'ipfs_pins' 
  AND table_name IN ('pins_by_cid', 'placements_by_cid', 'pins_by_peer', 'pin_ttl_queue')
ORDER BY table_name, column_name;

-- Check for any secondary indexes (should be minimal)
SELECT keyspace_name, table_name, index_name, kind, options
FROM system_schema.indexes
WHERE keyspace_name = 'ipfs_pins';

-- Check materialized views (should be none by default)
SELECT keyspace_name, view_name, base_table_name
FROM system_schema.views
WHERE keyspace_name = 'ipfs_pins';
`
}

// getPerformanceOptimizationScript returns a script for performance tuning
func getPerformanceOptimizationScript() string {
	return `
-- Performance optimization script
-- These settings can be applied to improve performance for high-scale deployments

-- Optimize compaction settings for high write throughput
ALTER TABLE ipfs_pins.pins_by_cid WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160',
    'tombstone_threshold': '0.2'
};

-- Optimize caching for frequently accessed data
ALTER TABLE ipfs_pins.pins_by_cid WITH caching = {
    'keys': 'ALL', 
    'rows_per_partition': '1000'
};

-- Set optimal bloom filter settings for read performance
ALTER TABLE ipfs_pins.pins_by_cid WITH bloom_filter_fp_chance = 0.01;

-- Configure read repair for consistency (adjust based on consistency requirements)
ALTER TABLE ipfs_pins.pins_by_cid WITH read_repair_chance = 0.1;
ALTER TABLE ipfs_pins.pins_by_cid WITH dclocal_read_repair_chance = 0.1;

-- Apply similar optimizations to other core tables
ALTER TABLE ipfs_pins.placements_by_cid WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160'
};

ALTER TABLE ipfs_pins.pins_by_peer WITH caching = {
    'keys': 'ALL', 
    'rows_per_partition': '200'
};
`
}

// getMultiDCOptimizationScript returns a script for multi-datacenter optimization
func getMultiDCOptimizationScript() string {
	return `
-- Multi-datacenter optimization script
-- These settings optimize the schema for multi-DC deployments

-- Update keyspace replication for multi-DC (example for 2 DCs)
-- Note: This should be customized based on actual datacenter names and requirements
-- ALTER KEYSPACE ipfs_pins WITH replication = {
--     'class': 'NetworkTopologyStrategy',
--     'datacenter1': '3',
--     'datacenter2': '3'
-- };

-- Set LOCAL_QUORUM as default consistency for multi-DC
-- This is typically done at the application level, not in schema

-- Optimize for cross-DC latency by using appropriate compaction
ALTER TABLE ipfs_pins.pins_by_cid WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160',
    'tombstone_threshold': '0.2'
};

-- Use local read repair to minimize cross-DC traffic
ALTER TABLE ipfs_pins.pins_by_cid WITH dclocal_read_repair_chance = 0.1;
ALTER TABLE ipfs_pins.pins_by_cid WITH read_repair_chance = 0.0;
`
}
