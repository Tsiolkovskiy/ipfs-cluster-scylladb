# Design Document

## Overview

This project implements ScyllaDB integration as a state storage backend for IPFS-Cluster, replacing existing consensus-based solutions (etcd/Consul/CRDT) with a scalable, high-performance distributed database. The integration will enable managing trillions of pins with predictable performance and linear scalability.

## Architecture

### High-Level Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  IPFS-Cluster   │    │  IPFS-Cluster   │    │  IPFS-Cluster   │
│     Node 1      │    │     Node 2      │    │     Node N      │
│                 │    │                 │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │ScyllaDB     │ │    │ │ScyllaDB     │ │    │ │ScyllaDB     │ │
│ │State        │ │    │ │State        │ │    │ │State        │ │
│ │Storage      │ │    │ │Storage      │ │    │ │Storage      │ │
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

### Integration Architecture

ScyllaDB State Store implements the `state.State` interface and integrates into IPFS-Cluster as a new state storage type:

```
┌─────────────────────────────────────────────────────────────┐
│                    IPFS-Cluster                             │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │  Cluster    │  │ Consensus   │  │   State             │  │
│  │  Manager    │  │   Layer     │  │   Storage           │  │
│  │             │  │             │  │                     │  │
│  │             │  │             │  │ ┌─────────────────┐ │  │
│  │             │  │             │  │ │  ScyllaDB       │ │  │
│  │             │  │             │  │ │  Storage        │ │  │
│  │             │  │             │  │ │  (New)          │ │  │
│  │             │  │             │  │ └─────────────────┘ │  │
│  │             │  │             │  │ ┌─────────────────┐ │  │
│  │             │  │             │  │ │  CRDT           │ │  │
│  │             │  │             │  │ │  (Existing)     │ │  │
│  │             │  │             │  │ └─────────────────┘ │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │ ScyllaDB Driver │
                    │    (gocql)      │
                    └─────────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │ ScyllaDB Cluster│
                    └─────────────────┘
```

## Components and Interfaces

### 1. ScyllaDB State Storage

**Location:** `state/scyllastate/`

**Main Files:**
- `scyllastate.go` - main implementation
- `config.go` - configuration
- `migrations.go` - DB schema and migrations
- `scyllastate_test.go` - tests

**Interfaces:**
```go
// Implements state.State
type ScyllaState struct {
    session     *gocql.Session
    keyspace    string
    config      *Config
    codecHandle codec.Handle
    totalPins   int64
    metrics     *Metrics
}

// Implements state.BatchingState  
type ScyllaBatchingState struct {
    *ScyllaState
    batch *gocql.Batch
}
```

### 2. Configuration

**Configuration Structure:**
```go
type Config struct {
    config.Saver
    
    // Cluster connection
    Hosts               []string
    Port                int
    Keyspace            string
    Username            string
    Password            string
    
    // TLS settings
    TLSEnabled          bool
    TLSCertFile         string
    TLSKeyFile          string
    TLSCAFile           string
    TLSInsecureSkipVerify bool
    
    // Performance settings
    NumConns            int
    Timeout             time.Duration
    ConnectTimeout      time.Duration
    
    // Consistency settings
    Consistency         gocql.Consistency
    SerialConsistency   gocql.SerialConsistency
    
    // Retry and backoff settings
    RetryPolicy         RetryPolicyConfig
    
    // Batch processing settings
    BatchSize           int
    BatchTimeout        time.Duration
    
    // Monitoring
    MetricsEnabled      bool
    TracingEnabled      bool
}

type RetryPolicyConfig struct {
    NumRetries      int
    MinRetryDelay   time.Duration
    MaxRetryDelay   time.Duration
}
```

### 3. Data Schema

**Keyspace and Tables:**

```cql
-- Main table for storing pin metadata (source of truth)
CREATE TABLE IF NOT EXISTS ipfs_pins.pins_by_cid (
    mh_prefix smallint,          -- First 2 bytes of digest for sharding
    cid_bin blob,                -- Binary multihash
    pin_type tinyint,            -- 0=direct, 1=recursive
    rf tinyint,                  -- Required replication factor
    owner text,                  -- Owner/tenant
    tags set<text>,              -- Tags for categorization
    ttl timestamp,               -- Scheduled auto-removal (NULL = permanent)
    metadata map<text, text>,    -- Additional metadata
    created_at timestamp,        -- Pin creation time
    updated_at timestamp,        -- Last update time
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {'class': 'LeveledCompactionStrategy'};

-- Placement tracking table - desired vs actual peer assignments
CREATE TABLE IF NOT EXISTS ipfs_pins.placements_by_cid (
    mh_prefix smallint,
    cid_bin blob,
    desired set<text>,           -- List of peer_id where pin should be placed
    actual set<text>,            -- List of peer_id where pin is confirmed
    updated_at timestamp,        -- Last placement update time
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {'class': 'LeveledCompactionStrategy'};

-- Reverse index - pins by peer for efficient worker queries
CREATE TABLE IF NOT EXISTS ipfs_pins.pins_by_peer (
    peer_id text,
    mh_prefix smallint,
    cid_bin blob,
    state tinyint,               -- 0=queued, 1=pinning, 2=pinned, 3=error, 4=unpinned
    last_seen timestamp,         -- Last status update from peer
    PRIMARY KEY ((peer_id), mh_prefix, cid_bin)
) WITH compaction = {'class': 'LeveledCompactionStrategy'};

-- TTL queue for scheduled pin removal
CREATE TABLE IF NOT EXISTS ipfs_pins.pin_ttl_queue (
    ttl_bucket timestamp,        -- Hourly bucket (UTC, truncated to hour)
    cid_bin blob,
    owner text,
    ttl timestamp,               -- Exact TTL time
    PRIMARY KEY ((ttl_bucket), ttl, cid_bin)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'DAYS', 
    'compaction_window_size': '1'
};

-- Operation deduplication for idempotency
CREATE TABLE IF NOT EXISTS ipfs_pins.op_dedup (
    op_id text,                  -- ULID/UUID from client
    ts timestamp,                -- Operation timestamp
    PRIMARY KEY (op_id)
) WITH default_time_to_live = 604800; -- 7 days
```

## Data Models

### Pin Serialization

Uses existing `api.Pin.ProtoMarshal()` serialization mechanism for compatibility:

```go
func (s *ScyllaState) serializePin(pin api.Pin) ([]byte, error) {
    return pin.ProtoMarshal()
}

func (s *ScyllaState) deserializePin(cid api.Cid, data []byte) (api.Pin, error) {
    var pin api.Pin
    err := pin.ProtoUnmarshal(data)
    if err != nil {
        return api.Pin{}, err
    }
    pin.Cid = cid
    return pin, nil
}
```

### Keys and Partitioning

```go
// Extract multihash prefix for partitioning
func mhPrefix(cidBin []byte) int16 {
    if len(cidBin) < 2 {
        return 0
    }
    return int16(cidBin[0])<<8 | int16(cidBin[1])
}

// Even distribution across 65536 partitions
func cidToPartitionedKey(cidBin []byte) (int16, []byte) {
    prefix := mhPrefix(cidBin)
    return prefix, cidBin
}
```

## Error Handling

### Retry Strategies

```go
type RetryPolicy struct {
    numRetries    int
    minDelay      time.Duration
    maxDelay      time.Duration
    backoffFactor float64
}

func (rp *RetryPolicy) Execute(ctx context.Context, operation func() error) error {
    var lastErr error
    
    for attempt := 0; attempt <= rp.numRetries; attempt++ {
        if attempt > 0 {
            delay := rp.calculateDelay(attempt)
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(delay):
            }
        }
        
        if err := operation(); err != nil {
            lastErr = err
            if !rp.isRetryable(err) {
                return err
            }
            continue
        }
        return nil
    }
    
    return fmt.Errorf("operation failed after %d attempts: %w", 
                     rp.numRetries, lastErr)
}
```

### Node Failure Handling

```go
func (s *ScyllaState) handleNodeFailure(err error) error {
    if gocql.IsTimeoutError(err) || gocql.IsUnavailableError(err) {
        // Log warning but don't return critical error
        logger.Warnf("ScyllaDB node temporarily unavailable: %v", err)
        return err
    }
    
    if gocql.IsWriteTimeoutError(err) {
        // For write timeout try with lower consistency level
        logger.Warnf("Write timeout, may need to adjust consistency level: %v", err)
        return err
    }
    
    return err
}
```

## Testing Strategy

### Unit Tests

```go
// Tests with mock ScyllaDB
func TestScyllaState_Add(t *testing.T) {
    mockSession := &MockSession{}
    state := &ScyllaState{session: mockSession}
    
    pin := api.Pin{
        Cid: testCid,
        Type: api.DataType,
        // ...
    }
    
    err := state.Add(context.Background(), pin)
    assert.NoError(t, err)
    
    // Verify correct CQL query was executed
    assert.Equal(t, expectedQuery, mockSession.LastQuery())
}
```

### Integration Tests

