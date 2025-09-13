# C4 Context Diagram - Подробное объяснение

## Обзор диаграммы

**Файл:** `C4_Context_Diagram.puml`  
**Уровень C4:** System Context  
**Цель:** Показать ScyllaDB State Store в контексте всей IPFS-Cluster экосистемы

## Архитектурный дизайн → Реализация кода

### 1. Cluster Administrator (Администратор кластера)

**Архитектурная роль:**
- Настраивает подключение к ScyllaDB
- Управляет конфигурацией кластера
- Мониторит производительность системы

**Реализация в коде:**
```go
// state/scyllastate/config.go
type Config struct {
    // Подключение к кластеру - настраивается администратором
    Hosts               []string          `json:"hosts"`
    Port                int               `json:"port"`
    Keyspace            string            `json:"keyspace"`
    Username            string            `json:"username"`
    Password            string            `json:"password"`
    
    // TLS настройки для безопасности
    TLSEnabled          bool              `json:"tls_enabled"`
    TLSCertFile         string            `json:"tls_cert_file"`
    TLSKeyFile          string            `json:"tls_key_file"`
    TLSCAFile           string            `json:"tls_ca_file"`
}
```

**Конфигурационный файл (JSON):**
```json
{
  "state": {
    "datastore": "scylladb",
    "scylladb": {
      "hosts": ["scylla1.example.com", "scylla2.example.com"],
      "port": 9042,
      "keyspace": "ipfs_pins",
      "username": "cluster_admin",
      "password": "secure_password"
    }
  }
}
```

### 2. Developer (Разработчик)

**Архитектурная роль:**
- Использует IPFS-Cluster API для управления пинами
- Интегрирует приложения с кластером
- Работает с state.State интерфейсом

**Реализация в коде:**
```go
// Интерфейс, который использует разработчик
type State interface {
    Add(context.Context, api.Pin) error
    Rm(context.Context, api.Cid) error
    Get(context.Context, api.Cid) (api.Pin, error)
    Has(context.Context, api.Cid) (bool, error)
    List(context.Context, chan<- api.Pin) error
}

// Пример использования разработчиком
func (app *MyApp) PinContent(cid api.Cid) error {
    pin := api.Pin{
        Cid:  cid,
        Type: api.DataType,
        ReplicationFactorMin: 3,
        ReplicationFactorMax: 5,
    }
    
    return app.cluster.State().Add(context.Background(), pin)
}
```

### 3. IPFS-Cluster System (Система IPFS-Cluster)

**Архитектурная роль:**
- Центральная система оркестрации пинов
- Интегрирует ScyllaDB State Store как бэкенд
- Обеспечивает высокоуровневую логику управления

**Реализация в коде:**
```go
// state/scyllastate/scyllastate.go
type ScyllaState struct {
    session     *gocql.Session    // Подключение к ScyllaDB
    keyspace    string            // Keyspace для операций
    config      *Config           // Конфигурация подключения
    prepared    PreparedStatements // Подготовленные запросы
    metrics     *Metrics          // Метрики производительности
}

// Реализация интерфейса state.State
func (s *ScyllaState) Add(ctx context.Context, pin api.Pin) error {
    // Сериализация пина
    data, err := s.serializePin(pin)
    if err != nil {
        return err
    }
    
    // Вычисление ключа партиционирования
    mhPrefix := s.getMhPrefix(pin.Cid)
    
    // Выполнение CQL запроса
    return s.session.Query(`
        INSERT INTO pins_by_cid (mh_prefix, cid_bin, pin_data, created_at)
        VALUES (?, ?, ?, ?)
    `, mhPrefix, pin.Cid.Bytes(), data, time.Now()).
        WithContext(ctx).Exec()
}
```

### 4. ScyllaDB Cluster (Кластер ScyllaDB)

**Архитектурная роль:**
- Высокопроизводительное хранилище метаданных пинов
- Обеспечивает trillion-scale масштабируемость
- Поддерживает multi-DC репликацию

**Реализация в коде:**
```sql
-- state/scyllastate/schema.sql
-- Keyspace с NetworkTopologyStrategy для multi-DC
CREATE KEYSPACE IF NOT EXISTS ipfs_pins
WITH replication = {
    'class': 'NetworkTopologyStrategy', 
    'datacenter1': '3'
} AND durable_writes = true;

-- Основная таблица с оптимизированным партиционированием
CREATE TABLE IF NOT EXISTS ipfs_pins.pins_by_cid (
    mh_prefix smallint,          -- Партиционирование по первым 2 байтам
    cid_bin blob,                -- Бинарный CID
    pin_type tinyint,            -- Тип пина
    rf tinyint,                  -- Фактор репликации
    owner text,                  -- Владелец пина
    created_at timestamp,        -- Время создания
    updated_at timestamp,        -- Время обновления
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160'
};
```

