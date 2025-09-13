# C4 Component Diagram - Подробное объяснение

## Обзор диаграммы

**Файл:** `C4_Component_Diagram.puml`  
**Уровень C4:** Component  
**Цель:** Детальная архитектура компонентов внутри ScyllaDB State Store с показом всех модулей и их взаимодействий

## Архитектурный дизайн → Реализация кода

### 1. Interface Layer (Слой интерфейсов)

#### state.State Interface

**Архитектурная роль:**
- Определяет контракт для всех операций с состоянием
- Обеспечивает совместимость с существующим кодом IPFS-Cluster
- Поддерживает как синхронные, так и пакетные операции

**Реализация в коде:**
```go
// state/state.go (концептуальный интерфейс)
type State interface {
    ReadOnlyState
    WriteOnlyState
    BatchingState
}

type ReadOnlyState interface {
    Get(context.Context, api.Cid) (api.Pin, error)
    Has(context.Context, api.Cid) (bool, error)
    List(context.Context, chan<- api.Pin) error
}

type WriteOnlyState interface {
    Add(context.Context, api.Pin) error
    Rm(context.Context, api.Cid) error
}

type BatchingState interface {
    NewBatch(context.Context) (BatchingState, error)
    Commit(context.Context) error
}
```

**Проверка реализации интерфейса:**
```go
// state/scyllastate/scyllastate.go
// Compile-time проверка реализации интерфейсов
var _ state.State = (*ScyllaState)(nil)
var _ state.BatchingState = (*ScyllaBatchingState)(nil)
```

### 2. Core Logic Layer (Слой основной логики)

#### ScyllaState (Основная реализация)

**Архитектурная роль:**
- Центральный компонент, реализующий все операции с состоянием
- Управляет подключением к ScyllaDB через gocql.Session
- Координирует работу всех подсистем

**Реализация в коде:**
```go
// state/scyllastate/scyllastate.go
type ScyllaState struct {
    // Подключение к базе данных
    session     *gocql.Session
    keyspace    string
    
    // Конфигурация и зависимости
    config      *Config
    migrator    *MigrationManager
    
    // Оптимизации производительности
    prepared    *PreparedStatements
    metrics     *Metrics
    
    // Управление состоянием
    totalPins   int64
    codecHandle codec.Handle
}

// Основные операции CRUD
func (s *ScyllaState) Add(ctx context.Context, pin api.Pin) error {
    start := time.Now()
    defer s.recordMetrics("add", start, nil)
    
    // Валидация входных данных
    if err := s.validatePin(pin); err != nil {
        return fmt.Errorf("invalid pin: %w", err)
    }
    
    // Сериализация пина
    data, err := s.serializePin(pin)
    if err != nil {
        return fmt.Errorf("serialization failed: %w", err)
    }
    
    // Вычисление ключа партиционирования
    mhPrefix := s.getMhPrefix(pin.Cid)
    
    // Выполнение CQL запроса с retry логикой
    return s.executeWithRetry(ctx, func() error {
        return s.prepared.insertPin.WithContext(ctx).Bind(
            mhPrefix,
            pin.Cid.Bytes(),
            int(pin.Type),
            pin.ReplicationFactorMin,
            pin.Owner,
            pin.Tags,
            pin.ExpireAt,
            pin.Metadata,
            time.Now(),
            time.Now(),
            1, // version для optimistic locking
        ).Exec()
    })
}

func (s *ScyllaState) Get(ctx context.Context, cid api.Cid) (api.Pin, error) {
    start := time.Now()
    defer s.recordMetrics("get", start, nil)
    
    mhPrefix := s.getMhPrefix(cid)
    
    var (
        pinType    int
        rf         int
        owner      string
        tags       []string
        expireAt   *time.Time
        metadata   map[string]string
        createdAt  time.Time
        updatedAt  time.Time
        version    int
    )
    
    err := s.prepared.selectPin.WithContext(ctx).Bind(
        mhPrefix, cid.Bytes(),
    ).Scan(
        &pinType, &rf, &owner, &tags, &expireAt,
        &metadata, &createdAt, &updatedAt, &version,
    )
    
    if err != nil {
        if err == gocql.ErrNotFound {
            return api.Pin{}, state.ErrNotFound
        }
        return api.Pin{}, fmt.Errorf("query failed: %w", err)
    }
    
    return api.Pin{
        Cid:                  cid,
        Type:                 api.PinType(pinType),
        ReplicationFactorMin: rf,
        ReplicationFactorMax: rf,
        Owner:                owner,
        Tags:                 tags,
        ExpireAt:             expireAt,
        Metadata:             metadata,
        Timestamp:            createdAt,
    }, nil
}
```

