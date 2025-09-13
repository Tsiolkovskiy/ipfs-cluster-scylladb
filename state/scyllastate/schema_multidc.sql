-- ScyllaDB Multi-Datacenter Schema for IPFS-Cluster Pin State Store
-- Version: 1.1.0
-- 
-- This schema is optimized for multi-datacenter deployments
-- Addresses requirement 6.1: Multi-DC replication strategies
-- Addresses requirement 6.4: DC-aware routing optimization

-- Create keyspace with multi-DC replication strategy
-- Configure replication factors for each datacenter
CREATE KEYSPACE IF NOT EXISTS ipfs_pins
WITH replication = {
    'class': 'NetworkTopologyStrategy',
    'datacenter1': '3',          -- Primary DC with 3 replicas
    'datacenter2': '3',          -- Secondary DC with 3 replicas
    'datacenter3': '2'           -- Tertiary DC with 2 replicas (optional)
} AND durable_writes = true;

-- Use the keyspace
USE ipfs_pins;

-- Main pin metadata table with multi-DC optimizations
CREATE TABLE IF NOT EXISTS pins_by_cid (
    mh_prefix smallint,
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
    origin_dc text,              -- Datacenter where pin was originally created
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160',
    'tombstone_threshold': '0.2'
} AND gc_grace_seconds = 864000
  AND bloom_filter_fp_chance = 0.01
  AND caching = {'keys': 'ALL', 'rows_per_partition': '1000'}
  AND compression = {'class': 'LZ4Compressor'}
  AND read_repair_chance = 0.0  -- Disable cross-DC read repair for performance
  AND dclocal_read_repair_chance = 0.1  -- Enable local DC read repair only
  AND comment = 'Main pin metadata storage - multi-DC optimized';

-- Pin placement tracking with DC awareness
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
    dc_placements map<text, set<text>>,  -- Placements per datacenter
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160',
    'tombstone_threshold': '0.2'
} AND gc_grace_seconds = 864000
  AND bloom_filter_fp_chance = 0.01
  AND caching = {'keys': 'ALL', 'rows_per_partition': '500'}
  AND compression = {'class': 'LZ4Compressor'}
  AND read_repair_chance = 0.0
  AND dclocal_read_repair_chance = 0.1
  AND comment = 'Pin placement tracking - multi-DC aware';

-- Pins by peer with datacenter information
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
    peer_dc text,                -- Datacenter of the peer
    PRIMARY KEY ((peer_id), mh_prefix, cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160'
} AND gc_grace_seconds = 864000
  AND bloom_filter_fp_chance = 0.01
  AND caching = {'keys': 'ALL', 'rows_per_partition': '200'}
  AND compression = {'class': 'LZ4Compressor'}
  AND read_repair_chance = 0.0
  AND dclocal_read_repair_chance = 0.1
  AND comment = 'Reverse index - pins by peer with DC awareness';

-- TTL queue with datacenter-specific processing
CREATE TABLE IF NOT EXISTS pin_ttl_queue (
    ttl_bucket timestamp,
    cid_bin blob,
    owner text,
    ttl timestamp,
    origin_dc text,              -- Process TTL in origin DC first
    PRIMARY KEY ((ttl_bucket), ttl, cid_bin)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'DAYS',
    'compaction_window_size': '1'
} AND default_time_to_live = 2592000
  AND read_repair_chance = 0.0
  AND dclocal_read_repair_chance = 0.0  -- No read repair for TTL queue
  AND comment = 'TTL queue for scheduled pin removal - DC aware';

-- Operation deduplication (local to each DC)
CREATE TABLE IF NOT EXISTS op_dedup (
    op_id text,
    ts timestamp,
    dc text,                     -- Datacenter where operation originated
    PRIMARY KEY (op_id)
) WITH default_time_to_live = 604800
  AND read_repair_chance = 0.0
  AND dclocal_read_repair_chance = 0.0
  AND comment = 'Operation deduplication - DC local';

-- Pin statistics per datacenter
CREATE TABLE IF NOT EXISTS pin_stats (
    stat_key text,
    dc text,                     -- Datacenter for the statistic
    stat_value counter,
    PRIMARY KEY (stat_key, dc)
) WITH read_repair_chance = 0.0
  AND dclocal_read_repair_chance = 0.1
  AND comment = 'Pin statistics per datacenter';

-- Pin events log with datacenter tracking
CREATE TABLE IF NOT EXISTS pin_events (
    mh_prefix smallint,
    cid_bin blob,
    ts timeuuid,
    peer_id text,
    event_type text,
    details text,
    dc text,                     -- Datacenter where event occurred
    PRIMARY KEY ((mh_prefix, cid_bin), ts)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'DAYS',
    'compaction_window_size': '7'
} AND default_time_to_live = 2592000
  AND read_repair_chance = 0.0
  AND dclocal_read_repair_chance = 0.0
  AND comment = 'Pin events log with datacenter tracking';

-- Datacenter health and connectivity tracking
CREATE TABLE IF NOT EXISTS dc_health (
    dc_name text,
    metric_name text,
    metric_value double,
    updated_at timestamp,
    PRIMARY KEY (dc_name, metric_name)
) WITH compaction = {'class': 'LeveledCompactionStrategy'}
  AND default_time_to_live = 86400
  AND read_repair_chance = 0.0
  AND dclocal_read_repair_chance = 0.1
  AND comment = 'Datacenter health metrics';

-- Cross-DC replication lag tracking
CREATE TABLE IF NOT EXISTS replication_lag (
    source_dc text,
    target_dc text,
    table_name text,
    lag_ms bigint,
    measured_at timestamp,
    PRIMARY KEY ((source_dc, target_dc), table_name)
) WITH compaction = {'class': 'LeveledCompactionStrategy'}
  AND default_time_to_live = 86400
  AND comment = 'Cross-datacenter replication lag monitoring';