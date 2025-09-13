# C4 Container Diagram - Подробное объяснение

## Обзор диаграммы

**Файл:** `C4_Container_Diagram.puml`  
**Уровень C4:** Container  
**Цель:** Показать архитектуру контейнеров внутри IPFS-Cluster системы с фокусом на ScyllaDB State Store

## Архитектурный дизайн → Реализация кода

### 1. Cluster Core (Ядро кластера)

**Архитектурная роль:**
- Главная логика оркестрации IPFS-Cluster
- Использует State Store через интерфейс
- Координирует операции пиннинга

**Реализация в коде:**
```go
// cluster/cluster.go (концептуальный пример)
type Cluster struct {
    state     state.State        // Интерфейс к хранилищу состояния
    consensus consensus.State    // Консенсус слой
    api       *restapi.API      // REST API
    ipfs      ipfsconn.IPFSConnector
}

// Использование State Store
func (c *Cluster) Pin(ctx context.Context, cid api.Cid, opts api.PinOptions) error {
    pin := api.Pin{
        Cid:                  cid,
        Type:                 opts.Type,
        ReplicationFactorMin: opts.ReplicationFactorMin,
        ReplicationFactorMax: opts.ReplicationFactorMax,
        Metadata:             opts.Metadata,
    }
    
    // Сохранение в State Store (ScyllaDB)
    return c.state.Add(ctx, pin)
}
```

**Интеграция с ScyllaState:**
```go
// cluster/config.go
type Config struct {
    State struct {
        DataStore string `json:"datastore"` // "scylladb"
        ScyllaDB  *scyllastate.Config `json:"scylladb,omitempty"`
    } `json:"state"`
}

// Фабричный метод для создания State Store
func (cfg *Config) NewStateStore() (state.State, error) {
    switch cfg.State.DataStore {
    case "scylladb":
        return scyllastate.New(context.Background(), cfg.State.ScyllaDB)
    default:
        return nil, fmt.Errorf("unknown datastore: %s", cfg.State.DataStore)
    }
}
```

### 2. ScyllaState (Основная реализация)

**Архитектурная роль:**
- Реализует интерфейс state.State
- Управляет подключением к ScyllaDB
- Выполняет CRUD операции с метаданными пинов

**Реализация в коде:**
```go
// state/scyllastate/scyllastate.go
type ScyllaState struct {
    session     *gocql.Session
    keyspace    string
    config      *Config
    prepared    *PreparedStatements
    metrics     *Metrics
    migrator    *MigrationManager
}

// Реализация интерфейса state.State
func (s *ScyllaState) Add(ctx context.Context, pin api.Pin) error {
    // Начало метрики
    start := time.Now()
    defer func() {
        s.metrics.RecordOperation("add", time.Since(start), nil)
    }()
    
    // Сериализация пина
    data, err := s.serializePin(pin)
    if err != nil {
        return fmt.Errorf("failed to serialize pin: %w", err)
    }
    
    // Вычисление ключа партиционирования
    mhPrefix := s.getMhPrefix(pin.Cid)
    
    // Выполнение подготовленного запроса
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
    ).Exec()
}

func (s *ScyllaState) Get(ctx context.Context, cid api.Cid) (api.Pin, error) {
    mhPrefix := s.getMhPrefix(cid)
    
    var pinData []byte
    var pinType int
    var rf int
    var owner string
    var createdAt time.Time
    
    err := s.prepared.selectPin.WithContext(ctx).Bind(
        mhPrefix, cid.Bytes(),
    ).Scan(&pinData, &pinType, &rf, &owner, &createdAt)
    
    if err != nil {
        if err == gocql.ErrNotFound {
            return api.Pin{}, state.ErrNotFound
        }
        return api.Pin{}, err
    }
    
    return s.deserializePin(cid, pinData)
}
```