#### ScyllaBatchingState (Пакетные операции)

**Архитектурная роль:**
- Реализует пакетные операции для повышения производительности
- Группирует множественные операции в одну транзакцию
- Обеспечивает атомарность пакетных изменений

**Реализация в коде:**
```go
// state/scyllastate/batching.go
type ScyllaBatchingState struct {
    *ScyllaState
    batch     *gocql.Batch
    operations []BatchOperation
}

type BatchOperation struct {
    Type string      // "add", "remove"
    Pin  api.Pin
    Cid  api.Cid
}

func (s *ScyllaState) NewBatch(ctx context.Context) (state.BatchingState, error) {
    batch := s.session.NewBatch(gocql.LoggedBatch)
    
    return &ScyllaBatchingState{
        ScyllaState: s,
        batch:       batch,
        operations:  make([]BatchOperation, 0),
    }, nil
}

func (bs *ScyllaBatchingState) Add(ctx context.Context, pin api.Pin) error {
    // Добавление операции в пакет
    bs.operations = append(bs.operations, BatchOperation{
        Type: "add",
        Pin:  pin,
    })
    
    // Подготовка CQL statement для пакета
    mhPrefix := bs.getMhPrefix(pin.Cid)
    data, err := bs.serializePin(pin)
    if err != nil {
        return err
    }
    
    bs.batch.Query(`
        INSERT INTO pins_by_cid (mh_prefix, cid_bin, pin_data, created_at)
        VALUES (?, ?, ?, ?)
    `, mhPrefix, pin.Cid.Bytes(), data, time.Now())
    
    return nil
}

func (bs *ScyllaBatchingState) Commit(ctx context.Context) error {
    start := time.Now()
    defer bs.recordMetrics("batch_commit", start, nil)
    
    if len(bs.operations) == 0 {
        return nil // Нет операций для выполнения
    }
    
    // Выполнение пакета с retry логикой
    return bs.executeWithRetry(ctx, func() error {
        return bs.session.ExecuteBatch(bs.batch.WithContext(ctx))
    })
}
```

### 3. Configuration Layer (Слой конфигурации)

#### Config (Конфигурация)

**Архитектурная роль:**
- Централизованное управление всеми настройками
- Поддержка различных источников конфигурации
- Валидация параметров подключения

**Реализация в коде:**
```go
// state/scyllastate/config.go
type Config struct {
    config.Saver `json:"-"`
    
    // Подключение к кластеру
    Hosts               []string `json:"hosts" validate:"required,min=1"`
    Port                int      `json:"port" validate:"min=1,max=65535"`
    Keyspace            string   `json:"keyspace" validate:"required"`
    Username            string   `json:"username"`
    Password            string   `json:"password"`
    
    // Настройки TLS
    TLSEnabled          bool   `json:"tls_enabled"`
    TLSCertFile         string `json:"tls_cert_file"`
    TLSKeyFile          string `json:"tls_key_file"`
    TLSCAFile           string `json:"tls_ca_file"`
    TLSInsecureSkipVerify bool `json:"tls_insecure_skip_verify"`
    
    // Настройки производительности
    NumConns            int           `json:"num_conns" validate:"min=1,max=100"`
    Timeout             time.Duration `json:"timeout"`
    ConnectTimeout      time.Duration `json:"connect_timeout"`
    Consistency         string        `json:"consistency"`
    SerialConsistency   string        `json:"serial_consistency"`
    
    // Настройки повторов
    RetryPolicy         RetryPolicyConfig `json:"retry_policy"`
    
    // Настройки пакетной обработки
    BatchSize           int           `json:"batch_size" validate:"min=1,max=10000"`
    BatchTimeout        time.Duration `json:"batch_timeout"`
    
    // Мониторинг
    MetricsEnabled      bool `json:"metrics_enabled"`
    TracingEnabled      bool `json:"tracing_enabled"`
}

// Значения по умолчанию
func DefaultConfig() *Config {
    return &Config{
        Hosts:              []string{"127.0.0.1"},
        Port:               9042,
        Keyspace:           "ipfs_pins",
        NumConns:           5,
        Timeout:            30 * time.Second,
        ConnectTimeout:     10 * time.Second,
        Consistency:        "QUORUM",
        SerialConsistency:  "SERIAL",
        BatchSize:          1000,
        BatchTimeout:       1 * time.Second,
        MetricsEnabled:     true,
        TracingEnabled:     false,
        RetryPolicy: RetryPolicyConfig{
            NumRetries:    3,
            MinRetryDelay: 100 * time.Millisecond,
            MaxRetryDelay: 10 * time.Second,
        },
    }
}
```

