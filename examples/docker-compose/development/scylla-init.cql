-- Initialize ScyllaDB keyspace and tables for IPFS-Cluster development

-- Create keyspace with simple replication for single node
CREATE KEYSPACE IF NOT EXISTS ipfs_pins 
WITH REPLICATION = {
    'class': 'SimpleStrategy',
    'replication_factor': 1
};

USE ipfs_pins;

-- Main table for storing pin metadata
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
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {'class': 'LeveledCompactionStrategy'};

-- Placement tracking table
CREATE TABLE IF NOT EXISTS placements_by_cid (
    mh_prefix smallint,
    cid_bin blob,
    desired set<text>,
    actual set<text>,
    updated_at timestamp,
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {'class': 'LeveledCompactionStrategy'};

-- Reverse index - pins by peer
CREATE TABLE IF NOT EXISTS pins_by_peer (
    peer_id text,
    mh_prefix smallint,
    cid_bin blob,
    state tinyint,
    last_seen timestamp,
    PRIMARY KEY ((peer_id), mh_prefix, cid_bin)
) WITH compaction = {'class': 'LeveledCompactionStrategy'};

-- TTL queue for scheduled pin removal
CREATE TABLE IF NOT EXISTS pin_ttl_queue (
    ttl_bucket timestamp,
    cid_bin blob,
    owner text,
    ttl timestamp,
    PRIMARY KEY ((ttl_bucket), ttl, cid_bin)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'DAYS', 
    'compaction_window_size': '1'
};

-- Operation deduplication
CREATE TABLE IF NOT EXISTS op_dedup (
    op_id text,
    ts timestamp,
    PRIMARY KEY (op_id)
) WITH default_time_to_live = 604800;