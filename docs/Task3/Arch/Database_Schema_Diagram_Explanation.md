# Database Schema Diagram - Подробное объяснение

## Обзор диаграммы

**Файл:** `Database_Schema_Diagram.puml`  
**Цель:** Показать полную схему ScyllaDB с оптимизацией для trillion-scale управления пинами  
**Фокус:** Партиционирование, производительность, масштабируемость

## Архитектурный дизайн → Реализация кода

### 1. Keyspace: ipfs_pins

**Архитектурная роль:**
- Логическое пространство имен для всех таблиц
- Настройка репликации для multi-DC развертывания
- Управление консистентностью данных

**Реализация в коде:**
```sql
-- state/scyllastate/schema.sql
CREATE KEYSPACE IF NOT EXISTS ipfs_pins
WITH replication = {
    'class': 'NetworkTopologyStrategy', 
    'datacenter1': '3'
} AND durable_writes = true;
```

**Конфигурация в Go:**
```go
// state/scyllastate/config.go
type Config struct {
    Keyspace string `json:"keyspace" validate:"required"`
    // Для multi-DC можно настроить через конфигурацию
    ReplicationStrategy map[string]int `json:"replication_strategy"`
}

// Создание keyspace программно
func (s *ScyllaState) ensureKeyspace(ctx context.Context) error {
    replicationMap := map[string]interface{}{
        "class": "NetworkTopologyStrategy",
    }
    
    // Добавление настроек репликации по датацентрам
    for dc, rf := range s.config.ReplicationStrategy {
        replicationMap[dc] = rf
    }
    
    return s.session.Query(`
        CREATE KEYSPACE IF NOT EXISTS ? 
        WITH replication = ? AND durable_writes = true
    `, s.config.Keyspace, replicationMap).WithContext(ctx).Exec()
}
```

### 2. Main Tables (Основные таблицы)

#### pins_by_cid (Основная таблица метаданных)

**Архитектурная роль:**
- Источник истины для всех пинов
- Оптимизированное партиционирование по mh_prefix
- Поддержка conditional updates через version field

**Реализация схемы:**
```sql
CREATE TABLE IF NOT EXISTS ipfs_pins.pins_by_cid (
    mh_prefix smallint,          -- Партиционирование (0..65535)
    cid_bin blob,                -- Кластерный ключ
    pin_type tinyint,            -- 0=direct, 1=recursive, 2=indirect
    rf tinyint,                  -- Требуемый фактор репликации
    owner text,                  -- Владелец/арендатор для multi-tenancy
    tags set<text>,              -- Теги для категоризации и фильтрации
    ttl timestamp,               -- Плановое авто-удаление (NULL = постоянно)
    metadata map<text, text>,    -- Дополнительные key-value метаданные
    created_at timestamp,        -- Время создания пина
    updated_at timestamp,        -- Время последнего обновления
    size bigint,                 -- Размер пина в байтах
    status tinyint,              -- 0=pending, 1=active, 2=failed, 3=removing
    version int,                 -- Версия для optimistic locking
    checksum text,               -- Контрольная сумма для валидации
    priority tinyint,            -- 0=low, 1=normal, 2=high, 3=critical
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

**Реализация в Go:**
```go
// state/scyllastate/pin.go
type PinMetadata struct {
    MhPrefix  int16             `cql:"mh_prefix"`
    CidBin    []byte            `cql:"cid_bin"`
    PinType   int8              `cql:"pin_type"`
    RF        int8              `cql:"rf"`
    Owner     string            `cql:"owner"`
    Tags      []string          `cql:"tags"`
    TTL       *time.Time        `cql:"ttl"`
    Metadata  map[string]string `cql:"metadata"`
    CreatedAt time.Time         `cql:"created_at"`
    UpdatedAt time.Time         `cql:"updated_at"`
    Size      int64             `cql:"size"`
    Status    int8              `cql:"status"`
    Version   int               `cql:"version"`
    Checksum  string            `cql:"checksum"`
    Priority  int8              `cql:"priority"`
}

