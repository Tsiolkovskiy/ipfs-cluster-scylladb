# Migration System Flow - Подробное объяснение

## Обзор диаграммы

**Файл:** `Migration_System_Flow.puml`  
**Цель:** Показать полный поток выполнения системы миграций от запуска приложения до готовности схемы  
**Фокус:** Автоматические миграции, валидация, обработка ошибок

## Архитектурный дизайн → Реализация кода

### 1. Application Startup (Запуск приложения)

**Архитектурная роль:**
- Точка входа в систему миграций
- Инициализация всех компонентов
- Обеспечение готовности схемы перед началом работы

**Реализация в коде:**
```go
// state/scyllastate/scyllastate.go
func New(ctx context.Context, cfg *Config) (*ScyllaState, error) {
    // Валидация конфигурации
    if err := cfg.Validate(); err != nil {
        return nil, fmt.Errorf("invalid config: %w", err)
    }
    
    // Создание подключения к ScyllaDB
    session, err := createSession(cfg)
    if err != nil {
        return nil, fmt.Errorf("failed to create session: %w", err)
    }
    
    // Инициализация менеджера миграций
    migrator := NewMigrationManager(session, cfg.Keyspace)
    
    // Автоматическое применение миграций
    if err := migrator.AutoMigrate(ctx); err != nil {
        session.Close()
        return nil, fmt.Errorf("migration failed: %w", err)
    }
    
    // Создание ScyllaState после успешной миграции
    state := &ScyllaState{
        session:  session,
        keyspace: cfg.Keyspace,
        config:   cfg,
        migrator: migrator,
    }
    
    // Подготовка запросов
    if err := state.prepareStatements(); err != nil {
        session.Close()
        return nil, fmt.Errorf("failed to prepare statements: %w", err)
    }
    
    return state, nil
}
```

### 2. Initialize MigrationManager (Инициализация менеджера миграций)

**Архитектурная роль:**
- Создание центрального координатора миграций
- Регистрация всех доступных миграций
- Настройка политик выполнения

**Реализация в коде:**
```go
// state/scyllastate/migrations.go
func NewMigrationManager(session *gocql.Session, keyspace string) *MigrationManager {
    mm := &MigrationManager{
        session:  session,
        keyspace: keyspace,
    }
    
    // Регистрация всех миграций
    mm.registerMigrations()
    
    return mm
}

func (mm *MigrationManager) registerMigrations() {
    mm.migrations = []Migration{
        {
            Version:     "1.0.0",
            Description: "Initial schema creation",
            UpScript:    getInitialSchemaScript(),
            DownScript:  "", // Rollback не поддерживается для начальной схемы
            Validate:    mm.validateInitialSchema,
        },
        {
            Version:     "1.1.0",
            Description: "Add enhanced pin metadata and performance tables",
            UpScript:    getV110MigrationScript(),
            DownScript:  getRollbackV110Script(),
            Validate:    mm.validateV110Schema,
        },
        // Будущие миграции добавляются здесь
    }
}
```

### 3. Check Schema Version Table (Проверка таблицы версий)

**Архитектурная роль:**
- Определение текущего состояния схемы
- Создание инфраструктуры версионирования при первом запуске
- Обеспечение отслеживания истории миграций

**Реализация в коде:**
```go
// state/scyllastate/migrations.go
func (mm *MigrationManager) InitializeSchema(ctx context.Context) error {
    // Создание таблицы версий схемы
    createVersionTable := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS %s.%s (
            version text PRIMARY KEY,
            applied_at timestamp,
            comment text
        ) WITH comment = 'Schema version tracking for migrations'`,
        mm.keyspace, SchemaVersionTable)
    
    if err := mm.session.Query(createVersionTable).WithContext(ctx).Exec(); err != nil {
        return fmt.Errorf("failed to create schema version table: %w", err)
    }
    
    return nil
}

