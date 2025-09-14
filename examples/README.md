# IPFS-Cluster ScyllaDB Integration Examples

This directory contains comprehensive examples demonstrating how to integrate and deploy IPFS-Cluster with ScyllaDB state storage for enterprise-scale pin management.

## Overview

The examples showcase:
- **Optimized Configuration**: Production-ready configurations for different deployment scenarios
- **Docker Compose Deployments**: Quick test environments with ScyllaDB, IPFS-Cluster, and monitoring
- **Performance Benchmarking**: Automated scripts to measure and validate performance
- **Monitoring Dashboards**: Grafana dashboards for comprehensive system observability

## Directory Structure

```
examples/
├── README.md                           # This file
├── configurations/                     # Optimized configuration examples
│   ├── single-node/                   # Single node development setup
│   ├── multi-node/                    # Multi-node cluster setup
│   ├── multi-datacenter/              # Multi-DC deployment
│   └── enterprise/                    # Enterprise production setup
├── docker-compose/                    # Docker compose environments
│   ├── development/                   # Development environment
│   ├── testing/                       # Testing environment with load generation
│   ├── monitoring/                    # Full monitoring stack
│   └── enterprise/                    # Enterprise-grade deployment
├── benchmarks/                        # Performance benchmarking scripts
│   ├── pin-operations/                # Pin/unpin operation benchmarks
│   ├── state-migration/               # State migration performance tests
│   ├── load-testing/                  # High-load scenario tests
│   └── reports/                       # Benchmark result templates
├── monitoring/                        # Monitoring and observability
│   ├── grafana/                       # Grafana dashboards
│   ├── prometheus/                    # Prometheus configurations
│   └── alerting/                      # Alert rules and configurations
└── scripts/                          # Utility scripts
    ├── setup/                         # Environment setup scripts
    ├── migration/                     # Data migration utilities
    └── maintenance/                   # Maintenance and troubleshooting
```

## Quick Start

### 1. Development Environment (Pre-built Images)

Start a complete development environment with ScyllaDB and IPFS-Cluster:

```bash
cd examples/docker-compose/development
docker-compose up -d
```

### 2. Build from Source

Build custom IPFS-Cluster image with ScyllaDB integration:

**Linux/macOS:**
```bash
cd examples/docker
chmod +x build.sh
./build.sh --tag latest
```

**Windows:**
```cmd
cd examples\docker
build.bat --tag latest
```

**Use built image:**
```bash
cd examples/docker-compose/build
docker-compose up -d
```

This will start:
- ScyllaDB cluster (3 nodes)
- IPFS-Cluster with ScyllaDB state storage (built from source)
- IPFS nodes
- Basic monitoring (Prometheus + Grafana)

### 2. Performance Testing

Run comprehensive performance benchmarks:

**Linux/macOS:**
```bash
cd examples/benchmarks
chmod +x run-all-benchmarks.sh
./run-all-benchmarks.sh
```

**Windows (Command Prompt):**
```cmd
cd examples\benchmarks
run-all-benchmarks.bat
```

**Windows (PowerShell):**
```powershell
cd examples\benchmarks
go run pin-operations\pin_benchmark.go -cluster-api="http://localhost:9094" -scenario="concurrent_pin" -concurrency=50 -operations=1000
```

### 3. Monitoring Dashboard

Access the Grafana dashboard at `http://localhost:3000` (admin/admin) to view:
- ScyllaDB performance metrics
- IPFS-Cluster operation statistics
- Pin state distribution and health
- System resource utilization

## Configuration Examples

### Single Node Development
Perfect for local development and testing:
- Single ScyllaDB node
- Single IPFS-Cluster node
- Minimal resource requirements
- Fast startup time

### Multi-Node Cluster
Production-ready cluster setup:
- 3+ ScyllaDB nodes with replication
- Multiple IPFS-Cluster nodes
- Load balancing and failover
- Optimized for performance

### Multi-Datacenter
Geo-distributed deployment:
- ScyllaDB with datacenter-aware replication
- IPFS-Cluster nodes in multiple regions
- Network optimization for cross-DC latency
- Disaster recovery capabilities

### Enterprise
Full enterprise deployment:
- High availability ScyllaDB cluster
- Auto-scaling IPFS-Cluster nodes
- Comprehensive monitoring and alerting
- Security hardening and compliance

## Performance Benchmarks

The benchmark suite includes:

### Pin Operations
- **Throughput**: Pins per second under various loads
- **Latency**: P50, P95, P99 latencies for pin operations
- **Concurrency**: Performance with multiple concurrent clients
- **Batch Operations**: Efficiency of batch pin operations

### State Migration
- **Migration Speed**: Time to migrate from existing backends
- **Data Integrity**: Verification of migrated data
- **Downtime**: Minimal downtime migration strategies
- **Rollback**: Safe rollback procedures

### Load Testing
- **Sustained Load**: Performance under continuous high load
- **Spike Testing**: Handling of traffic spikes
- **Stress Testing**: System behavior at breaking points
- **Endurance**: Long-running stability tests

## Monitoring and Observability

### Key Metrics Tracked
- **ScyllaDB Metrics**: Query latency, throughput, error rates
- **IPFS-Cluster Metrics**: Pin operations, state consistency, node health
- **System Metrics**: CPU, memory, disk, network utilization
- **Business Metrics**: Total pins, storage usage, user activity

### Dashboards Available
- **Overview Dashboard**: High-level system health and performance
- **ScyllaDB Dashboard**: Detailed database performance metrics
- **IPFS-Cluster Dashboard**: Cluster-specific operational metrics
- **Infrastructure Dashboard**: System resource monitoring

### Alerting Rules
- **Critical Alerts**: System failures, data inconsistencies
- **Warning Alerts**: Performance degradation, resource constraints
- **Info Alerts**: Maintenance events, configuration changes

## Best Practices

### Configuration
- Use appropriate consistency levels for your use case
- Configure connection pools based on expected load
- Enable compression for network efficiency
- Set up proper retry policies and timeouts

### Deployment
- Use container orchestration (Kubernetes/Docker Swarm)
- Implement proper health checks and readiness probes
- Configure resource limits and requests
- Use persistent volumes for data storage

### Monitoring
- Set up comprehensive monitoring from day one
- Configure alerting for critical metrics
- Regularly review and tune alert thresholds
- Implement log aggregation and analysis

### Security
- Enable TLS for all communications
- Use strong authentication mechanisms
- Implement network segmentation
- Regular security updates and patches

## Troubleshooting

Common issues and solutions:

### Connection Issues
- Check network connectivity between components
- Verify ScyllaDB cluster health
- Review authentication and TLS configuration

### Performance Issues
- Monitor resource utilization
- Check ScyllaDB query performance
- Review connection pool settings
- Analyze slow query logs

### Data Consistency
- Verify replication factor settings
- Check consistency level configuration
- Monitor repair operations
- Review conflict resolution policies

## Contributing

To add new examples or improve existing ones:

1. Follow the established directory structure
2. Include comprehensive documentation
3. Add appropriate monitoring and alerting
4. Test thoroughly in different environments
5. Update this README with new examples

## Support

For issues and questions:
- Check the troubleshooting section
- Review the IPFS-Cluster documentation
- Open an issue in the project repository
- Join the community discussions