```go
// Tests with real ScyllaDB in Docker
func TestScyllaState_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }
    
    // Start ScyllaDB in Docker container
    container := startScyllaDBContainer(t)
    defer container.Stop()
    
    config := &Config{
        Hosts: []string{container.Host()},
        Port: container.Port(),
        // ...
    }
    
    state, err := New(context.Background(), config)
    require.NoError(t, err)
    
    // Test full pin lifecycle operations
    testFullPinLifecycle(t, state)
}
```

### Performance Tests

```go
func BenchmarkScyllaState_Add(b *testing.B) {
    state := setupBenchmarkState(b)
    pins := generateTestPins(b.N)
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        i := 0
        for pb.Next() {
            err := state.Add(context.Background(), pins[i%len(pins)])
            if err != nil {
                b.Fatal(err)
            }
            i++
        }
    })
}
```

## Monitoring and Metrics

### Prometheus Metrics

```go
type Metrics struct {
    // Operational metrics
    operationDuration *prometheus.HistogramVec
    operationCounter  *prometheus.CounterVec
    
    // Connection metrics
    activeConnections prometheus.Gauge
    connectionErrors  prometheus.Counter
    
    // State metrics
    totalPins         prometheus.Gauge
    pinOperationsRate prometheus.Counter
    
    // ScyllaDB metrics
    queryLatency      *prometheus.HistogramVec
    timeoutErrors     prometheus.Counter
    unavailableErrors prometheus.Counter
}

func (m *Metrics) RecordOperation(operation string, duration time.Duration, err error) {
    labels := prometheus.Labels{"operation": operation}
    
    if err != nil {
        labels["status"] = "error"
        m.operationCounter.With(labels).Inc()
    } else {
        labels["status"] = "success"
        m.operationCounter.With(labels).Inc()
        m.operationDuration.With(labels).Observe(duration.Seconds())
    }
}
```

### Logging

```go
// Structured logging with context
func (s *ScyllaState) logOperation(ctx context.Context, operation string, cid api.Cid, duration time.Duration, err error) {
    fields := map[string]interface{}{
        "operation": operation,
        "cid":       cid.String(),
        "duration":  duration,
    }
    
    if err != nil {
        fields["error"] = err.Error()
        logger.WithFields(fields).Error("ScyllaDB operation failed")
    } else {
        logger.WithFields(fields).Debug("ScyllaDB operation completed")
    }
}
```

## Migration and Compatibility

### Migration from Existing Backends

```go
type Migrator struct {
    source      state.State
    destination *ScyllaState
    batchSize   int
}

func (m *Migrator) Migrate(ctx context.Context) error {
    pinChan := make(chan api.Pin, m.batchSize)
    
    // Start goroutine to read from source
    go func() {
        defer close(pinChan)
        err := m.source.List(ctx, pinChan)
        if err != nil {
            logger.Errorf("Error listing pins from source: %v", err)
        }
    }()
    
    // Batch write to ScyllaDB
    batch := make([]api.Pin, 0, m.batchSize)
    
    for pin := range pinChan {
        batch = append(batch, pin)
        
        if len(batch) >= m.batchSize {
            if err := m.writeBatch(ctx, batch); err != nil {
                return err
            }
            batch = batch[:0]
        }
    }
    
    // Write remaining pins
    if len(batch) > 0 {
        return m.writeBatch(ctx, batch)
    }
    
    return nil
}
```

### Backward Compatibility

The implementation is fully compatible with the `state.State` interface, allowing it to be used as a drop-in replacement for existing implementations without changing IPFS-Cluster code.

## Deployment and Configuration

### Example Configuration

```json
{
  "state": {
    "datastore": "scylladb",
    "scylladb": {
      "hosts": ["scylla1.example.com", "scylla2.example.com", "scylla3.example.com"],
      "port": 9042,
      "keyspace": "ipfs_pins",
      "username": "cluster_user",
      "password": "secure_password",
      "tls_enabled": true,
      "tls_cert_file": "/etc/ssl/certs/client.crt",
      "tls_key_file": "/etc/ssl/private/client.key",
      "tls_ca_file": "/etc/ssl/certs/ca.crt",
      "num_conns": 10,
      "timeout": "30s",
      "connect_timeout": "10s",
      "consistency": "QUORUM",
      "serial_consistency": "SERIAL",
      "retry_policy": {
        "num_retries": 3,
        "min_retry_delay": "100ms",
        "max_retry_delay": "10s"
      },
      "batch_size": 1000,
      "batch_timeout": "1s",
      "metrics_enabled": true,
      "tracing_enabled": false
    }
  }
}
```

### Multi-Datacenter Deployment

```json
{
  "scylladb": {
    "hosts": [
      "dc1-scylla1.example.com",
      "dc1-scylla2.example.com", 
      "dc2-scylla1.example.com",
      "dc2-scylla2.example.com"
    ],
    "local_dc": "datacenter1",
    "consistency": "LOCAL_QUORUM",
    "dc_aware_routing": true,
    "token_aware_routing": true
  }
}
```

## Security

### TLS Configuration

```go
func (c *Config) createTLSConfig() (*tls.Config, error) {
    if !c.TLSEnabled {
        return nil, nil
    }
    
    tlsConfig := &tls.Config{
        InsecureSkipVerify: c.TLSInsecureSkipVerify,
    }
    
    if c.TLSCertFile != "" && c.TLSKeyFile != "" {
        cert, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile)
        if err != nil {
            return nil, fmt.Errorf("failed to load client certificate: %w", err)
        }
        tlsConfig.Certificates = []tls.Certificate{cert}
    }
    
    if c.TLSCAFile != "" {
        caCert, err := ioutil.ReadFile(c.TLSCAFile)
        if err != nil {
            return nil, fmt.Errorf("failed to read CA certificate: %w", err)
        }
        
        caCertPool := x509.NewCertPool()
        if !caCertPool.AppendCertsFromPEM(caCert) {
            return nil, fmt.Errorf("failed to parse CA certificate")
        }
        tlsConfig.RootCAs = caCertPool
    }
    
    return tlsConfig, nil
}
```

### Authentication

```go
func (c *Config) createAuthenticator() gocql.Authenticator {
    if c.Username != "" && c.Password != "" {
        return gocql.PasswordAuthenticator{
            Username: c.Username,
            Password: c.Password,
        }
    }
    return nil
}
```

## Performance Optimization

### Connection Pools

```go
func (c *Config) createClusterConfig() *gocql.ClusterConfig {
    cluster := gocql.NewCluster(c.Hosts...)
    cluster.Port = c.Port
    cluster.Keyspace = c.Keyspace
    cluster.NumConns = c.NumConns
    cluster.Timeout = c.Timeout
    cluster.ConnectTimeout = c.ConnectTimeout
    
    // Settings for high performance
    cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(
        gocql.DCAwareRoundRobinPolicy(c.LocalDC),
    )
    
    return cluster
}
```

### Prepared Statements

```go
type PreparedStatements struct {
    insertPin    *gocql.Query
    selectPin    *gocql.Query
    deletePin    *gocql.Query
    listPins     *gocql.Query
    checkExists  *gocql.Query
}

func (s *ScyllaState) prepareStatements() error {
    var err error
    
    s.prepared.insertPin, err = s.session.Prepare(`
        INSERT INTO pins_by_cid (mh_prefix, cid_bin, pin_type, rf, owner, tags, ttl, metadata, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare insert query: %w", err)
    }
    
    // Prepare other queries...
    
    return nil
}
```

## Архитектура

### Высокоуровневая Архитектура

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  IPFS-Cluster   │    │  IPFS-Cluster   │    │  IPFS-Cluster   │
│     Узел 1      │    │     Узел 2      │    │     Узел N      │
│                 │    │                 │    │                 │
│ ┌─────────────┐ │    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│ │ScyllaDB     │ │    │ │ScyllaDB     │ │    │ │ScyllaDB     │ │
│ │Хранилище    │ │    │ │Хранилище    │ │    │ │Хранилище    │ │
│ │Состояния    │ │    │ │Состояния    │ │    │ │Состояния    │ │
│ └─────────────┘ │    │ └─────────────┘ │    │ └─────────────┘ │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                    ┌─────────────────────────┐
                    │    Кластер ScyllaDB     │
                    │                         │
                    │  ┌─────┐ ┌─────┐ ┌─────┐│
                    │  │Узел1│ │Узел2│ │УзелN││
                    │  └─────┘ └─────┘ └─────┘│
                    └─────────────────────────┘
```

### Интеграционная Архитектура

ScyllaDB State Store реализует интерфейс `state.State` и интегрируется в IPFS-Cluster как новый тип хранилища состояния:

```
┌─────────────────────────────────────────────────────────────┐
│                    IPFS-Cluster                             │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │  Менеджер   │  │   Слой      │  │   Хранилище         │  │
│  │  Кластера   │  │ Консенсуса  │  │   Состояния         │  │
│  │             │  │             │  │                     │  │
│  │             │  │             │  │ ┌─────────────────┐ │  │
│  │             │  │             │  │ │  ScyllaDB       │ │  │
│  │             │  │             │  │ │  Хранилище      │ │  │
│  │             │  │             │  │ │  (Новое)        │ │  │
│  │             │  │             │  │ └─────────────────┘ │  │
│  │             │  │             │  │ ┌─────────────────┐ │  │
│  │             │  │             │  │ │  CRDT           │ │  │
│  │             │  │             │  │ │  (Существующее) │ │  │
│  │             │  │             │  │ └─────────────────┘ │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │ Драйвер ScyllaDB│
                    │    (gocql)      │
                    └─────────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │ Кластер ScyllaDB│
                    └─────────────────┘
```