**Подключение к кластеру:**
```go
// state/scyllastate/scyllastate.go
func New(ctx context.Context, cfg *Config) (*ScyllaState, error) {
    // Создание конфигурации кластера
    cluster := gocql.NewCluster(cfg.Hosts...)
    cluster.Port = cfg.Port
    cluster.Keyspace = cfg.Keyspace
    cluster.Authenticator = gocql.PasswordAuthenticator{
        Username: cfg.Username,
        Password: cfg.Password,
    }
    
    // Настройки производительности
    cluster.NumConns = cfg.NumConns
    cluster.Timeout = cfg.Timeout
    cluster.Consistency = gocql.Quorum
    
    // Создание сессии
    session, err := cluster.CreateSession()
    if err != nil {
        return nil, fmt.Errorf("failed to connect to ScyllaDB: %w", err)
    }
    
    return &ScyllaState{
        session:  session,
        keyspace: cfg.Keyspace,
        config:   cfg,
    }, nil
}
```

### 5. IPFS Network (Сеть IPFS)

**Архитектурная роль:**
- Хранит фактические блоки данных
- Выполняет операции пиннинга по командам кластера
- Обеспечивает распределенное хранение контента

**Взаимодействие с кодом:**
```go
// Кластер координирует операции с IPFS узлами
// ScyllaDB хранит только метаданные, не сами данные
func (s *ScyllaState) Add(ctx context.Context, pin api.Pin) error {
    // 1. Сохраняем метаданные в ScyllaDB
    err := s.storeMetadata(ctx, pin)
    if err != nil {
        return err
    }
    
    // 2. Кластер отправляет команды IPFS узлам для фактического пиннинга
    // (это происходит на уровне выше, не в ScyllaState)
    
    return nil
}
```

### 6. Monitoring Stack (Система мониторинга)

**Архитектурная роль:**
- Собирает метрики производительности
- Обеспечивает наблюдаемость системы
- Поддерживает алертинг и дашборды

**Реализация в коде:**
```go
// state/scyllastate/metrics.go
type Metrics struct {
    operationDuration *prometheus.HistogramVec
    operationCounter  *prometheus.CounterVec
    activeConnections prometheus.Gauge
    scyllaErrors      prometheus.Counter
}

func (s *ScyllaState) recordMetrics(operation string, duration time.Duration, err error) {
    labels := prometheus.Labels{"operation": operation}
    
    if err != nil {
        labels["status"] = "error"
        s.metrics.scyllaErrors.Inc()
    } else {
        labels["status"] = "success"
        s.metrics.operationDuration.With(labels).Observe(duration.Seconds())
    }
    
    s.metrics.operationCounter.With(labels).Inc()
}
```

## Потоки данных и взаимодействия

### 1. Конфигурация системы
```
Administrator → Config Files → IPFS-Cluster → ScyllaState.New()
```

### 2. Операции с пинами
```
Developer → Cluster API → ScyllaState.Add/Get/Rm → ScyllaDB CQL
```

### 3. Мониторинг
```
ScyllaState → Prometheus Metrics → Grafana Dashboards → Administrator
```

## Ключевые архитектурные решения

### 1. Разделение ответственности
- **IPFS-Cluster:** Оркестрация и бизнес-логика
- **ScyllaDB State Store:** Хранение метаданных
- **ScyllaDB:** Физическое хранение и репликация
- **IPFS Network:** Хранение фактических данных

### 2. Интерфейсная совместимость
```go
// ScyllaState реализует существующий интерфейс
var _ state.State = (*ScyllaState)(nil)
var _ state.BatchingState = (*ScyllaBatchingState)(nil)
```

### 3. Конфигурационная гибкость
- JSON конфигурация для администраторов
- Программная конфигурация для разработчиков
- Environment variables для контейнеризации

## Выводы

Context диаграмма показывает, как ScyllaDB State Store интегрируется в экосистему IPFS-Cluster:

✅ **Прозрачная интеграция** - реализует существующие интерфейсы  
✅ **Масштабируемость** - использует ScyllaDB для trillion-scale  
✅ **Наблюдаемость** - интегрируется с системами мониторинга  
✅ **Безопасность** - поддерживает TLS и аутентификацию  
✅ **Гибкость** - настраивается через конфигурацию  

Диаграмма служит мостом между высокоуровневыми требованиями и конкретной реализацией кода.