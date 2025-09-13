# ScyllaDB Performance Tuning Guide

This guide provides recommendations for optimizing ScyllaDB performance for the IPFS-Cluster pin state store.

## Table-Specific Optimizations

### pins_by_cid (Main Table)

#### Compaction Settings
```cql
ALTER TABLE ipfs_pins.pins_by_cid 
WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '320',        -- Larger for better compression
    'tombstone_threshold': '0.1'        -- Aggressive tombstone cleanup
};
```

#### Caching Optimization
```cql
ALTER TABLE ipfs_pins.pins_by_cid 
WITH caching = {
    'keys': 'ALL',                      -- Cache all partition keys
    'rows_per_partition': '2000'        -- Increase for hot partitions
};
```

#### Bloom Filter Tuning
```cql
ALTER TABLE ipfs_pins.pins_by_cid 
WITH bloom_filter_fp_chance = 0.001;   -- Lower false positive rate
```

### placements_by_cid (Placement Tracking)

#### Read Repair Configuration
```cql
ALTER TABLE ipfs_pins.placements_by_cid 
WITH read_repair_chance = 0.05         -- Reduced for performance
AND dclocal_read_repair_chance = 0.05;
```

#### Speculative Retry
```cql
ALTER TABLE ipfs_pins.placements_by_cid 
WITH speculative_retry = '95percentile'; -- Aggressive retry
```

### pins_by_peer (Worker Queries)

#### Memtable Flush Optimization
```cql
ALTER TABLE ipfs_pins.pins_by_peer 
WITH memtable_flush_period_in_ms = 1800000; -- 30 minutes
```

#### Compression Tuning
```cql
ALTER TABLE ipfs_pins.pins_by_peer 
WITH compression = {
    'class': 'LZ4Compressor',
    'chunk_length_in_kb': '16'          -- Optimized chunk size
};
```

## Cluster-Level Optimizations

### Connection Pool Settings

#### Go Client (gocql) Configuration
```go
cluster := gocql.NewCluster(hosts...)
cluster.NumConns = 10                   // Connections per host
cluster.Timeout = 30 * time.Second
cluster.ConnectTimeout = 10 * time.Second
cluster.SocketKeepalive = 60 * time.Second
cluster.MaxPreparedStmts = 1000         // Cache prepared statements
cluster.MaxRoutingKeyInfo = 1000        // Token-aware routing cache
```

#### Connection Pool Sizing
- **Small clusters (3-6 nodes)**: 5-10 connections per host
- **Medium clusters (6-20 nodes)**: 3-5 connections per host  
- **Large clusters (20+ nodes)**: 2-3 connections per host

### Consistency Level Optimization

#### Read Operations
```go
// For read-heavy workloads
session.Query("SELECT ...").Consistency(gocql.LocalOne)

// For consistency-critical reads
session.Query("SELECT ...").Consistency(gocql.LocalQuorum)
```

#### Write Operations
```go
// For write-heavy workloads
session.Query("INSERT ...").Consistency(gocql.LocalOne)

// For consistency-critical writes
session.Query("INSERT ...").Consistency(gocql.LocalQuorum)
```

### Batch Operation Optimization

#### Batch Size Guidelines
- **Small batches**: 10-50 operations for low latency
- **Medium batches**: 50-200 operations for balanced throughput
- **Large batches**: 200-1000 operations for maximum throughput

#### Batch Configuration
```go
batch := session.NewBatch(gocql.LoggedBatch)
batch.Cons = gocql.LocalQuorum
batch.DefaultTimestamp = true

// Add operations to batch
for _, pin := range pins {
    batch.Query("INSERT INTO pins_by_cid ...", pin.Args()...)
}

// Execute batch
err := session.ExecuteBatch(batch)
```

## Partitioning Optimization

### Hot Partition Detection

#### Monitor Partition Distribution
```cql
-- Check partition sizes
SELECT mh_prefix, COUNT(*) as pin_count
FROM ipfs_pins.pins_by_cid
GROUP BY mh_prefix
ORDER BY pin_count DESC
LIMIT 20;
```

#### Partition Statistics Table
```cql
-- Update partition stats regularly
INSERT INTO ipfs_pins.partition_stats (mh_prefix, stat_type, stat_value, updated_at)
VALUES (?, 'pin_count', ?, toTimestamp(now()));

INSERT INTO ipfs_pins.partition_stats (mh_prefix, stat_type, stat_value, updated_at)
VALUES (?, 'total_size', ?, toTimestamp(now()));
```

### Load Balancing Strategies

#### Token-Aware Routing
```go
cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(
    gocql.DCAwareRoundRobinPolicy("datacenter1"),
)
```

#### Partition Key Distribution
```go
// Ensure even distribution of mh_prefix values
func validatePartitionDistribution(cidBin []byte) bool {
    prefix := mhPrefix(cidBin)
    // Check if prefix is in acceptable range
    return prefix >= 0 && prefix <= 65535
}
```

