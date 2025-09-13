# ScyllaDB State Store - Structured Logging Implementation

## Overview

This document describes the comprehensive structured logging implementation for the ScyllaDB state store in IPFS-Cluster. The implementation provides detailed, contextual logging for all ScyllaDB operations, enabling effective monitoring, debugging, and troubleshooting.

## Features

### 1. Contextual Operation Logging
- **Operation Context**: Each operation is logged with full context including CID, peer ID, duration, consistency level, and custom metadata
- **Error Classification**: ScyllaDB-specific error codes for detailed error analysis
- **Performance Tracking**: Duration metrics and throughput calculations
- **Retry Tracking**: Detailed logging of retry attempts with exponential backoff context

### 2. Query-Level Tracing
- **CQL Query Logging**: Individual query execution with sanitized query text and arguments
- **Query Type Classification**: Automatic detection of SELECT, INSERT, UPDATE, DELETE, etc.
- **Query Performance**: Duration tracking and slow query detection
- **Query Sanitization**: Sensitive data masking for secure logging

### 3. Batch Operation Monitoring
- **Batch Metrics**: Size, type (LOGGED/UNLOGGED/COUNTER), and throughput tracking
- **Batch Performance**: Duration and operations-per-second calculations
- **Batch Error Handling**: Detailed error logging with context

### 4. Connection and Infrastructure Logging
- **Connection Events**: Establishment, loss, failure, and restoration events
- **Node Health Monitoring**: Individual node status and datacenter information
- **Graceful Degradation**: Consistency level changes and degradation events
- **Migration Events**: Data migration progress and status tracking

## Implementation Details

### Core Components

#### StructuredLogger
The main logging component that provides all structured logging capabilities:

```go
type StructuredLogger struct {
    logger logging.StandardLogger
    config *LoggingConfig
}
```

#### OperationContext
Holds contextual information for logging operations:

```go
type OperationContext struct {
    Operation   string                 // Operation name (e.g., "Add", "Get", "List")
    CID         CidInterface           // CID being operated on (if applicable)
    PeerID      string                 // Peer ID involved (if applicable)
    Duration    time.Duration          // Operation duration
    Error       error                  // Error that occurred (if any)
    Retries     int                    // Number of retries attempted
    Consistency string                 // Consistency level used
    QueryType   string                 // Type of CQL query executed
    BatchSize   int                    // Size of batch operation (if applicable)
    RecordCount int64                  // Number of records processed
    Metadata    map[string]interface{} // Additional metadata
}
```

#### Error Classification
ScyllaDB-specific error codes for detailed error analysis:

```go
type ScyllaErrorCode string

const (
    ErrorCodeTimeout          ScyllaErrorCode = "TIMEOUT"
    ErrorCodeUnavailable      ScyllaErrorCode = "UNAVAILABLE"
    ErrorCodeReadTimeout      ScyllaErrorCode = "READ_TIMEOUT"
    ErrorCodeWriteTimeout     ScyllaErrorCode = "WRITE_TIMEOUT"
    ErrorCodeReadFailure      ScyllaErrorCode = "READ_FAILURE"
    ErrorCodeWriteFailure     ScyllaErrorCode = "WRITE_FAILURE"
    ErrorCodeSyntax           ScyllaErrorCode = "SYNTAX_ERROR"
    ErrorCodeUnauthorized     ScyllaErrorCode = "UNAUTHORIZED"
    ErrorCodeInvalid          ScyllaErrorCode = "INVALID"
    ErrorCodeConfigError      ScyllaErrorCode = "CONFIG_ERROR"
    ErrorCodeAlreadyExists    ScyllaErrorCode = "ALREADY_EXISTS"
    ErrorCodeUnprepared       ScyllaErrorCode = "UNPREPARED"
    ErrorCodeConnectionFailed ScyllaErrorCode = "CONNECTION_FAILED"
    ErrorCodeUnknown          ScyllaErrorCode = "UNKNOWN"
)
```

### Usage Examples

#### Basic Operation Logging
```go
// Create operation context
opCtx := NewOperationContext("Add").
    WithCID(cid).
    WithDuration(duration).
    WithConsistency("QUORUM").
    WithQueryType("INSERT")

// Log the operation
structuredLogger.LogOperation(ctx, opCtx)
```

