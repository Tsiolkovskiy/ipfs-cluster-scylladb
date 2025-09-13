# ScyllaDB State Store for IPFS-Cluster

This package implements a ScyllaDB-based state store for IPFS-Cluster, enabling management of trillion-scale pin sets with high performance and availability.

## Overview

The ScyllaDB state store replaces traditional consensus-based storage (etcd/Consul/CRDT) with a distributed, linearly scalable database optimized for high-throughput pin metadata operations.

### Key Features

- **Trillion-scale capacity**: Handles 10^12+ pins with linear scalability
- **High performance**: Optimized for low-latency pin operations
- **Multi-datacenter**: Built-in support for geographic distribution
- **Fault tolerance**: Automatic failover and recovery
- **IPFS-Cluster compatibility**: Drop-in replacement for existing state backends

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  IPFS-Cluster   │    │  IPFS-Cluster   │    │  IPFS-Cluster   │
│     Node 1      │    │     Node 2      │    │     Node N      │
│                 │    │                 │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │ScyllaDB     │ │    │ │ScyllaDB     │ │    │ │ScyllaDB     │ │
│ │State Store  │ │    │ │State Store  │ │    │ │State Store  │ │
│ └─────────────┘ │    │ └─────────────┘ │    │ └─────────────┘ │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                    ┌─────────────────────────┐
                    │    ScyllaDB Cluster     │
                    │                         │
                    │  ┌─────┐ ┌─────┐ ┌─────┐│
                    │  │Node1│ │Node2│ │NodeN││
                    │  └─────┘ └─────┘ └─────┘│
                    └─────────────────────────┘
```

## Data Model

The state store uses a denormalized data model optimized for ScyllaDB:

### Tables

1. **pins_by_cid**: Main pin metadata (source of truth)
2. **placements_by_cid**: Desired vs actual peer assignments
3. **pins_by_peer**: Reverse index for peer-specific queries
4. **pin_ttl_queue**: TTL-based cleanup queue
5. **op_dedup**: Operation deduplication for idempotency
6. **pin_stats**: Statistics and counters
7. **pin_events**: Audit log and debugging

### Partitioning Strategy

- **Primary partitioning**: By `mh_prefix` (first 2 bytes of multihash)
- **Even distribution**: 65,536 partitions for uniform load
- **Co-location**: Related tables use same partitioning key

## Setup

### Prerequisites

- ScyllaDB cluster (3+ nodes recommended)
- Go 1.21+
- IPFS-Cluster

### ScyllaDB Setup

1. **Install ScyllaDB** following the [official documentation](https://docs.scylladb.com/stable/getting-started/)

2. **Create keyspace and tables**:
   ```bash
   cqlsh -f state/scyllastate/schema.sql
   ```

3. **Configure replication** for your datacenter topology:
   ```cql
   ALTER KEYSPACE ipfs_pins 
   WITH replication = {
     'class': 'NetworkTopologyStrategy', 
     'dc1': '3', 
     'dc2': '3'
   };
   ```

### IPFS-Cluster Configuration

Add ScyllaDB configuration to your cluster config:

```json
{
  "state": {
    "datastore": "scylladb"
  },
  "scylladb": {
    "hosts": ["scylla1.example.com", "scylla2.example.com", "scylla3.example.com"],
    "port": 9042,
    "keyspace": "ipfs_pins",
    "local_dc": "dc1",
    "consistency": "LOCAL_QUORUM",
    "timeout": "30s",
    "connect_timeout": "10s",
    "num_conns": 10,
    "retry_policy": {
      "num_retries": 3,
      "min_retry_delay": "100ms",
      "max_retry_delay": "10s"
    }
  }
}
```

## Usage

### Basic Operations

```go
import "github.com/ipfs-cluster/ipfs-cluster/state/scyllastate"

// Create store
store, err := scyllastate.New(ctx, config)
if err != nil {
    log.Fatal(err)
}

// Add a pin
spec := scyllastate.PinSpec{
    CIDBin:  cidBytes,
    Owner:   "user123",
    RF:      3,
    PinType: 1, // recursive
    Tags:    []string{"video", "important"},
}

err = store.UpsertPin(ctx, "operation-id-123", spec)
if err != nil {
    log.Error("Failed to add pin:", err)
}

// Set desired placement
err = store.SetDesiredPlacement(ctx, cidBytes, []string{"peer1", "peer2", "peer3"})
if err != nil {
    log.Error("Failed to set placement:", err)
}

// Acknowledge placement
err = store.AckPlacement(ctx, cidBytes, "peer1", scyllastate.StatePinned)
if err != nil {
    log.Error("Failed to ack placement:", err)
}
```

### Batch Operations

```go
// Create batch
batch, err := store.BeginBatch(ctx)
if err != nil {
    log.Fatal(err)
}

// Add multiple operations
batch.UpsertPin("op1", spec1)
batch.UpsertPin("op2", spec2)
batch.SetDesiredPlacement(cid1, peers1)

// Commit batch
err = batch.Commit(ctx)
if err != nil {
    log.Error("Batch failed:", err)
    batch.Rollback()
}
```

## Performance Tuning

### ScyllaDB Configuration

```yaml
# scylla.yaml optimizations for pin workload
compaction_throughput_mb_per_sec: 256
concurrent_reads: 32
concurrent_writes: 32
memtable_allocation_type: offheap_objects
```

### Connection Tuning

```json
{
  "scylladb": {
    "num_conns": 20,
    "consistency": "LOCAL_QUORUM",
    "serial_consistency": "LOCAL_SERIAL",
    "timeout": "10s",
    "page_size": 5000
  }
}
```

### Monitoring

Key metrics to monitor:

- **Operation latency**: p95/p99 for pin operations
- **Throughput**: Operations per second
- **Error rates**: Failed operations by type
- **Connection health**: Active connections and timeouts
- **ScyllaDB metrics**: CPU, memory, disk I/O

## Testing

### Unit Tests

```bash
go test ./state/scyllastate/...
```

### Integration Tests

Requires running ScyllaDB instance:

```bash
# Start ScyllaDB in Docker
docker run -d --name scylla -p 9042:9042 scylladb/scylla

# Run integration tests
go test -tags=integration ./state/scyllastate/...
```

### Performance Tests

```bash
go test -bench=. -benchmem ./state/scyllastate/...
```

## Migration

### From Existing State Backends

1. **Dual write phase**: Write to both old and new backends
2. **Backfill**: Copy existing state to ScyllaDB
3. **Validation**: Verify data consistency
4. **Switch**: Change read path to ScyllaDB
5. **Cleanup**: Remove old backend

Example migration:

```go
migrator := scyllastate.NewMigrator(oldState, newState)
err := migrator.Migrate(ctx)
if err != nil {
    log.Fatal("Migration failed:", err)
}
```

## Troubleshooting

### Common Issues

1. **Connection timeouts**: Increase `connect_timeout` and `timeout`
2. **High latency**: Check network between cluster and ScyllaDB
3. **Consistency errors**: Verify replication factor and consistency levels
4. **Memory issues**: Tune ScyllaDB memory settings

### Debug Logging

Enable debug logging:

```json
{
  "scylladb": {
    "log_level": "debug",
    "trace_queries": true
  }
}
```

### Health Checks

```bash
# Check ScyllaDB cluster status
nodetool status

# Check keyspace
cqlsh -e "DESCRIBE KEYSPACE ipfs_pins;"

# Check table stats
cqlsh -e "SELECT * FROM system.size_estimates WHERE keyspace_name='ipfs_pins';"
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass
5. Submit a pull request

## License

This project is licensed under the same terms as IPFS-Cluster.