# Task 3.1: Определить схему таблиц ScyllaDB - Полный обзор

## Описание задачи

**Task 3.1**: Определить схему таблиц ScyllaDB
- Создать CQL скрипты для keyspace и таблиц pins_by_cid, placements_by_cid, pins_by_peer
- Реализовать оптимизированное партиционирование по mh_prefix
- Добавить настройки компакции и TTL для оптимальной производительности
- **Требования**: 2.1, 2.2, 7.3

## Созданные файлы

### 1. Основной файл схемы: `state/scyllastate/schema.sql`

**Назначение**: Главный файл с определением всей схемы базы данных ScyllaDB

**Содержимое**:
- **Keyspace**: `ipfs_pins` с NetworkTopologyStrategy
- **10 оптимизированных таблиц** для trillion-scale нагрузки
- **Партиционирование по mh_prefix** (первые 2 байта multihash)
- **Настройки производительности**: компакция, кэширование, сжатие

**Ключевые таблицы**:

#### 1.1 `pins_by_cid` - Основная таблица метаданных пинов
```sql
CREATE TABLE ipfs_pins.pins_by_cid (
    mh_prefix smallint,          -- Партиционирование (0..65535)
    cid_bin blob,                -- Бинарный CID
    pin_type tinyint,            -- Тип пина (direct/recursive/indirect)
    rf tinyint,                  -- Фактор репликации
    owner text,                  -- Владелец (multi-tenancy)
    tags set<text>,              -- Теги для категоризации
    ttl timestamp,               -- TTL для автоудаления
    metadata map<text, text>,    -- Дополнительные метаданные
    created_at timestamp,        -- Время создания
    updated_at timestamp,        -- Время обновления
    size bigint,                 -- Размер в байтах
    status tinyint,              -- Статус пина
    version int,                 -- Версия для optimistic locking
    checksum text,               -- Контрольная сумма
    priority tinyint,            -- Приоритет пина
    PRIMARY KEY ((mh_prefix), cid_bin)
);
```

#### 1.2 `placements_by_cid` - Отслеживание размещения пинов
```sql
CREATE TABLE ipfs_pins.placements_by_cid (
    mh_prefix smallint,          -- Тот же ключ партиционирования
    cid_bin blob,                -- Бинарный CID
    desired set<text>,           -- Желаемые peer_id
    actual set<text>,            -- Фактические peer_id
    failed set<text>,            -- Неудачные peer_id
    in_progress set<text>,       -- Peer_id в процессе
    updated_at timestamp,        -- Время обновления
    reconcile_count int,         -- Количество попыток согласования
    last_reconcile timestamp,    -- Последнее согласование
    drift_detected boolean,      -- Флаг расхождения desired != actual
    PRIMARY KEY ((mh_prefix), cid_bin)
);
```

#### 1.3 `pins_by_peer` - Обратный индекс для запросов по пирам
```sql
CREATE TABLE ipfs_pins.pins_by_peer (
    peer_id text,                -- Партиционирование по peer
    mh_prefix smallint,
    cid_bin blob,
    state tinyint,               -- Состояние (queued/pinning/pinned/failed)
    last_seen timestamp,         -- Последнее обновление статуса
    assigned_at timestamp,       -- Время назначения
    completed_at timestamp,      -- Время завершения
    error_message text,          -- Сообщение об ошибке
    retry_count int,             -- Количество повторов
    pin_size bigint,             -- Размер пина
    PRIMARY KEY ((peer_id), mh_prefix, cid_bin)
);
```

#### 1.4 Дополнительные таблицы
- `pin_ttl_queue` - Очередь TTL с временными окнами
- `op_dedup` - Дедупликация операций для идемпотентности
- `pin_stats` - Статистика и счетчики
- `pin_events` - Лог событий для аудита
- `partition_stats` - Статистика партиций для балансировки
- `performance_metrics` - Метрики производительности
- `batch_operations` - Отслеживание пакетных операций

### 2. Файл миграций: `state/scyllastate/migrations.sql`

**Назначение**: SQL-скрипты для миграций между версиями схемы

**Содержимое**:
- Миграция с версии 1.0.0 до 1.1.0
- Добавление новых колонок в существующие таблицы
- Создание новых таблиц для расширенной функциональности
- Обновление настроек производительности

**Ключевые изменения в v1.1.0**:
```sql
-- Добавление новых колонок в pins_by_cid
ALTER TABLE ipfs_pins.pins_by_cid ADD version int;
ALTER TABLE ipfs_pins.pins_by_cid ADD checksum text;
ALTER TABLE ipfs_pins.pins_by_cid ADD priority tinyint;

-- Добавление колонок в placements_by_cid
ALTER TABLE ipfs_pins.placements_by_cid ADD in_progress set<text>;
ALTER TABLE ipfs_pins.placements_by_cid ADD last_reconcile timestamp;

-- Создание новых таблиц для мониторинга
CREATE TABLE ipfs_pins.partition_stats (...);
CREATE TABLE ipfs_pins.performance_metrics (...);
CREATE TABLE ipfs_pins.batch_operations (...);
```

