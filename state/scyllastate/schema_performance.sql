-- ScyllaDB High-Performance Schema for IPFS-Cluster Pin State Store
-- Version: 1.1.0
-- 
-- This schema is optimized for maximum throughput and low latency
-- Addresses requirement 7.1: Connection pools and prepared queries
-- Addresses requirement 7.2: Efficient batch operations
-- Addresses requirement 7.3: Caching strategies for predictable patterns

-- Create keyspace with optimized settings for performance
CREATE KEYSPACE IF NOT EXISTS ipfs_pins
WITH replication = {
    'class': 'NetworkTopologyStrategy',
    'datacenter1': '3'
} AND durable_writes = true;

USE ipfs_pins;

-- High-performance pin metadata table
-- Optimized for maximum write throughput and read performance
CREATE TABLE IF NOT EXISTS pins_by_cid (
    mh_prefix smallint,          -- 65536 partitions for optimal distribution
    cid_bin blob,
    pin_type tinyint,
    rf tinyint,
    owner text,
    tags set<text>,
    ttl timestamp,
    metadata map<text, text>,
    created_at timestamp,
    updated_at timestamp,
    size bigint,
    status tinyint,
    version int,
    checksum text,
    priority tinyint,
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '320',     -- Larger SSTables for better compression
    'tombstone_threshold': '0.1'     -- Aggressive tombstone cleanup
} AND gc_grace_seconds = 432000      -- Reduced GC grace for faster cleanup
  AND bloom_filter_fp_chance = 0.001 -- Lower false positive rate
  AND caching = {
    'keys': 'ALL',
    'rows_per_partition': '2000'     -- Increased cache size
  }
  AND compression = {
    'class': 'LZ4Compressor',
    'chunk_length_in_kb': '16'       -- Optimized chunk size
  }
  AND memtable_flush_period_in_ms = 3600000  -- 1 hour flush period
  AND read_repair_chance = 0.05      -- Reduced read repair overhead
  AND dclocal_read_repair_chance = 0.05
  AND speculative_retry = '95percentile'  -- Aggressive speculative retry
  AND comment = 'High-performance pin metadata storage';

-- Optimized placement tracking with minimal overhead
CREATE TABLE IF NOT EXISTS placements_by_cid (
    mh_prefix smallint,
    cid_bin blob,
    desired set<text>,
    actual set<text>,
    failed set<text>,
    in_progress set<text>,
    updated_at timestamp,
    reconcile_count int,
    last_reconcile timestamp,
    drift_detected boolean,
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '320',
    'tombstone_threshold': '0.1'
} AND gc_grace_seconds = 432000
  AND bloom_filter_fp_chance = 0.001
  AND caching = {
    'keys': 'ALL',
    'rows_per_partition': '1000'
  }
  AND compression = {'class': 'LZ4Compressor'}
  AND memtable_flush_period_in_ms = 3600000
  AND read_repair_chance = 0.05
  AND dclocal_read_repair_chance = 0.05
  AND speculative_retry = '95percentile'
  AND comment = 'High-performance placement tracking';