**Подготовленные запросы:**
```go
// state/scyllastate/prepared.go
type PreparedStatements struct {
    insertPin    *gocql.Query
    selectPin    *gocql.Query
    deletePin    *gocql.Query
    listPins     *gocql.Query
    updatePin    *gocql.Query
}

func (s *ScyllaState) prepareStatements() error {
    var err error
    
    // Подготовка запроса вставки
    s.prepared.insertPin, err = s.session.Prepare(`
        INSERT INTO pins_by_cid (
            mh_prefix, cid_bin, pin_type, rf, owner, tags, 
            ttl, metadata, created_at, updated_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare insert statement: %w", err)
    }
    
    // Подготовка запроса выборки
    s.prepared.selectPin, err = s.session.Prepare(`
        SELECT pin_type, rf, owner, tags, ttl, metadata, created_at, updated_at
        FROM pins_by_cid 
        WHERE mh_prefix = ? AND cid_bin = ?
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare select statement: %w", err)
    }
    
    return nil
}
```

### 3. Configuration Manager (Менеджер конфигурации)

**Архитектурная роль:**
- Управляет настройками подключения к ScyllaDB
- Обрабатывает TLS и аутентификацию
- Валидирует конфигурацию

**Реализация в коде:**
```go
// state/scyllastate/config.go
type Config struct {
    config.Saver `json:"-"`
    
    // Подключение к кластеру
    Hosts               []string `json:"hosts"`
    Port                int      `json:"port"`
    Keyspace            string   `json:"keyspace"`
    Username            string   `json:"username"`
    Password            string   `json:"password"`
    
    // Настройки TLS
    TLSEnabled          bool   `json:"tls_enabled"`
    TLSCertFile         string `json:"tls_cert_file"`
    TLSKeyFile          string `json:"tls_key_file"`
    TLSCAFile           string `json:"tls_ca_file"`
    TLSInsecureSkipVerify bool `json:"tls_insecure_skip_verify"`
    
    // Настройки производительности
    NumConns            int           `json:"num_conns"`
    Timeout             time.Duration `json:"timeout"`
    ConnectTimeout      time.Duration `json:"connect_timeout"`
    Consistency         string        `json:"consistency"`
    
    // Настройки повторов
    RetryPolicy         RetryPolicyConfig `json:"retry_policy"`
}

// Валидация конфигурации
func (cfg *Config) Validate() error {
    if len(cfg.Hosts) == 0 {
        return errors.New("at least one host must be specified")
    }
    
    if cfg.Port <= 0 || cfg.Port > 65535 {
        return errors.New("port must be between 1 and 65535")
    }
    
    if cfg.Keyspace == "" {
        return errors.New("keyspace must be specified")
    }
    
    if cfg.TLSEnabled {
        if cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
            return errors.New("TLS cert and key files must be specified when TLS is enabled")
        }
    }
    
    return nil
}

// Создание TLS конфигурации
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
```

### 4. Migration System (Система миграций)

**Архитектурная роль:**
- Управляет версионированием схемы
- Выполняет автоматические миграции
- Валидирует структуру схемы

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

// Автоматическое применение миграций
func (mm *MigrationManager) AutoMigrate(ctx context.Context) error {
    // Инициализация таблицы версий
    if err := mm.InitializeSchema(ctx); err != nil {
        return fmt.Errorf("failed to initialize schema: %w", err)
    }
    
    // Получение текущей версии
    currentVersion, err := mm.GetCurrentVersion(ctx)
    if err != nil {
        return fmt.Errorf("failed to get current version: %w", err)
    }
    
    // Определение необходимых миграций
    var startVersion string
    if currentVersion == nil {
        startVersion = "0.0.0"
    } else {
        startVersion = currentVersion.Version
    }
    
    pendingMigrations := mm.getPendingMigrations(startVersion)
    
    // Применение миграций
    for _, migration := range pendingMigrations {
        if err := mm.applyMigration(ctx, migration); err != nil {
            return fmt.Errorf("failed to apply migration %s: %w", migration.Version, err)
        }
    }
    
    return nil
}

// Применение одной миграции
func (mm *MigrationManager) applyMigration(ctx context.Context, migration Migration) error {
    // Парсинг SQL скрипта
    statements := mm.parseStatements(migration.UpScript)
    
    // Выполнение каждого statement
    for _, stmt := range statements {
        if strings.TrimSpace(stmt) == "" {
            continue
        }
        
        if err := mm.session.Query(stmt).WithContext(ctx).Exec(); err != nil {
            if !mm.isIgnorableError(err) {
                return fmt.Errorf("failed to execute statement: %w", err)
            }
        }
    }
    
    // Валидация миграции
    if migration.Validate != nil {
        if err := migration.Validate(mm.session); err != nil {
            return fmt.Errorf("migration validation failed: %w", err)
        }
    }
    
    // Запись версии
    return mm.SetVersion(ctx, migration.Version, migration.Description)
}
```

**Регистрация миграций:**
```go
// state/scyllastate/migration_scripts.go
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
```

### 5. ScyllaDB Cluster (Кластер ScyllaDB)

**Архитектурная роль:**
- Физическое хранение данных
- Обеспечение репликации и консистентности
- Высокая производительность и масштабируемость

**Схема данных:**
```sql
-- state/scyllastate/schema.sql
CREATE KEYSPACE IF NOT EXISTS ipfs_pins
WITH replication = {
    'class': 'NetworkTopologyStrategy', 
    'datacenter1': '3'
} AND durable_writes = true;

-- Основная таблица метаданных пинов
CREATE TABLE IF NOT EXISTS ipfs_pins.pins_by_cid (
    mh_prefix smallint,          -- Ключ партиционирования
    cid_bin blob,                -- Кластерный ключ
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
    version int,                 -- Для optimistic locking
    checksum text,
    priority tinyint,
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

**Подключение к кластеру:**
```go
// state/scyllastate/connection.go
func (cfg *Config) createClusterConfig() (*gocql.ClusterConfig, error) {
    cluster := gocql.NewCluster(cfg.Hosts...)
    cluster.Port = cfg.Port
    cluster.Keyspace = cfg.Keyspace
    
    // Аутентификация
    if cfg.Username != "" && cfg.Password != "" {
        cluster.Authenticator = gocql.PasswordAuthenticator{
            Username: cfg.Username,
            Password: cfg.Password,
        }
    }
    
    // TLS конфигурация
    if cfg.TLSEnabled {
        tlsConfig, err := cfg.createTLSConfig()
        if err != nil {
            return nil, err
        }
        cluster.SslOpts = &gocql.SslOptions{
            Config: tlsConfig,
        }
    }
    
    // Настройки производительности
    cluster.NumConns = cfg.NumConns
    cluster.Timeout = cfg.Timeout
    cluster.ConnectTimeout = cfg.ConnectTimeout
    
    // Политика выбора хостов
    cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(
        gocql.DCAwareRoundRobinPolicy("datacenter1"),
    )
    
    // Уровень консистентности
    consistency, err := parseConsistency(cfg.Consistency)
    if err != nil {
        return nil, err
    }
    cluster.Consistency = consistency
    
    return cluster, nil
}
```

## Потоки данных между контейнерами

### 1. Инициализация системы
```
Configuration Manager → ScyllaState.New() → Migration System.AutoMigrate() → ScyllaDB
```

### 2. Операции с пинами
```
Cluster Core → ScyllaState.Add/Get/Rm → ScyllaDB CQL Operations
```

### 3. Мониторинг
```
ScyllaState → Metrics Collection → Monitoring System
```

## Ключевые архитектурные паттерны

### 1. Dependency Injection
```go
// ScyllaState получает зависимости через конструктор
func New(ctx context.Context, cfg *Config) (*ScyllaState, error) {
    // Создание подключения
    session, err := createSession(cfg)
    if err != nil {
        return nil, err
    }
    
    // Создание менеджера миграций
    migrator := NewMigrationManager(session, cfg.Keyspace)
    
    // Применение миграций
    if err := migrator.AutoMigrate(ctx); err != nil {
        return nil, err
    }
    
    return &ScyllaState{
        session:  session,
        config:   cfg,
        migrator: migrator,
    }, nil
}
```

### 2. Interface Segregation
```go
// Разделение интерфейсов по ответственности
type ReadOnlyState interface {
    Get(context.Context, api.Cid) (api.Pin, error)
    Has(context.Context, api.Cid) (bool, error)
    List(context.Context, chan<- api.Pin) error
}

type WriteOnlyState interface {
    Add(context.Context, api.Pin) error
    Rm(context.Context, api.Cid) error
}

type State interface {
    ReadOnlyState
    WriteOnlyState
}
```

### 3. Configuration as Code
```go
// Конфигурация как структура с валидацией
type Config struct {
    // Поля конфигурации с тегами для JSON
    Hosts []string `json:"hosts" validate:"required,min=1"`
    Port  int      `json:"port" validate:"min=1,max=65535"`
}

func (cfg *Config) Validate() error {
    return validator.New().Struct(cfg)
}
```

## Выводы

Container диаграмма показывает четкое разделение ответственности:

✅ **Cluster Core** - бизнес-логика оркестрации  
✅ **ScyllaState** - реализация хранилища состояния  
✅ **Configuration Manager** - управление настройками  
✅ **Migration System** - версионирование схемы  
✅ **ScyllaDB** - физическое хранение данных  

Каждый контейнер имеет четко определенные интерфейсы и зоны ответственности, что обеспечивает модульность и тестируемость системы.