## Компоненты и Интерфейсы

### 1. Хранилище Состояния ScyllaDB

**Расположение:** `state/scyllastate/`

**Основные файлы:**
- `scyllastate.go` - основная реализация
- `config.go` - конфигурация
- `migrations.go` - схема БД и миграции
- `scyllastate_test.go` - тесты

**Интерфейсы:**
```go
// Реализует state.State
type ScyllaState struct {
    session     *gocql.Session
    keyspace    string
    config      *Config
    codecHandle codec.Handle
    totalPins   int64
    metrics     *Metrics
}

// Реализует state.BatchingState  
type ScyllaBatchingState struct {
    *ScyllaState
    batch *gocql.Batch
}
```

### 2. Конфигурация

**Структура конфигурации:**
```go
type Config struct {
    config.Saver
    
    // Подключение к кластеру
    Hosts               []string
    Port                int
    Keyspace            string
    Username            string
    Password            string
    
    // Настройки TLS
    TLSEnabled          bool
    TLSCertFile         string
    TLSKeyFile          string
    TLSCAFile           string
    TLSInsecureSkipVerify bool
    
    // Настройки производительности
    NumConns            int
    Timeout             time.Duration
    ConnectTimeout      time.Duration
    
    // Настройки согласованности
    Consistency         gocql.Consistency
    SerialConsistency   gocql.SerialConsistency
    
    // Настройки повторов и задержек
    RetryPolicy         RetryPolicyConfig
    
    // Настройки пакетной обработки
    BatchSize           int
    BatchTimeout        time.Duration
    
    // Мониторинг
    MetricsEnabled      bool
    TracingEnabled      bool
}

type RetryPolicyConfig struct {
    NumRetries      int
    MinRetryDelay   time.Duration
    MaxRetryDelay   time.Duration
}
```

### 3. Схема Данных

**Keyspace и таблицы:**

```cql
-- Основная таблица для хранения метаданных пинов (источник истины)
CREATE TABLE IF NOT EXISTS ipfs_pins.pins_by_cid (
    mh_prefix smallint,          -- Первые 2 байта digest для шардирования
    cid_bin blob,                -- Бинарный multihash
    pin_type tinyint,            -- 0=direct, 1=recursive
    rf tinyint,                  -- Требуемый фактор репликации
    owner text,                  -- Владелец/арендатор
    tags set<text>,              -- Теги для категоризации
    ttl timestamp,               -- Плановое авто-удаление (NULL = постоянно)
    metadata map<text, text>,    -- Дополнительные метаданные
    created_at timestamp,        -- Время создания пина
    updated_at timestamp,        -- Время последнего обновления
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {'class': 'LeveledCompactionStrategy'};

-- Таблица отслеживания размещений - желаемые vs фактические назначения пиров
CREATE TABLE IF NOT EXISTS ipfs_pins.placements_by_cid (
    mh_prefix smallint,
    cid_bin blob,
    desired set<text>,           -- Список peer_id где пин должен быть размещен
    actual set<text>,            -- Список peer_id где пин подтвержден
    updated_at timestamp,        -- Время последнего обновления размещения
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {'class': 'LeveledCompactionStrategy'};

-- Обратный индекс - пины по пирам для эффективных запросов воркеров
CREATE TABLE IF NOT EXISTS ipfs_pins.pins_by_peer (
    peer_id text,
    mh_prefix smallint,
    cid_bin blob,
    state tinyint,               -- 0=в очереди, 1=пинится, 2=закреплен, 3=ошибка, 4=откреплен
    last_seen timestamp,         -- Последнее обновление статуса от пира
    PRIMARY KEY ((peer_id), mh_prefix, cid_bin)
) WITH compaction = {'class': 'LeveledCompactionStrategy'};

-- Очередь TTL для запланированного удаления пинов
CREATE TABLE IF NOT EXISTS ipfs_pins.pin_ttl_queue (
    ttl_bucket timestamp,        -- Часовой бакет (UTC, обрезанный до часа)
    cid_bin blob,
    owner text,
    ttl timestamp,               -- Точное время TTL
    PRIMARY KEY ((ttl_bucket), ttl, cid_bin)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'DAYS', 
    'compaction_window_size': '1'
};

-- Дедупликация операций для идемпотентности
CREATE TABLE IF NOT EXISTS ipfs_pins.op_dedup (
    op_id text,                  -- ULID/UUID от клиента
    ts timestamp,                -- Время операции
    PRIMARY KEY (op_id)
) WITH default_time_to_live = 604800; -- 7 дней
```

## Модели Данных

### Сериализация Пинов

Используется существующий механизм сериализации `api.Pin.ProtoMarshal()` для совместимости:

```go
func (s *ScyllaState) serializePin(pin api.Pin) ([]byte, error) {
    return pin.ProtoMarshal()
}

func (s *ScyllaState) deserializePin(cid api.Cid, data []byte) (api.Pin, error) {
    var pin api.Pin
    err := pin.ProtoUnmarshal(data)
    if err != nil {
        return api.Pin{}, err
    }
    pin.Cid = cid
    return pin, nil
}
```

### Ключи и Партиционирование

```go
// Извлечение префикса multihash для партиционирования
func mhPrefix(cidBin []byte) int16 {
    if len(cidBin) < 2 {
        return 0
    }
    return int16(cidBin[0])<<8 | int16(cidBin[1])
}

// Равномерное распределение по 65536 партициям
func cidToPartitionedKey(cidBin []byte) (int16, []byte) {
    prefix := mhPrefix(cidBin)
    return prefix, cidBin
}
```

## Обработка Ошибок

### Стратегии Повторов

```go
type RetryPolicy struct {
    numRetries    int
    minDelay      time.Duration
    maxDelay      time.Duration
    backoffFactor float64
}

func (rp *RetryPolicy) Execute(ctx context.Context, operation func() error) error {
    var lastErr error
    
    for attempt := 0; attempt <= rp.numRetries; attempt++ {
        if attempt > 0 {
            delay := rp.calculateDelay(attempt)
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(delay):
            }
        }
        
        if err := operation(); err != nil {
            lastErr = err
            if !rp.isRetryable(err) {
                return err
            }
            continue
        }
        return nil
    }
    
    return fmt.Errorf("операция не удалась после %d попыток: %w", 
                     rp.numRetries, lastErr)
}
```

### Обработка Сбоев Узлов

```go
func (s *ScyllaState) handleNodeFailure(err error) error {
    if gocql.IsTimeoutError(err) || gocql.IsUnavailableError(err) {
        // Логируем предупреждение, но не возвращаем критическую ошибку
        logger.Warnf("Узел ScyllaDB временно недоступен: %v", err)
        return err
    }
    
    if gocql.IsWriteTimeoutError(err) {
        // Для write timeout пытаемся с меньшим уровнем согласованности
        logger.Warnf("Таймаут записи, возможно нужно настроить уровень согласованности: %v", err)
        return err
    }
    
    return err
}
```

## Стратегия Тестирования

### Модульные Тесты

```go
// Тесты с mock ScyllaDB
func TestScyllaState_Add(t *testing.T) {
    mockSession := &MockSession{}
    state := &ScyllaState{session: mockSession}
    
    pin := api.Pin{
        Cid: testCid,
        Type: api.DataType,
        // ...
    }
    
    err := state.Add(context.Background(), pin)
    assert.NoError(t, err)
    
    // Проверяем, что правильный CQL запрос был выполнен
    assert.Equal(t, expectedQuery, mockSession.LastQuery())
}
```

### Интеграционные Тесты

```go
// Тесты с реальным ScyllaDB в Docker
func TestScyllaState_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Пропускаем интеграционный тест")
    }
    
    // Запуск ScyllaDB в Docker контейнере
    container := startScyllaDBContainer(t)
    defer container.Stop()
    
    config := &Config{
        Hosts: []string{container.Host()},
        Port: container.Port(),
        // ...
    }
    
    state, err := New(context.Background(), config)
    require.NoError(t, err)
    
    // Тестируем полный жизненный цикл операций
    testFullPinLifecycle(t, state)
}
```

### Тесты Производительности

```go
func BenchmarkScyllaState_Add(b *testing.B) {
    state := setupBenchmarkState(b)
    pins := generateTestPins(b.N)
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        i := 0
        for pb.Next() {
            err := state.Add(context.Background(), pins[i%len(pins)])
            if err != nil {
                b.Fatal(err)
            }
            i++
        }
    })
}
```

## Мониторинг и Метрики

### Метрики Prometheus