func (mm *MigrationManager) GetCurrentVersion(ctx context.Context) (*SchemaVersion, error) {
    var version string
    var appliedAt time.Time
    var comment string
    
    query := fmt.Sprintf(`
        SELECT version, applied_at, comment 
        FROM %s.%s 
        ORDER BY version DESC 
        LIMIT 1`,
        mm.keyspace, SchemaVersionTable)
    
    iter := mm.session.Query(query).WithContext(ctx).Iter()
    defer iter.Close()
    
    if iter.Scan(&version, &appliedAt, &comment) {
        return &SchemaVersion{
            Version:   version,
            AppliedAt: appliedAt,
            Comment:   comment,
        }, nil
    }
    
    if err := iter.Close(); err != nil {
        return nil, fmt.Errorf("failed to query schema version: %w", err)
    }
    
    // Версия не найдена - свежая установка
    return nil, nil
}
```

### 4. Version Compatibility Check (Проверка совместимости версий)

**Архитектурная роль:**
- Предотвращение несовместимых миграций
- Защита от downgrade операций
- Обеспечение безопасности обновлений

**Реализация в коде:**
```go
// state/scyllastate/migrations.go
func (mm *MigrationManager) CheckCompatibility(ctx context.Context) error {
    currentVersion, err := mm.GetCurrentVersion(ctx)
    if err != nil {
        return fmt.Errorf("failed to get current schema version: %w", err)
    }
    
    if currentVersion == nil {
        // Свежая установка - проблем совместимости нет
        return nil
    }
    
    // Проверка минимальной поддерживаемой версии
    if !mm.isVersionSupported(currentVersion.Version) {
        return fmt.Errorf("schema version %s is not supported (minimum: %s, current: %s)",
            currentVersion.Version, MinSupportedVersion, CurrentSchemaVersion)
    }
    
    // Проверка на попытку downgrade
    if mm.compareVersions(currentVersion.Version, CurrentSchemaVersion) > 0 {
        return fmt.Errorf("schema version %s is newer than supported version %s - please upgrade the application",
            currentVersion.Version, CurrentSchemaVersion)
    }
    
    return nil
}

func (mm *MigrationManager) isVersionSupported(version string) bool {
    return mm.compareVersions(version, MinSupportedVersion) >= 0
}

func (mm *MigrationManager) compareVersions(v1, v2 string) int {
    parts1 := strings.Split(v1, ".")
    parts2 := strings.Split(v2, ".")
    
    // Дополнение нулями для одинаковой длины
    maxLen := len(parts1)
    if len(parts2) > maxLen {
        maxLen = len(parts2)
    }
    
    for len(parts1) < maxLen {
        parts1 = append(parts1, "0")
    }
    for len(parts2) < maxLen {
        parts2 = append(parts2, "0")
    }
    
    // Сравнение каждой части
    for i := 0; i < maxLen; i++ {
        num1, err1 := strconv.Atoi(parts1[i])
        num2, err2 := strconv.Atoi(parts2[i])
        
        if err1 != nil || err2 != nil {
            // Строковое сравнение если не числа
            if parts1[i] < parts2[i] {
                return -1
            } else if parts1[i] > parts2[i] {
                return 1
            }
            continue
        }
        
        if num1 < num2 {
            return -1
        } else if num1 > num2 {
            return 1
        }
    }
    
    return 0
}
```

### 5. Get Pending Migrations (Получение необходимых миграций)

**Архитектурная роль:**
- Определение списка миграций для применения
- Сортировка миграций в правильном порядке
- Оптимизация процесса обновления

**Реализация в коде:**
```go
// state/scyllastate/migrations.go
func (mm *MigrationManager) getPendingMigrations(currentVersion string) []Migration {
    var pending []Migration
    
    for _, migration := range mm.migrations {
        if mm.compareVersions(migration.Version, currentVersion) > 0 {
            pending = append(pending, migration)
        }
    }
    
    // Сортировка по версиям для правильного порядка применения
    sort.Slice(pending, func(i, j int) bool {
        return mm.compareVersions(pending[i].Version, pending[j].Version) < 0
    })
    
    return pending
}

