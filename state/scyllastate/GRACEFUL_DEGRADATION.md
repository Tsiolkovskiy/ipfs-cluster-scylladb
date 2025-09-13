# Graceful Degradation Implementation

This document describes the implementation of graceful degradation features for the ScyllaDB state store, addressing requirements 3.1, 3.3, and 3.4 from the specification.

## Overview

The graceful degradation system provides automatic handling of node failures, network partitions, and consistency issues to ensure the ScyllaDB state store remains operational even when some nodes are unavailable.

## Key Components

### 1. GracefulDegradationManager

The main component that orchestrates all graceful degradation features:

- **Node Health Tracking**: Monitors the health of all ScyllaDB nodes
- **Consistency Fallback**: Automatically degrades consistency levels when needed
- **Network Partition Detection**: Detects and handles network partitions
- **Circuit Breaker**: Prevents cascading failures by temporarily isolating failed nodes
- **Recovery Monitoring**: Automatically recovers to higher consistency levels when possible

### 2. Node Health Monitoring

**Requirement 3.1**: Handle ScyllaDB node unavailability

- Tracks health status of each configured ScyllaDB node
- Monitors consecutive failures and response times
- Automatically marks nodes as unhealthy after multiple failures
- Continues operations with remaining available nodes

**Implementation Details**:
```go
type NodeHealthStatus struct {
    Host                string
    IsHealthy           bool
    LastSeen            time.Time
    ConsecutiveFailures int
    LastError           error
    ResponseTime        time.Duration
    IsConnected         bool
    ConnectionErrors    int
}
```

### 3. Automatic Replica Switching

**Requirement 3.3**: Automatic switching to available replicas when read operations encounter node failures

- Detects read failures and automatically retries with available replicas
- Uses gocql's built-in host selection policies (token-aware, DC-aware)
- Degrades consistency levels to ensure operations can complete with available nodes
- Tracks which nodes are responding and routes traffic accordingly

**Key Features**:
- Token-aware routing for optimal data locality
- DC-aware routing for multi-datacenter deployments
- Automatic failover to healthy replicas
- Consistency level degradation when insufficient replicas are available

### 4. Network Partition Handling

**Requirement 3.4**: Handle network partitions with eventual consistency and proper partition recovery

- Detects network partitions when majority of nodes become unavailable
- Switches to relaxed consistency levels (ONE, ANY) during partitions
- Maintains eventual consistency semantics
- Automatically recovers when partition is resolved

**Partition Detection Logic**:
```go
// Consider it a partition if more than half the nodes are unhealthy
isPartition := unhealthyCount > totalNodes/2
```

**Partition Handling Strategy**:
1. Detect partition based on node health
2. Switch to most relaxed consistency level (ONE or ANY)
3. Continue operations with available nodes
4. Monitor for partition recovery
5. Gradually restore higher consistency levels

### 5. Consistency Level Fallback

Automatic degradation of consistency levels based on error conditions:

**Fallback Chains**:
- `ALL` → `QUORUM` → `LOCAL_QUORUM` → `ONE`
- `QUORUM` → `LOCAL_QUORUM` → `ONE`
- `LOCAL_QUORUM` → `LOCAL_ONE` → `ONE`

**Triggers for Degradation**:
- `RequestErrUnavailable`: Insufficient replicas available
- "not enough replicas" errors
- "cannot achieve consistency" errors
- "quorum not available" errors

### 6. Circuit Breaker Pattern

Prevents cascading failures by temporarily isolating failed nodes:

**States**:
- **Closed**: Normal operation, requests pass through
- **Open**: Node is isolated, requests are not sent to this node
- **Half-Open**: Testing if node has recovered

**Configuration**:
- Failure threshold: 5 consecutive failures
- Timeout: 30 seconds before attempting recovery
- Reset timeout: 60 seconds for full recovery

### 7. Recovery Mechanisms

Automatic recovery to higher consistency levels when conditions improve:

**Recovery Triggers**:
- Node health checks show improved availability
- Successful operations at current consistency level
- Time-based recovery attempts (every 5 minutes)