```go
type Metrics struct {
    // Операционные метрики
    operationDuration *prometheus.HistogramVec
    operationCounter  *prometheus.CounterVec
    
    // Метрики соединений
    activeConnections prometheus.Gauge
    connectionErrors  prometheus.Counter
    
    // Метрики состояния
    totalPins         prometheus.Gauge
    pinOperationsRate prometheus.Counter
    
    // Метрики ScyllaDB
    queryLatency      *prometheus.HistogramVec
    timeoutErrors     prometheus.Counter
    unavailableErrors prometheus.Counter
}

func (m *Metrics) RecordOperation(operation string, duration time.Duration, err error) {
    labels := prometheus.Labels{"operation": operation}
    
    if err != nil {
        labels["status"] = "error"
        m.operationCounter.With(labels).Inc()
    } else {
        labels["status"] = "success"
        m.operationCounter.With(labels).Inc()
        m.operationDuration.With(labels).Observe(duration.Seconds())
    }
}
```

### Журналирование

```go
// Структурированное журналирование с контекстом
func (s *ScyllaState) logOperation(ctx context.Context, operation string, cid api.Cid, duration time.Duration, err error) {
    fields := map[string]interface{}{
        "operation": operation,
        "cid":       cid.String(),
        "duration":  duration,
    }
    
    if err != nil {
        fields["error"] = err.Error()
        logger.WithFields(fields).Error("Операция ScyllaDB не удалась")
    } else {
        logger.WithFields(fields).Debug("Операция ScyllaDB завершена")
    }
}
```

## Миграция и Совместимость

### Миграция с Существующих Бэкендов

```go
type Migrator struct {
    source      state.State
    destination *ScyllaState
    batchSize   int
}

func (m *Migrator) Migrate(ctx context.Context) error {
    pinChan := make(chan api.Pin, m.batchSize)
    
    // Запускаем горутину для чтения из источника
    go func() {
        defer close(pinChan)
        err := m.source.List(ctx, pinChan)
        if err != nil {
            logger.Errorf("Ошибка получения списка пинов из источника: %v", err)
        }
    }()
    
    // Пакетная запись в ScyllaDB
    batch := make([]api.Pin, 0, m.batchSize)
    
    for pin := range pinChan {
        batch = append(batch, pin)
        
        if len(batch) >= m.batchSize {
            if err := m.writeBatch(ctx, batch); err != nil {
                return err
            }
            batch = batch[:0]
        }
    }
    
    // Записываем оставшиеся пины
    if len(batch) > 0 {
        return m.writeBatch(ctx, batch)
    }
    
    return nil
}
```

### Обратная Совместимость

Реализация полностью совместима с интерфейсом `state.State`, что позволяет использовать её как прямую замену существующих реализаций без изменения кода IPFS-Cluster.

## Развертывание и Конфигурация

### Пример Конфигурации

```json
{
  "state": {
    "datastore": "scylladb",
    "scylladb": {
      "hosts": ["scylla1.example.com", "scylla2.example.com", "scylla3.example.com"],
      "port": 9042,
      "keyspace": "ipfs_pins",
      "username": "cluster_user",
      "password": "secure_password",
      "tls_enabled": true,
      "tls_cert_file": "/etc/ssl/certs/client.crt",
      "tls_key_file": "/etc/ssl/private/client.key",
      "tls_ca_file": "/etc/ssl/certs/ca.crt",
      "num_conns": 10,
      "timeout": "30s",
      "connect_timeout": "10s",
      "consistency": "QUORUM",
      "serial_consistency": "SERIAL",
      "retry_policy": {
        "num_retries": 3,
        "min_retry_delay": "100ms",
        "max_retry_delay": "10s"
      },
      "batch_size": 1000,
      "batch_timeout": "1s",
      "metrics_enabled": true,
      "tracing_enabled": false
    }
  }
}
```

### Мульти-Датацентровое Развертывание

```json
{
  "scylladb": {
    "hosts": [
      "dc1-scylla1.example.com",
      "dc1-scylla2.example.com", 
      "dc2-scylla1.example.com",
      "dc2-scylla2.example.com"
    ],
    "local_dc": "datacenter1",
    "consistency": "LOCAL_QUORUM",
    "dc_aware_routing": true,
    "token_aware_routing": true
  }
}
```

## Безопасность

### Конфигурация TLS

```go
func (c *Config) createTLSConfig() (*tls.Config, error) {
    if !c.TLSEnabled {
        return nil, nil
    }
    
    tlsConfig := &tls.Config{
        InsecureSkipVerify: c.TLSInsecureSkipVerify,
    }
    
    if c.TLSCertFile != "" && c.TLSKeyFile != "" {
        cert, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile)
        if err != nil {
            return nil, fmt.Errorf("не удалось загрузить клиентский сертификат: %w", err)
        }
        tlsConfig.Certificates = []tls.Certificate{cert}
    }
    
    if c.TLSCAFile != "" {
        caCert, err := ioutil.ReadFile(c.TLSCAFile)
        if err != nil {
            return nil, fmt.Errorf("не удалось прочитать сертификат CA: %w", err)
        }
        
        caCertPool := x509.NewCertPool()
        if !caCertPool.AppendCertsFromPEM(caCert) {
            return nil, fmt.Errorf("не удалось разобрать сертификат CA")
        }
        tlsConfig.RootCAs = caCertPool
    }
    
    return tlsConfig, nil
}
```

### Аутентификация

```go
func (c *Config) createAuthenticator() gocql.Authenticator {
    if c.Username != "" && c.Password != "" {
        return gocql.PasswordAuthenticator{
            Username: c.Username,
            Password: c.Password,
        }
    }
    return nil
}
```

## Оптимизация Производительности

### Пулы Соединений

```go
func (c *Config) createClusterConfig() *gocql.ClusterConfig {
    cluster := gocql.NewCluster(c.Hosts...)
    cluster.Port = c.Port
    cluster.Keyspace = c.Keyspace
    cluster.NumConns = c.NumConns
    cluster.Timeout = c.Timeout
    cluster.ConnectTimeout = c.ConnectTimeout
    
    // Настройки для высокой производительности
    cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(
        gocql.DCAwareRoundRobinPolicy(c.LocalDC),
    )
    
    return cluster
}
```

### Подготовленные Запросы

```go
type PreparedStatements struct {
    insertPin    *gocql.Query
    selectPin    *gocql.Query
    deletePin    *gocql.Query
    listPins     *gocql.Query
    checkExists  *gocql.Query
}

func (s *ScyllaState) prepareStatements() error {
    var err error
    
    s.prepared.insertPin, err = s.session.Prepare(`
        INSERT INTO pins_by_cid (mh_prefix, cid_bin, pin_type, rf, owner, tags, ttl, metadata, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)
    if err != nil {
        return fmt.Errorf("не удалось подготовить запрос вставки: %w", err)
    }
    
    // Подготавливаем остальные запросы...
    
    return nil
}
```

## C4 Компонентная Диаграмма

### MVP Архитектура - Корпоративная Серо-Синяя Палитра

```plantuml
@startuml C4-Component-IPFS-Scylla-MVP-Enterprise
!include https://raw.githubusercontent.com/plantuml-stdlib/C4-PlantUML/master/C4_Component.puml

LAYOUT_TOP_DOWN()

title IPFS Pin Orchestration — MVP (C4 Component)

' Enterprise Gray-Blue цветовая палитра
AddElementTag("service",  $bgColor="#5C7CFA", $fontColor="#FFFFFF", $borderColor="#3B5BDB", $legendText="Service")
AddElementTag("database", $bgColor="#6941C6", $fontColor="#FFFFFF", $borderColor="#5335A3", $legendText="Database")
AddElementTag("bus",      $bgColor="#344054", $fontColor="#FFFFFF", $borderColor="#1D2939", $legendText="Message Bus")
AddElementTag("worker",   $bgColor="#475467", $fontColor="#FFFFFF", $borderColor="#344054", $legendText="Worker")
AddElementTag("external", $bgColor="#667085", $fontColor="#FFFFFF", $borderColor="#475467", $legendText="External")

' Основные компоненты (минимальная версия)
ContainerDb(scylla, "ScyllaDB", "CQL", "Keyspace ipfs_pins: pins_by_cid, placements_by_cid, pins_by_peer, pin_ttl_queue", $tags="database")

Container(bus, "NATS (JetStream)", "NATS", "Topics: pin.request, pin.assign, pin.status", $tags="bus")

System_Ext(kubo, "IPFS Kubo Node", "Хранит блоки; локальный pinset; GC", $tags="external")

Container_Boundary(cluster, "Cluster Service (Go)", $tags="service") {
    Component(api,  "API (REST/gRPC)", "Go", "POST /pins, GET /pins/{cid}")
    Component(plan, "Planner / Reconciler", "Go", "Рассчитывает desired; лечит дрейф")
    Component(ttl,  "TTL Scheduler", "Go", "Сканирует TTL‑бакеты; плановый unpin")
}

Container_Boundary(worker, "Pin Worker Agent (Go)", $tags="worker") {
    Component(agent, "NATS Consumer", "Go", "sub pin.assign; pub pin.status")
    Component(kconn, "Kubo Connector", "HTTP API", "ipfs pin add/rm; защита от GC")
}

