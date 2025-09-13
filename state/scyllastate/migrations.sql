-- ScyllaDB Migration Scripts for IPFS-Cluster Pin State Store
-- Version: 1.1.0
-- 
-- This file contains migration scripts to upgrade existing schemas
-- and handle version compatibility

-- Migration from version 1.0.0 to 1.1.0
-- Adds new columns and tables for enhanced functionality

-- Check if we need to migrate pins_by_cid table
-- Add new columns if they don't exist
ALTER TABLE ipfs_pins.pins_by_cid ADD version int;
ALTER TABLE ipfs_pins.pins_by_cid ADD checksum text;
ALTER TABLE ipfs_pins.pins_by_cid ADD priority tinyint;

-- Update placements_by_cid table
ALTER TABLE ipfs_pins.placements_by_cid ADD in_progress set<text>;
ALTER TABLE ipfs_pins.placements_by_cid ADD last_reconcile timestamp;
ALTER TABLE ipfs_pins.placements_by_cid ADD drift_detected boolean;

-- Update pins_by_peer table
ALTER TABLE ipfs_pins.pins_by_peer ADD assigned_at timestamp;
ALTER TABLE ipfs_pins.pins_by_peer ADD completed_at timestamp;
ALTER TABLE ipfs_pins.pins_by_peer ADD error_message text;
ALTER TABLE ipfs_pins.pins_by_peer ADD retry_count int;
ALTER TABLE ipfs_pins.pins_by_peer ADD pin_size bigint;

-- Create new tables (these will be ignored if they already exist)
CREATE TABLE IF NOT EXISTS ipfs_pins.partition_stats (
    mh_prefix smallint,
    stat_type text,
    stat_value bigint,
    updated_at timestamp,
    PRIMARY KEY (mh_prefix, stat_type)
) WITH compaction = {'class': 'LeveledCompactionStrategy'}
  AND default_time_to_live = 86400;

CREATE TABLE IF NOT EXISTS ipfs_pins.performance_metrics (
    metric_name text,
    time_bucket timestamp,
    metric_value double,
    tags map<text, text>,
    PRIMARY KEY (metric_name, time_bucket)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'HOURS',
    'compaction_window_size': '24'
} AND default_time_to_live = 604800;

CREATE TABLE IF NOT EXISTS ipfs_pins.batch_operations (
    batch_id uuid,
    operation_type text,
    cid_list list<blob>,
    status text,
    created_at timestamp,
    completed_at timestamp,
    error_details text,
    PRIMARY KEY (batch_id)
) WITH compaction = {'class': 'LeveledCompactionStrategy'}
  AND default_time_to_live = 2592000;

-- Update table properties for better performance
-- Note: These may require manual execution during maintenance windows

-- ALTER TABLE ipfs_pins.pins_by_cid WITH compression = {'class': 'LZ4Compressor'};
-- ALTER TABLE ipfs_pins.pins_by_cid WITH read_repair_chance = 0.1;
-- ALTER TABLE ipfs_pins.pins_by_cid WITH dclocal_read_repair_chance = 0.1;

-- ALTER TABLE ipfs_pins.placements_by_cid WITH compression = {'class': 'LZ4Compressor'};
-- ALTER TABLE ipfs_pins.placements_by_cid WITH read_repair_chance = 0.1;

-- ALTER TABLE ipfs_pins.pins_by_peer WITH compression = {'class': 'LZ4Compressor'};
-- ALTER TABLE ipfs_pins.pins_by_peer WITH caching = {'keys': 'ALL', 'rows_per_partition': '200'};