// Вычисление ключа партиционирования
func (s *ScyllaState) getMhPrefix(cid api.Cid) int16 {
    cidBytes := cid.Bytes()
    if len(cidBytes) < 2 {
        return 0
    }
    return int16(cidBytes[0])<<8 | int16(cidBytes[1])
}

// CRUD операции
func (s *ScyllaState) insertPin(ctx context.Context, pin api.Pin) error {
    mhPrefix := s.getMhPrefix(pin.Cid)
    
    return s.prepared.insertPin.WithContext(ctx).Bind(
        mhPrefix,
        pin.Cid.Bytes(),
        int8(pin.Type),
        int8(pin.ReplicationFactorMin),
        pin.Owner,
        pin.Tags,
        pin.ExpireAt,
        pin.Metadata,
        time.Now(),
        time.Now(),
        pin.Size,
        int8(pin.Status),
        1, // initial version
        pin.Checksum,
        int8(pin.Priority),
    ).Exec()
}

// Conditional update с optimistic locking
func (s *ScyllaState) updatePinConditional(ctx context.Context, pin api.Pin, expectedVersion int) error {
    mhPrefix := s.getMhPrefix(pin.Cid)
    
    applied, err := s.session.Query(`
        UPDATE pins_by_cid 
        SET rf = ?, tags = ?, metadata = ?, updated_at = ?, version = ?
        WHERE mh_prefix = ? AND cid_bin = ?
        IF version = ?
    `, 
        int8(pin.ReplicationFactorMin),
        pin.Tags,
        pin.Metadata,
        time.Now(),
        expectedVersion+1,
        mhPrefix,
        pin.Cid.Bytes(),
        expectedVersion,
    ).WithContext(ctx).ScanCAS()
    
    if err != nil {
        return err
    }
    
    if !applied {
        return ErrVersionMismatch
    }
    
    return nil
}
```

#### placements_by_cid (Отслеживание размещений)

**Архитектурная роль:**
- Отслеживание желаемых vs фактических размещений пинов
- Co-location с pins_by_cid для эффективных join операций
- Поддержка reconciliation логики

**Реализация схемы:**
```sql
CREATE TABLE IF NOT EXISTS ipfs_pins.placements_by_cid (
    mh_prefix smallint,          -- Тот же ключ партиционирования
    cid_bin blob,                -- Тот же кластерный ключ
    desired set<text>,           -- Список peer_id где пин должен быть
    actual set<text>,            -- Список peer_id где пин подтвержден
    failed set<text>,            -- Список peer_id где пин не удался
    in_progress set<text>,       -- Список peer_id в процессе пиннинга
    updated_at timestamp,        -- Время последнего обновления
    reconcile_count int,         -- Количество попыток согласования
    last_reconcile timestamp,    -- Время последнего согласования
    drift_detected boolean,      -- Флаг расхождения desired != actual
    PRIMARY KEY ((mh_prefix), cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160'
} AND gc_grace_seconds = 864000;
```

**Реализация в Go:**
```go
// state/scyllastate/placements.go
type PinPlacement struct {
    MhPrefix       int16     `cql:"mh_prefix"`
    CidBin         []byte    `cql:"cid_bin"`
    Desired        []string  `cql:"desired"`
    Actual         []string  `cql:"actual"`
    Failed         []string  `cql:"failed"`
    InProgress     []string  `cql:"in_progress"`
    UpdatedAt      time.Time `cql:"updated_at"`
    ReconcileCount int       `cql:"reconcile_count"`
    LastReconcile  time.Time `cql:"last_reconcile"`
    DriftDetected  bool      `cql:"drift_detected"`
}

// Обновление размещений
func (s *ScyllaState) updatePlacements(ctx context.Context, cid api.Cid, desired, actual []string) error {
    mhPrefix := s.getMhPrefix(cid)
    
    // Определение дрейфа
    driftDetected := !stringSlicesEqual(desired, actual)
    
    return s.prepared.updatePlacements.WithContext(ctx).Bind(
        desired,
        actual,
        time.Now(),
        driftDetected,
        mhPrefix,
        cid.Bytes(),
    ).Exec()
}

// Получение пинов с дрейфом для reconciliation
func (s *ScyllaState) getPinsWithDrift(ctx context.Context, limit int) ([]api.Cid, error) {
    var cids []api.Cid
    
    iter := s.session.Query(`
        SELECT cid_bin FROM placements_by_cid 
        WHERE drift_detected = true 
        LIMIT ?
    `, limit).WithContext(ctx).Iter()
    
    var cidBin []byte
    for iter.Scan(&cidBin) {
        cid, err := api.CidFromBytes(cidBin)
        if err != nil {
            continue
        }
        cids = append(cids, cid)
    }
    
    return cids, iter.Close()
}
```

### 3. Index Tables (Индексные таблицы)

#### pins_by_peer (Обратный индекс)

**Архитектурная роль:**
- Эффективные запросы по peer_id для worker agents
- Денормализованные данные для производительности
- Отслеживание состояния пинов на каждом peer

**Реализация схемы:**
```sql
CREATE TABLE IF NOT EXISTS ipfs_pins.pins_by_peer (
    peer_id text,                -- Партиционирование по peer
    mh_prefix smallint,          -- Кластерный ключ
    cid_bin blob,                -- Кластерный ключ
    state tinyint,               -- 0=queued, 1=pinning, 2=pinned, 3=failed
    last_seen timestamp,         -- Последнее обновление статуса
    assigned_at timestamp,       -- Время назначения пина
    completed_at timestamp,      -- Время завершения операции
    error_message text,          -- Сообщение об ошибке если state=failed
    retry_count int,             -- Количество повторных попыток
    pin_size bigint,             -- Размер пина для мониторинга
    PRIMARY KEY ((peer_id), mh_prefix, cid_bin)
) WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160'
} AND caching = {'keys': 'ALL', 'rows_per_partition': '200'};
```

**Реализация в Go:**
```go
// state/scyllastate/peer_pins.go
type PeerPin struct {
    PeerID       string     `cql:"peer_id"`
    MhPrefix     int16      `cql:"mh_prefix"`
    CidBin       []byte     `cql:"cid_bin"`
    State        int8       `cql:"state"`
    LastSeen     time.Time  `cql:"last_seen"`
    AssignedAt   time.Time  `cql:"assigned_at"`
    CompletedAt  *time.Time `cql:"completed_at"`
    ErrorMessage string     `cql:"error_message"`
    RetryCount   int        `cql:"retry_count"`
    PinSize      int64      `cql:"pin_size"`
}

