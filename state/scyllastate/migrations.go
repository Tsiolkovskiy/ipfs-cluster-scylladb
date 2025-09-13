package scyllastate

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gocql/gocql"
)

const (
	// CurrentSchemaVersion is the latest schema version
	CurrentSchemaVersion = "1.1.0"

	// MinSupportedVersion is the minimum schema version we can migrate from
	MinSupportedVersion = "1.0.0"

	// SchemaVersionTable stores the current schema version
	SchemaVersionTable = "schema_version"
)

// SchemaVersion represents a database schema version
type SchemaVersion struct {
	Version   string    `json:"version"`
	AppliedAt time.Time `json:"applied_at"`
	Comment   string    `json:"comment,omitempty"`
}

// Migration represents a single database migration
type Migration struct {
	Version     string
	Description string
	UpScript    string
	DownScript  string                     // Optional rollback script
	Validate    func(*gocql.Session) error // Optional validation function
}

// MigrationManager handles database schema migrations
type MigrationManager struct {
	session    *gocql.Session
	keyspace   string
	migrations []Migration
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager(session *gocql.Session, keyspace string) *MigrationManager {
	mm := &MigrationManager{
		session:  session,
		keyspace: keyspace,
	}

	// Register all available migrations
	mm.registerMigrations()

	return mm
}

// registerMigrations registers all available migrations in order
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

// InitializeSchema creates the schema version table if it doesn't exist
func (mm *MigrationManager) InitializeSchema(ctx context.Context) error {
	// Create schema version table
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

// GetCurrentVersion returns the current schema version from the database
func (mm *MigrationManager) GetCurrentVersion(ctx context.Context) (*SchemaVersion, error) {
	var version string
	var appliedAt time.Time
	var comment string

	query := fmt.Sprintf("SELECT version, applied_at, comment FROM %s.%s ORDER BY version DESC LIMIT 1",
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

	// No version found - this is a fresh installation
	return nil, nil
}

// SetVersion records a new schema version in the database
func (mm *MigrationManager) SetVersion(ctx context.Context, version, comment string) error {
	query := fmt.Sprintf("INSERT INTO %s.%s (version, applied_at, comment) VALUES (?, ?, ?)",
		mm.keyspace, SchemaVersionTable)

	if err := mm.session.Query(query, version, time.Now(), comment).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("failed to set schema version: %w", err)
	}

	return nil
}

// CheckCompatibility verifies if the current schema version is compatible
func (mm *MigrationManager) CheckCompatibility(ctx context.Context) error {
	currentVersion, err := mm.GetCurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current schema version: %w", err)
	}

	if currentVersion == nil {
		// Fresh installation - no compatibility issues
		return nil
	}

	// Check if current version is supported
	if !mm.isVersionSupported(currentVersion.Version) {
		return fmt.Errorf("schema version %s is not supported (minimum: %s, current: %s)",
			currentVersion.Version, MinSupportedVersion, CurrentSchemaVersion)
	}

	// Check if current version is newer than what we support
	if mm.compareVersions(currentVersion.Version, CurrentSchemaVersion) > 0 {
		return fmt.Errorf("schema version %s is newer than supported version %s - please upgrade the application",
			currentVersion.Version, CurrentSchemaVersion)
	}

	return nil
}

// ApplyMigrations applies all pending migrations to bring schema to current version
func (mm *MigrationManager) ApplyMigrations(ctx context.Context) error {
	// Initialize schema version table if needed
	if err := mm.InitializeSchema(ctx); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Check compatibility first
	if err := mm.CheckCompatibility(ctx); err != nil {
		return fmt.Errorf("compatibility check failed: %w", err)
	}

	currentVersion, err := mm.GetCurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	var startVersion string
	if currentVersion == nil {
		startVersion = "0.0.0" // Fresh installation
	} else {
		startVersion = currentVersion.Version
	}

	// Find migrations to apply
	pendingMigrations := mm.getPendingMigrations(startVersion)

	if len(pendingMigrations) == 0 {
		// Already at current version
		return nil
	}

	// Apply migrations in order
	for _, migration := range pendingMigrations {
		if err := mm.applyMigration(ctx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.Version, err)
		}
	}

	return nil
}

