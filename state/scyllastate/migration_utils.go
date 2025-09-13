package scyllastate

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"
)

// MigrationConfig holds configuration for migration operations
type MigrationConfig struct {
	// AutoMigrate enables automatic migration at startup
	AutoMigrate bool `json:"auto_migrate"`

	// MigrationTimeout sets the timeout for migration operations
	MigrationTimeout time.Duration `json:"migration_timeout"`

	// ValidateSchema enables schema validation after migration
	ValidateSchema bool `json:"validate_schema"`

	// AllowDowngrade allows downgrading to older schema versions (dangerous)
	AllowDowngrade bool `json:"allow_downgrade"`

	// BackupBeforeMigration creates a backup before applying migrations
	BackupBeforeMigration bool `json:"backup_before_migration"`
}

// DefaultMigrationConfig returns default migration configuration
func DefaultMigrationConfig() *MigrationConfig {
	return &MigrationConfig{
		AutoMigrate:           true,
		MigrationTimeout:      5 * time.Minute,
		ValidateSchema:        true,
		AllowDowngrade:        false,
		BackupBeforeMigration: false, // Backup is typically handled externally
	}
}

// MigrationResult contains the result of a migration operation
type MigrationResult struct {
	Success           bool              `json:"success"`
	PreviousVersion   string            `json:"previous_version,omitempty"`
	CurrentVersion    string            `json:"current_version"`
	MigrationsApplied []string          `json:"migrations_applied"`
	Duration          time.Duration     `json:"duration"`
	Error             string            `json:"error,omitempty"`
	ValidationResult  *ValidationResult `json:"validation_result,omitempty"`
}

// ValidationResult contains schema validation results
type ValidationResult struct {
	Valid          bool     `json:"valid"`
	MissingTables  []string `json:"missing_tables,omitempty"`
	MissingColumns []string `json:"missing_columns,omitempty"`
	Issues         []string `json:"issues,omitempty"`
}

// MigrationStatus represents the current migration status
type MigrationStatus struct {
	CurrentVersion    string    `json:"current_version"`
	TargetVersion     string    `json:"target_version"`
	MigrationsNeeded  bool      `json:"migrations_needed"`
	PendingMigrations []string  `json:"pending_migrations"`
	LastMigration     time.Time `json:"last_migration,omitempty"`
	SchemaValid       bool      `json:"schema_valid"`
}

// EnsureSchemaReady ensures the schema is ready for use, applying migrations if needed
func EnsureSchemaReady(ctx context.Context, session *gocql.Session, keyspace string, config *MigrationConfig) (*MigrationResult, error) {
	if config == nil {
		config = DefaultMigrationConfig()
	}

	// Create migration manager
	mm := NewMigrationManager(session, keyspace)

	// Set timeout context
	migrationCtx := ctx
	if config.MigrationTimeout > 0 {
		var cancel context.CancelFunc
		migrationCtx, cancel = context.WithTimeout(ctx, config.MigrationTimeout)
		defer cancel()
	}

	startTime := time.Now()
	result := &MigrationResult{
		MigrationsApplied: []string{},
	}

	// Get current version
	currentVersion, err := mm.GetCurrentVersion(migrationCtx)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get current version: %v", err)
		return result, err
	}

	if currentVersion != nil {
		result.PreviousVersion = currentVersion.Version
	}

	// Check if migrations are needed
	if config.AutoMigrate {
		// Apply migrations
		if err := mm.AutoMigrate(migrationCtx); err != nil {
			result.Error = fmt.Sprintf("migration failed: %v", err)
			result.Duration = time.Since(startTime)
			return result, err
		}

		// Get list of applied migrations
		history, err := mm.GetMigrationHistory(migrationCtx)
		if err == nil {
			for _, h := range history {
				if currentVersion == nil || mm.compareVersions(h.Version, currentVersion.Version) > 0 {
					result.MigrationsApplied = append(result.MigrationsApplied, h.Version)
				}
			}
		}
	}

	// Get final version
	finalVersion, err := mm.GetCurrentVersion(migrationCtx)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get final version: %v", err)
		result.Duration = time.Since(startTime)
		return result, err
	}

	if finalVersion != nil {
		result.CurrentVersion = finalVersion.Version
	} else {
		result.CurrentVersion = "none"
	}

	// Validate schema if requested
	if config.ValidateSchema {
		validationResult, err := ValidateSchemaStructure(migrationCtx, mm)
		if err != nil {
			result.Error = fmt.Sprintf("schema validation failed: %v", err)
			result.Duration = time.Since(startTime)
			return result, err
		}
		result.ValidationResult = validationResult
	}

	result.Success = true
	result.Duration = time.Since(startTime)

	return result, nil
}

