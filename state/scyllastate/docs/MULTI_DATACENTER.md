# Multi-Datacenter Deployment Guide

This guide covers deploying the ScyllaDB state store across multiple datacenters for high availability, disaster recovery, and global distribution.

## Table of Contents

- [Overview](#overview)
- [Architecture Patterns](#architecture-patterns)
- [Network Requirements](#network-requirements)
- [ScyllaDB Cluster Setup](#scylladb-cluster-setup)
- [Configuration](#configuration)
- [Consistency Strategies](#consistency-strategies)
- [Monitoring and Operations](#monitoring-and-operations)
- [Disaster Recovery](#disaster-recovery)
- [Best Practices](#best-practices)

## Overview

Multi-datacenter deployments provide:

- **High Availability**: Service continues even if an entire datacenter fails
- **Disaster Recovery**: Data is replicated across geographically distributed locations
- **Global Distribution**: Reduced latency for users in different regions
- **Load Distribution**: Workload can be distributed across multiple datacenters

## Architecture Patterns

### Active-Active Pattern

Both datacenters serve read and write traffic simultaneously.

```
┌─────────────────┐         ┌─────────────────┐
│   Datacenter 1  │◄────────┤   Datacenter 2  │
│                 │         │                 │
│ ┌─────────────┐ │         │ ┌─────────────┐ │
│ │IPFS-Cluster │ │         │ │IPFS-Cluster │ │
│ │   Node 1    │ │         │ │   Node 2    │ │
│ └─────────────┘ │         │ └─────────────┘ │
│        │        │         │        │        │
│ ┌─────────────┐ │         │ ┌─────────────┐ │
│ │  ScyllaDB   │ │◄────────┤ │  ScyllaDB   │ │
│ │  Cluster    │ │         │ │  Cluster    │ │
│ └─────────────┘ │         │ └─────────────┘ │
└─────────────────┘         └─────────────────┘
```

**Advantages:**
- Maximum availability
- Load distribution
- No single point of failure

**Considerations:**
- Requires conflict resolution
- More complex consistency management

### Active-Passive Pattern

One datacenter serves traffic while the other is on standby.

```
┌─────────────────┐         ┌─────────────────┐
│   Datacenter 1  │         │   Datacenter 2  │
│    (Active)     │────────►│   (Standby)     │
│                 │         │                 │
│ ┌─────────────┐ │         │ ┌─────────────┐ │
│ │IPFS-Cluster │ │         │ │IPFS-Cluster │ │
│ │   Active    │ │         │ │   Standby   │ │
│ └─────────────┘ │         │ └─────────────┘ │
│        │        │         │        │        │
│ ┌─────────────┐ │         │ ┌─────────────┐ │
│ │  ScyllaDB   │ │────────►│ │  ScyllaDB   │ │
│ │   Primary   │ │         │ │  Replica    │ │
│ └─────────────┘ │         │ └─────────────┘ │
└─────────────────┘         └─────────────────┘
```

**Advantages:**
- Simpler consistency model
- Easier to manage
- Clear failover process

**Considerations:**
- Underutilized resources
- Manual failover required

### Hub-and-Spoke Pattern

Central datacenter with regional spokes.

```
        ┌─────────────────┐
        │   Datacenter 1  │
        │   (Hub/Primary) │
        │                 │
        │ ┌─────────────┐ │
        │ │  ScyllaDB   │ │
        │ │   Primary   │ │
        │ └─────────────┘ │
        └─────────┬───────┘
                  │
    ┌─────────────┼─────────────┐
    │             │             │
┌───▼───┐     ┌───▼───┐     ┌───▼───┐
│ DC 2  │     │ DC 3  │     │ DC 4  │
│(Spoke)│     │(Spoke)│     │(Spoke)│
└───────┘     └───────┘     └───────┘
```

**Advantages:**
- Centralized management
- Consistent data model
- Regional optimization

**Considerations:**
- Single point of failure at hub
- Higher latency for spoke-to-spoke communication

## Network Requirements

### Bandwidth Requirements

- **Minimum**: 100 Mbps between datacenters
- **Recommended**: 1 Gbps+ for production workloads
- **Optimal**: 10 Gbps+ for high-throughput scenarios

### Latency Requirements

- **Same Region**: <5ms RTT
- **Cross-Region**: <100ms RTT
- **Global**: <200ms RTT

### Network Configuration

```bash
# Test network connectivity between datacenters
ping -c 10 scylla-dc2-node1.example.com

# Test bandwidth
iperf3 -c scylla-dc2-node1.example.com -t 60

# Test ScyllaDB connectivity
telnet scylla-dc2-node1.example.com 9042
```

## ScyllaDB Cluster Setup

### Two-Datacenter Setup

#### Datacenter 1 Configuration

```yaml
# /etc/scylla/scylla.yaml - DC1 nodes
cluster_name: 'ipfs_cluster_multidc'
seeds: "10.1.1.10,10.2.1.10"
listen_address: 10.1.1.10
rpc_address: 10.1.1.10
broadcast_address: 10.1.1.10
broadcast_rpc_address: 10.1.1.10

endpoint_snitch: GossipingPropertyFileSnitch
dc: datacenter1
rack: rack1

# Network topology
prefer_local: true
```

#### Datacenter 2 Configuration

```yaml
# /etc/scylla/scylla.yaml - DC2 nodes
cluster_name: 'ipfs_cluster_multidc'
seeds: "10.1.1.10,10.2.1.10"
listen_address: 10.2.1.10
rpc_address: 10.2.1.10
broadcast_address: 10.2.1.10
broadcast_rpc_address: 10.2.1.10

endpoint_snitch: GossipingPropertyFileSnitch
dc: datacenter2
rack: rack1

# Network topology
prefer_local: true
```

#### Keyspace Creation

```cql
CREATE KEYSPACE ipfs_pins_multidc 
WITH REPLICATION = {
    'class': 'NetworkTopologyStrategy',
    'datacenter1': 3,
    'datacenter2': 3
};
```

### Three-Datacenter Setup

#### Global Distribution

```cql
CREATE KEYSPACE ipfs_pins_global 
WITH REPLICATION = {
    'class': 'NetworkTopologyStrategy',
    'us_east': 3,
    'us_west': 3,
    'eu_west': 3
};
```

#### Regional Configuration

```yaml
# US East
dc: us_east
rack: rack1

# US West  
dc: us_west
rack: rack1

# EU West
dc: eu_west
rack: rack1
```

## Configuration

### IPFS-Cluster Configuration

#### Active-Active Configuration

```json
{
  "hosts": [
    "scylla-dc1-node1.example.com",
    "scylla-dc1-node2.example.com",
    "scylla-dc1-node3.example.com",
    "scylla-dc2-node1.example.com",
    "scylla-dc2-node2.example.com",
    "scylla-dc2-node3.example.com"
  ],
  "port": 9042,
  "keyspace": "ipfs_pins_multidc",
  
  "consistency": "LOCAL_QUORUM",
  "serial_consistency": "LOCAL_SERIAL",
  
  "dc_aware_routing": true,
  "local_dc": "datacenter1",
  "token_aware_routing": true,
  
  "timeout": "30s",
  "connect_timeout": "15s",
  "num_conns": 6,
  
  "retry_policy": {
    "num_retries": 5,
    "min_retry_delay": "200ms",
    "max_retry_delay": "60s"
  }
}
```

#### Per-Datacenter Configuration

**Datacenter 1 (Primary)**
```json
{
  "hosts": [
    "scylla-dc1-node1.internal",
    "scylla-dc1-node2.internal", 
    "scylla-dc1-node3.internal"
  ],
  "local_dc": "datacenter1",
  "consistency": "LOCAL_QUORUM",
  "timeout": "15s"
}
```

**Datacenter 2 (Secondary)**
```json
{
  "hosts": [
    "scylla-dc2-node1.internal",
    "scylla-dc2-node2.internal",
    "scylla-dc2-node3.internal"
  ],
  "local_dc": "datacenter2", 
  "consistency": "LOCAL_QUORUM",
  "timeout": "20s"
}
```

### Load Balancer Configuration

#### HAProxy Configuration

```haproxy
# /etc/haproxy/haproxy.cfg
global
    daemon
    maxconn 4096

defaults
    mode tcp
    timeout connect 5000ms
    timeout client 50000ms
    timeout server 50000ms

# ScyllaDB DC1
listen scylla_dc1
    bind *:9042
    balance roundrobin
    server scylla-dc1-1 10.1.1.10:9042 check
    server scylla-dc1-2 10.1.1.11:9042 check
    server scylla-dc1-3 10.1.1.12:9042 check

# ScyllaDB DC2  
listen scylla_dc2
    bind *:9043
    balance roundrobin
    server scylla-dc2-1 10.2.1.10:9042 check
    server scylla-dc2-2 10.2.1.11:9042 check
    server scylla-dc2-3 10.2.1.12:9042 check
```

## Consistency Strategies

### Consistency Levels

#### Local Consistency (Recommended)

```json
{
  "consistency": "LOCAL_QUORUM",
  "serial_consistency": "LOCAL_SERIAL"
}
```

**Use Cases:**
- Normal operations
- High availability requirements
- Acceptable eventual consistency

#### Global Consistency

```json
{
  "consistency": "EACH_QUORUM",
  "serial_consistency": "SERIAL"
}
```

**Use Cases:**
- Critical operations
- Strong consistency requirements
- Cross-datacenter coordination

#### Performance-Optimized

```json
{
  "consistency": "LOCAL_ONE",
  "serial_consistency": "LOCAL_SERIAL"
}
```

**Use Cases:**
- High-throughput scenarios
- Read-heavy workloads
- Acceptable data lag

### Consistency Configuration by Operation

```go
func getConsistencyForOperation(operation string) gocql.Consistency {
    switch operation {
    case "critical_write":
        return gocql.EachQuorum
    case "normal_write":
        return gocql.LocalQuorum
    case "read":
        return gocql.LocalOne
    case "list":
        return gocql.LocalOne
    default:
        return gocql.LocalQuorum
    }
}
```

## Monitoring and Operations

### Health Monitoring

#### Cross-Datacenter Health Check

```bash
#!/bin/bash
# health-check-multidc.sh

DCS=("datacenter1" "datacenter2")
NODES=("10.1.1.10" "10.1.1.11" "10.1.1.12" "10.2.1.10" "10.2.1.11" "10.2.1.12")

for node in "${NODES[@]}"; do
    echo "Checking node: $node"
    
    # Check connectivity
    if ! nc -z $node 9042; then
        echo "ERROR: Cannot connect to $node:9042"
        continue
    fi
    
    # Check node status
    nodetool -h $node status
    
    # Check cross-DC latency
    nodetool -h $node ring | grep -E "(datacenter1|datacenter2)"
done
```

#### Replication Monitoring

```bash
#!/bin/bash
# check-replication.sh

# Check replication status
nodetool -h 10.1.1.10 describering ipfs_pins_multidc

# Check repair status
nodetool -h 10.1.1.10 compactionstats

# Check streaming operations
nodetool -h 10.1.1.10 netstats
```

### Metrics Collection

#### Prometheus Configuration

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'scylla-dc1'
    static_configs:
      - targets: 
        - '10.1.1.10:9180'
        - '10.1.1.11:9180' 
        - '10.1.1.12:9180'
    labels:
      datacenter: 'datacenter1'

  - job_name: 'scylla-dc2'
    static_configs:
      - targets:
        - '10.2.1.10:9180'
        - '10.2.1.11:9180'
        - '10.2.1.12:9180'
    labels:
      datacenter: 'datacenter2'

  - job_name: 'ipfs-cluster-dc1'
    static_configs:
      - targets: ['10.1.1.100:8888']
    labels:
      datacenter: 'datacenter1'

  - job_name: 'ipfs-cluster-dc2'
    static_configs:
      - targets: ['10.2.1.100:8888']
    labels:
      datacenter: 'datacenter2'
```

#### Key Metrics to Monitor

```promql
# Cross-datacenter latency
scylla_storage_proxy_coordinator_write_latency{datacenter!="local"}

# Replication lag
scylla_hints_pending_hints

# Cross-DC traffic
rate(scylla_transport_requests_total[5m])

# Node availability per DC
up{job=~"scylla-dc.*"} by (datacenter)

# IPFS-Cluster state store metrics
rate(scylla_state_operations_total[5m]) by (datacenter, operation)
```

### Alerting Rules

```yaml
# alerts.yml
groups:
  - name: multidc_alerts
    rules:
      - alert: DatacenterDown
        expr: up{job=~"scylla-dc.*"} == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "ScyllaDB datacenter {{ $labels.datacenter }} is down"

      - alert: CrossDCLatencyHigh
        expr: scylla_storage_proxy_coordinator_write_latency{datacenter!="local"} > 100
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "High cross-datacenter latency detected"

      - alert: ReplicationLag
        expr: scylla_hints_pending_hints > 1000
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "Replication lag detected"
```

## Disaster Recovery

### Backup Strategy

#### Automated Backup Script

```bash
#!/bin/bash
# backup-multidc.sh

KEYSPACE="ipfs_pins_multidc"
BACKUP_DIR="/backup/scylla"
DATE=$(date +%Y%m%d_%H%M%S)

# Create backup directory
mkdir -p $BACKUP_DIR/$DATE

# Backup from primary datacenter
nodetool -h 10.1.1.10 snapshot $KEYSPACE -t $DATE

# Copy snapshots to backup location
for node in 10.1.1.10 10.1.1.11 10.1.1.12; do
    rsync -av $node:/var/lib/scylla/data/$KEYSPACE/*/snapshots/$DATE/ \
          $BACKUP_DIR/$DATE/$node/
done

# Backup schema
cqlsh 10.1.1.10 -e "DESCRIBE KEYSPACE $KEYSPACE" > $BACKUP_DIR/$DATE/schema.cql

echo "Backup completed: $BACKUP_DIR/$DATE"
```

#### Cross-Datacenter Backup Verification

```bash
#!/bin/bash
# verify-backup-consistency.sh

KEYSPACE="ipfs_pins_multidc"

# Compare row counts between datacenters
DC1_COUNT=$(cqlsh 10.1.1.10 -e "SELECT COUNT(*) FROM $KEYSPACE.pins_by_cid" | grep -o '[0-9]*')
DC2_COUNT=$(cqlsh 10.2.1.10 -e "SELECT COUNT(*) FROM $KEYSPACE.pins_by_cid" | grep -o '[0-9]*')

echo "DC1 row count: $DC1_COUNT"
echo "DC2 row count: $DC2_COUNT"

if [ "$DC1_COUNT" -eq "$DC2_COUNT" ]; then
    echo "✓ Row counts match between datacenters"
else
    echo "✗ Row count mismatch detected"
    exit 1
fi
```

### Failover Procedures

#### Automatic Failover Configuration

```go
// Failover configuration
config := &scyllastate.Config{
    Hosts: []string{
        "scylla-dc1-lb.example.com", // Primary
        "scylla-dc2-lb.example.com", // Failover
    },
    DCAwareRouting: true,
    LocalDC:        "datacenter1",
    
    RetryPolicy: &scyllastate.RetryPolicyConfig{
        NumRetries:    10,
        MinRetryDelay: 1 * time.Second,
        MaxRetryDelay: 60 * time.Second,
    },
    
    // Aggressive timeouts for faster failover
    Timeout:        10 * time.Second,
    ConnectTimeout: 5 * time.Second,
}
```

#### Manual Failover Script

```bash
#!/bin/bash
# failover-to-dc2.sh

echo "Initiating failover to datacenter2..."

# Update load balancer to point to DC2
curl -X POST http://lb-manager:8080/api/failover \
     -d '{"primary_dc": "datacenter2"}'

# Update IPFS-Cluster configuration
kubectl patch configmap ipfs-cluster-config \
  --patch '{"data":{"scylla_local_dc":"datacenter2"}}'

# Restart IPFS-Cluster pods
kubectl rollout restart deployment/ipfs-cluster

echo "Failover completed. Monitor cluster health."
```

### Recovery Procedures

#### Datacenter Recovery

```bash
#!/bin/bash
# recover-datacenter.sh

FAILED_DC="datacenter1"
ACTIVE_DC="datacenter2"

echo "Recovering $FAILED_DC from $ACTIVE_DC..."

# 1. Rebuild failed nodes
for node in 10.1.1.10 10.1.1.11 10.1.1.12; do
    echo "Rebuilding node $node..."
    nodetool -h $node rebuild $ACTIVE_DC
done

# 2. Run repair to ensure consistency
nodetool -h 10.1.1.10 repair ipfs_pins_multidc

# 3. Verify data consistency
./verify-backup-consistency.sh

echo "Recovery completed for $FAILED_DC"
```

## Best Practices

### Design Principles

1. **Prefer Local Operations**: Use LOCAL_QUORUM for most operations
2. **Plan for Partitions**: Design for network partition scenarios
3. **Monitor Cross-DC Latency**: Keep latency under acceptable thresholds
4. **Automate Repairs**: Schedule regular repair operations
5. **Test Failover**: Regularly test failover procedures

### Configuration Best Practices

1. **Use Appropriate Consistency**: Match consistency to business requirements
2. **Configure Timeouts**: Set timeouts based on cross-DC latency
3. **Enable DC-Aware Routing**: Always enable datacenter-aware routing
4. **Monitor Resource Usage**: Track CPU, memory, and network usage
5. **Plan Capacity**: Account for replication overhead

### Operational Best Practices

1. **Regular Backups**: Implement automated backup procedures
2. **Monitor Replication**: Track replication lag and repair status
3. **Test Disaster Recovery**: Regularly test DR procedures
4. **Document Procedures**: Maintain up-to-date runbooks
5. **Train Operations Team**: Ensure team understands multi-DC operations

### Security Considerations

1. **Encrypt Inter-DC Traffic**: Use TLS for cross-datacenter communication
2. **Network Segmentation**: Isolate datacenter networks appropriately
3. **Access Controls**: Implement proper authentication and authorization
4. **Audit Logging**: Enable comprehensive audit logging
5. **Regular Security Reviews**: Conduct regular security assessments

### Performance Optimization

1. **Optimize Network**: Ensure adequate bandwidth between datacenters
2. **Tune Compaction**: Adjust compaction strategies for multi-DC
3. **Monitor Streaming**: Track streaming operations during repairs
4. **Optimize Queries**: Design queries to minimize cross-DC traffic
5. **Use Batching**: Batch operations to reduce network overhead

For more information, see the [main configuration guide](./CONFIGURATION.md) and [performance tuning guide](./PERFORMANCE.md).