const (
    PinStateQueued  = 0
    PinStatePinning = 1
    PinStatePinned  = 2
    PinStateFailed  = 3
    PinStateUnpinned = 4
)

// Получение пинов для конкретного peer (worker query)
func (s *ScyllaState) getPinsForPeer(ctx context.Context, peerID string, state int8) ([]PeerPin, error) {
    var pins []PeerPin
    
    iter := s.session.Query(`
        SELECT mh_prefix, cid_bin, state, last_seen, assigned_at, 
               completed_at, error_message, retry_count, pin_size
        FROM pins_by_peer 
        WHERE peer_id = ? AND state = ?
    `, peerID, state).WithContext(ctx).Iter()
    
    var pin PeerPin
    pin.PeerID = peerID
    
    for iter.Scan(&pin.MhPrefix, &pin.CidBin, &pin.State, &pin.LastSeen,
                  &pin.AssignedAt, &pin.CompletedAt, &pin.ErrorMessage,
                  &pin.RetryCount, &pin.PinSize) {
        pins = append(pins, pin)
    }
    
    return pins, iter.Close()
}

// Обновление статуса пина на peer
func (s *ScyllaState) updatePeerPinStatus(ctx context.Context, peerID string, cid api.Cid, 
                                         state int8, errorMsg string) error {
    mhPrefix := s.getMhPrefix(cid)
    
    var completedAt *time.Time
    if state == PinStatePinned || state == PinStateFailed {
        now := time.Now()
        completedAt = &now
    }
    
    return s.prepared.updatePeerPin.WithContext(ctx).Bind(
        state,
        time.Now(),
        completedAt,
        errorMsg,
        peerID,
        mhPrefix,
        cid.Bytes(),
    ).Exec()
}
```

### 4. Queue Tables (Очередные таблицы)

#### pin_ttl_queue (Очередь TTL)

**Архитектурная роль:**
- Эффективная обработка TTL с временными окнами
- Автоматическая очистка просроченных пинов
- Оптимизированная компакция для time-series данных

**Реализация схемы:**
```sql
CREATE TABLE IF NOT EXISTS ipfs_pins.pin_ttl_queue (
    ttl_bucket timestamp,        -- Часовой бакет (UTC, обрезанный до часа)
    cid_bin blob,                -- CID для удаления
    owner text,                  -- Владелец пина
    ttl timestamp,               -- Точное время TTL
    PRIMARY KEY ((ttl_bucket), ttl, cid_bin)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'DAYS', 
    'compaction_window_size': '1'
} AND default_time_to_live = 2592000; -- 30 дней
```

**Реализация в Go:**
```go
// state/scyllastate/ttl.go
type TTLEntry struct {
    TTLBucket time.Time `cql:"ttl_bucket"`
    CidBin    []byte    `cql:"cid_bin"`
    Owner     string    `cql:"owner"`
    TTL       time.Time `cql:"ttl"`
}