// GetMigrationStatus returns the current migration status
func GetMigrationStatus(ctx context.Context, session *gocql.Session, keyspace string) (*MigrationStatus, error) {
	mm := NewMigrationManager(session, keyspace)

	status := &MigrationStatus{
		TargetVersion: CurrentSchemaVersion,
	}

	// Get current version
	currentVersion, err := mm.GetCurrentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current version: %w", err)
	}

	if currentVersion != nil {
		status.CurrentVersion = currentVersion.Version
		status.LastMigration = currentVersion.AppliedAt
	} else {
		status.CurrentVersion = "none"
	}

	// Check for pending migrations
	pendingMigrations := mm.getPendingMigrations(status.CurrentVersion)
	status.MigrationsNeeded = len(pendingMigrations) > 0

	for _, migration := range pendingMigrations {
		status.PendingMigrations = append(status.PendingMigrations, migration.Version)
	}

	// Validate schema
	validationResult, err := ValidateSchemaStructure(ctx, mm)
	if err != nil {
		status.SchemaValid = false
	} else {
		status.SchemaValid = validationResult.Valid
	}

	return status, nil
}

// ValidateSchemaStructure performs comprehensive schema validation
func ValidateSchemaStructure(ctx context.Context, mm *MigrationManager) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:          true,
		MissingTables:  []string{},
		MissingColumns: []string{},
		Issues:         []string{},
	}

	// Check required tables
	requiredTables := []string{
		"pins_by_cid",
		"placements_by_cid",
		"pins_by_peer",
		"pin_ttl_queue",
		"op_dedup",
		"pin_stats",
		"pin_events",
		SchemaVersionTable,
	}

	for _, table := range requiredTables {
		if err := mm.validateTableExists(ctx, table); err != nil {
			result.Valid = false
			result.MissingTables = append(result.MissingTables, table)
			result.Issues = append(result.Issues, fmt.Sprintf("Missing table: %s", table))
		}
	}

	// Check for version-specific features
	currentVersion, err := mm.GetCurrentVersion(ctx)
	if err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("Cannot determine schema version: %v", err))
		result.Valid = false
		return result, nil
	}

	if currentVersion != nil && mm.compareVersions(currentVersion.Version, "1.1.0") >= 0 {
		// Check v1.1.0 features
		v110Tables := []string{"partition_stats", "performance_metrics", "batch_operations"}
		for _, table := range v110Tables {
			if err := mm.validateTableExists(ctx, table); err != nil {
				result.Valid = false
				result.MissingTables = append(result.MissingTables, table)
				result.Issues = append(result.Issues, fmt.Sprintf("Missing v1.1.0 table: %s", table))
			}
		}

		// Check for new columns in existing tables
		expectedColumns := map[string][]string{
			"pins_by_cid":       {"version", "checksum", "priority"},
			"placements_by_cid": {"in_progress", "last_reconcile"},
			"pins_by_peer":      {"assigned_at", "completed_at", "error_message"},
		}

		for table, columns := range expectedColumns {
			for _, column := range columns {
				if err := mm.validateColumnExists(ctx, table, column); err != nil {
					result.Valid = false
					result.MissingColumns = append(result.MissingColumns, fmt.Sprintf("%s.%s", table, column))
					result.Issues = append(result.Issues, fmt.Sprintf("Missing v1.1.0 column: %s.%s", table, column))
				}
			}
		}
	}

	return result, nil
}

// validateColumnExists checks if a column exists in a table
func (mm *MigrationManager) validateColumnExists(ctx context.Context, tableName, columnName string) error {
	query := `SELECT column_name FROM system_schema.columns WHERE keyspace_name = ? AND table_name = ? AND column_name = ?`

	var foundColumn string
	if err := mm.session.Query(query, mm.keyspace, tableName, columnName).WithContext(ctx).Scan(&foundColumn); err != nil {
		if err == gocql.ErrNotFound {
			return fmt.Errorf("column %s does not exist in table %s", columnName, tableName)
		}
		return fmt.Errorf("failed to check column existence: %w", err)
	}

	return nil
}