-- Worker-optimized pins by peer table
CREATE TABLE IF NOT EXISTS pins_by_peer (
    peer_id text,
    mh_prefix smallint,
    cid_bin blob,
    state tinyint,
    last_seen timestamp,
    assigned_at timestamp,
    completed_at timestamp,
    error_message text,
    retry_count int,
    pin_size bigint,
    PRIMARY KEY ((peer_id), mh_prefix, cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '320'
} AND gc_grace_seconds = 432000
  AND bloom_filter_fp_chance = 0.001
  AND caching = {
    'keys': 'ALL',
    'rows_per_partition': '500'      -- Optimized for worker queries
  }
  AND compression = {'class': 'LZ4Compressor'}
  AND memtable_flush_period_in_ms = 1800000  -- 30 min for frequent updates
  AND read_repair_chance = 0.05
  AND dclocal_read_repair_chance = 0.05
  AND speculative_retry = '95percentile'
  AND comment = 'Worker-optimized pins by peer';

-- High-throughput TTL queue with time-window compaction
CREATE TABLE IF NOT EXISTS pin_ttl_queue (
    ttl_bucket timestamp,
    cid_bin blob,
    owner text,
    ttl timestamp,
    PRIMARY KEY ((ttl_bucket), ttl, cid_bin)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'HOURS',
    'compaction_window_size': '6',   -- Smaller windows for faster processing
    'max_threshold': '8'             -- Reduced threshold for faster compaction
} AND default_time_to_live = 2592000
  AND bloom_filter_fp_chance = 0.01
  AND memtable_flush_period_in_ms = 900000  -- 15 min for TTL processing
  AND read_repair_chance = 0.0      -- No read repair for TTL queue
  AND dclocal_read_repair_chance = 0.0
  AND comment = 'High-throughput TTL queue';

-- Minimal operation deduplication
CREATE TABLE IF NOT EXISTS op_dedup (
    op_id text,
    ts timestamp,
    PRIMARY KEY (op_id)
) WITH default_time_to_live = 604800
  AND bloom_filter_fp_chance = 0.01
  AND memtable_flush_period_in_ms = 1800000
  AND read_repair_chance = 0.0
  AND dclocal_read_repair_chance = 0.0
  AND comment = 'Minimal operation deduplication';

-- High-performance counters
CREATE TABLE IF NOT EXISTS pin_stats (
    stat_key text,
    stat_value counter,
    PRIMARY KEY (stat_key)
) WITH compaction = {'class': 'LeveledCompactionStrategy'}
  AND read_repair_chance = 0.0
  AND dclocal_read_repair_chance = 0.05
  AND comment = 'High-performance pin statistics';

-- Streamlined event logging
CREATE TABLE IF NOT EXISTS pin_events (
    mh_prefix smallint,
    cid_bin blob,
    ts timeuuid,
    peer_id text,
    event_type text,
    details text,
    PRIMARY KEY ((mh_prefix, cid_bin), ts)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'HOURS',
    'compaction_window_size': '12',
    'max_threshold': '8'
} AND default_time_to_live = 1209600  -- 14 days (reduced retention)
  AND bloom_filter_fp_chance = 0.01
  AND memtable_flush_period_in_ms = 1800000
  AND read_repair_chance = 0.0
  AND dclocal_read_repair_chance = 0.0
  AND comment = 'Streamlined event logging';

-- Partition load balancing table
CREATE TABLE IF NOT EXISTS partition_stats (
    mh_prefix smallint,
    stat_type text,
    stat_value bigint,
    updated_at timestamp,
    PRIMARY KEY (mh_prefix, stat_type)
) WITH compaction = {'class': 'LeveledCompactionStrategy'}
  AND default_time_to_live = 43200  -- 12 hours (frequent updates)
  AND bloom_filter_fp_chance = 0.01
  AND memtable_flush_period_in_ms = 900000
  AND comment = 'Partition load balancing statistics';

-- Performance metrics with time-series optimization
CREATE TABLE IF NOT EXISTS performance_metrics (
    metric_name text,
    time_bucket timestamp,
    metric_value double,
    tags map<text, text>,
    PRIMARY KEY (metric_name, time_bucket)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'HOURS',
    'compaction_window_size': '6',
    'max_threshold': '6'
} AND default_time_to_live = 259200  -- 3 days (reduced retention)
  AND bloom_filter_fp_chance = 0.01
  AND memtable_flush_period_in_ms = 900000
  AND read_repair_chance = 0.0
  AND dclocal_read_repair_chance = 0.0
  AND comment = 'High-frequency performance metrics';

-- Batch operations tracking for throughput optimization
CREATE TABLE IF NOT EXISTS batch_operations (
    batch_id uuid,
    operation_type text,
    cid_count int,               -- Count instead of full list for performance
    status text,
    created_at timestamp,
    completed_at timestamp,
    error_details text,
    throughput_ops_per_sec double,
    PRIMARY KEY (batch_id)
) WITH compaction = {'class': 'LeveledCompactionStrategy'}
  AND default_time_to_live = 604800  -- 7 days
  AND bloom_filter_fp_chance = 0.01
  AND memtable_flush_period_in_ms = 1800000
  AND comment = 'Batch operations for throughput optimization';