' Связи
Rel(api, scylla, "Upsert/Read pin metadata (CQL)")
Rel(plan, scylla, "Read desired/actual; update placements")
Rel(ttl,  scylla, "Read TTL buckets; schedule unpins")
Rel(api,  bus,    "Publish pin.request")
Rel(plan, bus,    "Publish pin.assign")
Rel(agent,bus,    "Subscribe pin.assign / Publish pin.status")
Rel(kconn, kubo,  "HTTP → /api/v0/pin/*")

@enduml
```

### Enterprise Архитектура с Биллингом и Безопасностью

```plantuml
@startuml C4-Component-IPFS-Scylla-Enterprise-Full
!include https://raw.githubusercontent.com/plantuml-stdlib/C4-PlantUML/master/C4_Component.puml

LAYOUT_TOP_DOWN()

title IPFS Pin Orchestration — Enterprise MVP\n+ Биллинг + Безопасность

' Enterprise Gray-Blue палитра с акцентными цветами
AddElementTag("service",   $bgColor="#5C7CFA", $fontColor="#FFFFFF", $borderColor="#3B5BDB", $legendText="Service")
AddElementTag("database",  $bgColor="#6941C6", $fontColor="#FFFFFF", $borderColor="#5335A3", $legendText="Database")
AddElementTag("bus",       $bgColor="#344054", $fontColor="#FFFFFF", $borderColor="#1D2939", $legendText="Message Bus")
AddElementTag("worker",    $bgColor="#475467", $fontColor="#FFFFFF", $borderColor="#344054", $legendText="Worker")
AddElementTag("billing",   $bgColor="#F79009", $fontColor="#FFFFFF", $borderColor="#B54708", $legendText="Billing")
AddElementTag("security",  $bgColor="#D92D20", $fontColor="#FFFFFF", $borderColor="#A32018", $legendText="Security")
AddElementTag("external",  $bgColor="#667085", $fontColor="#FFFFFF", $borderColor="#475467", $legendText="External")

' Основные таблицы данных
ContainerDb(scylla, "ScyllaDB Cluster", "CQL / Keyspace: ipfs_pins", $tags="database") {
    ComponentDb(pins,      "pin_metadata", "cid PK, owner, tags, ttl, status, repl_factor")
    ComponentDb(locations, "pin_locations", "cid, peer_id, is_healthy, last_seen")
    ComponentDb(owners,    "owner_pins", "owner_id, cid, created_at")
    ComponentDb(ttlq,      "pin_ttl_queue", "ttl_bucket, cid")
}

' Биллинговые таблицы
Container_Boundary(billing_tables, "Billing Tables (ScyllaDB)", $tags="billing") {
    ComponentDb(usage,     "storage_usage", "owner_id, day, total_bytes")
    ComponentDb(billing_e, "billing_events", "event_id, owner_id, amount, currency")
}

' Шина сообщений
Container(bus, "NATS Server (JetStream)", "Topics + Durable Consumers", $tags="bus") {
    ComponentQueue(req,   "pin.request", "CID, owner, repl, ttl, tags")
    ComponentQueue(assign,"pin.assign", "CID → target_peer")
    ComponentQueue(status, "pin.status", "CID, peer, success/size/error")
    ComponentQueue(bill_ev,"billing.event", "owner_id, GB, cost, period")
}

' Внешние системы
System_Ext(kubo, "IPFS Kubo Node", "Хранит блоки; GC; локальный pinset", $tags="external")
System_Ext(billing_sys, "External Billing System", "Stripe / Filecoin PayCh", $tags="external")
System_Ext(idp, "Identity Provider", "Keycloak / DID Resolver", $tags="external")

' Сервис безопасности
Container_Boundary(auth_svc, "Auth Service (Go)", "AuthN & AuthZ; RBAC/ABAC", $tags="security") {
    Component(authn, "Authenticator", "JWT/API Key/DID", "Validates credentials → owner_id")
    Component(authz, "Authorizer", "Policy Engine", "Checks: can owner_id pin this CID?")
    Component(audit, "Audit Logger", "Go", "Logs all access attempts")
}

' Основной сервис кластера
Container_Boundary(cluster_svc, "Cluster Service (Go)", "API + Planner + Scheduler", $tags="service") {
    Component(api,     "API Gateway", "REST + gRPC", "Delegates AuthN/Z; routes to handlers")
    Component(plan,    "Reconciler Engine", "Control Loop", "Desired vs Actual → publish pin.assign")
    Component(ttl_eng, "TTL Scheduler", "Cron-like", "Scan TTL → publish unpin requests")
}

' Воркер-агент
Container_Boundary(worker_agent, "Pin Worker Agent (Go)", "Stateless; horizontal scale", $tags="worker") {
    Component(nats_sub, "NATS Consumer", "JetStream", "Sub pin.assign → execute")
    Component(kubo_cli, "Kubo HTTP Client", "Retry + Timeout", "ipfs pin add/rm/stat")
    Component(reporter, "Status Reporter", "NATS Publisher", "Pub pin.status after action")
}

' Биллинговый сервис
Container_Boundary(billing_svc, "Billing Service (Go)", "Usage & Cost Calculation", $tags="billing") {
    Component(usage_collector, "Usage Collector", "Cron", "Aggregates → storage_usage")
    Component(cost_calculator, "Cost Calculator", "Go", "Emit billing.event")
    Component(invoice_emitter, "Invoice Emitter", "Go", "Call External Billing System")
}

' Потоки безопасности
Rel(api, authn, "gRPC: ValidateToken(token) → owner_id, roles", "sync")
Rel(authn, idp, "OIDC/DID resolve (if needed)", "sync")
Rel(api, authz, "gRPC: CheckPermission(owner_id, action, resource)", "sync")
Rel(authz, audit, "Log: access_granted/denied", "async")

' Основные потоки
Rel(api, pins, "CQL: INSERT/UPDATE (after AuthZ)", "sync")
Rel(plan, scylla, "CQL: read desired/actual; update", "sync")
Rel(ttl_eng, ttlq, "CQL: SELECT expired", "sync")
Rel(kubo_cli, kubo, "HTTP: /api/v0/pin/add|rm|stat", "sync")
Rel(api, req, "PUB pin.request {json}", "async")
Rel(plan, assign, "PUB pin.assign {cid, peer}", "async")
Rel(nats_sub, assign, "SUB pin.assign", "async")
Rel(reporter, status, "PUB pin.status {json}", "async")

' Биллинговые потоки
Rel(reporter, usage, "CQL: UPSERT storage_usage (per owner)", "async")
Rel(usage_collector, usage, "CQL: SELECT daily rollup", "sync")
Rel(cost_calculator, bill_ev, "PUB billing.event", "async")
Rel(invoice_emitter, bill_ev, "SUB billing.event", "async")
Rel(invoice_emitter, billing_sys, "HTTP/gRPC: createInvoice()", "sync")
Rel(invoice_emitter, billing_e, "CQL: INSERT billing_events", "sync")

@enduml
```

### Цветовая Палитра Enterprise Gray-Blue

| Роль         | Цвет фона     | Цвет границы  | Назначение |
|--------------|---------------|---------------|------------|
| Service      | `#5C7CFA`     | `#3B5BDB`     | Основной оркестратор — спокойный синий, надёжность |
| Database     | `#6941C6`     | `#5335A3`     | ScyllaDB — фиолетово-серый, важность данных |
| Message Bus  | `#344054`     | `#1D2939`     | NATS — тёмно-серый, инфраструктурный цвет |
| Worker       | `#475467`     | `#344054`     | Воркеры — нейтральный серый, фоновые исполнители |
| External     | `#667085`     | `#475467`     | Kubo — внешняя система, партнёрская интеграция |
| Billing      | `#F79009`     | `#B54708`     | Биллинг — оранжевый акцент, коммерческая функция |
| Security     | `#D92D20`     | `#A32018`     | Безопасность — красный акцент, защита и контроль |

> ✅ Все цвета взяты из enterprise-палитр (IBM Carbon, Microsoft Fluent, Atlassian) — спокойные, профессиональные, подходят для печати и презентаций.

## SLO и Мониторинг

### Целевые SLO

**SLO-1: Сквозная задержка пинов**
- p95 pin_end_to_end_seconds ≤ 60s на окне 30м
- Измеряется от API запроса до подтверждения пина

**SLO-2: Задержка назначения**
- p95 pin_assign_to_pinned_seconds ≤ 30s на окне 30м
- Измеряется от публикации pin.assign до pin.status=pinned

**SLO-3: Дрейф состояния**
- Глобальный дрейф slo:global_drift_ratio ≤ 0.5% на окне 30м
- Доля пинов где desired != actual

**SLO-4: Горячие партиции**
- Ни один mh_prefix не превышает hot_bucket_ratio > 3 более 5м
- Предотвращение неравномерной нагрузки

**Error Budget**
- 2% времени в месяц допускается превышение SLO-1/2
- Для SLO-3 — не более 1% времени

### Prometheus Метрики