// CreateMigrationReport generates a detailed migration report
func CreateMigrationReport(ctx context.Context, session *gocql.Session, keyspace string) (map[string]interface{}, error) {
	mm := NewMigrationManager(session, keyspace)

	report := make(map[string]interface{})

	// Get schema info
	schemaInfo, err := mm.GetSchemaInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema info: %w", err)
	}
	report["schema_info"] = schemaInfo

	// Get migration status
	status, err := GetMigrationStatus(ctx, session, keyspace)
	if err != nil {
		return nil, fmt.Errorf("failed to get migration status: %w", err)
	}
	report["migration_status"] = status

	// Get validation results
	validationResult, err := ValidateSchemaStructure(ctx, mm)
	if err != nil {
		return nil, fmt.Errorf("failed to validate schema: %w", err)
	}
	report["validation"] = validationResult

	// Get table statistics
	tableStats, err := getTableStatistics(ctx, session, keyspace)
	if err != nil {
		// Don't fail the report if we can't get stats
		report["table_stats_error"] = err.Error()
	} else {
		report["table_statistics"] = tableStats
	}

	// Add metadata
	report["generated_at"] = time.Now()
	report["keyspace"] = keyspace

	return report, nil
}

// getTableStatistics retrieves basic statistics about tables
func getTableStatistics(ctx context.Context, session *gocql.Session, keyspace string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get table list
	query := `SELECT table_name FROM system_schema.tables WHERE keyspace_name = ?`
	iter := session.Query(query, keyspace).WithContext(ctx).Iter()
	defer iter.Close()

	var tableName string
	tables := []string{}

	for iter.Scan(&tableName) {
		tables = append(tables, tableName)
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("failed to get table list: %w", err)
	}

	stats["table_count"] = len(tables)
	stats["tables"] = tables

	// Get keyspace info
	keyspaceQuery := `SELECT replication FROM system_schema.keyspaces WHERE keyspace_name = ?`
	var replication map[string]string
	if err := session.Query(keyspaceQuery, keyspace).WithContext(ctx).Scan(&replication); err != nil {
		return nil, fmt.Errorf("failed to get keyspace info: %w", err)
	}

	stats["replication"] = replication

	return stats, nil
}

// MigrationHealthCheck performs a health check on the migration system
func MigrationHealthCheck(ctx context.Context, session *gocql.Session, keyspace string) error {
	mm := NewMigrationManager(session, keyspace)

	// Check if we can connect and query the schema version table
	if err := mm.InitializeSchema(ctx); err != nil {
		return fmt.Errorf("cannot initialize schema version table: %w", err)
	}

	// Check compatibility
	if err := mm.CheckCompatibility(ctx); err != nil {
		return fmt.Errorf("schema compatibility check failed: %w", err)
	}

	// Validate schema structure
	if err := mm.ValidateSchema(ctx); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}

	return nil
}

// GetMigrationRecommendations provides recommendations for migration and schema optimization
func GetMigrationRecommendations(ctx context.Context, session *gocql.Session, keyspace string) ([]string, error) {
	recommendations := []string{}

	status, err := GetMigrationStatus(ctx, session, keyspace)
	if err != nil {
		return nil, fmt.Errorf("failed to get migration status: %w", err)
	}

	// Check if migrations are needed
	if status.MigrationsNeeded {
		recommendations = append(recommendations,
			fmt.Sprintf("Apply pending migrations: %v", status.PendingMigrations))
	}

	// Check if schema is valid
	if !status.SchemaValid {
		recommendations = append(recommendations, "Fix schema validation issues")
	}

	// Check version currency
	if status.CurrentVersion != CurrentSchemaVersion {
		recommendations = append(recommendations,
			fmt.Sprintf("Upgrade from version %s to %s", status.CurrentVersion, CurrentSchemaVersion))
	}

	// Performance recommendations based on version
	if status.CurrentVersion == "1.0.0" {
		recommendations = append(recommendations,
			"Upgrade to v1.1.0 for enhanced performance monitoring and batch operations")
	}

	return recommendations, nil
}