// Добавление пина в TTL очередь
func (s *ScyllaState) addToTTLQueue(ctx context.Context, cid api.Cid, owner string, ttl time.Time) error {
    // Округление до часа для создания бакета
    ttlBucket := ttl.Truncate(time.Hour)
    
    return s.prepared.insertTTL.WithContext(ctx).Bind(
        ttlBucket,
        cid.Bytes(),
        owner,
        ttl,
    ).Exec()
}

// Получение просроченных пинов для удаления
func (s *ScyllaState) getExpiredPins(ctx context.Context, beforeTime time.Time) ([]api.Cid, error) {
    var expiredCids []api.Cid
    
    // Получение всех бакетов до указанного времени
    buckets := s.getTTLBuckets(beforeTime)
    
    for _, bucket := range buckets {
        iter := s.session.Query(`
            SELECT cid_bin FROM pin_ttl_queue 
            WHERE ttl_bucket = ? AND ttl <= ?
        `, bucket, beforeTime).WithContext(ctx).Iter()
        
        var cidBin []byte
        for iter.Scan(&cidBin) {
            cid, err := api.CidFromBytes(cidBin)
            if err != nil {
                continue
            }
            expiredCids = append(expiredCids, cid)
        }
        
        if err := iter.Close(); err != nil {
            return nil, err
        }
    }
    
    return expiredCids, nil
}

// Генерация списка TTL бакетов
func (s *ScyllaState) getTTLBuckets(beforeTime time.Time) []time.Time {
    var buckets []time.Time
    
    // Начинаем с текущего часа и идем назад
    current := time.Now().Truncate(time.Hour)
    end := beforeTime.Truncate(time.Hour)
    
    for current.After(end) {
        buckets = append(buckets, current)
        current = current.Add(-time.Hour)
    }
    
    return buckets
}
```

### 5. Metadata Tables (Таблицы метаданных)

#### partition_stats (Статистика партиций)

**Архитектурная роль:**
- Мониторинг распределения нагрузки по партициям
- Обнаружение горячих партиций
- Оптимизация производительности

**Реализация схемы:**
```sql
CREATE TABLE IF NOT EXISTS ipfs_pins.partition_stats (
    mh_prefix smallint,          -- Префикс партиции
    stat_type text,              -- Тип статистики
    stat_value bigint,           -- Значение статистики
    updated_at timestamp,        -- Время обновления
    PRIMARY KEY (mh_prefix, stat_type)
) WITH compaction = {'class': 'LeveledCompactionStrategy'}
  AND default_time_to_live = 86400; -- 24 часа
