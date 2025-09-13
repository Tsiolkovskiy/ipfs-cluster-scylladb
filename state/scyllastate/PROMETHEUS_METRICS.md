# Prometheus Metrics Implementation

This document describes the comprehensive Prometheus metrics implementation for the ScyllaDB state store in IPFS-Cluster.

## Overview

The metrics implementation provides detailed monitoring and observability for all ScyllaDB operations, connection health, and system performance. It satisfies requirements 5.1 and 5.3 for comprehensive monitoring and detailed performance metrics.

## Metrics Categories

### 1. Operation Metrics

**Latency Histograms:**
- `ipfs_cluster_scylladb_state_operation_duration_seconds`
  - Labels: `operation`, `consistency_level`
  - Buckets: 1ms to 10s (0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10)
  - Tracks duration of all ScyllaDB operations

**Operation Counters:**
- `ipfs_cluster_scylladb_state_operations_total`
  - Labels: `operation`, `status` (success/error)
  - Counts total operations by type and outcome

**Error Classification:**
- `ipfs_cluster_scylladb_state_operation_errors_total`
  - Labels: `operation`, `error_type`
  - Error types: timeout, unavailable, connection, authentication, authorization, syntax, invalid_request, overloaded, truncated, other

### 2. Connection Pool Metrics

**Active Connections:**
- `ipfs_cluster_scylladb_state_active_connections`
  - Current number of active connections to ScyllaDB cluster

**Connection Errors:**
- `ipfs_cluster_scylladb_state_connection_errors_total`
  - Total connection errors encountered

**Pool Status:**
- `ipfs_cluster_scylladb_state_connection_pool_status`
  - Labels: `host`, `datacenter`
  - Values: 1=healthy, 0=unhealthy

### 3. ScyllaDB-Specific Metrics

**Timeout Errors:**
- `ipfs_cluster_scylladb_state_scylla_timeouts_total`
  - Labels: `timeout_type`, `operation`
  - Timeout types: read_timeout, write_timeout, connect_timeout, request_timeout, general_timeout

**Unavailable Errors:**
- `ipfs_cluster_scylladb_state_scylla_unavailable_total`
  - Labels: `operation`, `consistency_level`
  - Tracks unavailable node errors

**Retry Operations:**
- `ipfs_cluster_scylladb_state_scylla_retries_total`
  - Labels: `operation`, `retry_reason`
  - Counts retry attempts with reasons

**Consistency Levels:**
- `ipfs_cluster_scylladb_state_scylla_consistency_level`
  - Labels: `operation_type`
  - Encoded consistency levels (ANY=0, ONE=1, QUORUM=4, etc.)

### 4. Query Performance Metrics

**Query Latency:**
- `ipfs_cluster_scylladb_state_query_latency_seconds`
  - Labels: `query_type`, `keyspace`
  - Buckets: 0.1ms to 1s (0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1)

**Prepared Statements:**
- `ipfs_cluster_scylladb_state_prepared_statements_cached`
  - Number of prepared statements currently cached

### 5. Batch Operation Metrics

**Batch Operations:**
- `ipfs_cluster_scylladb_state_batch_operations_total`
  - Labels: `batch_type`, `status`
  - Counts batch operations by type and outcome

**Batch Size:**
- `ipfs_cluster_scylladb_state_batch_size`
  - Labels: `batch_type`
  - Buckets: 1 to 5000 (1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000)

### 6. State Metrics

**Total Pins:**
- `ipfs_cluster_scylladb_state_total_pins`
  - Current total number of pins stored

**Pin Operations:**
- `ipfs_cluster_scylladb_state_pin_operations_total`
  - Labels: `operation_type`
  - Tracks pin-specific operations (pin, unpin, update)

### 7. Graceful Degradation Metrics

**Degradation Status:**
- `ipfs_cluster_scylladb_state_degradation_active`
  - Values: 1=active, 0=inactive

**Degradation Level:**
- `ipfs_cluster_scylladb_state_degradation_level`
  - Labels: `degradation_type`
  - Levels: 0=none, 1=reduced_consistency, 2=local_only, 3=read_only

**Node Health:**
- `ipfs_cluster_scylladb_state_node_health`
  - Labels: `host`, `datacenter`, `rack`
  - Values: 1=healthy, 0=unhealthy

**Partition Detection:**
- `ipfs_cluster_scylladb_state_partition_detected`
  - Values: 1=partitioned, 0=not_partitioned

## Usage Examples

### Recording Operations
```go
metrics := NewPrometheusMetrics()

// Record a successful operation
start := time.Now()
err := performScyllaOperation()
metrics.RecordOperation("add_pin", time.Since(start), "QUORUM", err)
```

### Recording Queries
```go
start := time.Now()
result := session.Query("SELECT * FROM pins_by_cid WHERE mh_prefix = ?", prefix).Exec()
metrics.RecordQuery("SELECT", "ipfs_pins", time.Since(start))
```

### Updating Connection Status
```go
poolStatus := map[string]bool{
    "scylla-node-1": true,
    "scylla-node-2": false,
    "scylla-node-3": true,
}
metrics.UpdateConnectionMetrics(activeConnections, poolStatus)
```

### Recording Batch Operations
```go
batchSize := 100
err := executeBatch(batch)
metrics.RecordBatchOperation("LOGGED", batchSize, err)
```

## Error Classification

The metrics system automatically classifies errors into categories:

- **timeout**: Connection, read, write, or request timeouts
- **unavailable**: Service unavailable errors
- **connection**: Connection-related errors
- **authentication**: Authentication failures
- **authorization**: Authorization failures
- **syntax**: CQL syntax errors
- **invalid_request**: Invalid request parameters
- **overloaded**: System overload errors
- **truncated**: Truncated response errors
- **other**: Unclassified errors

## Consistency Level Encoding

Consistency levels are encoded as integers for metrics:

| Level | Code |
|-------|------|
| ANY | 0 |
| ONE | 1 |
| TWO | 2 |
| THREE | 3 |
| QUORUM | 4 |
| ALL | 5 |
| LOCAL_QUORUM | 6 |
| EACH_QUORUM | 7 |
| LOCAL_ONE | 8 |
| SERIAL | 9 |
| LOCAL_SERIAL | 10 |

## Integration

The metrics are automatically integrated into the ScyllaState implementation and will be recorded for all operations when metrics are enabled in the configuration.

To enable metrics:
```json
{
  "scylladb": {
    "metrics_enabled": true
  }
}
```

## Requirements Satisfied

- ✅ **Requirement 5.1**: Comprehensive monitoring and observability for ScyllaDB operations
- ✅ **Requirement 5.3**: Detailed performance and error metrics for troubleshooting
- ✅ **Task 7.1**: Complete Prometheus metrics implementation with latency, throughput, error rates, connection pool status, and ScyllaDB-specific metrics

## Files

- `metrics.go`: Main metrics implementation
- `metrics_simple_test.go`: Unit tests for metrics functionality
- `validate_metrics.go`: Validation script for metrics implementation
- `PROMETHEUS_METRICS.md`: This documentation file