## Memory and CPU Optimization

### ScyllaDB Server Settings

#### Memory Configuration
```yaml
# scylla.yaml
memory_allocator: seastar
# Use 80% of available memory for ScyllaDB
```

#### CPU Configuration
```yaml
# Use all available CPU cores
cpuset: "0-15"  # Adjust based on server specs
```

#### I/O Configuration
```yaml
# Optimize for SSD storage
io_properties_file: /etc/scylla.d/io_properties.yaml
```

### Client-Side Optimization

#### Connection Pooling
```go
// Pool configuration
type PoolConfig struct {
    MaxConns        int           // Maximum connections per host
    MaxIdleConns    int           // Maximum idle connections
    ConnMaxLifetime time.Duration // Connection lifetime
    ConnMaxIdleTime time.Duration // Idle connection timeout
}
```

#### Query Preparation
```go
// Prepare frequently used queries
var (
    insertPinStmt    *gocql.Query
    selectPinStmt    *gocql.Query
    updatePinStmt    *gocql.Query
    deletePinStmt    *gocql.Query
)

func prepareStatements(session *gocql.Session) error {
    insertPinStmt = session.Query(`
        INSERT INTO pins_by_cid (mh_prefix, cid_bin, ...) 
        VALUES (?, ?, ...)
    `)
    // Prepare other statements...
    return nil
}
```

## Monitoring and Alerting

### Key Performance Metrics

#### Latency Metrics
- p95/p99 read latency per table
- p95/p99 write latency per table
- Query timeout rates

#### Throughput Metrics
- Operations per second per table
- Batch operation throughput
- Connection pool utilization

#### Resource Metrics
- CPU utilization per core
- Memory usage and allocation
- Disk I/O and space utilization
- Network bandwidth usage

### Prometheus Metrics Configuration

#### ScyllaDB Exporter
```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'scylladb'
    static_configs:
      - targets: ['scylla1:9180', 'scylla2:9180', 'scylla3:9180']
    scrape_interval: 15s
```

#### Application Metrics
```go
// Custom metrics for IPFS-Cluster
var (
    pinOperationDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "ipfs_pin_operation_duration_seconds",
            Help: "Duration of pin operations",
        },
        []string{"operation", "status"},
    )
    
    partitionHotness = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "ipfs_partition_hotness_ratio",
            Help: "Partition hotness ratio",
        },
        []string{"mh_prefix"},
    )
)
```

## Troubleshooting Performance Issues

### Common Issues and Solutions

#### High Read Latency
1. **Check bloom filter settings**: Lower `bloom_filter_fp_chance`
2. **Increase caching**: Higher `rows_per_partition` values
3. **Optimize queries**: Use partition key in WHERE clauses
4. **Check compaction**: Monitor compaction backlog

#### High Write Latency
1. **Batch operations**: Group multiple writes
2. **Reduce consistency**: Use `LocalOne` for non-critical writes
3. **Optimize memtable**: Adjust flush periods
4. **Check disk I/O**: Monitor disk utilization

#### Hot Partitions
1. **Monitor distribution**: Use `partition_stats` table
2. **Implement backpressure**: Rate limit hot partitions
3. **Consider resharding**: If consistently hot
4. **Load balancing**: Distribute load across time

#### Memory Issues
1. **Monitor heap usage**: Check ScyllaDB memory metrics
2. **Optimize caching**: Reduce cache sizes if needed
3. **Compaction tuning**: Adjust compaction strategies
4. **Query optimization**: Avoid large result sets

### Performance Testing

#### Load Testing Script
```bash
#!/bin/bash
# Load test with cassandra-stress
cassandra-stress write n=1000000 \
  -schema 'replication(factor=3)' \
  -node scylla1,scylla2,scylla3 \
  -rate threads=50 \
  -log file=stress_write.log

cassandra-stress read n=1000000 \
  -node scylla1,scylla2,scylla3 \
  -rate threads=50 \
  -log file=stress_read.log
```

#### Benchmark Queries
```cql
-- Test partition key performance
SELECT * FROM pins_by_cid WHERE mh_prefix = ? LIMIT 1000;

-- Test peer queries
SELECT * FROM pins_by_peer WHERE peer_id = ? LIMIT 1000;

-- Test placement queries
SELECT * FROM placements_by_cid WHERE mh_prefix = ? LIMIT 1000;
```

## Best Practices Summary

1. **Use prepared statements** for all repeated queries
2. **Batch operations** when possible, but keep batches reasonable
3. **Monitor partition distribution** and watch for hot partitions
4. **Tune consistency levels** based on application requirements
5. **Optimize caching** for frequently accessed data
6. **Use token-aware routing** for better performance
7. **Monitor key metrics** and set up alerting
8. **Regular performance testing** and optimization
9. **Keep schemas simple** and avoid secondary indexes
10. **Plan for growth** and monitor resource utilization