```yaml
# Гистограммы времени
pin_end_to_end_seconds_bucket{source="api|reconciler", op="pin|unpin"}
pin_assign_to_pinned_seconds_bucket{op="pin"}

# Счётчики событий
pin_events_total{type="upsert|assign|status|ttl|error", mh_prefix="0000"}

# Агрегаты состояния
cluster_pin_desired_total{tenant="t1", mh_prefix="0000"}
cluster_pin_actual_total{tenant="t1", mh_prefix="0000"}

# Технические метрики воркеров
worker_assign_queue_depth{worker="w1"}
worker_failures_total{kind="ipfs|nats|scylla"}
```

### Recording Rules

```yaml
# p95 сквозной latency pin (за 5 минут)
- record: slo:p95_pin_e2e_seconds
  expr: |
    histogram_quantile(0.95, 
      sum by (le) (rate(pin_end_to_end_seconds_bucket{op="pin"}[5m])))

# Глобальный дрейф desired vs actual
- record: slo:global_drift_ratio
  expr: |
    (sum(cluster_pin_desired_total) - sum(cluster_pin_actual_total))
    / clamp_min(sum(cluster_pin_desired_total), 1)

# Топ горячих бакетов по интенсивности событий
- record: slo:hot_buckets_events_rate
  expr: |
    topk(10, sum by (mh_prefix) (rate(pin_events_total[5m])))
```

### Алерты

```yaml
- alert: PinLatencyHighP95
  expr: slo:p95_pin_e2e_seconds > 60
  for: 10m
  labels: { severity: warning }
  annotations:
    summary: "p95 end-to-end pin latency > 60s"
    description: "Сквозная задержка pin превышает SLO"

- alert: GlobalDriftTooHigh
  expr: slo:global_drift_ratio > 0.005
  for: 15m
  labels: { severity: critical }
  annotations:
    summary: "Доля дрейфа desired/actual > 0.5%"
    description: "Кластер не успевает конвергировать"

- alert: HotPartition
  expr: slo:hot_bucket_ratio > 3
  for: 5m
  labels: { severity: warning }
  annotations:
    summary: "Горячий mh_prefix: нагрузка >3× средней"
```

### Runbook для Устранения Неполадок

**p95 задержка выросла:**
1. Проверить `worker_failures_total` и `worker_assign_queue_depth`
2. Проверить задержки ScyllaDB (диск/compaction)
3. Проверить NATS (stream lag)

**Дрейф увеличился:**
1. Включить форс-ребаланс
2. Увеличить конкуренцию воркеров
3. Проверить capacity целевых пиров

**Горячие бакеты:**
1. Временно снизить assign в этих mh_prefix
2. Включить дополнительный воркер-шард
3. Проверить распределение данных

### Grafana Панели

1. **p95 Pin Latency** (timeseries): `slo:p95_pin_e2e_seconds`
2. **Global Drift %** (stat + sparkline): `100 * slo:global_drift_ratio`
3. **Drift по бакетам** (heatmap): `slo:drift_ratio_by_bucket`
4. **Топ горячих бакетов** (table): `slo:hot_buckets_events_rate`
5. **Очередь назначений** (gauge): `max by (worker) (worker_assign_queue_depth)`
6. **Assign→Pinned p95** (timeseries): `slo:p95_assign_to_pinned_seconds`
##
# Детализированная Enterprise Диаграмма

```plantuml
@startuml C4-Component-IPFS-Scylla-MVP-Enterprise-GrayBlue
!include https://raw.githubusercontent.com/plantuml-stdlib/C4-PlantUML/master/C4_Component.puml

' Светлый фон -- стандарт для enterprise docs
skinparam defaultFontColor #344054
skinparam sequenceMessageFontColor #475467
skinparam titleFontSize 18
skinparam titleFontColor #1D2939

LAYOUT_TOP_DOWN()

title IPFS Pin Orchestration -- Enterprise MVP (C4 Component)\nScyllaDB + NATS + Kubo -- Gray-Blue Corporate Theme

' Custom Tags -- корпоративная серо-синяя палитра
AddElementTag("service",   $bgColor="#5C7CFA", $fontColor="#FFFFFF", $borderColor="#3B5BDB", $legendText="Service")
AddElementTag("database",  $bgColor="#6941C6", $fontColor="#FFFFFF", $borderColor="#5335A3", $legendText="Database")
AddElementTag("bus",       $bgColor="#344054", $fontColor="#FFFFFF", $borderColor="#1D2939", $legendText="Message Bus")
AddElementTag("worker",    $bgColor="#475467", $fontColor="#FFFFFF", $borderColor="#344054", $legendText="Worker")
AddElementTag("external",  $bgColor="#667085", $fontColor="#FFFFFF", $borderColor="#475467", $legendText="External")

' ===== DATABASE =====
ContainerDb(scylla, "ScyllaDB Cluster", "CQL / Keyspace: ipfs_pins", $tags="database") {
    ComponentDb(pins,      "pin_metadata", "cid PK, owner, tags, ttl, status, repl_factor")
    ComponentDb(locations, "pin_locations", "cid, peer_id, is_healthy, last_seen")
    ComponentDb(owners,    "owner_pins", "owner_id, cid, created_at -- for listing")
    ComponentDb(ttlq,      "pin_ttl_queue", "ttl_bucket, cid -- for scheduler")
}

' ===== MESSAGE BUS =====
Container(bus, "NATS Server (JetStream)", "Topics + Durable Consumers", $tags="bus") {
    ComponentQueue(req,   "pin.request", "CID, owner, repl, ttl, tags")
    ComponentQueue(assign,"pin.assign", "CID → target_peer")
    ComponentQueue(status, "pin.status", "CID, peer, success/size/error")
}

' ===== EXTERNAL =====
System_Ext(kubo, "IPFS Kubo Node (go-ipfs)", "Stores blocks; GC; local pinset; HTTP API /api/v0/pin/*", $tags="external")

' ===== CLUSTER SERVICE =====
Container_Boundary(cluster_svc, "Cluster Service (Go)", "Stateless API + Planner + Scheduler", $tags="service") {
    Component(api,     "API Gateway", "Go / REST+gRPC", "POST /pins → write Scylla + pub NATS\nGET /pins/{cid} → read Scylla")
    Component(plan,    "Reconciler Engine", "Go / Control Loop", "Compares desired (Scylla) vs actual (pin_locations)\nGenerates replication/deletion tasks → pub pin.assign")
    Component(ttl_eng, "TTL Scheduler", "Go / Cron-like", "Every 5 min: SELECT expired from pin_ttl_queue\nPublishes pin.request with action=unpin")
}

' ===== WORKER AGENT =====
Container_Boundary(worker_agent, "Pin Worker Agent (Go)", "Stateless, scales horizontally", $tags="worker") {
    Component(nats_sub, "NATS Consumer", "Go / JetStream", "Subscribes to pin.assign\nDeserializes task → calls Kubo")
    Component(kubo_cli, "Kubo HTTP Client", "Go / Retry+Timeout", "Executes:\n→ POST /api/v0/pin/add?arg=<CID>\n→ DELETE /api/v0/pin/rm?arg=<CID>\n→ GET /api/v0/block/stat?arg=<CID>")
    Component(reporter, "Status Reporter", "Go", "After execution → pub pin.status {cid, peer, success, size, error}")
}

' ===== RELATIONS =====
Rel(api, pins, "CQL: INSERT/UPDATE pin_metadata\n+ owner_pins, tags_index", "Sync")
Rel(api, ttlq, "CQL: INSERT INTO pin_ttl_queue (if TTL)", "Sync")

Rel(plan, pins, "CQL: SELECT desired state", "Sync")
Rel(plan, locations, "CQL: SELECT actual → INSERT/UPDATE", "Sync")

Rel(ttl_eng, ttlq, "CQL: SELECT expired pins", "Sync")
Rel(ttl_eng, pins, "CQL: UPDATE status = 'expiring'", "Sync")

Rel(api, req, "PUB: pin.request {json}", "Async")
Rel(plan, assign, "PUB: pin.assign {cid, peer}", "Async")

Rel(nats_sub, assign, "SUB: pin.assign", "Async")
Rel(reporter, status, "PUB: pin.status {json}", "Async")

Rel(kubo_cli, kubo, "HTTP: /api/v0/pin/add/rm/stat", "Sync")
Rel(plan, status, "SUB: pin.status (JetStream durable)", "Async")

LAYOUT_WITH_LEGEND()
@enduml
```

## Преимущества Enterprise Gray-Blue Палитры

### ✅ Почему эта версия идеальна для enterprise:

- **Не отвлекает** — нет ярких красных/зелёных, только спокойные, профессиональные тона
- **Универсальна** — подходит для PDF, Confluence, Notion, PowerPoint, печати
- **Семантически точна** — цвета соответствуют роли: синий = ядро, фиолетовый = данные, серый = инфраструктура
- **Легко расширять** — можно добавить Security, Billing, Monitoring слои в той же палитре
- **Выглядит "дорого"** — как диаграмма из архитектурного документа AWS, Azure или Google Cloud

### 🖼️ Примеры использования:

- Вставить в **архитектурное решение для CTO**
- Приложить к **грантовой заявке на Filecoin Foundation**
- Использовать в **техническом докладе на конференции**
- Включить в **документацию SRE-команды**
- Презентации для инвесторов и партнёров