```

**Реализация в Go:**
```go
// state/scyllastate/stats.go
type PartitionStat struct {
    MhPrefix  int16     `cql:"mh_prefix"`
    StatType  string    `cql:"stat_type"`
    StatValue int64     `cql:"stat_value"`
    UpdatedAt time.Time `cql:"updated_at"`
}

const (
    StatTypePinCount        = "pin_count"
    StatTypeTotalSize       = "total_size"
    StatTypeOperationsPerHour = "operations_per_hour"
)

// Обновление статистики партиции
func (s *ScyllaState) updatePartitionStats(ctx context.Context, mhPrefix int16, 
                                          statType string, value int64) error {
    return s.prepared.updatePartitionStats.WithContext(ctx).Bind(
        value,
        time.Now(),
        mhPrefix,
        statType,
    ).Exec()
}

// Получение горячих партиций
func (s *ScyllaState) getHotPartitions(ctx context.Context, threshold int64) ([]int16, error) {
    var hotPartitions []int16
    
    iter := s.session.Query(`
        SELECT mh_prefix FROM partition_stats 
        WHERE stat_type = ? AND stat_value > ?
        ALLOW FILTERING
    `, StatTypeOperationsPerHour, threshold).WithContext(ctx).Iter()
    
    var mhPrefix int16
    for iter.Scan(&mhPrefix) {
        hotPartitions = append(hotPartitions, mhPrefix)
    }
    
    return hotPartitions, iter.Close()
}

// Периодическое обновление статистики
func (s *ScyllaState) updateStatsBackground(ctx context.Context) {
    ticker := time.NewTicker(time.Hour)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.calculatePartitionStats(ctx)
        }
    }
}

func (s *ScyllaState) calculatePartitionStats(ctx context.Context) error {
    // Подсчет пинов по партициям
    iter := s.session.Query(`
        SELECT mh_prefix, COUNT(*) as pin_count, SUM(size) as total_size
        FROM pins_by_cid 
        GROUP BY mh_prefix
    `).WithContext(ctx).Iter()
    
    var mhPrefix int16
    var pinCount, totalSize int64
    
    for iter.Scan(&mhPrefix, &pinCount, &totalSize) {
        // Обновление статистики
        s.updatePartitionStats(ctx, mhPrefix, StatTypePinCount, pinCount)
        s.updatePartitionStats(ctx, mhPrefix, StatTypeTotalSize, totalSize)
    }
    
    return iter.Close()
}
```

### 6. Audit Tables (Таблицы аудита)

#### pin_events (События пинов)

**Архитектурная роль:**
- Аудит всех операций с пинами
- Отладка и мониторинг системы
- Автоматическая очистка старых событий

**Реализация схемы:**
```sql
CREATE TABLE IF NOT EXISTS ipfs_pins.pin_events (
    mh_prefix smallint,          -- Партиционирование как в pins_by_cid
    cid_bin blob,                -- CID события
    ts timeuuid,                 -- Временная метка с уникальностью
    peer_id text,                -- Peer, сгенерировавший событие
    event_type text,             -- Тип события
    details text,                -- JSON детали или сообщение об ошибке
    PRIMARY KEY ((mh_prefix, cid_bin), ts)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'DAYS',
    'compaction_window_size': '7'
} AND default_time_to_live = 2592000; -- 30 дней
```

**Реализация в Go:**
```go
// state/scyllastate/events.go
type PinEvent struct {
    MhPrefix  int16     `cql:"mh_prefix"`
    CidBin    []byte    `cql:"cid_bin"`
    Timestamp gocql.UUID `cql:"ts"`
    PeerID    string    `cql:"peer_id"`
    EventType string    `cql:"event_type"`
    Details   string    `cql:"details"`
}