func (mm *MigrationManager) ApplyMigrations(ctx context.Context) error {
    currentVersion, err := mm.GetCurrentVersion(ctx)
    if err != nil {
        return fmt.Errorf("failed to get current version: %w", err)
    }
    
    var startVersion string
    if currentVersion == nil {
        startVersion = "0.0.0" // Свежая установка
    } else {
        startVersion = currentVersion.Version
    }
    
    // Получение списка необходимых миграций
    pendingMigrations := mm.getPendingMigrations(startVersion)
    
    if len(pendingMigrations) == 0 {
        // Уже на текущей версии
        return nil
    }
    
    // Применение миграций по порядку
    for _, migration := range pendingMigrations {
        if err := mm.applyMigration(ctx, migration); err != nil {
            return fmt.Errorf("failed to apply migration %s: %w", migration.Version, err)
        }
    }
    
    return nil
}
```

### 6. Parse SQL Statements (Парсинг SQL скриптов)

**Архитектурная роль:**
- Разбор сложных SQL скриптов на отдельные команды
- Обработка строк с точками с запятой
- Фильтрация пустых команд

**Реализация в коде:**
```go
// state/scyllastate/migrations.go
func (mm *MigrationManager) parseStatements(script string) []string {
    statements := []string{}
    current := ""
    inString := false
    
    for i, char := range script {
        if char == '\'' && (i == 0 || script[i-1] != '\\') {
            inString = !inString
        }
        
        if char == ';' && !inString {
            statements = append(statements, strings.TrimSpace(current))
            current = ""
        } else {
            current += string(char)
        }
    }
    
    // Добавление финального statement если не заканчивается на ;
    if strings.TrimSpace(current) != "" {
        statements = append(statements, strings.TrimSpace(current))
    }
    
    // Фильтрация пустых statements
    var filtered []string
    for _, stmt := range statements {
        if stmt != "" && !strings.HasPrefix(strings.TrimSpace(stmt), "--") {
            filtered = append(filtered, stmt)
        }
    }
    
    return filtered
}
```

### 7. Execute Migration Script (Выполнение скрипта миграции)

**Архитектурная роль:**
- Выполнение SQL команд в правильном порядке
- Обработка ошибок и retry логика
- Обеспечение атомарности миграций

**Реализация в коде:**
```go
// state/scyllastate/migrations.go
func (mm *MigrationManager) applyMigration(ctx context.Context, migration Migration) error {
    // Парсинг SQL скрипта
    statements := mm.parseStatements(migration.UpScript)
    
    // Выполнение каждого statement
    for _, stmt := range statements {
        if strings.TrimSpace(stmt) == "" {
            continue
        }
        
        if err := mm.executeStatement(ctx, stmt); err != nil {
            if !mm.isIgnorableError(err) {
                return fmt.Errorf("failed to execute statement '%s': %w", stmt, err)
            }
            // Логирование предупреждения для игнорируемых ошибок
            logger.Warnf("Ignoring migration error: %v", err)
        }
    }
    
    // Валидация миграции
    if migration.Validate != nil {
        if err := migration.Validate(mm.session); err != nil {
            return fmt.Errorf("migration validation failed: %w", err)
        }
    }
    
    // Запись успешной миграции
    if err := mm.SetVersion(ctx, migration.Version, migration.Description); err != nil {
        return fmt.Errorf("failed to record migration: %w", err)
    }
    
    return nil
}