### 🎨 Семантика цветов:

| Компонент | Цвет | Значение |
|-----------|------|----------|
| **Service** | Синий `#5C7CFA` | Надёжность, основная логика |
| **Database** | Фиолетовый `#6941C6` | Важность данных, постоянство |
| **Message Bus** | Тёмно-серый `#344054` | Инфраструктура, связующее звено |
| **Worker** | Серый `#475467` | Исполнители, фоновые процессы |
| **External** | Светло-серый `#667085` | Внешние системы, интеграции |

Эта палитра взята из enterprise-стандартов (IBM Carbon, Microsoft Fluent, Atlassian Design System) и обеспечивает профессиональный, не утомляющий глаза вид в любом формате.### Ente
rprise Архитектура с Биллингом

```plantuml
@startuml C4-Component-IPFS-Scylla-MVP-Enterprise-With-Billing
!include https://raw.githubusercontent.com/plantuml-stdlib/C4-PlantUML/master/C4_Component.puml

' Светлый фон -- стандарт для enterprise docs
skinparam defaultFontColor #344054
skinparam sequenceMessageFontColor #475467
skinparam titleFontSize 18
skinparam titleFontColor #1D2939

LAYOUT_TOP_DOWN()

title IPFS Pin Orchestration -- Enterprise MVP + Billing\nScyllaDB + NATS + Kubo -- Gray-Blue Corporate Theme

' Custom Tags -- корпоративная серо-синяя палитра
AddElementTag("service",   $bgColor="#5C7CFA", $fontColor="#FFFFFF", $borderColor="#3B5BDB", $legendText="Service")
AddElementTag("database",  $bgColor="#6941C6", $fontColor="#FFFFFF", $borderColor="#5335A3", $legendText="Database")
AddElementTag("bus",       $bgColor="#344054", $fontColor="#FFFFFF", $borderColor="#1D2939", $legendText="Message Bus")
AddElementTag("worker",    $bgColor="#475467", $fontColor="#FFFFFF", $borderColor="#344054", $legendText="Worker")
AddElementTag("external",  $bgColor="#667085", $fontColor="#FFFFFF", $borderColor="#475467", $legendText="External")
AddElementTag("billing",   $bgColor="#F79009", $fontColor="#FFFFFF", $borderColor="#B54708", $legendText="Billing")

' ===== DATABASE -- ОСНОВНЫЕ ТАБЛИЦЫ =====
ContainerDb(scylla, "ScyllaDB Cluster", "CQL / Keyspace: ipfs_pins", $tags="database") {
    ComponentDb(pins,      "pin_metadata", "cid PK, owner, tags, ttl, status, repl_factor")
    ComponentDb(locations, "pin_locations", "cid, peer_id, is_healthy, last_seen")
    ComponentDb(owners,    "owner_pins", "owner_id, cid, created_at -- for listing")
    ComponentDb(ttlq,      "pin_ttl_queue", "ttl_bucket, cid -- for scheduler")
}

' ===== BILLING TABLES -- ВЫНЕСЛИ В ОТДЕЛЬНЫЙ КОНТЕЙНЕР =====
Container_Boundary(billing_tables, "Billing Tables (ScyllaDB)", $tags="billing") {
    ComponentDb(usage,     "storage_usage", "owner_id, day, total_bytes -- daily rollup")
    ComponentDb(billing_e, "billing_events", "event_id, owner_id, amount, currency, status, created_at")
}

' ===== MESSAGE BUS =====
Container(bus, "NATS Server (JetStream)", "Topics + Durable Consumers", $tags="bus") {
    ComponentQueue(req,   "pin.request", "CID, owner, repl, ttl, tags")
    ComponentQueue(assign,"pin.assign", "CID → target_peer")
    ComponentQueue(status, "pin.status", "CID, peer, success/size/error")
    ComponentQueue(bill_ev,"billing.event", "owner_id, GB, cost, period, invoice_id")
}

' ===== EXTERNAL =====
System_Ext(kubo, "IPFS Kubo Node (go-ipfs)", "Stores blocks; GC; local pinset; HTTP API /api/v0/pin/*", $tags="external")
System_Ext(billing_sys, "External Billing System", "Stripe / Filecoin PayCh / L2 Wallet", $tags="external")

' ===== CLUSTER SERVICE =====
Container_Boundary(cluster_svc, "Cluster Service (Go)", "Stateless API + Planner + Scheduler", $tags="service") {
    Component(api,     "API Gateway", "Go / REST+gRPC", "POST /pins → write Scylla + pub NATS\nGET /pins/{cid} → read Scylla")
    Component(plan,    "Reconciler Engine", "Go / Control Loop", "Compares desired (Scylla) vs actual (pin_locations)\nGenerates replication/deletion tasks → pub pin.assign")
    Component(ttl_eng, "TTL Scheduler", "Go / Cron-like", "Every 5 min: SELECT expired from pin_ttl_queue\nPublishes pin.request with action=unpin")
}

' ===== WORKER AGENT =====
Container_Boundary(worker_agent, "Pin Worker Agent (Go)", "Stateless, scales horizontally", $tags="worker") {
    Component(nats_sub, "NATS Consumer", "Go / JetStream", "Subscribes to pin.assign\nDeserializes task → calls Kubo")
    Component(kubo_cli, "Kubo HTTP Client", "Go / Retry+Timeout", "Executes:\n→ POST /api/v0/pin/add?arg=<CID>\n→ DELETE /api/v0/pin/rm?arg=<CID>\n→ GET /api/v0/block/stat?arg=<CID>")
    Component(reporter, "Status Reporter", "Go", "After execution → pub pin.status {cid, peer, success, size, error}")
}

' ===== BILLING SERVICE =====
Container_Boundary(billing_svc, "Billing Service (Go)", "Calculates usage & cost; emits billing events", $tags="billing") {
    Component(usage_collector, "Usage Collector", "Go / Cron", "Daily: aggregates pin_metadata → storage_usage")
    Component(cost_calculator, "Cost Calculator", "Go", "Reads storage_usage + pricing policy → emits billing.event")
    Component(invoice_emitter, "Invoice Emitter", "Go", "Listens to billing.event → calls External Billing System")
}

' ===== CORE RELATIONS =====
Rel(api, pins, "CQL: INSERT/UPDATE", "Sync")
Rel(plan, scylla, "CQL: read desired/actual; update", "Sync")
Rel(ttl_eng, ttlq, "CQL: SELECT expired", "Sync")
Rel(reporter, status, "PUB: pin.status", "Async")
Rel(kubo_cli, kubo, "HTTP: pin/add/rm", "Sync")
Rel(api, req, "PUB: pin.request", "Async")
Rel(plan, assign, "PUB: pin.assign", "Async")
Rel(nats_sub, assign, "SUB: pin.assign", "Async")

' ===== BILLING RELATIONS =====
Rel(reporter, usage, "CQL: UPSERT storage_usage", "Async")
Rel(usage_collector, usage, "CQL: SELECT daily rollup", "Sync")
Rel(cost_calculator, bill_ev, "PUB: billing.event", "Async")
Rel(invoice_emitter, bill_ev, "SUB: billing.event", "Async")
Rel(invoice_emitter, billing_sys, "HTTP/gRPC: createInvoice()", "Sync")
Rel(invoice_emitter, billing_e, "CQL: INSERT billing_events", "Sync")

LAYOUT_WITH_LEGEND()
@enduml
```

## Как работает биллинг-слой

### 📊 Storage Usage Tracking
Когда Worker успешно пинит CID, он обновляет `storage_usage` (owner_id + размер в байтах). Можно также делать ежедневный агрегат через `usage_collector`.

### 💰 Cost Calculation
`cost_calculator` читает `storage_usage` + политику ценообразования → генерирует событие `billing.event`.

### 🧾 Invoice Emission
`invoice_emitter` слушает `billing.event` → вызывает внешнюю систему (Stripe, смарт-контракт) → сохраняет событие в `billing_events`.

### 💡 Возможные расширения биллинг-системы:

- **Pricing Policy Engine** — таблица тарифов в ScyllaDB
- **Webhook для уведомлений** — когда счёт выставлен
- **Debt Enforcement** — если платёж не прошёл, инициировать unpin через тот же NATS
- **Dashboard** — Grafana + ScyllaDB для отображения расходов клиентов
- **Multi-currency support** — поддержка разных валют и токенов
- **Usage analytics** — детальная аналитика использования по клиентам
- **Automated scaling pricing** — динамическое ценообразование на основе нагрузки
### En
terprise Архитектура с Биллингом и Безопасностью

