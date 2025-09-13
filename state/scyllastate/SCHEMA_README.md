# ScyllaDB Schema Documentation

This directory contains ScyllaDB schema definitions for the IPFS-Cluster pin state store. The schemas are optimized for trillion-scale pin management with different deployment scenarios.

## Schema Files

### 1. `schema.sql` - Default Schema
The main schema file optimized for general-purpose deployments.

**Features:**
- Optimized partitioning by `mh_prefix` (65536 partitions)
- Support for conditional updates and race condition prevention
- Comprehensive caching strategies
- Performance monitoring tables
- TTL-based automatic cleanup

**Use Cases:**
- Single datacenter deployments
- Development and testing environments
- Small to medium scale deployments (up to billions of pins)

### 2. `schema_multidc.sql` - Multi-Datacenter Schema
Enhanced schema for multi-datacenter deployments with cross-DC replication.

**Features:**
- NetworkTopologyStrategy with configurable DC replication
- DC-aware routing optimizations
- Cross-DC replication lag monitoring
- Datacenter health tracking
- Reduced cross-DC read repair overhead

**Use Cases:**
- Global deployments across multiple datacenters
- High availability requirements
- Disaster recovery scenarios
- Geographically distributed IPFS clusters

### 3. `schema_performance.sql` - High-Performance Schema
Optimized for maximum throughput and minimal latency.

**Features:**
- Aggressive caching and compression settings
- Larger SSTable sizes for better compression ratios
- Reduced GC grace periods for faster cleanup
- Speculative retry for improved read performance
- Streamlined tables with minimal overhead

**Use Cases:**
- High-throughput production environments
- Latency-sensitive applications
- Large-scale deployments (trillions of pins)
- Performance-critical scenarios

### 4. `migrations.sql` - Migration Scripts
Scripts for upgrading existing schemas and handling version compatibility.

**Features:**
- Safe column additions with ALTER TABLE
- New table creation with IF NOT EXISTS
- Performance optimization updates
- Version compatibility handling

**Use Cases:**
- Upgrading from older schema versions
- Adding new features to existing deployments
- Schema evolution without downtime

## Requirements Addressed

### Requirement 2.1: Pin Metadata Storage
- **CID Storage**: Binary multihash format for efficient storage
- **Replication Factor**: Configurable per pin
- **Timestamps**: Creation and update tracking
- **Placements**: Desired vs actual peer assignments

### Requirement 2.2: Consistent Read Semantics
- **Conditional Updates**: Version-based optimistic locking
- **Read Repair**: Configurable for consistency vs performance
- **Consistency Levels**: Support for QUORUM, LOCAL_QUORUM
- **Race Condition Prevention**: Atomic operations and versioning

### Requirement 7.3: Performance Optimization
- **Caching Strategies**: Comprehensive key and row caching
- **Partition Distribution**: Even distribution across 65536 partitions
- **Query Patterns**: Optimized for predictable access patterns
- **Batch Operations**: Support for efficient bulk operations

## Table Descriptions

### Core Tables

#### `pins_by_cid`
Primary table storing pin metadata, partitioned by `mh_prefix`.

**Key Columns:**
- `mh_prefix`: First 2 bytes of multihash for partitioning
- `cid_bin`: Binary CID representation
- `version`: For conditional updates
- `status`: Pin state tracking
- `priority`: Pin priority levels

#### `placements_by_cid`
Tracks desired vs actual pin placements across peers.

**Key Columns:**
- `desired`: Target peer set
- `actual`: Confirmed peer set
- `in_progress`: Currently pinning peers
- `drift_detected`: State inconsistency flag

#### `pins_by_peer`
Reverse index for efficient peer-based queries.

**Key Columns:**
- `peer_id`: Partition key for peer-specific queries
- `state`: Pin operation state
- `retry_count`: Failure tracking
- `error_message`: Detailed error information

### Supporting Tables

#### `pin_ttl_queue`
Time-based queue for automatic pin expiration.

#### `op_dedup`
Operation deduplication for idempotency.

#### `pin_stats`
Counters and statistics for monitoring.

#### `pin_events`
Audit log for pin operations.

#### `partition_stats`
Load balancing and hot partition detection.

#### `performance_metrics`
Time-series performance data.