func (mm *MigrationManager) executeStatement(ctx context.Context, statement string) error {
    // Retry логика для временных ошибок
    maxRetries := 3
    baseDelay := 100 * time.Millisecond
    
    for attempt := 0; attempt <= maxRetries; attempt++ {
        if attempt > 0 {
            delay := time.Duration(attempt) * baseDelay
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(delay):
            }
        }
        
        err := mm.session.Query(statement).WithContext(ctx).Exec()
        if err == nil {
            return nil
        }
        
        // Проверка на повторяемые ошибки
        if !mm.isRetryableError(err) {
            return err
        }
        
        if attempt == maxRetries {
            return fmt.Errorf("statement failed after %d retries: %w", maxRetries, err)
        }
    }
    
    return nil
}
```

### 8. Error Handling (Обработка ошибок)

**Архитектурная роль:**
- Различение критических и игнорируемых ошибок
- Обеспечение идемпотентности миграций
- Предотвращение прерывания процесса из-за ожидаемых ошибок

**Реализация в коде:**
```go
// state/scyllastate/migrations.go
func (mm *MigrationManager) isIgnorableError(err error) bool {
    errStr := strings.ToLower(err.Error())
    
    // Список игнорируемых ошибок для идемпотентности
    ignorablePatterns := []string{
        "already exists",
        "duplicate column name",
        "column already exists",
        "table already exists",
        "keyspace already exists",
        "index already exists",
    }
    
    for _, pattern := range ignorablePatterns {
        if strings.Contains(errStr, pattern) {
            return true
        }
    }
    
    return false
}

func (mm *MigrationManager) isRetryableError(err error) bool {
    errStr := strings.ToLower(err.Error())
    
    // Ошибки, которые можно повторить
    retryablePatterns := []string{
        "timeout",
        "connection refused",
        "no hosts available",
        "unavailable",
    }
    
    for _, pattern := range retryablePatterns {
        if strings.Contains(errStr, pattern) {
            return true
        }
    }
    
    return false
}
```

### 9. Migration Validation (Валидация миграции)

**Архитектурная роль:**
- Проверка корректности применения миграции
- Валидация структуры схемы
- Обеспечение целостности данных

**Реализация в коде:**
```go
// state/scyllastate/migrations.go
func (mm *MigrationManager) validateInitialSchema(session *gocql.Session) error {
    // Проверка основных таблиц
    requiredTables := []string{"pins_by_cid", "placements_by_cid", "pins_by_peer", "pin_ttl_queue"}
    
    for _, table := range requiredTables {
        query := `SELECT table_name FROM system_schema.tables WHERE keyspace_name = ? AND table_name = ?`
        var foundTable string
        if err := session.Query(query, mm.keyspace, table).Scan(&foundTable); err != nil {
            return fmt.Errorf("required table %s missing in initial schema", table)
        }
    }
    
    return nil
}

func (mm *MigrationManager) validateV110Schema(session *gocql.Session) error {
    // Проверка новых колонок в pins_by_cid
    expectedColumns := []string{"version", "checksum", "priority"}
    
    for _, column := range expectedColumns {
        query := `SELECT column_name FROM system_schema.columns WHERE keyspace_name = ? AND table_name = ? AND column_name = ?`
        var foundColumn string
        if err := session.Query(query, mm.keyspace, "pins_by_cid", column).Scan(&foundColumn); err != nil {
            return fmt.Errorf("column %s missing in pins_by_cid table", column)
        }
    }
    
    // Проверка новых таблиц
    newTables := []string{"partition_stats", "performance_metrics", "batch_operations"}
    
    for _, table := range newTables {
        query := `SELECT table_name FROM system_schema.tables WHERE keyspace_name = ? AND table_name = ?`
        var foundTable string
        if err := session.Query(query, mm.keyspace, table).Scan(&foundTable); err != nil {
            return fmt.Errorf("new table %s missing in v1.1.0 schema", table)
        }
    }
    
    return nil
}
```

### 10. Record Migration Version (Запись версии миграции)

**Архитектурная роль:**
- Сохранение информации о примененной миграции
- Создание истории изменений схемы
- Обеспечение отслеживания прогресса

**Реализация в коде:**
```go
// state/scyllastate/migrations.go
func (mm *MigrationManager) SetVersion(ctx context.Context, version, comment string) error {
    query := fmt.Sprintf(`
        INSERT INTO %s.%s (version, applied_at, comment) 
        VALUES (?, ?, ?)`,
        mm.keyspace, SchemaVersionTable)
    
    if err := mm.session.Query(query, version, time.Now(), comment).WithContext(ctx).Exec(); err != nil {
        return fmt.Errorf("failed to set schema version: %w", err)
    }
    
    return nil
}