// applyMigration applies a single migration
func (mm *MigrationManager) applyMigration(ctx context.Context, migration Migration) error {
	// Execute migration script
	statements := mm.parseStatements(migration.UpScript)

	for _, stmt := range statements {
		if strings.TrimSpace(stmt) == "" {
			continue
		}

		if err := mm.session.Query(stmt).WithContext(ctx).Exec(); err != nil {
			// Some ALTER TABLE statements may fail if column already exists
			// This is expected for idempotent migrations
			if !mm.isIgnorableError(err) {
				return fmt.Errorf("failed to execute migration statement: %w", err)
			}
		}
	}

	// Run validation if provided
	if migration.Validate != nil {
		if err := migration.Validate(mm.session); err != nil {
			return fmt.Errorf("migration validation failed: %w", err)
		}
	}

	// Record successful migration
	if err := mm.SetVersion(ctx, migration.Version, migration.Description); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return nil
}

// getPendingMigrations returns migrations that need to be applied
func (mm *MigrationManager) getPendingMigrations(currentVersion string) []Migration {
	var pending []Migration

	for _, migration := range mm.migrations {
		if mm.compareVersions(migration.Version, currentVersion) > 0 {
			pending = append(pending, migration)
		}
	}

	// Sort by version to ensure correct order
	sort.Slice(pending, func(i, j int) bool {
		return mm.compareVersions(pending[i].Version, pending[j].Version) < 0
	})

	return pending
}

// parseStatements splits a migration script into individual SQL statements
func (mm *MigrationManager) parseStatements(script string) []string {
	// Split by semicolon, but be careful about semicolons in strings
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

	// Add final statement if it doesn't end with semicolon
	if strings.TrimSpace(current) != "" {
		statements = append(statements, strings.TrimSpace(current))
	}

	return statements
}

