# ScyllaDB State Store Usage Guide

This guide provides comprehensive examples and best practices for using the ScyllaDB state store in various scenarios.

## Table of Contents

- [Getting Started](#getting-started)
- [Basic Operations](#basic-operations)
- [Batch Operations](#batch-operations)
- [Migration Scenarios](#migration-scenarios)
- [Monitoring and Observability](#monitoring-and-observability)
- [Production Deployment](#production-deployment)
- [Troubleshooting](#troubleshooting)
- [Best Practices](#best-practices)

## Getting Started

### Quick Start

```go
package main

import (
    "context"
    "log"
    "time"
    
    "github.com/ipfs-cluster/ipfs-cluster/state/scyllastate"
)

func main() {
    // 1. Create configuration
    config := &scyllastate.Config{
        Hosts:       []string{"127.0.0.1"},
        Port:        9042,
        Keyspace:    "ipfs_pins",
        Consistency: "QUORUM",
        Timeout:     10 * time.Second,
    }
    config.Default()

    // 2. Create state store
    ctx := context.Background()
    state, err := scyllastate.New(ctx, config)
    if err != nil {
        log.Fatal("Failed to create state store:", err)
    }
    defer state.Close()

    // 3. Use the state store
    // ... your application logic here
}
```

### Integration with IPFS-Cluster

```go
package main

import (
    "context"
    
    ipfscluster "github.com/ipfs-cluster/ipfs-cluster"
    "github.com/ipfs-cluster/ipfs-cluster/state/scyllastate"
)

func createClusterWithScyllaDB() (*ipfscluster.Cluster, error) {
    // Create ScyllaDB state store
    scyllaConfig := &scyllastate.Config{
        Hosts:       []string{"scylla-node1", "scylla-node2", "scylla-node3"},
        Port:        9042,
        Keyspace:    "ipfs_cluster_state",
        Consistency: "QUORUM",
    }
    scyllaConfig.Default()

    ctx := context.Background()
    state, err := scyllastate.New(ctx, scyllaConfig)
    if err != nil {
        return nil, err
    }

    // Create cluster configuration
    clusterConfig := &ipfscluster.Config{
        // ... other cluster configuration
    }

    // Create cluster with ScyllaDB state store
    cluster, err := ipfscluster.NewCluster(
        ctx,
        clusterConfig,
        state, // Use ScyllaDB as state store
        // ... other components
    )
    
    return cluster, err
}
```

## Basic Operations

### Adding Pins

```go
func addPinExample(state *scyllastate.ScyllaState) error {
    ctx := context.Background()
    
    // Create a pin
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
        MaxDepth:             -1,
        PinOptions: api.PinOptions{
            Name: "my-document",
            UserDefined: map[string]string{
                "category": "documents",
                "priority": "high",
            },
        },
    }

    // Add the pin
    err := state.Add(ctx, pin)
    if err != nil {
        return fmt.Errorf("failed to add pin: %w", err)
    }

    fmt.Printf("Pin added successfully: %s\n", pin.Cid.String())
    return nil
}
```

### Retrieving Pins

```go
func getPinExample(state *scyllastate.ScyllaState, cid api.Cid) error {
    ctx := context.Background()
    
    // Get pin by CID
    pin, err := state.Get(ctx, cid)
    if err != nil {
        if err.Error() == "not found" {
            fmt.Printf("Pin not found: %s\n", cid.String())
            return nil
        }
        return fmt.Errorf("failed to get pin: %w", err)
    }

    fmt.Printf("Retrieved pin: %s (name: %s)\n", 
        pin.Cid.String(), pin.PinOptions.Name)
    
    // Print pin details
    fmt.Printf("  Type: %s\n", pin.Type)
    fmt.Printf("  Replication: %d-%d\n", 
        pin.ReplicationFactorMin, pin.ReplicationFactorMax)
    fmt.Printf("  Allocations: %v\n", pin.Allocations)
    
    return nil
}
```

### Checking Pin Existence

```go
func checkPinExample(state *scyllastate.ScyllaState, cid api.Cid) error {
    ctx := context.Background()
    
    exists, err := state.Has(ctx, cid)
    if err != nil {
        return fmt.Errorf("failed to check pin existence: %w", err)
    }

    if exists {
        fmt.Printf("Pin exists: %s\n", cid.String())
    } else {
        fmt.Printf("Pin does not exist: %s\n", cid.String())
    }
    
    return nil
}
```

### Listing Pins

```go
func listPinsExample(state *scyllastate.ScyllaState) error {
    ctx := context.Background()
    
    pinChan := make(chan api.Pin, 100)
    
    // Start listing in goroutine
    go func() {
        defer close(pinChan)
        err := state.List(ctx, pinChan)
        if err != nil {
            log.Printf("Failed to list pins: %v", err)
        }
    }()

    // Process pins
    count := 0
    for pin := range pinChan {
        count++
        fmt.Printf("Pin %d: %s (%s)\n", 
            count, pin.Cid.String(), pin.PinOptions.Name)
        
        // Process pin as needed
        // ...
    }

    fmt.Printf("Total pins listed: %d\n", count)
    return nil
}
```

### Removing Pins

```go
func removePinExample(state *scyllastate.ScyllaState, cid api.Cid) error {
    ctx := context.Background()
    
    err := state.Rm(ctx, cid)
    if err != nil {
        return fmt.Errorf("failed to remove pin: %w", err)
    }

    fmt.Printf("Pin removed successfully: %s\n", cid.String())
    return nil
}
```

## Batch Operations

### Basic Batch Operations

```go
func batchAddExample(state *scyllastate.ScyllaState, pins []api.Pin) error {
    ctx := context.Background()
    
    // Create batch state
    batchState := state.Batch()
    if batchState == nil {
        return fmt.Errorf("failed to create batch state")
    }

    // Add pins to batch
    for i, pin := range pins {
        err := batchState.Add(ctx, pin)
        if err != nil {
            log.Printf("Failed to add pin %d to batch: %v", i, err)
            continue
        }
    }

    // Commit batch
    err := batchState.Commit(ctx)
    if err != nil {
        return fmt.Errorf("failed to commit batch: %w", err)
    }

    fmt.Printf("Batch of %d pins committed successfully\n", len(pins))
    return nil
}
```

### Large Batch Processing

```go
func largeBatchExample(state *scyllastate.ScyllaState, pins []api.Pin) error {
    ctx := context.Background()
    batchSize := 100
    
    batchState := state.Batch()
    processed := 0

    for i, pin := range pins {
        err := batchState.Add(ctx, pin)
        if err != nil {
            log.Printf("Failed to add pin %d: %v", i, err)
            continue
        }
        
        processed++

        // Commit every batchSize operations
        if processed%batchSize == 0 || i == len(pins)-1 {
            err := batchState.Commit(ctx)
            if err != nil {
                log.Printf("Failed to commit batch: %v", err)
                continue
            }
            
            fmt.Printf("Committed batch %d/%d\n", 
                (processed+batchSize-1)/batchSize, 
                (len(pins)+batchSize-1)/batchSize)
        }
    }

    fmt.Printf("Large batch processing completed: %d pins\n", processed)
    return nil
}
```

### Mixed Batch Operations

```go
func mixedBatchExample(state *scyllastate.ScyllaState) error {
    ctx := context.Background()
    
    batchState := state.Batch()

    // Add some pins
    newPins := createTestPins(5)
    for _, pin := range newPins {
        batchState.Add(ctx, pin)
    }

    // Remove some existing pins
    existingCids := []api.Cid{
        // ... CIDs of pins to remove
    }
    for _, cid := range existingCids {
        batchState.Rm(ctx, cid)
    }

    // Commit mixed batch
    err := batchState.Commit(ctx)
    if err != nil {
        return fmt.Errorf("failed to commit mixed batch: %w", err)
    }

    fmt.Printf("Mixed batch committed: %d adds, %d removes\n", 
        len(newPins), len(existingCids))
    return nil
}
```

## Migration Scenarios

### From File-Based State Store

```go
func migrateFromFileStore(sourcePath, scyllaConfig *scyllastate.Config) error {
    ctx := context.Background()
    
    // Open source file-based state
    sourceDS, err := badger.New(sourcePath)
    if err != nil {
        return fmt.Errorf("failed to open source datastore: %w", err)
    }
    defer sourceDS.Close()

    sourceState, err := dsstate.New(ctx, sourceDS, "", nil)
    if err != nil {
        return fmt.Errorf("failed to create source state: %w", err)
    }
    defer sourceState.Close()

    // Create ScyllaDB destination
    destState, err := scyllastate.New(ctx, scyllaConfig)
    if err != nil {
        return fmt.Errorf("failed to create destination state: %w", err)
    }
    defer destState.Close()

    // Perform migration
    var buf bytes.Buffer
    
    fmt.Println("Exporting from source state...")
    err = sourceState.Marshal(&buf)
    if err != nil {
        return fmt.Errorf("failed to export source state: %w", err)
    }

    fmt.Println("Importing to ScyllaDB state...")
    err = destState.Unmarshal(&buf)
    if err != nil {
        return fmt.Errorf("failed to import to destination state: %w", err)
    }

    fmt.Println("Migration completed successfully")
    return nil
}
```

### Streaming Migration for Large Datasets

```go
func streamingMigration(sourceState state.State, destState *scyllastate.ScyllaState) error {
    ctx := context.Background()
    
    pinChan := make(chan api.Pin, 1000)
    
    // Start reading from source
    go func() {
        defer close(pinChan)
        err := sourceState.List(ctx, pinChan)
        if err != nil {
            log.Printf("Failed to list source pins: %v", err)
        }
    }()

    // Stream to destination using batches
    batchState := destState.Batch()
    batchSize := 500
    processed := 0

    for pin := range pinChan {
        err := batchState.Add(ctx, pin)
        if err != nil {
            log.Printf("Failed to add pin to batch: %v", err)
            continue
        }
        
        processed++

        // Commit batch when full
        if processed%batchSize == 0 {
            err := batchState.Commit(ctx)
            if err != nil {
                log.Printf("Failed to commit batch: %v", err)
            } else {
                fmt.Printf("Migrated %d pins\n", processed)
            }
        }
    }

    // Commit final batch
    if processed%batchSize != 0 {
        err := batchState.Commit(ctx)
        if err != nil {
            log.Printf("Failed to commit final batch: %v", err)
        }
    }

    fmt.Printf("Streaming migration completed: %d pins\n", processed)
    return nil
}
```

## Monitoring and Observability

### Basic Metrics Collection

```go
func setupMetrics(state *scyllastate.ScyllaState) {
    // Metrics are automatically collected when MetricsEnabled is true
    // Access metrics via Prometheus /metrics endpoint
    
    http.Handle("/metrics", promhttp.Handler())
    
    // Add custom health check
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        if isHealthy(state) {
            w.WriteHeader(http.StatusOK)
            w.Write([]byte("OK"))
        } else {
            w.WriteHeader(http.StatusServiceUnavailable)
            w.Write([]byte("UNHEALTHY"))
        }
    })
    
    log.Println("Metrics server starting on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}

func isHealthy(state *scyllastate.ScyllaState) bool {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    // Perform health check operations
    testPin := createHealthCheckPin()
    
    // Test write
    if err := state.Add(ctx, testPin); err != nil {
        return false
    }
    
    // Test read
    if _, err := state.Get(ctx, testPin.Cid); err != nil {
        return false
    }
    
    // Cleanup
    state.Rm(ctx, testPin.Cid)
    
    return true
}
```

### Custom Metrics

```go
var (
    pinOperationDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "ipfs_cluster_pin_operation_duration_seconds",
            Help: "Duration of pin operations",
        },
        []string{"operation", "status"},
    )
    
    totalPinsGauge = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "ipfs_cluster_total_pins",
            Help: "Total number of pins in the cluster",
        },
    )
)

func init() {
    prometheus.MustRegister(pinOperationDuration)
    prometheus.MustRegister(totalPinsGauge)
}

func recordOperation(operation string, duration time.Duration, err error) {
    status := "success"
    if err != nil {
        status = "error"
    }
    
    pinOperationDuration.WithLabelValues(operation, status).Observe(duration.Seconds())
}

func updateTotalPins(count float64) {
    totalPinsGauge.Set(count)
}
```

## Production Deployment

### High Availability Configuration

```go
func createHAConfig() *scyllastate.Config {
    return &scyllastate.Config{
        Hosts: []string{
            "scylla-node1.prod.example.com",
            "scylla-node2.prod.example.com",
            "scylla-node3.prod.example.com",
        },
        Port:              9042,
        Keyspace:          "ipfs_cluster_prod",
        Username:          os.Getenv("SCYLLA_USERNAME"),
        Password:          os.Getenv("SCYLLA_PASSWORD"),
        Consistency:       "QUORUM",
        TokenAwareRouting: true,
        DCAwareRouting:    true,
        LocalDC:           "datacenter1",
        
        // Connection settings
        NumConns:       8,
        Timeout:        30 * time.Second,
        ConnectTimeout: 15 * time.Second,
        
        // Performance settings
        PageSize:                   10000,
        PreparedStatementCacheSize: 2000,
        
        // TLS configuration
        TLS: &scyllastate.TLSConfig{
            Enabled:  true,
            CertFile: "/etc/ssl/certs/client.pem",
            KeyFile:  "/etc/ssl/private/client-key.pem",
            CAFile:   "/etc/ssl/certs/ca.pem",
        },
        
        // Retry policy
        RetryPolicy: &scyllastate.RetryPolicyConfig{
            NumRetries:    5,
            MinRetryDelay: 100 * time.Millisecond,
            MaxRetryDelay: 30 * time.Second,
        },
        
        // Monitoring
        MetricsEnabled: true,
    }
}
```

### Connection Pool Management

```go
func manageConnectionPool(state *scyllastate.ScyllaState) {
    // Monitor connection health
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            // Check connection health
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            
            testPin := createHealthCheckPin()
            err := state.Add(ctx, testPin)
            
            if err != nil {
                log.Printf("Connection health check failed: %v", err)
                // Implement reconnection logic if needed
            } else {
                state.Rm(ctx, testPin.Cid)
                log.Println("Connection health check passed")
            }
            
            cancel()
        }
    }
}
```

### Graceful Shutdown

```go
func gracefulShutdown(state *scyllastate.ScyllaState) {
    // Set up signal handling
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    
    // Wait for shutdown signal
    <-sigChan
    
    log.Println("Shutting down gracefully...")
    
    // Close state store
    if err := state.Close(); err != nil {
        log.Printf("Error closing state store: %v", err)
    }
    
    log.Println("Shutdown complete")
}
```

## Troubleshooting

### Connection Issues

```go
func diagnoseConnection(config *scyllastate.Config) error {
    // Test basic connectivity
    for _, host := range config.Hosts {
        conn, err := net.DialTimeout("tcp", 
            fmt.Sprintf("%s:%d", host, config.Port), 
            5*time.Second)
        if err != nil {
            return fmt.Errorf("cannot connect to %s:%d: %w", host, config.Port, err)
        }
        conn.Close()
        fmt.Printf("✓ Connection to %s:%d successful\n", host, config.Port)
    }
    
    // Test ScyllaDB connection
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    state, err := scyllastate.New(ctx, config)
    if err != nil {
        return fmt.Errorf("failed to create ScyllaDB connection: %w", err)
    }
    defer state.Close()
    
    fmt.Println("✓ ScyllaDB connection successful")
    return nil
}
```

### Performance Debugging

```go
func debugPerformance(state *scyllastate.ScyllaState) {
    ctx := context.Background()
    
    // Test operation latencies
    operations := []struct {
        name string
        fn   func() error
    }{
        {"add", func() error {
            pin := createTestPin()
            return state.Add(ctx, pin)
        }},
        {"get", func() error {
            pin := createTestPin()
            state.Add(ctx, pin)
            _, err := state.Get(ctx, pin.Cid)
            state.Rm(ctx, pin.Cid)
            return err
        }},
        {"has", func() error {
            pin := createTestPin()
            state.Add(ctx, pin)
            _, err := state.Has(ctx, pin.Cid)
            state.Rm(ctx, pin.Cid)
            return err
        }},
    }
    
    for _, op := range operations {
        start := time.Now()
        err := op.fn()
        duration := time.Since(start)
        
        status := "✓"
        if err != nil {
            status = "✗"
        }
        
        fmt.Printf("%s %s operation: %v (error: %v)\n", 
            status, op.name, duration, err)
    }
}
```

## Best Practices

### 1. Configuration Management

```go
// Use environment variables for sensitive data
func loadConfigFromEnv() *scyllastate.Config {
    config := &scyllastate.Config{
        Hosts:       strings.Split(os.Getenv("SCYLLA_HOSTS"), ","),
        Port:        getEnvInt("SCYLLA_PORT", 9042),
        Keyspace:    getEnvString("SCYLLA_KEYSPACE", "ipfs_pins"),
        Username:    os.Getenv("SCYLLA_USERNAME"),
        Password:    os.Getenv("SCYLLA_PASSWORD"),
        Consistency: getEnvString("SCYLLA_CONSISTENCY", "QUORUM"),
    }
    
    config.Default()
    return config
}

func getEnvString(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
    if value := os.Getenv(key); value != "" {
        if intValue, err := strconv.Atoi(value); err == nil {
            return intValue
        }
    }
    return defaultValue
}
```

### 2. Error Handling

```go
func robustOperation(state *scyllastate.ScyllaState, pin api.Pin) error {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // Retry logic
    maxRetries := 3
    for attempt := 0; attempt < maxRetries; attempt++ {
        err := state.Add(ctx, pin)
        if err == nil {
            return nil
        }
        
        // Check if error is retryable
        if isRetryableError(err) && attempt < maxRetries-1 {
            backoff := time.Duration(attempt+1) * time.Second
            log.Printf("Operation failed (attempt %d/%d), retrying in %v: %v", 
                attempt+1, maxRetries, backoff, err)
            time.Sleep(backoff)
            continue
        }
        
        return fmt.Errorf("operation failed after %d attempts: %w", attempt+1, err)
    }
    
    return nil
}

func isRetryableError(err error) bool {
    if err == nil {
        return false
    }
    
    errStr := err.Error()
    retryableErrors := []string{
        "timeout",
        "unavailable",
        "overloaded",
        "connection refused",
    }
    
    for _, retryable := range retryableErrors {
        if strings.Contains(strings.ToLower(errStr), retryable) {
            return true
        }
    }
    
    return false
}
```

### 3. Resource Management

```go
func properResourceManagement() {
    config := createConfig()
    
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    state, err := scyllastate.New(ctx, config)
    if err != nil {
        log.Fatal("Failed to create state store:", err)
    }
    
    // Always close resources
    defer func() {
        if err := state.Close(); err != nil {
            log.Printf("Error closing state store: %v", err)
        }
    }()
    
    // Use the state store
    // ...
}
```

### 4. Batch Size Optimization

```go
func optimizedBatchProcessing(state *scyllastate.ScyllaState, pins []api.Pin) error {
    ctx := context.Background()
    
    // Determine optimal batch size based on data size
    optimalBatchSize := calculateOptimalBatchSize(pins)
    
    batchState := state.Batch()
    processed := 0
    
    for i, pin := range pins {
        err := batchState.Add(ctx, pin)
        if err != nil {
            log.Printf("Failed to add pin %d to batch: %v", i, err)
            continue
        }
        
        processed++
        
        // Commit when batch is optimal size
        if processed%optimalBatchSize == 0 || i == len(pins)-1 {
            start := time.Now()
            err := batchState.Commit(ctx)
            duration := time.Since(start)
            
            if err != nil {
                log.Printf("Batch commit failed: %v", err)
            } else {
                log.Printf("Committed batch of %d pins in %v", 
                    min(optimalBatchSize, processed), duration)
            }
        }
    }
    
    return nil
}

func calculateOptimalBatchSize(pins []api.Pin) int {
    // Simple heuristic: adjust batch size based on average pin size
    if len(pins) == 0 {
        return 100
    }
    
    // Estimate average pin size (simplified)
    avgSize := 1000 // bytes, rough estimate
    
    // Target batch size around 1MB
    targetBatchBytes := 1024 * 1024
    optimalSize := targetBatchBytes / avgSize
    
    // Clamp between reasonable bounds
    if optimalSize < 50 {
        return 50
    }
    if optimalSize > 1000 {
        return 1000
    }
    
    return optimalSize
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
```

For more detailed examples, see the [examples directory](../examples/) and refer to the [configuration guide](./CONFIGURATION.md) for advanced setup options.