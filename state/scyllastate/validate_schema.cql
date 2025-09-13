-- ScyllaDB Schema Validation Script
-- This script validates the schema structure and settings
-- Run this after schema creation to verify everything is correct

-- Validate keyspace exists and has correct replication
DESCRIBE KEYSPACE ipfs_pins;

-- Validate main tables exist
DESCRIBE TABLE ipfs_pins.pins_by_cid;
DESCRIBE TABLE ipfs_pins.placements_by_cid;
DESCRIBE TABLE ipfs_pins.pins_by_peer;
DESCRIBE TABLE ipfs_pins.pin_ttl_queue;
DESCRIBE TABLE ipfs_pins.op_dedup;
DESCRIBE TABLE ipfs_pins.pin_stats;
DESCRIBE TABLE ipfs_pins.pin_events;

-- Validate additional performance tables
DESCRIBE TABLE ipfs_pins.partition_stats;
DESCRIBE TABLE ipfs_pins.performance_metrics;
DESCRIBE TABLE ipfs_pins.batch_operations;

-- Check table settings and properties
SELECT table_name, 
       compaction,
       compression,
       caching,
       bloom_filter_fp_chance,
       gc_grace_seconds,
       default_time_to_live
FROM system_schema.tables 
WHERE keyspace_name = 'ipfs_pins';

-- Validate partition key distribution (run after data insertion)
-- SELECT mh_prefix, COUNT(*) as pin_count 
-- FROM ipfs_pins.pins_by_cid 
-- GROUP BY mh_prefix 
-- ORDER BY pin_count DESC 
-- LIMIT 20;

-- Check for any schema inconsistencies
SELECT keyspace_name, table_name, column_name, type
FROM system_schema.columns
WHERE keyspace_name = 'ipfs_pins'
ORDER BY table_name, column_name;

-- Validate indexes (should be minimal for ScyllaDB)
SELECT keyspace_name, table_name, index_name, kind, options
FROM system_schema.indexes
WHERE keyspace_name = 'ipfs_pins';

-- Check materialized views (if any)
SELECT keyspace_name, view_name, base_table_name
FROM system_schema.views
WHERE keyspace_name = 'ipfs_pins';

-- Validate UDTs (User Defined Types) if any
SELECT keyspace_name, type_name, field_names, field_types
FROM system_schema.types
WHERE keyspace_name = 'ipfs_pins';

-- Performance validation queries (run with sample data)
-- Test partition key distribution
-- SELECT token(mh_prefix), mh_prefix, COUNT(*) 
-- FROM ipfs_pins.pins_by_cid 
-- GROUP BY mh_prefix 
-- LIMIT 10;

-- Test query performance on main table
-- SELECT * FROM ipfs_pins.pins_by_cid 
-- WHERE mh_prefix = 0x1234 
-- LIMIT 10;

-- Test placement queries
-- SELECT * FROM ipfs_pins.placements_by_cid 
-- WHERE mh_prefix = 0x1234 
-- LIMIT 10;

-- Test peer-based queries
-- SELECT * FROM ipfs_pins.pins_by_peer 
-- WHERE peer_id = 'QmTest123' 
-- LIMIT 10;

-- Validation complete message
SELECT 'Schema validation complete. Review output above for any issues.' as status;