// isIgnorableError checks if an error can be safely ignored during migration
func (mm *MigrationManager) isIgnorableError(err error) bool {
	errStr := strings.ToLower(err.Error())

	// Common ignorable errors during idempotent migrations
	ignorablePatterns := []string{
		"already exists",
		"duplicate column name",
		"column already exists",
		"table already exists",
		"keyspace already exists",
	}

	for _, pattern := range ignorablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// isVersionSupported checks if a version is supported for migration
func (mm *MigrationManager) isVersionSupported(version string) bool {
	return mm.compareVersions(version, MinSupportedVersion) >= 0
}

// compareVersions compares two semantic version strings
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func (mm *MigrationManager) compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Pad shorter version with zeros
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

	// Compare each part
	for i := 0; i < maxLen; i++ {
		num1, err1 := strconv.Atoi(parts1[i])
		num2, err2 := strconv.Atoi(parts2[i])

		// If parsing fails, do string comparison
		if err1 != nil || err2 != nil {
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

// ValidateSchema validates the current schema structure
func (mm *MigrationManager) ValidateSchema(ctx context.Context) error {
	// Check that all required tables exist
	requiredTables := []string{
		"pins_by_cid",
		"placements_by_cid",
		"pins_by_peer",
		"pin_ttl_queue",
		"op_dedup",
		"pin_stats",
		"pin_events",
	}

	for _, table := range requiredTables {
		if err := mm.validateTableExists(ctx, table); err != nil {
			return fmt.Errorf("table validation failed for %s: %w", table, err)
		}
	}

	// Validate schema version table
	if err := mm.validateTableExists(ctx, SchemaVersionTable); err != nil {
		return fmt.Errorf("schema version table validation failed: %w", err)
	}

	return nil
}

// validateTableExists checks if a table exists and has the expected structure
func (mm *MigrationManager) validateTableExists(ctx context.Context, tableName string) error {
	query := `SELECT table_name FROM system_schema.tables WHERE keyspace_name = ? AND table_name = ?`

	var foundTable string
	if err := mm.session.Query(query, mm.keyspace, tableName).WithContext(ctx).Scan(&foundTable); err != nil {
		if err == gocql.ErrNotFound {
			return fmt.Errorf("table %s does not exist", tableName)
		}
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	return nil
}

// validateInitialSchema validates the initial schema (v1.0.0)
func (mm *MigrationManager) validateInitialSchema(session *gocql.Session) error {
	// Check core tables exist
	coreTable := []string{"pins_by_cid", "placements_by_cid", "pins_by_peer", "pin_ttl_queue"}

	for _, table := range coreTable {
		query := `SELECT table_name FROM system_schema.tables WHERE keyspace_name = ? AND table_name = ?`
		var foundTable string
		if err := session.Query(query, mm.keyspace, table).Scan(&foundTable); err != nil {
			return fmt.Errorf("core table %s missing in initial schema", table)
		}
	}

	return nil
}

// validateV110Schema validates the v1.1.0 schema enhancements
func (mm *MigrationManager) validateV110Schema(session *gocql.Session) error {
	// Check that new columns exist in pins_by_cid
	expectedColumns := []string{"version", "checksum", "priority"}

	for _, column := range expectedColumns {
		query := `SELECT column_name FROM system_schema.columns WHERE keyspace_name = ? AND table_name = ? AND column_name = ?`
		var foundColumn string
		if err := session.Query(query, mm.keyspace, "pins_by_cid", column).Scan(&foundColumn); err != nil {
			return fmt.Errorf("column %s missing in pins_by_cid table", column)
		}
	}

	// Check that new tables exist
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

// GetMigrationHistory returns the history of applied migrations
func (mm *MigrationManager) GetMigrationHistory(ctx context.Context) ([]SchemaVersion, error) {
	query := fmt.Sprintf("SELECT version, applied_at, comment FROM %s.%s", mm.keyspace, SchemaVersionTable)

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

	// Sort by version
	sort.Slice(history, func(i, j int) bool {
		return mm.compareVersions(history[i].Version, history[j].Version) < 0
	})

	return history, nil
}

// GetSchemaInfo returns detailed information about the current schema
func (mm *MigrationManager) GetSchemaInfo(ctx context.Context) (map[string]interface{}, error) {
	info := make(map[string]interface{})

	// Get current version
	currentVersion, err := mm.GetCurrentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current version: %w", err)
	}

	if currentVersion != nil {
		info["current_version"] = currentVersion.Version
		info["applied_at"] = currentVersion.AppliedAt
		info["comment"] = currentVersion.Comment
	} else {
		info["current_version"] = "none"
	}

	info["target_version"] = CurrentSchemaVersion
	info["min_supported_version"] = MinSupportedVersion

	// Check if migrations are needed
	pendingMigrations := mm.getPendingMigrations(func() string {
		if currentVersion != nil {
			return currentVersion.Version
		}
		return "0.0.0"
	}())

	info["migrations_needed"] = len(pendingMigrations) > 0
	info["pending_migrations"] = len(pendingMigrations)

	// Get migration history
	history, err := mm.GetMigrationHistory(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get migration history: %w", err)
	}

	info["migration_history"] = history

	return info, nil
}

// AutoMigrate performs automatic migration to the current schema version
// This is the main entry point for automatic migrations at startup
func (mm *MigrationManager) AutoMigrate(ctx context.Context) error {
	// Check compatibility first
	if err := mm.CheckCompatibility(ctx); err != nil {
		return fmt.Errorf("schema compatibility check failed: %w", err)
	}

	// Apply any pending migrations
	if err := mm.ApplyMigrations(ctx); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	// Validate final schema
	if err := mm.ValidateSchema(ctx); err != nil {
		return fmt.Errorf("schema validation failed after migration: %w", err)
	}

	return nil
}