const (
    EventTypePinned   = "pinned"
    EventTypeUnpinned = "unpinned"
    EventTypeFailed   = "failed"
    EventTypeAssigned = "assigned"
    EventTypeUpdated  = "updated"
)

// Запись события
func (s *ScyllaState) recordEvent(ctx context.Context, cid api.Cid, peerID, eventType, details string) error {
    mhPrefix := s.getMhPrefix(cid)
    
    return s.prepared.insertEvent.WithContext(ctx).Bind(
        mhPrefix,
        cid.Bytes(),
        gocql.TimeUUID(),
        peerID,
        eventType,
        details,
    ).Exec()
}

// Получение истории событий для пина
func (s *ScyllaState) getPinEvents(ctx context.Context, cid api.Cid, limit int) ([]PinEvent, error) {
    mhPrefix := s.getMhPrefix(cid)
    var events []PinEvent
    
    iter := s.session.Query(`
        SELECT ts, peer_id, event_type, details
        FROM pin_events 
        WHERE mh_prefix = ? AND cid_bin = ?
        ORDER BY ts DESC
        LIMIT ?
    `, mhPrefix, cid.Bytes(), limit).WithContext(ctx).Iter()
    
    var event PinEvent
    event.MhPrefix = mhPrefix
    event.CidBin = cid.Bytes()
    
    for iter.Scan(&event.Timestamp, &event.PeerID, &event.EventType, &event.Details) {
        events = append(events, event)
    }
    
    return events, iter.Close()
}
```

## Ключевые архитектурные решения

### 1. Партиционирование по mh_prefix

**Преимущества:**
- 65,536 партиций для равномерного распределения
- Предсказуемое распределение нагрузки
- Эффективные range queries

**Реализация:**
```go
func (s *ScyllaState) getMhPrefix(cid api.Cid) int16 {
    cidBytes := cid.Bytes()
    if len(cidBytes) < 2 {
        return 0
    }
    // Первые 2 байта multihash дают равномерное распределение
    return int16(cidBytes[0])<<8 | int16(cidBytes[1])
}
```

### 2. Co-location стратегия

**pins_by_cid** и **placements_by_cid** используют одинаковое партиционирование:
- Данные хранятся на одних узлах
- Эффективные join операции
- Минимальная сетевая задержка

### 3. Time-series оптимизации

**TTL Queue** и **Events** используют TimeWindowCompactionStrategy:
- Автоматическая очистка старых данных
- Оптимизированная компакция для временных данных
- Эффективное использование дискового пространства

### 4. Денормализация для производительности

**pins_by_peer** дублирует данные для эффективных peer queries:
- Быстрые запросы worker agents
- Независимые операции по peer
- Масштабируемость worker нагрузки

## Мониторинг и оптимизация

### 1. Partition Statistics
```go
// Мониторинг горячих партиций
func (s *ScyllaState) monitorHotPartitions(ctx context.Context) {
    hotPartitions, err := s.getHotPartitions(ctx, 1000) // threshold
    if err != nil {
        return
    }
    
    for _, partition := range hotPartitions {
        s.metrics.RecordHotPartition(partition)
        // Можно реализовать балансировку нагрузки
    }
}
```

### 2. Performance Metrics
```go
// Метрики производительности таблиц
func (s *ScyllaState) recordTableMetrics(table string, operation string, duration time.Duration) {
    s.metrics.TableOperationDuration.WithLabelValues(table, operation).Observe(duration.Seconds())
}
```

## Выводы

Database Schema диаграмма показывает оптимизированную архитектуру для trillion-scale:

✅ **Масштабируемость** - 65,536 партиций для равномерного распределения  
✅ **Производительность** - LeveledCompactionStrategy и оптимизированное кэширование  
✅ **Консистентность** - Optimistic locking через version поля  
✅ **Наблюдаемость** - Audit trails и статистика партиций  
✅ **Автоматизация** - TTL-based cleanup и time-window компакция  

Схема обеспечивает высокую производительность при сохранении гибкости и надежности системы.