func (mm *MigrationManager) GetMigrationHistory(ctx context.Context) ([]SchemaVersion, error) {
    query := fmt.Sprintf(`
        SELECT version, applied_at, comment 
        FROM %s.%s`,
        mm.keyspace, SchemaVersionTable)
    
    iter := mm.session.Query(query).WithContext(ctx).Iter()
    defer iter.Close()
    
    var history []SchemaVersion
    var version string
    var appliedAt time.Time
    var comment string
    
    for iter.Scan(&version, &appliedAt, &comment) {
        history = append(history, SchemaVersion{
            Version:   version,
            AppliedAt: appliedAt,
            Comment:   comment,
        })
    }
    
    if err := iter.Close(); err != nil {
        return nil, fmt.Errorf("failed to get migration history: %w", err)
    }
    
    // Сортировка по версиям
    sort.Slice(history, func(i, j int) bool {
        return mm.compareVersions(history[i].Version, history[j].Version) < 0
    })
    
    return history, nil
}
```

## CLI Integration (Интеграция с CLI)

**Архитектурная роль:**
- Предоставление инструментов для ручного управления миграциями
- Диагностика и отладка проблем миграций
- Административные операции

**Реализация в коде:**
```go
// state/scyllastate/migration_cli.go
func (cli *MigrationCLI) migrateCommand(args []string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()
    
    fmt.Printf("Applying migrations to keyspace '%s'...\n", cli.keyspace)
    
    config := DefaultMigrationConfig()
    result, err := EnsureSchemaReady(ctx, cli.session, cli.keyspace, config)
    if err != nil {
        return fmt.Errorf("migration failed: %w", err)
    }
    
    if result.Success {
        fmt.Printf("✓ Migration completed successfully\n")
        fmt.Printf("  Previous Version: %s\n", result.PreviousVersion)
        fmt.Printf("  Current Version:  %s\n", result.CurrentVersion)
        fmt.Printf("  Duration: %v\n", result.Duration)
        
        if len(result.MigrationsApplied) > 0 {
            fmt.Printf("  Applied Migrations:\n")
            for _, migration := range result.MigrationsApplied {
                fmt.Printf("    - %s\n", migration)
            }
        }
    }
    
    return nil
}
```

## Мониторинг и метрики

**Архитектурная роль:**
- Отслеживание производительности миграций
- Мониторинг успешности операций
- Алертинг при проблемах

**Реализация в коде:**
```go
// state/scyllastate/migration_metrics.go
type MigrationMetrics struct {
    migrationDuration *prometheus.HistogramVec
    migrationSuccess  *prometheus.CounterVec
    migrationFailures *prometheus.CounterVec
}

func (mm *MigrationManager) recordMigrationMetrics(version string, duration time.Duration, err error) {
    labels := prometheus.Labels{"version": version}
    
    if err != nil {
        labels["status"] = "failed"
        mm.metrics.migrationFailures.With(labels).Inc()
    } else {
        labels["status"] = "success"
        mm.metrics.migrationSuccess.With(labels).Inc()
        mm.metrics.migrationDuration.With(labels).Observe(duration.Seconds())
    }
}
```

## Выводы

Migration System Flow диаграмма показывает надежный процесс управления схемой:

✅ **Автоматизация** - полностью автоматические миграции при запуске  
✅ **Безопасность** - проверки совместимости и валидация  
✅ **Идемпотентность** - безопасное повторное выполнение  
✅ **Наблюдаемость** - полная история миграций и метрики  
✅ **Восстановление** - обработка ошибок и retry логика  

Система обеспечивает плавное обновление схемы без простоя и с минимальными рисками.