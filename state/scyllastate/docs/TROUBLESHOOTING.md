# ScyllaDB State Store Troubleshooting Guide

This guide helps diagnose and resolve common issues with the ScyllaDB state store.

## Table of Contents

- [Common Issues](#common-issues)
- [Connection Problems](#connection-problems)
- [Performance Issues](#performance-issues)
- [Data Consistency Issues](#data-consistency-issues)
- [Configuration Problems](#configuration-problems)
- [Monitoring and Debugging](#monitoring-and-debugging)
- [Recovery Procedures](#recovery-procedures)

## Common Issues

### Issue: "Failed to create ScyllaDB session"

**Symptoms:**
```
Error: failed to create ScyllaDB session: gocql: no hosts available
```

**Possible Causes:**
1. ScyllaDB is not running
2. Network connectivity issues
3. Incorrect host/port configuration
4. Authentication problems
5. Firewall blocking connections

**Solutions:**

1. **Check ScyllaDB Status:**
```bash
# For Docker
docker ps | grep scylla

# For systemd
systemctl status scylla-server

# Check ScyllaDB logs
docker logs scylla-container
# or
journalctl -u scylla-server
```

2. **Test Network Connectivity:**
```bash
# Test basic connectivity
telnet scylla-host 9042
nc -zv scylla-host 9042

# Test from application host
ping scylla-host
```

3. **Verify Configuration:**
```go
func diagnoseConnection(config *scyllastate.Config) {
    fmt.Printf("Testing connection to: %v:%d\n", config.Hosts, config.Port)
    
    for _, host := range config.Hosts {
        conn, err := net.DialTimeout("tcp", 
            fmt.Sprintf("%s:%d", host, config.Port), 
            5*time.Second)
        if err != nil {
            fmt.Printf("✗ Cannot connect to %s:%d: %v\n", host, config.Port, err)
        } else {
            fmt.Printf("✓ Connection to %s:%d successful\n", host, config.Port)
            conn.Close()
        }
    }
}
```

### Issue: "Query timeout"

**Symptoms:**
```
Error: query timeout after 10s
Warning: operation took longer than expected
```

**Possible Causes:**
1. Network latency issues
2. ScyllaDB overload
3. Large query results
4. Insufficient resources
5. Suboptimal query patterns

**Solutions:**

1. **Increase Timeouts:**
```go
config := &scyllastate.Config{
    Timeout:        30 * time.Second,  // Increase from 10s
    ConnectTimeout: 15 * time.Second,  // Increase from 5s
}
```

2. **Check ScyllaDB Performance:**
```bash
# Check ScyllaDB metrics
nodetool tpstats
nodetool cfstats

# Monitor system resources
top
iostat -x 1
```

3. **Optimize Queries:**
```go
// Use appropriate page sizes
config.PageSize = 1000  // Smaller pages for faster response

// Enable token-aware routing
config.TokenAwareRouting = true
```

### Issue: "Authentication failed"

**Symptoms:**
```
Error: authentication failed: bad username/password
Error: SSL/TLS connection failed
```

**Solutions:**

1. **Verify Credentials:**
```go
config := &scyllastate.Config{
    Username: os.Getenv("SCYLLA_USERNAME"),
    Password: os.Getenv("SCYLLA_PASSWORD"),
}

// Test credentials
fmt.Printf("Using username: %s\n", config.Username)
fmt.Printf("Password set: %t\n", config.Password != "")
```

2. **Check TLS Configuration:**
```go
config.TLS = &scyllastate.TLSConfig{
    Enabled:            true,
    CertFile:           "/path/to/client.crt",
    KeyFile:            "/path/to/client.key",
    CAFile:             "/path/to/ca.crt",
    InsecureSkipVerify: false,  // Set to true for testing only
}
```

## Connection Problems

### Intermittent Connection Failures

**Diagnosis:**
```go
func monitorConnections(state *scyllastate.ScyllaState) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            
            start := time.Now()
            testPin := createHealthCheckPin()
            err := state.Add(ctx, testPin)
            duration := time.Since(start)
            
            if err != nil {
                log.Printf("Connection test failed after %v: %v", duration, err)
            } else {
                log.Printf("Connection test passed in %v", duration)
                state.Rm(ctx, testPin.Cid)
            }
            
            cancel()
        }
    }
}
```

**Solutions:**

1. **Configure Retry Policy:**
```go
config.RetryPolicy = &scyllastate.RetryPolicyConfig{
    NumRetries:    5,
    MinRetryDelay: 100 * time.Millisecond,
    MaxRetryDelay: 30 * time.Second,
}
```

2. **Increase Connection Pool:**
```go
config.NumConns = 8        // More connections per host
config.MaxWaitTime = 5 * time.Second  // Longer wait for connections
```

### Connection Pool Exhaustion

**Symptoms:**
```
Error: connection pool exhausted
Warning: waiting for available connection
```

**Solutions:**

1. **Optimize Connection Usage:**
```go
// Ensure proper resource cleanup
func properResourceUsage(state *scyllastate.ScyllaState) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()  // Always cancel context
    
    // Perform operations
    err := state.Add(ctx, pin)
    // Context is automatically cleaned up
}
```

2. **Increase Pool Size:**
```go
config.NumConns = 16       // Increase connections per host
config.MaxWaitTime = 10 * time.Second  // Allow longer waits
```

## Performance Issues

### Slow Query Performance

**Diagnosis:**
```go
func measureOperationLatency(state *scyllastate.ScyllaState) {
    operations := []struct {
        name string
        fn   func() (time.Duration, error)
    }{
        {"add", func() (time.Duration, error) {
            start := time.Now()
            pin := createTestPin()
            err := state.Add(context.Background(), pin)
            return time.Since(start), err
        }},
        {"get", func() (time.Duration, error) {
            start := time.Now()
            pin := createTestPin()
            state.Add(context.Background(), pin)
            _, err := state.Get(context.Background(), pin.Cid)
            duration := time.Since(start)
            state.Rm(context.Background(), pin.Cid)
            return duration, err
        }},
    }
    
    for _, op := range operations {
        duration, err := op.fn()
        fmt.Printf("%s: %v (error: %v)\n", op.name, duration, err)
    }
}
```

**Solutions:**

1. **Enable Performance Optimizations:**
```go
config := &scyllastate.Config{
    TokenAwareRouting:          true,   // Route to correct nodes
    DCAwareRouting:            true,   // Prefer local datacenter
    PageSize:                  5000,   // Optimize page size
    PreparedStatementCacheSize: 2000,  // Cache more statements
}
```

2. **Use Batch Operations:**
```go
// Instead of individual operations
for _, pin := range pins {
    state.Add(ctx, pin)  // Slow
}

// Use batch operations
batchState := state.Batch()
for _, pin := range pins {
    batchState.Add(ctx, pin)
}
batchState.Commit(ctx)  // Fast
```

### High Memory Usage

**Diagnosis:**
```go
func monitorMemoryUsage() {
    var m runtime.MemStats
    
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            runtime.ReadMemStats(&m)
            
            fmt.Printf("Memory Usage:\n")
            fmt.Printf("  Alloc: %d KB\n", m.Alloc/1024)
            fmt.Printf("  TotalAlloc: %d KB\n", m.TotalAlloc/1024)
            fmt.Printf("  Sys: %d KB\n", m.Sys/1024)
            fmt.Printf("  NumGC: %d\n", m.NumGC)
        }
    }
}
```

**Solutions:**

1. **Optimize Page Sizes:**
```go
// For large datasets, use smaller page sizes
config.PageSize = 1000  // Reduce from default 5000
```

2. **Implement Streaming:**
```go
func streamLargeResults(state *scyllastate.ScyllaState) {
    pinChan := make(chan api.Pin, 100)  // Small buffer
    
    go func() {
        defer close(pinChan)
        state.List(context.Background(), pinChan)
    }()
    
    // Process pins as they arrive
    for pin := range pinChan {
        // Process immediately, don't accumulate
        processPin(pin)
    }
}
```

## Data Consistency Issues

### Inconsistent Read Results

**Symptoms:**
```
Warning: pin exists check returned false, but get succeeded
Error: data inconsistency detected between operations
```

**Diagnosis:**
```go
func checkConsistency(state *scyllastate.ScyllaState, cid api.Cid) {
    ctx := context.Background()
    
    // Check with different consistency levels
    consistencyLevels := []string{"ONE", "QUORUM", "ALL"}
    
    for _, level := range consistencyLevels {
        // Create new state with different consistency
        config := state.GetConfig()  // Hypothetical method
        config.Consistency = level
        
        testState, err := scyllastate.New(ctx, config)
        if err != nil {
            continue
        }
        
        exists, err := testState.Has(ctx, cid)
        fmt.Printf("Consistency %s: exists=%t, error=%v\n", level, exists, err)
        
        testState.Close()
    }
}
```

**Solutions:**

1. **Use Appropriate Consistency Levels:**
```go
// For critical operations
config.Consistency = "QUORUM"  // or "ALL"

// For read-heavy workloads where eventual consistency is acceptable
config.Consistency = "ONE"
```

2. **Wait for Consistency:**
```go
func waitForConsistency(state *scyllastate.ScyllaState, cid api.Cid) error {
    ctx := context.Background()
    maxAttempts := 10
    
    for attempt := 0; attempt < maxAttempts; attempt++ {
        exists, err := state.Has(ctx, cid)
        if err != nil {
            return err
        }
        
        if exists {
            return nil
        }
        
        time.Sleep(100 * time.Millisecond)
    }
    
    return fmt.Errorf("pin not found after %d attempts", maxAttempts)
}
```

### Replication Lag

**Diagnosis:**
```bash
# Check ScyllaDB replication status
nodetool status
nodetool describering keyspace_name

# Check for hints (indicating replication lag)
nodetool tpstats | grep -i hint
```

**Solutions:**

1. **Monitor and Repair:**
```bash
# Run repair to ensure consistency
nodetool repair keyspace_name

# Check repair status
nodetool compactionstats
```

2. **Adjust Consistency Requirements:**
```go
// For writes that must be immediately consistent
config.Consistency = "ALL"

// For eventual consistency scenarios
config.Consistency = "ONE"
```

## Configuration Problems

### Invalid Configuration Values

**Diagnosis:**
```go
func validateConfiguration(config *scyllastate.Config) error {
    var errors []string
    
    if len(config.Hosts) == 0 {
        errors = append(errors, "no hosts specified")
    }
    
    if config.Port <= 0 || config.Port > 65535 {
        errors = append(errors, fmt.Sprintf("invalid port: %d", config.Port))
    }
    
    if config.Keyspace == "" {
        errors = append(errors, "keyspace not specified")
    }
    
    validConsistencies := []string{"ONE", "TWO", "THREE", "QUORUM", "ALL", "LOCAL_QUORUM", "EACH_QUORUM"}
    if !contains(validConsistencies, config.Consistency) {
        errors = append(errors, fmt.Sprintf("invalid consistency: %s", config.Consistency))
    }
    
    if len(errors) > 0 {
        return fmt.Errorf("configuration errors: %s", strings.Join(errors, ", "))
    }
    
    return nil
}
```

### Environment Variable Issues

**Diagnosis:**
```go
func checkEnvironmentVariables() {
    requiredVars := []string{
        "SCYLLA_HOSTS",
        "SCYLLA_KEYSPACE", 
        "SCYLLA_USERNAME",
        "SCYLLA_PASSWORD",
    }
    
    for _, varName := range requiredVars {
        value := os.Getenv(varName)
        if value == "" {
            fmt.Printf("✗ Missing environment variable: %s\n", varName)
        } else {
            fmt.Printf("✓ %s is set\n", varName)
        }
    }
}
```

## Monitoring and Debugging

### Enable Debug Logging

```go
func enableDebugLogging(config *scyllastate.Config) {
    config.TracingEnabled = true
    
    // Set log level to debug
    log.SetLevel(log.DebugLevel)
    
    // Enable query tracing
    config.QueryObserver = func(ctx context.Context, q gocql.ObservedQuery) {
        log.Debugf("Query: %s, Duration: %v, Error: %v", 
            q.Statement, q.End.Sub(q.Start), q.Err)
    }
}
```

### Performance Profiling

```go
import _ "net/http/pprof"

func enableProfiling() {
    go func() {
        log.Println("Starting pprof server on :6060")
        log.Println(http.ListenAndServe(":6060", nil))
    }()
}

// Access profiling data:
// go tool pprof http://localhost:6060/debug/pprof/profile
// go tool pprof http://localhost:6060/debug/pprof/heap
```

### Custom Health Checks

```go
func comprehensiveHealthCheck(state *scyllastate.ScyllaState) error {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    checks := []struct {
        name string
        fn   func() error
    }{
        {"connectivity", func() error {
            testPin := createHealthCheckPin()
            return state.Add(ctx, testPin)
        }},
        {"read_performance", func() error {
            start := time.Now()
            testPin := createHealthCheckPin()
            state.Add(ctx, testPin)
            _, err := state.Get(ctx, testPin.Cid)
            duration := time.Since(start)
            
            if duration > 5*time.Second {
                return fmt.Errorf("read operation too slow: %v", duration)
            }
            return err
        }},
        {"write_performance", func() error {
            start := time.Now()
            testPin := createHealthCheckPin()
            err := state.Add(ctx, testPin)
            duration := time.Since(start)
            
            if duration > 5*time.Second {
                return fmt.Errorf("write operation too slow: %v", duration)
            }
            return err
        }},
    }
    
    for _, check := range checks {
        if err := check.fn(); err != nil {
            return fmt.Errorf("health check '%s' failed: %w", check.name, err)
        }
    }
    
    return nil
}
```

## Recovery Procedures

### Recovering from Connection Loss

```go
func recoverFromConnectionLoss(config *scyllastate.Config) (*scyllastate.ScyllaState, error) {
    maxAttempts := 5
    baseDelay := 1 * time.Second
    
    for attempt := 0; attempt < maxAttempts; attempt++ {
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        
        state, err := scyllastate.New(ctx, config)
        cancel()
        
        if err == nil {
            log.Printf("Successfully reconnected after %d attempts", attempt+1)
            return state, nil
        }
        
        delay := time.Duration(attempt+1) * baseDelay
        log.Printf("Connection attempt %d failed, retrying in %v: %v", 
            attempt+1, delay, err)
        
        time.Sleep(delay)
    }
    
    return nil, fmt.Errorf("failed to reconnect after %d attempts", maxAttempts)
}
```

### Data Recovery from Backup

```go
func recoverFromBackup(backupPath string, state *scyllastate.ScyllaState) error {
    ctx := context.Background()
    
    // Read backup data
    backupData, err := ioutil.ReadFile(backupPath)
    if err != nil {
        return fmt.Errorf("failed to read backup: %w", err)
    }
    
    // Restore data
    buf := bytes.NewReader(backupData)
    err = state.Unmarshal(buf)
    if err != nil {
        return fmt.Errorf("failed to restore from backup: %w", err)
    }
    
    log.Printf("Successfully restored from backup: %s", backupPath)
    return nil
}
```

### Cluster Recovery

```bash
#!/bin/bash
# cluster-recovery.sh

echo "Starting cluster recovery..."

# Check cluster status
nodetool status

# Repair all keyspaces
for keyspace in $(nodetool describecluster | grep "Keyspace" | awk '{print $2}'); do
    echo "Repairing keyspace: $keyspace"
    nodetool repair $keyspace
done

# Check for streaming operations
nodetool netstats

echo "Cluster recovery completed"
```

## Getting Help

### Collecting Diagnostic Information

```go
func collectDiagnostics(state *scyllastate.ScyllaState) map[string]interface{} {
    diagnostics := make(map[string]interface{})
    
    // Configuration information
    config := state.GetConfig()  // Hypothetical method
    diagnostics["config"] = map[string]interface{}{
        "hosts":       config.Hosts,
        "keyspace":    config.Keyspace,
        "consistency": config.Consistency,
        "timeout":     config.Timeout.String(),
    }
    
    // Runtime information
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    diagnostics["runtime"] = map[string]interface{}{
        "go_version":    runtime.Version(),
        "num_goroutine": runtime.NumGoroutine(),
        "memory_alloc":  m.Alloc,
        "memory_sys":    m.Sys,
    }
    
    // Health check
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    testPin := createHealthCheckPin()
    err := state.Add(ctx, testPin)
    diagnostics["health_check"] = map[string]interface{}{
        "status": err == nil,
        "error":  fmt.Sprintf("%v", err),
    }
    
    if err == nil {
        state.Rm(ctx, testPin.Cid)
    }
    
    return diagnostics
}
```

### Log Analysis

```bash
# Analyze ScyllaDB logs for errors
grep -i error /var/log/scylla/scylla.log | tail -20

# Check for timeout issues
grep -i timeout /var/log/scylla/scylla.log | tail -10

# Monitor connection issues
grep -i "connection" /var/log/scylla/scylla.log | tail -10

# Application logs
grep -E "(error|timeout|failed)" /var/log/ipfs-cluster/cluster.log
```

For additional support:
- Check the [ScyllaDB documentation](https://docs.scylladb.com/)
- Review [IPFS-Cluster documentation](https://cluster.ipfs.io/)
- Open an issue on [GitHub](https://github.com/ipfs-cluster/ipfs-cluster/issues)