**Recovery Strategy**:
1. Test if higher consistency level is achievable
2. Gradually increase consistency level
3. Monitor for stability before further increases
4. Reset to original consistency when all nodes are healthy

## Integration with ScyllaState

The graceful degradation manager is integrated into the main ScyllaState implementation:

```go
type ScyllaState struct {
    // ... other fields
    degradationManager *GracefulDegradationManager
}
```

**Helper Methods**:
- `executeWithGracefulDegradation()`: Wraps operations with degradation support
- `executeQueryWithDegradation()`: Executes single queries with degradation
- `scanQueryWithDegradation()`: Executes scan queries with degradation
- `iterQueryWithDegradation()`: Executes iterator queries with degradation

## Monitoring and Observability

**Metrics Tracked**:
- Node failures count
- Consistency fallbacks count
- Partition detections count
- Circuit breaker trips count
- Recovery events count
- Current healthy/unhealthy node counts
- Current consistency level
- Partition status

**Health Information**:
- Per-node health status
- Response times
- Consecutive failure counts
- Last error information
- Connection status

## Error Handling Strategy

**Retryable Errors** (handled with degradation):
- Connection timeouts
- Node unavailability
- Insufficient replicas
- Consistency failures
- Temporary overload conditions

**Non-Retryable Errors** (fail fast):
- Syntax errors
- Authentication failures
- Schema errors
- Authorization failures

## Configuration

The graceful degradation system is configured through the main ScyllaDB configuration:

```json
{
  "hosts": ["scylla1", "scylla2", "scylla3"],
  "consistency": "QUORUM",
  "retry_policy": {
    "num_retries": 3,
    "min_retry_delay": "100ms",
    "max_retry_delay": "10s"
  },
  "dc_aware_routing": true,
  "token_aware_routing": true,
  "local_dc": "datacenter1"
}
```

## Testing

Comprehensive test coverage includes:

1. **Unit Tests**: Test individual components in isolation
2. **Integration Tests**: Test full degradation scenarios
3. **Benchmark Tests**: Validate performance impact
4. **Failure Simulation**: Test various failure scenarios

**Test Scenarios**:
- Single node failures
- Multiple node failures
- Network partitions
- Consistency degradation
- Circuit breaker behavior
- Recovery scenarios
- Performance under degradation

## Performance Considerations

**Overhead**:
- Minimal overhead during normal operations
- Health checks run every 10 seconds
- Partition detection runs every 30 seconds
- Recovery attempts run every 60 seconds

**Optimizations**:
- Cached consistency level decisions
- Efficient node health tracking
- Minimal lock contention
- Asynchronous health monitoring

## Benefits

1. **High Availability**: System remains operational during node failures
2. **Automatic Recovery**: No manual intervention required for most scenarios
3. **Graceful Degradation**: Performance degrades gracefully rather than failing completely
4. **Observability**: Comprehensive metrics and health information
5. **Configurable**: Tunable parameters for different deployment scenarios

## Requirements Compliance

✅ **Requirement 3.1**: Handle ScyllaDB node unavailability
- Implemented through node health tracking and automatic failover
- System continues with remaining available nodes
- Circuit breaker prevents cascading failures

✅ **Requirement 3.3**: Automatic switching to available replicas
- Implemented through consistency degradation and retry logic
- Automatic failover to healthy replicas
- Token-aware and DC-aware routing for optimal replica selection

✅ **Requirement 3.4**: Handle network partitions with eventual consistency
- Implemented through partition detection and relaxed consistency
- Maintains eventual consistency during partitions
- Automatic recovery when partition is resolved

## Future Enhancements

1. **Adaptive Consistency**: Dynamic consistency level adjustment based on workload
2. **Predictive Failure Detection**: Machine learning-based failure prediction
3. **Cross-DC Failover**: Enhanced multi-datacenter failover strategies
4. **Custom Recovery Policies**: User-defined recovery strategies
5. **Integration with External Monitoring**: Integration with Prometheus, Grafana, etc.