#### TLS & Auth (TLS и аутентификация)

**Архитектурная роль:**
- Управление безопасными соединениями
- Обработка сертификатов и ключей
- Настройка аутентификации

**Реализация в коде:**
```go
// state/scyllastate/tls_auth.go
type TLSConfig struct {
    Enabled             bool   `json:"enabled"`
    CertFile            string `json:"cert_file"`
    KeyFile             string `json:"key_file"`
    CAFile              string `json:"ca_file"`
    InsecureSkipVerify  bool   `json:"insecure_skip_verify"`
}

func (cfg *Config) createTLSConfig() (*tls.Config, error) {
    if !cfg.TLSEnabled {
        return nil, nil
    }
    
    tlsConfig := &tls.Config{
        InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
    }
    
    // Загрузка клиентского сертификата
    if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
        cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
        if err != nil {
            return nil, fmt.Errorf("failed to load client certificate: %w", err)
        }
        tlsConfig.Certificates = []tls.Certificate{cert}
    }
    
    // Загрузка CA сертификата
    if cfg.TLSCAFile != "" {
        caCert, err := ioutil.ReadFile(cfg.TLSCAFile)
        if err != nil {
            return nil, fmt.Errorf("failed to read CA certificate: %w", err)
        }
        
        caCertPool := x509.NewCertPool()
        if !caCertPool.AppendCertsFromPEM(caCert) {
            return nil, errors.New("failed to parse CA certificate")
        }
        tlsConfig.RootCAs = caCertPool
    }
    
    return tlsConfig, nil
}

type AuthConfig struct {
    Username string `json:"username"`
    Password string `json:"password"`
}

func (cfg *Config) createAuthenticator() gocql.Authenticator {
    if cfg.Username != "" && cfg.Password != "" {
        return gocql.PasswordAuthenticator{
            Username: cfg.Username,
            Password: cfg.Password,
        }
    }
    return nil
}
```

#### Config Validation (Валидация конфигурации)

**Архитектурная роль:**
- Проверка корректности всех параметров
- Установка значений по умолчанию
- Предотвращение ошибок конфигурации

**Реализация в коде:**
```go
// state/scyllastate/validation.go
import "github.com/go-playground/validator/v10"

var validate = validator.New()

func (cfg *Config) Validate() error {
    // Структурная валидация через теги
    if err := validate.Struct(cfg); err != nil {
        return fmt.Errorf("config validation failed: %w", err)
    }
    
    // Кастомная валидация
    if err := cfg.validateHosts(); err != nil {
        return err
    }
    
    if err := cfg.validateTLS(); err != nil {
        return err
    }
    
    if err := cfg.validateConsistency(); err != nil {
        return err
    }
    
    return nil
}

func (cfg *Config) validateHosts() error {
    for _, host := range cfg.Hosts {
        if host == "" {
            return errors.New("empty host in hosts list")
        }
        // Проверка формата хоста (IP или hostname)
        if net.ParseIP(host) == nil {
            if _, err := net.LookupHost(host); err != nil {
                return fmt.Errorf("invalid host %s: %w", host, err)
            }
        }
    }
    return nil
}

func (cfg *Config) validateTLS() error {
    if !cfg.TLSEnabled {
        return nil
    }
    
    if cfg.TLSCertFile != "" {
        if _, err := os.Stat(cfg.TLSCertFile); os.IsNotExist(err) {
            return fmt.Errorf("TLS cert file does not exist: %s", cfg.TLSCertFile)
        }
    }
    
    if cfg.TLSKeyFile != "" {
        if _, err := os.Stat(cfg.TLSKeyFile); os.IsNotExist(err) {
            return fmt.Errorf("TLS key file does not exist: %s", cfg.TLSKeyFile)
        }
    }
    
    return nil
}

func (cfg *Config) validateConsistency() error {
    validConsistencies := []string{
        "ANY", "ONE", "TWO", "THREE", "QUORUM", 
        "ALL", "LOCAL_QUORUM", "EACH_QUORUM",
    }
    
    for _, valid := range validConsistencies {
        if cfg.Consistency == valid {
            return nil
        }
    }
    
    return fmt.Errorf("invalid consistency level: %s", cfg.Consistency)
}
```