#### Query Logging
```go
// Log CQL query execution
structuredLogger.LogQuery(ctx, query, args, duration, err)
```

#### Retry Logging
```go
// Log retry attempts
structuredLogger.LogRetry("Add", attempt, delay, lastError, "timeout")
```

#### Batch Operation Logging
```go
// Log batch operations
structuredLogger.LogBatchOperation("LOGGED", batchSize, duration, err)
```

#### Connection Event Logging
```go
// Log connection events
details := map[string]interface{}{
    "duration_ms": 100,
    "retry_count": 2,
}
structuredLogger.LogConnectionEvent("connection_established", host, datacenter, details)
```

### Log Output Examples

#### Successful Operation
```
2025-09-10T18:14:58.208-0400    INFO    scyllastate     Operation completed: operation=Add cid=QmTest123 duration_ms=100 consistency=QUORUM query_type=INSERT
```

#### Failed Operation with Error
```
2025-09-10T18:14:58.208-0400    ERROR   scyllastate     ScyllaDB operation failed: timeout error (error_code=TIMEOUT, error_type=*errors.errorString) operation=Get cid=QmTest456 duration_ms=5000 retries=3 consistency=LOCAL_QUORUM
```

#### Query Execution
```
2025-09-10T18:14:58.208-0400    ERROR   scyllastate     CQL query failed: duration_ms=2000 args_count=2 query=INSERT INTO pins_by_cid VALUES (?, ?, ?) args=[456 []byte{len=5}] error_code=TIMEOUT error_message=write timeout query_type=INSERT query_hash=q_af68
```

#### Batch Operation
```
2025-09-10T18:14:58.209-0400    ERROR   scyllastate     Batch operation failed: batch_type=UNLOGGED batch_size=50 duration_ms=1000 throughput_ops_per_sec=50 error_code=UNKNOWN error_message=batch too large
```

#### Connection Event
```
2025-09-10T18:14:58.209-0400    ERROR   scyllastate     ScyllaDB node unavailable: event_type=node_down host=192.168.1.100 datacenter=datacenter1 duration_ms=100 retry_count=2
```

## Configuration

### Logging Configuration
```go
type LoggingConfig struct {
    TracingEnabled bool // Enable detailed query tracing and debug logging
}
```

### Integration with IPFS-Cluster Logging
The structured logger integrates with the IPFS-Cluster logging system using the `go-log/v2` framework:

```go
// Logger registration in logging.go
var LoggingFacilities = map[string]string{
    // ... other facilities
    "scyllastate": "INFO",
    // ... other facilities
}
```

### Log Level Control
- **DEBUG**: Detailed tracing when `TracingEnabled` is true
- **INFO**: Normal operation completion and performance metrics
- **WARN**: Slow operations, retryable errors, connection issues
- **ERROR**: Critical errors, operation failures, node unavailability

## Security Considerations

### Data Sanitization
The logging implementation includes comprehensive data sanitization:

#### Query Sanitization
- String literals in queries are masked with `***`
- Long queries are truncated to prevent log flooding
- Sensitive patterns are automatically detected and masked

#### Argument Sanitization
- String arguments longer than 10 characters are partially masked
- Binary data is replaced with length information
- Numeric and boolean values are preserved for debugging

#### Example Sanitization
```go
// Original query
"SELECT * FROM pins WHERE owner = 'user123'"

// Sanitized query
"SELECT * FROM pins WHERE owner = '***'"

// Original args
[]interface{}{"sensitive_data", []byte{1,2,3}, 123}

// Sanitized args
[]interface{}{"sen***ata", "[]byte{len=3}", 123}
```

## Performance Considerations

### Minimal Overhead
- Structured logging is designed for minimal performance impact
- Field formatting is lazy and only occurs when logging is enabled
- Query sanitization is only performed when tracing is enabled

### Configurable Verbosity
- Debug-level logging can be disabled in production
- Query tracing can be toggled independently
- Field inclusion is conditional based on context

### Efficient Field Formatting
- Fields are formatted as key=value pairs for easy parsing
- Complex objects are converted to string representations
- Large data structures are summarized rather than fully logged

## Monitoring and Alerting Integration

### Structured Fields
All log entries include structured fields that can be parsed by log aggregation systems:

- `operation`: Type of operation (Add, Get, List, Rm, etc.)
- `cid`: Content identifier being operated on
- `duration_ms`: Operation duration in milliseconds
- `error_code`: ScyllaDB-specific error classification
- `consistency`: Consistency level used
- `query_type`: Type of CQL query (SELECT, INSERT, etc.)
- `batch_size`: Number of operations in batch
- `throughput_ops_per_sec`: Operations per second for batch operations

### Log Aggregation
The structured format is compatible with:
- **ELK Stack**: Elasticsearch, Logstash, Kibana
- **Prometheus**: Can extract metrics from log entries
- **Grafana**: Can create dashboards from log data
- **Fluentd**: Can parse and forward structured logs
- **Splunk**: Can index and search structured fields

### Alerting Rules
Example alerting rules based on log content:

```yaml
# High error rate
- alert: ScyllaDBHighErrorRate
  expr: rate(log_entries{level="ERROR",facility="scyllastate"}[5m]) > 0.1
  
# Slow operations
- alert: ScyllaDBSlowOperations
  expr: log_entries{level="WARN",message=~".*slow.*"} > 10
  
# Connection issues
- alert: ScyllaDBConnectionIssues
  expr: log_entries{level="ERROR",message=~".*connection.*"} > 0
```

## Testing

### Comprehensive Test Coverage
The structured logging implementation includes extensive tests:

- **Unit Tests**: All logging functions and error classification
- **Integration Tests**: Real logging output verification
- **Performance Tests**: Logging overhead measurement
- **Sanitization Tests**: Data masking verification

### Test Examples
```go
func TestStructuredLogger_LogOperation_Success(t *testing.T) {
    config := &LoggingConfig{TracingEnabled: true}
    logger := NewStructuredLogger(config)
    
    opCtx := NewOperationContext("Add").
        WithCID(MockCid{value: "QmTest123"}).
        WithDuration(100 * time.Millisecond).
        WithConsistency("QUORUM")
    
    logger.LogOperation(context.Background(), opCtx)
}
```

## Future Enhancements

### Planned Features
1. **Distributed Tracing**: Integration with OpenTelemetry for distributed tracing
2. **Metrics Extraction**: Automatic Prometheus metrics generation from logs
3. **Log Sampling**: Configurable sampling for high-volume environments
4. **Custom Formatters**: Pluggable log formatters for different output formats
5. **Log Correlation**: Request ID tracking across operations

### Performance Optimizations
1. **Async Logging**: Non-blocking log writing for high-throughput scenarios
2. **Log Buffering**: Batched log writing to reduce I/O overhead
3. **Field Caching**: Caching of frequently used field combinations
4. **Conditional Logging**: More granular control over what gets logged

## Troubleshooting Guide

### Common Issues

#### High Log Volume
- Disable tracing in production: `TracingEnabled: false`
- Increase log level to WARN or ERROR
- Implement log sampling for high-traffic operations

#### Missing Context
- Ensure operation contexts are properly created with `NewOperationContext()`
- Use builder methods to add relevant context: `WithCID()`, `WithDuration()`, etc.
- Check that structured logger is properly initialized

#### Performance Impact
- Monitor logging overhead in production
- Consider async logging for high-throughput scenarios
- Disable debug logging when not needed

### Debug Commands
```bash
# Set log level for scyllastate
export GOLOG_LOG_LEVEL="scyllastate=DEBUG"

# Enable tracing
export SCYLLADB_TRACING_ENABLED=true

# View structured logs
tail -f /var/log/ipfs-cluster.log | grep scyllastate
```

## Conclusion

The structured logging implementation provides comprehensive observability for the ScyllaDB state store, enabling effective monitoring, debugging, and troubleshooting. The implementation follows best practices for structured logging while maintaining high performance and security through data sanitization.

The logging system is designed to be:
- **Comprehensive**: Covers all aspects of ScyllaDB operations
- **Performant**: Minimal overhead in production environments
- **Secure**: Automatic sanitization of sensitive data
- **Extensible**: Easy to add new logging contexts and fields
- **Compatible**: Works with existing log aggregation and monitoring systems

This implementation fulfills the requirements for task 7.2 "Добавить структурированное журналирование" by providing:
- ✅ Contextual logging of operations with CID and duration
- ✅ Detailed error logging with ScyllaDB error codes
- ✅ Debug-level tracing for troubleshooting
- ✅ Integration with IPFS-Cluster logging framework
- ✅ Comprehensive test coverage