```plantuml
@startuml C4-Component-IPFS-Scylla-Enterprise-Full-Security
!include https://raw.githubusercontent.com/plantuml-stdlib/C4-PlantUML/master/C4_Component.puml

' Светлый фон -- стандарт для enterprise docs
skinparam defaultFontColor #344054
skinparam sequenceMessageFontColor #475467
skinparam titleFontSize 18
skinparam titleFontColor #1D2939

LAYOUT_TOP_DOWN()

title IPFS Pin Orchestration -- Enterprise MVP\\n+ Биллинг + Безопасность

' Custom Tags -- корпоративная палитра
AddElementTag("service",   $bgColor="#5C7CFA", $fontColor="#FFFFFF", $borderColor="#3B5BDB", $legendText="Service")
AddElementTag("database",  $bgColor="#6941C6", $fontColor="#FFFFFF", $borderColor="#5335A3", $legendText="Database")
AddElementTag("bus",       $bgColor="#344054", $fontColor="#FFFFFF", $borderColor="#1D2939", $legendText="Message Bus")
AddElementTag("worker",    $bgColor="#475467", $fontColor="#FFFFFF", $borderColor="#344054", $legendText="Worker")
AddElementTag("external",  $bgColor="#667085", $fontColor="#FFFFFF", $borderColor="#475467", $legendText="External")
AddElementTag("billing",   $bgColor="#F79009", $fontColor="#FFFFFF", $borderColor="#B54708", $legendText="Billing")
AddElementTag("security",  $bgColor="#D92D20", $fontColor="#FFFFFF", $borderColor="#A32018", $legendText="Security")

' ===== DATABASE -- ОСНОВНЫЕ ТАБЛИЦЫ =====
ContainerDb(scylla, "ScyllaDB Cluster", "CQL / Keyspace: ipfs_pins", $tags="database") {
    ComponentDb(pins,      "pin_metadata", "cid PK, owner, tags, ttl, status, repl_factor")
    ComponentDb(locations, "pin_locations", "cid, peer_id, is_healthy, last_seen")
    ComponentDb(owners,    "owner_pins", "owner_id, cid, created_at -- for listing")
    ComponentDb(ttlq,      "pin_ttl_queue", "ttl_bucket, cid -- for scheduler")
}

' ===== BILLING TABLES =====
Container_Boundary(billing_tables, "Billing Tables (ScyllaDB)", $tags="billing") {
    ComponentDb(usage,     "storage_usage", "owner_id, day, total_bytes -- daily rollup")
    ComponentDb(billing_e, "billing_events", "event_id, owner_id, amount, currency, status, created_at")
}

' ===== MESSAGE BUS =====
Container(bus, "NATS Server (JetStream)", "Topics + Durable Consumers", $tags="bus") {
    ComponentQueue(req,   "pin.request", "CID, owner, repl, ttl, tags")
    ComponentQueue(assign,"pin.assign", "CID → target_peer")
    ComponentQueue(status, "pin.status", "CID, peer, success/size/error")
    ComponentQueue(bill_ev,"billing.event", "owner_id, GB, cost, period, invoice_id")
}

' ===== EXTERNAL =====
System_Ext(kubo, "IPFS Kubo Node (go-ipfs)", "Stores blocks; GC; local pinset; HTTP API /api/v0/pin/*", $tags="external")
System_Ext(billing_sys, "External Billing System", "Stripe / Filecoin PayCh / L2 Wallet", $tags="external")
System_Ext(idp, "Identity Provider", "Keycloak / DID Resolver / Wallet Auth", $tags="external")

' ===== SECURITY SERVICE ===== (НОВЫЙ КОНТЕЙНЕР)
Container_Boundary(auth_svc, "Auth Service (Go)", "AuthN & AuthZ; RBAC/ABAC; Audit Log", $tags="security") {
    Component(authn, "Authenticator", "JWT/API Key/DID", "Validates credentials → returns owner_id/context")
    Component(authz, "Authorizer", "Go / Policy Engine", "Checks: can owner_id pin this CID? delete others?")
    Component(audit, "Audit Logger", "Go", "Logs all access attempts → optional storage")
}

' ===== CLUSTER SERVICE =====
Container_Boundary(cluster_svc, "Cluster Service (Go)", "Stateless API + Planner + Scheduler", $tags="service") {
    Component(api,     "API Gateway", "Go / REST+gRPC", "Delegates AuthN/Z to Auth Service\\nRoutes to handlers")
    Component(plan,    "Reconciler Engine", "Go / Control Loop", "Compares desired vs actual → pub pin.assign")
    Component(ttl_eng, "TTL Scheduler", "Go / Cron-like", "Scans TTL → pub unpin requests")
}

' ===== WORKER AGENT =====
Container_Boundary(worker_agent, "Pin Worker Agent (Go)", "Stateless, scales horizontally", $tags="worker") {
    Component(nats_sub, "NATS Consumer", "Go / JetStream", "Sub pin.assign → exec Kubo")
    Component(kubo_cli, "Kubo HTTP Client", "Go / Retry+Timeout", "ipfs pin add/rm/stat")
    Component(reporter, "Status Reporter", "Go", "Pub pin.status after action")
}

' ===== BILLING SERVICE =====
Container_Boundary(billing_svc, "Billing Service (Go)", "Calculates usage & cost; emits billing events", $tags="billing") {
    Component(usage_collector, "Usage Collector", "Go / Cron", "Aggregates → storage_usage")
    Component(cost_calculator, "Cost Calculator", "Go", "Emit billing.event")
    Component(invoice_emitter, "Invoice Emitter", "Go", "Call External Billing System")
}

' ===== SECURITY FLOW =====
Rel(api, authn, "gRPC: ValidateToken(token) → owner_id, roles", "Sync")
Rel(authn, idp, "OIDC/DID Resolve (if needed)", "Sync")
Rel(api, authz, "gRPC: CheckPermission(owner_id, action, resource)", "Sync")
Rel(authz, audit, "Log: access_granted/denied", "Async")

' ===== CORE FLOW =====
Rel(api, pins, "CQL: INSERT/UPDATE (after AuthZ)", "Sync")
Rel(plan, scylla, "CQL: read desired/actual; update", "Sync")
Rel(ttl_eng, ttlq, "CQL: SELECT expired", "Sync")
Rel(reporter, status, "PUB: pin.status", "Async")
Rel(kubo_cli, kubo, "HTTP: pin/add/rm", "Sync")
Rel(api, req, "PUB: pin.request", "Async")
Rel(plan, assign, "PUB: pin.assign", "Async")
Rel(nats_sub, assign, "SUB: pin.assign", "Async")

' ===== BILLING FLOW =====
Rel(reporter, usage, "CQL: UPSERT storage_usage", "Async")
Rel(usage_collector, usage, "CQL: SELECT daily rollup", "Sync")
Rel(cost_calculator, bill_ev, "PUB: billing.event", "Async")
Rel(invoice_emitter, bill_ev, "SUB: billing.event", "Async")
Rel(invoice_emitter, billing_sys, "HTTP/gRPC: createInvoice()", "Sync")
Rel(invoice_emitter, billing_e, "CQL: INSERT billing_events", "Sync")

LAYOUT_WITH_LEGEND()
@enduml
```

## 🔐 Security/AuthZ Слой

### Цель Security/AuthZ слоя:
- **Аутентификация (AuthN)** — кто делает запрос? (API Key, JWT, DID, EVM-подпись)
- **Авторизация (AuthZ)** — имеет ли право на операцию? (RBAC, ABAC — например, "может пинить только в своём tenant")
- **Аудит и логирование доступа** — кто, когда, что пытался сделать
- **Интеграция с Identity Provider** — Keycloak, Auth0, встроенный IAM, Web3 кошельки

### 🎨 Цветовая семантика Security:
**Красный/бордовый `#D92D20`** — в enterprise-дизайне тёмно-красный используется для безопасности (как в IBM, Palo Alto, AWS IAM). Он сигнализирует об ответственности и защите, но не кричит, как ярко-красный.

### 🔐 Как работает Security/AuthZ слой:
1. **Запрос приходит в API Gateway** — с токеном (JWT, API Key, подпись кошелька)
2. **API Gateway вызывает `Authenticator`** — тот валидирует токен и возвращает `owner_id` и роли
3. **API Gateway вызывает `Authorizer`** — проверяет, может ли `owner_id` выполнить действие (например, удалить чужой CID)
4. **Если разрешено** — запрос проходит дальше к логике пиннинга и записи в ScyllaDB
5. **Любая попытка доступа логируется** — для аудита и compliance

### 🖼️ Визуальные особенности:
- **Красный контейнер `Auth Service`** — сразу видно, где находится безопасность
- **Красная внешняя система `Identity Provider`** — подчёркивает, что аутентификация может быть внешней
- **Связи от API Gateway к AuthN/Z** — показывают, что безопасность — первый шаг обработки запроса
- **Совместимость с биллингом** — `owner_id`, полученный из Auth, используется и для учёта расходов

### 💡 Возможные расширения Security системы:
- **Rate Limiting** — на уровне Auth Service по owner_id
- **API Key Rotation** — интеграция с HashiCorp Vault
- **Web3 Signature Auth** — верификация подписей EVM/ED25519
- **Policy-as-Code** — OPA (Open Policy Agent) вместо встроенного Authorizer
- **Multi-factor Authentication** — поддержка 2FA/TOTP
- **Session Management** — управление сессиями и токенами
- **Compliance Logging** — детальные логи для SOX, GDPR, HIPAA
- **Zero Trust Architecture** — проверка каждого запроса независимо от источника