## Архитектурные решения

### 3.1 Партиционирование по mh_prefix

**Стратегия**: Использование первых 2 байтов multihash digest как ключа партиционирования

**Преимущества**:
- **65,536 партиций** для равномерного распределен��я
- **Предсказуемое распределение** данных по узлам кластера
- **Оптимальная производительность** для trillion-scale данных
- **Co-location** связанных данных (pins_by_cid и placements_by_cid)

**Реализация**:
```sql
-- Извлечение mh_prefix из CID
mh_prefix = first_2_bytes(multihash_digest)
-- Диапазон: 0..65535 (smallint)
```

### 3.2 Настройки компакции и производительности

**LeveledCompactionStrategy**:
```sql
WITH compaction = {
    'class': 'LeveledCompactionStrategy',
    'sstable_size_in_mb': '160',
    'tombstone_threshold': '0.2'
}
```

**Оптимизация кэширования**:
```sql
AND caching = {'keys': 'ALL', 'rows_per_partition': '1000'}
```

**Сжатие данных**:
```sql
AND compression = {'class': 'LZ4Compressor'}
```

**Read repair для консистентности**:
```sql
AND read_repair_chance = 0.1
AND dclocal_read_repair_chance = 0.1
```

### 3.3 TTL и автоматическая очистка

**TTL Queue с временными окнами**:
```sql
CREATE TABLE ipfs_pins.pin_ttl_queue (
    ttl_bucket timestamp,        -- Часовые корзины
    cid_bin blob,
    ttl timestamp,               -- Точное время TTL
    PRIMARY KEY ((ttl_bucket), ttl, cid_bin)
) WITH compaction = {
    'class': 'TimeWindowCompactionStrategy',
    'compaction_window_unit': 'DAYS',
    'compaction_window_size': '1'
} AND default_time_to_live = 2592000;  -- 30 дней
```

**Автоматическая очистка событий**:
```sql
CREATE TABLE ipfs_pins.pin_events (
    ...
) WITH default_time_to_live = 2592000  -- 30 дней
```

## Соответствие требованиям

### Требование 2.1: Store pin metadata
✅ **Выполнено**:
- Таблица `pins_by_cid` хранит все метаданные пинов
- CID, фактор репликации, временные метки, размещения
- Дополнительные поля: owner, tags, metadata, size, status

### Требование 2.2: Support consistent read semantics and conditional updates
✅ **Выполнено**:
- Поле `version` для optimistic locking
- Read repair настройки для консистентности
- Conditional updates через CAS операции
- Отслеживание `updated_at` для разрешения конфликтов

### Требование 7.3: Implement caching strategies for predictable query patterns
✅ **Выполнено**:
- Оптимизированное кэширование ключей и строк
- Bloom filter настройки для быстрого поиска
- Партиционирование для предсказуемых паттернов доступа
- Денормализованные таблицы для эффективных запросов

## Производительность и масштабируемость

### Оптимизации для trillion-scale:

1. **Партиционирование**: 65,536 партиций для равномерного распределения
2. **Компакция**: LeveledCompactionStrategy для высокой нагрузки записи
3. **Кэширование**: Агрессивное кэширование часто используемых данных
4. **Сжатие**: LZ4 для быстрого сжатия/распаковки
5. **TTL**: Автоматическая очистка устаревших данных
6. **Денормализация**: Отдельные таблицы для разных паттернов запросов

### Мониторинг производительности:

- `partition_stats` - статистика по партициям
- `performance_metrics` - метрики производительности
- `pin_events` - лог событий для анализа

## Multi-DC поддержка

**NetworkTopologyStrategy**:
```sql
CREATE KEYSPACE ipfs_pins
WITH replication = {
    'class': 'NetworkTopologyStrategy', 
    'datacenter1': '3'
} AND durable_writes = true;
```

**Настройки для multi-DC**:
- Local read repair для минимизации cross-DC трафика
- Настраиваемая репликация по датацентрам
- Оптимизация для cross-DC латентности

## Заключение

Task 3.1 успешно выполнен с созданием:

✅ **Полной схемы ScyllaDB** с 10 оптимизированными таблицами
✅ **Партиционирования по mh_prefix** для trillion-scale производительности  
✅ **Настроек компакции и TTL** для оптимальной производительности
✅ **Поддержки всех требований** 2.1, 2.2, 7.3
✅ **Multi-DC готовности** с NetworkTopologyStrategy
✅ **Системы миграций** для управления версиями схемы

Схема готова для использования в production среде с trillion-scale нагрузкой.