# IPFS-Cluster ScyllaDB Integration Examples

This directory contains practical examples demonstrating how to integrate IPFS-Cluster with ScyllaDB state storage in various deployment scenarios.

## Quick Start

### 1. Development Environment

The fastest way to get started is with the development environment:

**Automated Setup (Recommended):**

**Linux/macOS:**
```bash
cd examples/scripts/setup
chmod +x setup-development.sh
./setup-development.sh
```

**Windows (Command Prompt):**
```cmd
cd examples\scripts\setup
setup-development.bat
```

**Windows (PowerShell):**
```powershell
cd examples\scripts\setup
.\setup-development.ps1
```

**Manual Setup:**
```bash
# Navigate to the development setup
cd examples/docker-compose/development

# Start the environment
docker-compose up -d

# Wait for services to be ready (about 2-3 minutes)
# Check status
docker-compose ps

# Access Grafana dashboard
# Windows: start http://localhost:3000
# macOS: open http://localhost:3000
# Linux: xdg-open http://localhost:3000
# Or just navigate to http://localhost:3000 in your browser (admin/admin)
```

### 2. Test Basic Operations

Once the environment is running, test basic pin operations:

```bash
# Check cluster status
curl http://localhost:9094/api/v0/id

# Pin a test file
curl -X POST "http://localhost:9094/api/v0/pins/QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"

# List all pins
curl http://localhost:9094/api/v0/pins

# Check ScyllaDB data
docker exec -it scylla-dev cqlsh -e "SELECT * FROM ipfs_pins.pins_by_cid LIMIT 10;"
```

### 3. Run Performance Benchmarks

Execute the comprehensive benchmark suite:

**Linux/macOS:**
```bash
# Navigate to benchmarks directory
cd examples/benchmarks

# Make script executable
chmod +x run-all-benchmarks.sh

# Run all benchmarks
./run-all-benchmarks.sh

# Or run specific benchmarks
./run-all-benchmarks.sh --benchmarks pin-ops,load
```

**Windows (Command Prompt):**
```cmd
cd examples\benchmarks
run-all-benchmarks.bat
```

**Windows (PowerShell) - Individual Benchmark:**
```powershell
cd examples\benchmarks\pin-operations
go run pin_benchmark.go -cluster-api="http://localhost:9094" -scenario="concurrent_pin" -concurrency=50 -operations=1000
```

## Configuration Examples

### Single Node Development

**Use Case**: Local development and testing
**File**: `configurations/single-node/cluster.json`

Key features:
- Single ScyllaDB node with simple replication
- Minimal resource requirements
- Fast startup time
- Basic monitoring enabled

```json
{
  "state": {
    "scylladb": {
      "hosts": ["127.0.0.1"],
      "port": 9042,
      "keyspace": "ipfs_pins",
      "consistency": "ONE",
      "num_conns": 2,
      "batch_size": 100
    }
  }
}
```

### Multi-Node Production

**Use Case**: Production cluster deployment
**File**: `configurations/multi-node/cluster.json`

Key features:
- 3+ ScyllaDB nodes with QUORUM consistency
- TLS encryption enabled
- Optimized connection pooling
- Enhanced monitoring and tracing

```json
{
  "state": {
    "scylladb": {
      "hosts": ["scylla-1", "scylla-2", "scylla-3"],
      "consistency": "QUORUM",
      "tls_enabled": true,
      "num_conns": 10,
      "batch_size": 1000,
      "dc_aware_routing": true
    }
  }
}
```

### Enterprise Deployment

**Use Case**: Large-scale enterprise deployment
**File**: `configurations/enterprise/cluster.json`

Key features:
- Multi-datacenter ScyllaDB deployment
- Advanced security configuration
- High-performance settings
- Comprehensive monitoring

```json
{
  "state": {
    "scylladb": {
      "hosts": ["dc1-scylla1", "dc1-scylla2", "dc2-scylla1", "dc2-scylla2"],
      "consistency": "LOCAL_QUORUM",
      "num_conns": 20,
      "batch_size": 5000,
      "local_dc": "datacenter1",
      "fallback_dc": "datacenter2"
    }
  }
}
```

## Docker Compose Environments

### Development Environment

**Location**: `docker-compose/development/`

Services included:
- ScyllaDB (single node)
- IPFS-Cluster (single node)
- IPFS (single node)
- Prometheus (metrics collection)
- Grafana (visualization)

```bash
cd examples/docker-compose/development
docker-compose up -d
```

### Testing Environment

**Location**: `docker-compose/testing/`

Services included:
- ScyllaDB cluster (3 nodes)
- IPFS-Cluster cluster (3 nodes)
- IPFS nodes (3 nodes)
- Load testing tools
- Full monitoring stack

```bash
cd examples/docker-compose/testing
docker-compose up -d
```

## Performance Benchmarks

### Pin Operations Benchmark

Tests various pin operation scenarios:

