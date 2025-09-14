# ScyllaDB State Store for IPFS-Cluster

The ScyllaDB state store provides a high-performance, distributed backend for IPFS-Cluster state management using ScyllaDB as the underlying database. This implementation offers superior scalability, fault tolerance, and performance compared to traditional file-based state stores.

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Configuration](#configuration)
- [Multi-Datacenter Setup](#multi-datacenter-setup)
- [Performance Tuning](#performance-tuning)
- [Monitoring](#monitoring)
- [Migration](#migration)
- [Troubleshooting](#troubleshooting)
- [Examples](#examples)

## Overview

The ScyllaDB state store implements the IPFS-Cluster `state.State` interface using ScyllaDB as the backend database. It provides:

- **High Performance**: Optimized for high-throughput pin operations with sub-millisecond latencies
- **Horizontal Scalability**: Scales seamlessly across multiple nodes and datacenters
- **Fault Tolerance**: Built-in replication and automatic failover capabilities
- **Consistency Options**: Configurable consistency levels (ONE, QUORUM, ALL)
- **Graceful Degradation**: Continues operating during node failures and network partitions

## Features

### Core Features
- ✅ Full `state.State` interface implementation
- ✅ Batch operations for high-throughput scenarios
- ✅ Automatic retry policies with exponential backoff
- ✅ Connection pooling and prepared statement caching
- ✅ Token-aware and datacenter-aware routing
- ✅ TLS encryption and authentication support

### Advanced Features
- ✅ Graceful degradation during failures
- ✅ Prometheus metrics integration
- ✅ Structured logging with operation tracing
- ✅ Data migration utilities
- ✅ Multi-datacenter deployment support
- ✅ Comprehensive testing suite

## Prerequisites

### ScyllaDB Requirements
- **ScyllaDB Version**: 4.0 or later (5.x recommended)
- **Minimum Nodes**: 1 (3+ recommended for production)
- **Memory**: 4GB+ per node (16GB+ recommended)
- **Storage**: SSD storage recommended for optimal performance

### Network Requirements
- **Ports**: 9042 (CQL), 7000 (inter-node), 7001 (TLS inter-node)
- **Bandwidth**: 1Gbps+ recommended for multi-node clusters
- **Latency**: <10ms between nodes in same datacenter

### Go Requirements
- **Go Version**: 1.19 or later
- **Dependencies**: Automatically managed via Go modules

## Installation

### 1. Install ScyllaDB

#### Using Docker (Development)
```bash
# Single node for development
docker run -d --name scylla \
  -p 9042:9042 \
  scylladb/scylla:5.2 \
  --smp 2 --memory 2G --overprovisioned 1

# Wait for ScyllaDB to start
docker logs -f scylla
```

#### Using Docker Compose (Multi-node)
```yaml
version: '3.8'
services:
  scylla-node1:
    image: scylladb/scylla:5.2
    container_name: scylla-node1
    command: --seeds=scylla-node1 --smp 2 --memory 2G --overprovisioned 1
    ports:
      - "9042:9042"
    networks:
      - scylla-net

  scylla-node2:
    image: scylladb/scylla:5.2
    container_name: scylla-node2
    command: --seeds=scylla-node1 --smp 2 --memory 2G --overprovisioned 1
    ports:
      - "9043:9042"
    networks:
      - scylla-net
    depends_on:
      - scylla-node1

  scylla-node3:
    image: scylladb/scylla:5.2
    container_name: scylla-node3
    command: --seeds=scylla-node1 --smp 2 --memory 2G --overprovisioned 1
    ports:
      - "9044:9042"
    networks:
      - scylla-net
    depends_on:
      - scylla-node1

networks:
  scylla-net:
    driver: bridge
```

#### Production Installation
Follow the [official ScyllaDB installation guide](https://docs.scylladb.com/stable/getting-started/) for your platform.

### 2. Add to IPFS-Cluster

Add the ScyllaDB state store dependency to your IPFS-Cluster project:

```bash
go get github.com/ipfs-cluster/ipfs-cluster/state/scyllastate
```

## Configuration

### Basic Configuration

The ScyllaDB state store is configured through the `Config` structure:

```go
package main

import (
    "context"
    "log"
    
    "github.com/ipfs-cluster/ipfs-cluster/state/scyllastate"
)

func main() {
    config := &scyllastate.Config{
        Hosts:            []string{"127.0.0.1"},
        Port:             9042,
        Keyspace:         "ipfs_pins",
        Consistency:      "QUORUM",
        Timeout:          10 * time.Second,
        ConnectTimeout:   5 * time.Second,
        NumConns:         4,
    }
    
    state, err := scyllastate.New(context.Background(), config)
    if err != nil {
        log.Fatal(err)
    }
    defer state.Close()
}
```

### Configuration Options

#### Connection Settings
```go
type Config struct {
    // Basic connection settings
    Hosts            []string      `json:"hosts"`              // ScyllaDB node addresses
    Port             int           `json:"port"`               // CQL port (default: 9042)
    Keyspace         string        `json:"keyspace"`           // Keyspace name
    Username         string        `json:"username"`           // Authentication username
    Password         string        `json:"password"`           // Authentication password
    
    // Timeout settings
    Timeout          time.Duration `json:"timeout"`            // Query timeout
    ConnectTimeout   time.Duration `json:"connect_timeout"`    // Connection timeout
    
    // Connection pool settings
    NumConns         int           `json:"num_conns"`          // Connections per host
    MaxWaitTime      time.Duration `json:"max_wait_time"`      // Max wait for connection
    
    // Consistency settings
    Consistency      string        `json:"consistency"`        // Consistency level
    SerialConsistency string       `json:"serial_consistency"` // Serial consistency level
}
```

#### Performance Settings
```go
// Performance optimization settings
config.TokenAwareRouting = true          // Enable token-aware routing
config.DCAwareRouting = true             // Enable datacenter-aware routing
config.LocalDC = "datacenter1"           // Local datacenter name
config.PageSize = 5000                   // Query page size
config.PreparedStatementCacheSize = 1000 // Prepared statement cache size
```

#### TLS Configuration
```go
// TLS settings
config.TLS = &TLSConfig{
    Enabled:            true,
    CertFile:           "/path/to/cert.pem",
    KeyFile:            "/path/to/key.pem",
    CAFile:             "/path/to/ca.pem",
    InsecureSkipVerify: false,
}
```

#### Retry Policy Configuration
```go
// Retry policy settings
config.RetryPolicy = &RetryPolicyConfig{
    NumRetries:      3,
    MinRetryDelay:   100 * time.Millisecond,
    MaxRetryDelay:   10 * time.Second,
    RetryableErrors: []string{"timeout", "unavailable"},
}
```

### Configuration File Example

Create a configuration file `scylla-config.json`:

```json
{
  "hosts": ["10.0.1.10", "10.0.1.11", "10.0.1.12"],
  "port": 9042,
  "keyspace": "ipfs_pins_prod",
  "username": "ipfs_user",
  "password": "secure_password",
  "timeout": "10s",
  "connect_timeout": "5s",
  "num_conns": 4,
  "consistency": "QUORUM",
  "serial_consistency": "SERIAL",
  "token_aware_routing": true,
  "dc_aware_routing": true,
  "local_dc": "datacenter1",
  "page_size": 5000,
  "prepared_statement_cache_size": 1000,
  "tls": {
    "enabled": true,
    "cert_file": "/etc/ssl/certs/scylla-client.pem",
    "key_file": "/etc/ssl/private/scylla-client-key.pem",
    "ca_file": "/etc/ssl/certs/scylla-ca.pem",
    "insecure_skip_verify": false
  },
  "retry_policy": {
    "num_retries": 3,
    "min_retry_delay": "100ms",
    "max_retry_delay": "10s",
    "retryable_errors": ["timeout", "unavailable", "overloaded"]
  },
  "metrics_enabled": true,
  "tracing_enabled": false
}
```

Load configuration from file:

```go
func loadConfig(filename string) (*scyllastate.Config, error) {
    data, err := ioutil.ReadFile(filename)
    if err != nil {
        return nil, err
    }
    
    config := &scyllastate.Config{}
    err = json.Unmarshal(data, config)
    if err != nil {
        return nil, err
    }
    
    // Apply defaults
    config.Default()
    
    return config, nil
}
```

## Multi-Datacenter Setup

### Datacenter Configuration

For multi-datacenter deployments, configure datacenter-aware routing:

```go
config := &scyllastate.Config{
    Hosts: []string{
        "dc1-node1.example.com",
        "dc1-node2.example.com", 
        "dc2-node1.example.com",
        "dc2-node2.example.com",
    },
    DCAwareRouting: true,
    LocalDC:        "datacenter1",
    Consistency:    "LOCAL_QUORUM",
}
```

### Keyspace Creation for Multi-DC

Create keyspace with NetworkTopologyStrategy:

```cql
CREATE KEYSPACE ipfs_pins_multidc 
WITH REPLICATION = {
    'class': 'NetworkTopologyStrategy',
    'datacenter1': 3,
    'datacenter2': 3
};
```

### Cross-DC Consistency Levels

Choose appropriate consistency levels for multi-DC:

- **LOCAL_QUORUM**: Recommended for most operations
- **EACH_QUORUM**: For critical operations requiring cross-DC consistency
- **LOCAL_ONE**: For high-performance, eventually consistent operations

```go
// Different consistency for different operations
config.Consistency = "LOCAL_QUORUM"        // Default
config.SerialConsistency = "LOCAL_SERIAL"  // For lightweight transactions
```

## Performance Tuning

### Connection Pool Tuning

Optimize connection pools based on workload:

```go
// High-throughput configuration
config.NumConns = 8                    // More connections per host
config.MaxWaitTime = 1 * time.Second   // Shorter wait time
config.Timeout = 5 * time.Second       // Aggressive timeouts
```

### Query Optimization

Configure query parameters for optimal performance:

```go
config.PageSize = 10000                           // Larger page size for bulk operations
config.PreparedStatementCacheSize = 2000         // More cached statements
config.TokenAwareRouting = true                  // Enable token-aware routing
```

### Batch Operation Tuning

Optimize batch operations:

```go
// Use batch operations for bulk inserts
batchState := state.Batch()

// Add multiple pins
for _, pin := range pins {
    batchState.Add(ctx, pin)
}

// Commit in optimal batch sizes (100-1000 operations)
batchState.Commit(ctx)
```

### Memory and Resource Tuning

ScyllaDB-side optimizations:

```bash
# ScyllaDB configuration optimizations
# /etc/scylla/scylla.yaml

# Memory settings
memory_allocator: seastar
commitlog_total_space_in_mb: 2048

# Compaction settings
compaction_throughput_mb_per_sec: 256
concurrent_compactors: 4

# Cache settings
row_cache_size_in_mb: 1024
key_cache_size_in_mb: 512
```

## Monitoring

### Prometheus Metrics

Enable Prometheus metrics collection:

```go
config.MetricsEnabled = true
```

Key metrics to monitor:

- `scylla_state_operations_total` - Total operations by type
- `scylla_state_operation_duration_seconds` - Operation latency
- `scylla_state_errors_total` - Error count by type
- `scylla_state_connections_active` - Active connections
- `scylla_state_batch_size` - Batch operation sizes

### Grafana Dashboard

Example Grafana queries:

```promql
# Operations per second
rate(scylla_state_operations_total[5m])

# Average latency
rate(scylla_state_operation_duration_seconds_sum[5m]) / 
rate(scylla_state_operation_duration_seconds_count[5m])

# Error rate
rate(scylla_state_errors_total[5m]) / 
rate(scylla_state_operations_total[5m])
```

### Logging Configuration

Configure structured logging:

```go
config.LogLevel = "info"        // Log level: debug, info, warn, error
config.TracingEnabled = true    // Enable operation tracing
```

Log output example:
```json
{
  "timestamp": "2024-01-15T10:30:45Z",
  "level": "info",
  "operation": "add",
  "cid": "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
  "duration": "15ms",
  "consistency": "QUORUM",
  "success": true
}
```

## Migration

### From File-based State Store

Migrate from existing file-based state store:

```go
package main

import (
    "context"
    "log"
    
    "github.com/ipfs-cluster/ipfs-cluster/state/dsstate"
    "github.com/ipfs-cluster/ipfs-cluster/state/scyllastate"
    "github.com/ipfs-cluster/ipfs-cluster/datastore/badger"
)

func migrateFromFileStore() error {
    // Open existing file-based state
    ds, err := badger.New("/path/to/existing/state")
    if err != nil {
        return err
    }
    
    sourceState, err := dsstate.New(context.Background(), ds, "", nil)
    if err != nil {
        return err
    }
    defer sourceState.Close()
    
    // Create ScyllaDB state
    config := &scyllastate.Config{
        Hosts:       []string{"127.0.0.1"},
        Keyspace:    "ipfs_pins",
        Consistency: "QUORUM",
    }
    config.Default()
    
    destState, err := scyllastate.New(context.Background(), config)
    if err != nil {
        return err
    }
    defer destState.Close()
    
    // Perform migration
    var buf bytes.Buffer
    
    // Export from source
    err = sourceState.Marshal(&buf)
    if err != nil {
        return err
    }
    
    // Import to destination
    err = destState.Unmarshal(&buf)
    if err != nil {
        return err
    }
    
    log.Println("Migration completed successfully")
    return nil
}
```

### Migration Utilities

Use the built-in migration utilities:

```bash
# Export existing state
ipfs-cluster-service state export --format=json > state-backup.json

# Import to ScyllaDB
ipfs-cluster-service state import --backend=scylladb state-backup.json
```

### Large Dataset Migration

For large datasets, use batch migration:

```go
func migrateLargeDataset(source state.State, dest *scyllastate.ScyllaState) error {
    ctx := context.Background()
    pinChan := make(chan api.Pin, 1000)
    
    // Start reading from source
    go func() {
        defer close(pinChan)
        source.List(ctx, pinChan)
    }()
    
    // Batch write to destination
    batchState := dest.Batch()
    batchSize := 0
    
    for pin := range pinChan {
        err := batchState.Add(ctx, pin)
        if err != nil {
            return err
        }
        
        batchSize++
        
        // Commit every 1000 pins
        if batchSize >= 1000 {
            err = batchState.Commit(ctx)
            if err != nil {
                return err
            }
            batchSize = 0
            log.Printf("Migrated %d pins", batchSize)
        }
    }
    
    // Commit remaining pins
    if batchSize > 0 {
        return batchState.Commit(ctx)
    }
    
    return nil
}
```

## Troubleshooting

### Common Issues

#### Connection Issues

**Problem**: Cannot connect to ScyllaDB
```
Error: failed to create ScyllaDB session: gocql: no hosts available
```

**Solutions**:
1. Verify ScyllaDB is running: `docker ps` or `systemctl status scylla-server`
2. Check network connectivity: `telnet <host> 9042`
3. Verify firewall settings
4. Check authentication credentials

#### Performance Issues

**Problem**: Slow query performance
```
Warning: Query timeout after 10s
```

**Solutions**:
1. Increase timeout values in configuration
2. Check ScyllaDB node health and resources
3. Optimize query patterns (avoid large scans)
4. Enable token-aware routing
5. Monitor ScyllaDB metrics

#### Consistency Issues

**Problem**: Inconsistent read results
```
Error: Data inconsistency detected
```

**Solutions**:
1. Use higher consistency levels (QUORUM, ALL)
2. Check node synchronization
3. Verify replication factor settings
4. Monitor repair operations

### Debug Mode

Enable debug logging for troubleshooting:

```go
config.LogLevel = "debug"
config.TracingEnabled = true
```

### Health Checks

Implement health checks:

```go
func healthCheck(state *scyllastate.ScyllaState) error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    // Test basic connectivity
    testPin := createTestPin()
    
    // Test write
    err := state.Add(ctx, testPin)
    if err != nil {
        return fmt.Errorf("write test failed: %w", err)
    }
    
    // Test read
    _, err = state.Get(ctx, testPin.Cid)
    if err != nil {
        return fmt.Errorf("read test failed: %w", err)
    }
    
    // Cleanup
    state.Rm(ctx, testPin.Cid)
    
    return nil
}
```

### Performance Monitoring

Monitor key performance indicators:

```bash
# ScyllaDB metrics
nodetool status
nodetool tpstats
nodetool cfstats

# System metrics
iostat -x 1
sar -u 1
free -h
```

## Examples

### Basic Usage Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"
    
    "github.com/ipfs-cluster/ipfs-cluster/api"
    "github.com/ipfs-cluster/ipfs-cluster/state/scyllastate"
)

func main() {
    // Configure ScyllaDB connection
    config := &scyllastate.Config{
        Hosts:       []string{"127.0.0.1"},
        Port:        9042,
        Keyspace:    "ipfs_pins",
        Consistency: "QUORUM",
        Timeout:     10 * time.Second,
    }
    config.Default()
    
    // Create state store
    ctx := context.Background()
    state, err := scyllastate.New(ctx, config)
    if err != nil {
        log.Fatal("Failed to create state store:", err)
    }
    defer state.Close()
    
    // Create a test pin
    cid, _ := api.DecodeCid("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
    pin := api.Pin{
        Cid:  cid,
        Type: api.DataType,
        Allocations: []api.PeerID{
            api.PeerID("peer1"),
            api.PeerID("peer2"),
        },
        ReplicationFactorMin: 2,
        ReplicationFactorMax: 3,
        PinOptions: api.PinOptions{
            Name: "example-pin",
        },
    }
    
    // Add pin
    err = state.Add(ctx, pin)
    if err != nil {
        log.Fatal("Failed to add pin:", err)
    }
    fmt.Println("Pin added successfully")
    
    // Get pin
    retrievedPin, err := state.Get(ctx, cid)
    if err != nil {
        log.Fatal("Failed to get pin:", err)
    }
    fmt.Printf("Retrieved pin: %s\n", retrievedPin.PinOptions.Name)
    
    // List all pins
    pinChan := make(chan api.Pin, 100)
    go func() {
        defer close(pinChan)
        err := state.List(ctx, pinChan)
        if err != nil {
            log.Printf("Failed to list pins: %v", err)
        }
    }()
    
    fmt.Println("All pins:")
    for pin := range pinChan {
        fmt.Printf("  - %s: %s\n", pin.Cid, pin.PinOptions.Name)
    }
    
    // Remove pin
    err = state.Rm(ctx, cid)
    if err != nil {
        log.Fatal("Failed to remove pin:", err)
    }
    fmt.Println("Pin removed successfully")
}
```

### Batch Operations Example

```go
func batchOperationsExample(state *scyllastate.ScyllaState) error {
    ctx := context.Background()
    
    // Create batch state
    batchState := state.Batch()
    
    // Add multiple pins in batch
    for i := 0; i < 1000; i++ {
        cid, _ := api.DecodeCid(fmt.Sprintf("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPb%04d", i))
        pin := api.Pin{
            Cid:  cid,
            Type: api.DataType,
            Allocations: []api.PeerID{
                api.PeerID("peer1"),
            },
            ReplicationFactorMin: 1,
            ReplicationFactorMax: 2,
            PinOptions: api.PinOptions{
                Name: fmt.Sprintf("batch-pin-%d", i),
            },
        }
        
        err := batchState.Add(ctx, pin)
        if err != nil {
            return fmt.Errorf("failed to add pin to batch: %w", err)
        }
        
        // Commit every 100 operations
        if (i+1)%100 == 0 {
            err = batchState.Commit(ctx)
            if err != nil {
                return fmt.Errorf("failed to commit batch: %w", err)
            }
            fmt.Printf("Committed batch %d\n", (i+1)/100)
        }
    }
    
    return nil
}
```

### High Availability Setup Example

```go
func highAvailabilityExample() {
    config := &scyllastate.Config{
        Hosts: []string{
            "scylla-node1.example.com",
            "scylla-node2.example.com",
            "scylla-node3.example.com",
        },
        Port:              9042,
        Keyspace:          "ipfs_pins_ha",
        Consistency:       "QUORUM",
        TokenAwareRouting: true,
        DCAwareRouting:    true,
        LocalDC:           "datacenter1",
        
        // Retry configuration for resilience
        RetryPolicy: &scyllastate.RetryPolicyConfig{
            NumRetries:    5,
            MinRetryDelay: 100 * time.Millisecond,
            MaxRetryDelay: 30 * time.Second,
        },
        
        // Connection pool for high availability
        NumConns:    6,
        MaxWaitTime: 2 * time.Second,
        
        // Timeouts
        Timeout:        15 * time.Second,
        ConnectTimeout: 10 * time.Second,
    }
    
    config.Default()
    
    ctx := context.Background()
    state, err := scyllastate.New(ctx, config)
    if err != nil {
        log.Fatal("Failed to create HA state store:", err)
    }
    defer state.Close()
    
    // The state store will automatically handle node failures
    // and route requests to available nodes
}
```

For more examples and advanced usage patterns, see the [examples directory](./examples/) and [test files](./integration_test.go).

## Support

- **Documentation**: [ScyllaDB Documentation](https://docs.scylladb.com/)
- **Community**: [ScyllaDB Community Forum](https://forum.scylladb.com/)
- **Issues**: [GitHub Issues](https://github.com/ipfs-cluster/ipfs-cluster/issues)

## License

This project is licensed under the same license as IPFS-Cluster.