#### `batch_operations`
Batch operation tracking and optimization.

## Partitioning Strategy

### mh_prefix Partitioning
The schema uses the first 2 bytes of the multihash digest as the partition key, providing:

- **65536 partitions** for optimal distribution
- **Even load distribution** across cluster nodes
- **Predictable routing** for token-aware clients
- **Hot partition avoidance** through hash-based distribution

### Partition Key Calculation
```go
func mhPrefix(cidBin []byte) int16 {
    if len(cidBin) < 2 {
        return 0
    }
    return int16(cidBin[0])<<8 | int16(cidBin[1])
}
```

## Performance Optimizations

### Compaction Strategies

#### LeveledCompactionStrategy
Used for frequently updated tables:
- `pins_by_cid`
- `placements_by_cid`
- `pins_by_peer`

**Benefits:**
- Predictable read performance
- Efficient space utilization
- Good for mixed read/write workloads

#### TimeWindowCompactionStrategy
Used for time-series data:
- `pin_ttl_queue`
- `pin_events`
- `performance_metrics`

**Benefits:**
- Automatic data aging
- Efficient for time-based queries
- Reduced compaction overhead

### Caching Configuration

#### Key Caching
All tables use `'keys': 'ALL'` for maximum key cache utilization.

#### Row Caching
Configured per table based on access patterns:
- `pins_by_cid`: 1000-2000 rows per partition
- `placements_by_cid`: 500-1000 rows per partition
- `pins_by_peer`: 200-500 rows per partition

### Compression Settings

#### LZ4Compressor
Fast compression for frequently accessed data:
- Low CPU overhead
- Good compression ratios
- Suitable for hot data

## Deployment Guidelines

### Single Datacenter
Use `schema.sql` with these settings:
```cql
CREATE KEYSPACE ipfs_pins
WITH replication = {
    'class': 'NetworkTopologyStrategy',
    'datacenter1': '3'
};
```

### Multi-Datacenter
Use `schema_multidc.sql` with appropriate replication:
```cql
CREATE KEYSPACE ipfs_pins
WITH replication = {
    'class': 'NetworkTopologyStrategy',
    'datacenter1': '3',
    'datacenter2': '3',
    'datacenter3': '2'
};
```

### High-Performance
Use `schema_performance.sql` for maximum throughput:
- Larger SSTable sizes (320MB)
- Aggressive caching settings
- Reduced GC grace periods
- Speculative retry enabled

## Migration Process

1. **Backup existing data** before migration
2. **Run migration scripts** during maintenance window
3. **Verify schema changes** with DESCRIBE commands
4. **Test application compatibility** with new schema
5. **Monitor performance** after migration

### Example Migration Command
```bash
cqlsh -f migrations.sql
```

## Monitoring and Maintenance

### Key Metrics to Monitor
- Partition distribution across `mh_prefix` values
- Read/write latencies per table
- Compaction performance
- Cache hit rates
- Cross-DC replication lag (multi-DC deployments)

### Maintenance Tasks
- Regular compaction monitoring
- Partition hotspot detection
- TTL queue processing verification
- Performance metrics analysis
- Schema optimization based on usage patterns

## Troubleshooting

### Hot Partitions
Monitor `partition_stats` table for uneven distribution:
```cql
SELECT mh_prefix, stat_value 
FROM partition_stats 
WHERE stat_type = 'pin_count' 
ORDER BY stat_value DESC 
LIMIT 10;
```

### Performance Issues
Check `performance_metrics` for bottlenecks:
```cql
SELECT metric_name, AVG(metric_value) as avg_value
FROM performance_metrics 
WHERE time_bucket > '2024-01-01' 
GROUP BY metric_name;
```

### Replication Lag (Multi-DC)
Monitor cross-DC lag:
```cql
SELECT source_dc, target_dc, table_name, lag_ms
FROM replication_lag
WHERE lag_ms > 1000;
```

## Best Practices

1. **Use prepared statements** for all queries
2. **Batch operations** when possible for better throughput
3. **Monitor partition distribution** regularly
4. **Tune consistency levels** based on requirements
5. **Use token-aware routing** in client applications
6. **Regular schema maintenance** and optimization
7. **Proper TTL configuration** for automatic cleanup
8. **Monitor and alert** on key performance metrics