```bash
cd examples/benchmarks/pin-operations
go run pin_benchmark.go \
  -cluster-api="http://localhost:9094" \
  -scenario="concurrent_pin" \
  -concurrency=50 \
  -operations=1000
```

Available scenarios:
- `single_pin`: Sequential pin operations
- `batch_pin`: Batched pin operations
- `concurrent_pin`: Concurrent pin operations
- `mixed_operations`: Mixed pin/unpin/list operations

### Load Testing

Comprehensive load testing with multiple scenarios:

```bash
cd examples/benchmarks/load-testing
go run cluster_load_test.go \
  -cluster-endpoints="http://localhost:9094" \
  -duration="10m" \
  -pin-rate="100" \
  -unpin-rate="50"
```

## Monitoring and Observability

### Grafana Dashboards

Access the pre-configured dashboard at `http://localhost:3000`:

- **Overview**: High-level cluster health and performance
- **ScyllaDB Metrics**: Database-specific performance metrics
- **Pin Operations**: Pin operation statistics and latencies
- **System Resources**: CPU, memory, and network utilization

### Key Metrics to Monitor

1. **Pin Operations**:
   - `cluster_pin_operations_total` - Total pin operations
   - `cluster_pins_total` - Current number of pins
   - `cluster_pin_duration_seconds` - Pin operation latency

2. **ScyllaDB Performance**:
   - `scylladb_operation_duration_seconds` - Query latency
   - `scylladb_operations_total` - Operations per second
   - `scylladb_active_connections` - Connection pool status

3. **System Health**:
   - `cluster_state_drift_ratio` - State consistency metric
   - `process_cpu_seconds_total` - CPU usage
   - `process_resident_memory_bytes` - Memory usage

### Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
  - name: ipfs-cluster-scylladb
    rules:
      - alert: HighPinLatency
        expr: histogram_quantile(0.95, rate(cluster_pin_duration_seconds_bucket[5m])) > 60
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High pin operation latency"
          
      - alert: ScyllaDBHighLatency
        expr: histogram_quantile(0.95, rate(scylladb_operation_duration_seconds_bucket[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High ScyllaDB query latency"
```

## Best Practices

### Configuration Optimization

1. **Connection Pooling**:
   ```json
   {
     "num_conns": 10,  // Adjust based on expected load
     "timeout": "30s",
     "connect_timeout": "10s"
   }
   ```

2. **Consistency Levels**:
   - Development: `ONE` (fastest)
   - Production: `QUORUM` (balanced)
   - Critical: `ALL` (strongest consistency)

3. **Batch Operations**:
   ```json
   {
     "batch_size": 1000,     // Larger batches for better throughput
     "batch_timeout": "1s"   // Prevent indefinite batching
   }
   ```

### Performance Tuning

1. **ScyllaDB Optimization**:
   - Use appropriate compaction strategies
   - Configure proper replication factors
   - Enable compression for network efficiency

2. **IPFS-Cluster Tuning**:
   - Adjust concurrent pin limits
   - Configure appropriate timeouts
   - Enable metrics and monitoring

3. **System Resources**:
   - Allocate sufficient memory for ScyllaDB
   - Use SSD storage for better performance
   - Configure network for low latency

### Security Considerations

1. **TLS Configuration**:
   ```json
   {
     "tls_enabled": true,
     "tls_cert_file": "/path/to/cert.pem",
     "tls_key_file": "/path/to/key.pem",
     "tls_ca_file": "/path/to/ca.pem"
   }
   ```

2. **Authentication**:
   - Use strong passwords for ScyllaDB
   - Implement proper access controls
   - Regular security updates

3. **Network Security**:
   - Use private networks for cluster communication
   - Implement firewall rules
   - Monitor for suspicious activity

## Troubleshooting

### Common Issues

1. **Connection Timeouts**:
   - Check network connectivity
   - Verify ScyllaDB cluster health
   - Adjust timeout settings

2. **High Latency**:
   - Monitor ScyllaDB performance
   - Check system resources
   - Review consistency level settings

3. **Data Inconsistency**:
   - Verify replication factor
   - Check repair operations
   - Monitor state drift metrics

### Debugging Commands

```bash
# Check ScyllaDB cluster status
docker exec scylla-1 nodetool status

# View IPFS-Cluster logs
docker logs cluster-1 -f

# Check ScyllaDB query performance
docker exec scylla-1 cqlsh -e "TRACING ON; SELECT * FROM ipfs_pins.pins_by_cid LIMIT 1;"

# Monitor metrics
curl http://localhost:9090/api/v1/query?query=cluster_pins_total
```

## Contributing

To add new examples or improve existing ones:

1. Follow the established directory structure
2. Include comprehensive documentation
3. Add appropriate monitoring configuration
4. Test in different deployment scenarios
5. Update this README with new examples

## Support

For issues and questions:
- Check the troubleshooting section above
- Review the main IPFS-Cluster documentation
- Open an issue in the project repository
- Join the community discussions