### 4. Migration Layer (Слой миграций)

#### MigrationManager (Менеджер миграций)

**Архитектурная роль:**
- Координирует все операции миграций
- Отслеживает версии схемы
- Обеспечивает безопасное обновление

**Реализация в коде:**
```go
// state/scyllastate/migrations.go
type MigrationManager struct {
    session    *gocql.Session
    keyspace   string
    migrations []Migration
}

type Migration struct {
    Version     string
    Description string
    UpScript    string
    DownScript  string
    Validate    func(*gocql.Session) error
}

func NewMigrationManager(session *gocql.Session, keyspace string) *MigrationManager {
    mm := &MigrationManager{
        session:  session,
        keyspace: keyspace,
    }
    mm.registerMigrations()
    return mm
}

func (mm *MigrationManager) registerMigrations() {
    mm.migrations = []Migration{
        {
            Version:     "1.0.0",
            Description: "Initial schema creation",
            UpScript:    getInitialSchemaScript(),
            Validate:    mm.validateInitialSchema,
        },
        {
            Version:     "1.1.0",
            Description: "Add enhanced pin metadata and performance tables",
            UpScript:    getV110MigrationScript(),
            Validate:    mm.validateV110Schema,
        },
    }
}

func (mm *MigrationManager) AutoMigrate(ctx context.Context) error {
    // Инициализация таблицы версий
    if err := mm.InitializeSchema(ctx); err != nil {
        return err
    }
    
    // Проверка совместимости
    if err := mm.CheckCompatibility(ctx); err != nil {
        return err
    }
    
    // Применение миграций
    if err := mm.ApplyMigrations(ctx); err != nil {
        return err
    }
    
    // Финальная валидация
    return mm.ValidateSchema(ctx)
}
```

#### Migration Scripts (Скрипты миграций)

**Архитектурная роль:**
- Содержит SQL скрипты для всех версий схемы
- Обеспечивает идемпотентность операций
- Поддерживает откат изменений

**Реализация в коде:**
```go
// state/scyllastate/migration_scripts.go
func getInitialSchemaScript() string {
    return `
-- Initial ScyllaDB schema for IPFS-Cluster pin metadata storage (v1.0.0)

CREATE KEYSPACE IF NOT EXISTS ipfs_pins
WITH replication = {
    'class': 'NetworkTopologyStrategy', 
    'datacenter1': '3'
} AND durable_writes = true;

CREATE TABLE IF NOT EXISTS ipfs_pins.pins_by_cid (
    mh_prefix smallint,
    cid_bin blob,
    pin_type tinyint,
    rf tinyint,
    owner text,
    tags set<text>,
    ttl timestamp,
    metadata map<text, text>,
    created_at timestamp,
    updated_at timestamp,
    size bigint,
    status tinyint,
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160'
};

-- Дополнительные таблицы...
`
}

func getV110MigrationScript() string {
    return `
-- Migration from version 1.0.0 to 1.1.0
-- Adds enhanced pin metadata and performance optimization tables

-- Add new columns to pins_by_cid table
ALTER TABLE ipfs_pins.pins_by_cid ADD version int;
ALTER TABLE ipfs_pins.pins_by_cid ADD checksum text;
ALTER TABLE ipfs_pins.pins_by_cid ADD priority tinyint;

-- Create new performance monitoring tables
CREATE TABLE IF NOT EXISTS ipfs_pins.partition_stats (
    mh_prefix smallint,
    stat_type text,
    stat_value bigint,
    updated_at timestamp,
    PRIMARY KEY (mh_prefix, stat_type)
) WITH compaction = {'class': 'LeveledCompactionStrategy'};

-- Дополнительные изменения...
`
}
```

### 5. Testing Layer (Слой тестирования)

#### Unit Tests (Модульные тесты)

**Архитектурная роль:**
- Тестирование всех компонентов в изоляции
- Проверка корректности бизнес-логики
- Обеспечение покрытия кода тестами

**Реализация в коде:**
```go
// state/scyllastate/migrations_test.go
func TestMigrationManager_compareVersions(t *testing.T) {
    mm := &MigrationManager{}
    
    tests := []struct {
        name     string
        v1       string
        v2       string
        expected int
    }{
        {"equal versions", "1.0.0", "1.0.0", 0},
        {"v1 less than v2", "1.0.0", "1.1.0", -1},
        {"v1 greater than v2", "1.1.0", "1.0.0", 1},
        {"major version difference", "2.0.0", "1.9.9", 1},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := mm.compareVersions(tt.v1, tt.v2)
            assert.Equal(t, tt.expected, result)
        })
    }
}

func TestScyllaState_Add(t *testing.T) {
    // Mock setup
    mockSession := &MockSession{}
    state := &ScyllaState{
        session: mockSession,
        config:  DefaultConfig(),
    }
    
    // Test data
    pin := api.Pin{
        Cid:  testCid,
        Type: api.DataType,
        ReplicationFactorMin: 3,
    }
    
    // Execute
    err := state.Add(context.Background(), pin)
    
    // Verify
    assert.NoError(t, err)
    assert.Equal(t, 1, mockSession.QueryCount())
}
```

### 6. Database Layer (Слой базы данных)

#### ScyllaDB Tables (Таблицы ScyllaDB)

**Архитектурная роль:**
- Физическое хранение метаданных пинов
- Оптимизированное партиционирование
- Высокая производительность и масштабируемость

**Реализация схемы:**
```sql
-- Основная таблица метаданных пинов
CREATE TABLE ipfs_pins.pins_by_cid (
    mh_prefix smallint,          -- Ключ партиционирования (65536 партиций)
    cid_bin blob,                -- Кластерный ключ
    pin_type tinyint,            -- Тип пина
    rf tinyint,                  -- Фактор репликации
    owner text,                  -- Владелец пина
    tags set<text>,              -- Теги для категоризации
    ttl timestamp,               -- TTL для автоудаления
    metadata map<text, text>,    -- Дополнительные метаданные
    created_at timestamp,        -- Время создания
    updated_at timestamp,        -- Время обновления
    size bigint,                 -- Размер пина
    status tinyint,              -- Статус пина
    version int,                 -- Версия для optimistic locking
    checksum text,               -- Контрольная сумма
    priority tinyint,            -- Приоритет пина
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160',
    'tombstone_threshold': '0.2'
} AND gc_grace_seconds = 864000
  AND bloom_filter_fp_chance = 0.01
  AND caching = {'keys': 'ALL', 'rows_per_partition': '1000'}
  AND compression = {'class': 'LZ4Compressor'};
```

## Взаимодействие компонентов

### 1. Инициализация системы
```
IPFS-Cluster → Config.Validate() → ScyllaState.New() → MigrationManager.AutoMigrate() → PreparedStatements.prepare()
```

### 2. Операция добавления пина
```
Cluster Core → ScyllaState.Add() → serializePin() → getMhPrefix() → prepared.insertPin.Exec() → recordMetrics()
```

### 3. Пакетные операции
```
Cluster Core → ScyllaState.NewBatch() → ScyllaBatchingState.Add() → ScyllaBatchingState.Commit() → ExecuteBatch()
```

### 4. Миграция схемы
```
MigrationManager.AutoMigrate() → GetCurrentVersion() → getPendingMigrations() → applyMigration() → ValidateSchema()
```

## Ключевые архитектурные паттерны

### 1. Layered Architecture
- **Interface Layer:** Определение контрактов
- **Core Logic:** Бизнес-логика
- **Configuration:** Управление настройками
- **Migration:** Версионирование схемы
- **Testing:** Обеспечение качества

### 2. Dependency Injection
```go
type ScyllaState struct {
    session  *gocql.Session      // Инжектируется при создании
    config   *Config             // Инжектируется при создании
    migrator *MigrationManager   // Инжектируется при создании
}
```

### 3. Strategy Pattern
```go
type RetryPolicy interface {
    Execute(context.Context, func() error) error
}

type ExponentialBackoffRetry struct {
    maxRetries int
    baseDelay  time.Duration
}
```

### 4. Template Method Pattern
```go
func (s *ScyllaState) executeWithRetry(ctx context.Context, operation func() error) error {
    return s.retryPolicy.Execute(ctx, operation)
}
```

## Выводы

Component диаграмма показывает детальную архитектуру с четким разделением ответственности:

✅ **Модульность** - каждый компонент имеет четко определенную роль  
✅ **Тестируемость** - все компоненты могут тестироваться независимо  
✅ **Расширяемость** - новые функции легко добавляются  
✅ **Сопровождаемость** - четкая структура упрощает поддержку  
✅ **Производительность** - оптимизации на всех уровнях  
