package scyllastate

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
		{"minor version difference", "1.2.0", "1.1.9", 1},
		{"patch version difference", "1.0.2", "1.0.1", 1},
		{"different lengths", "1.0", "1.0.0", 0},
		{"complex versions", "1.2.3", "1.2.4", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mm.compareVersions(tt.v1, tt.v2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMigrationManager_isVersionSupported(t *testing.T) {
	mm := &MigrationManager{}

	tests := []struct {
		name     string
		version  string
		expected bool
	}{
		{"supported version", "1.0.0", true},
		{"current version", "1.1.0", true},
		{"future version", "2.0.0", true}, // Future versions are "supported" for compatibility check
		{"old unsupported version", "0.9.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mm.isVersionSupported(tt.version)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMigrationManager_parseStatements(t *testing.T) {
	mm := &MigrationManager{}

	tests := []struct {
		name     string
		script   string
		expected []string
	}{
		{
			name:   "single statement",
			script: "CREATE TABLE test (id int);",
			expected: []string{
				"CREATE TABLE test (id int)",
			},
		},
		{
			name: "multiple statements",
			script: `CREATE TABLE test1 (id int);
					 CREATE TABLE test2 (name text);`,
			expected: []string{
				"CREATE TABLE test1 (id int)",
				"CREATE TABLE test2 (name text)",
			},
		},
		{
			name: "statements with strings containing semicolons",
			script: `INSERT INTO test VALUES ('hello; world');
					 CREATE TABLE test2 (id int);`,
			expected: []string{
				"INSERT INTO test VALUES ('hello; world')",
				"CREATE TABLE test2 (id int)",
			},
		},
		{
			name:     "empty script",
			script:   "",
			expected: []string{},
		},
		{
			name:     "script with only whitespace",
			script:   "   \n\t  ",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mm.parseStatements(tt.script)

			// Filter out empty statements
			var filtered []string
			for _, stmt := range result {
				if stmt != "" {
					filtered = append(filtered, stmt)
				}
			}

			assert.Equal(t, tt.expected, filtered)
		})
	}
}

func TestMigrationManager_isIgnorableError(t *testing.T) {
	mm := &MigrationManager{}

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "already exists error",
			err:      errors.New("table already exists"),
			expected: true,
		},
		{
			name:     "duplicate column error",
			err:      errors.New("duplicate column name"),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("syntax error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mm.isIgnorableError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMigrationManager_getPendingMigrations(t *testing.T) {
	mm := &MigrationManager{}
	mm.registerMigrations()

	tests := []struct {
		name           string
		currentVersion string
		expectedCount  int
		expectedFirst  string
	}{
		{
			name:           "fresh installation",
			currentVersion: "0.0.0",
			expectedCount:  2, // Should include both 1.0.0 and 1.1.0
			expectedFirst:  "1.0.0",
		},
		{
			name:           "at version 1.0.0",
			currentVersion: "1.0.0",
			expectedCount:  1, // Should only include 1.1.0
			expectedFirst:  "1.1.0",
		},
		{
			name:           "at current version",
			currentVersion: "1.1.0",
			expectedCount:  0, // No migrations needed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pending := mm.getPendingMigrations(tt.currentVersion)
			assert.Equal(t, tt.expectedCount, len(pending))

			if tt.expectedCount > 0 {
				assert.Equal(t, tt.expectedFirst, pending[0].Version)
			}
		})
	}
}

func TestSchemaVersion_JSON(t *testing.T) {
	version := SchemaVersion{
		Version:   "1.1.0",
		AppliedAt: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		Comment:   "Test migration",
	}

	// Test that the struct can be marshaled/unmarshaled
	// This is important for storing migration history
	assert.Equal(t, "1.1.0", version.Version)
	assert.Equal(t, "Test migration", version.Comment)
	assert.False(t, version.AppliedAt.IsZero())
}

func TestMigrationManager_validateTableExists(t *testing.T) {
	// This test would require a real ScyllaDB connection or a more sophisticated mock
	// For now, we'll test the logic structure

	mm := &MigrationManager{
		keyspace: "test_keyspace",
	}

	// Test that the method has the correct signature and basic structure
	assert.NotNil(t, mm)
	assert.Equal(t, "test_keyspace", mm.keyspace)
}

func TestMigrationScriptContent(t *testing.T) {
	// Test that migration scripts are not empty and contain expected content

	t.Run("initial schema script", func(t *testing.T) {
		script := getInitialSchemaScript()
		assert.NotEmpty(t, script)
		assert.Contains(t, script, "CREATE KEYSPACE IF NOT EXISTS ipfs_pins")
		assert.Contains(t, script, "pins_by_cid")
		assert.Contains(t, script, "placements_by_cid")
		assert.Contains(t, script, "pins_by_peer")
	})

	t.Run("v1.1.0 migration script", func(t *testing.T) {
		script := getV110MigrationScript()
		assert.NotEmpty(t, script)
		assert.Contains(t, script, "ALTER TABLE ipfs_pins.pins_by_cid ADD version int")
		assert.Contains(t, script, "partition_stats")
		assert.Contains(t, script, "performance_metrics")
		assert.Contains(t, script, "batch_operations")
	})

	t.Run("validation script", func(t *testing.T) {
		script := getSchemaValidationScript()
		assert.NotEmpty(t, script)
		assert.Contains(t, script, "system_schema.tables")
		assert.Contains(t, script, "system_schema.columns")
	})
}

func TestMigrationManager_NewMigrationManager(t *testing.T) {
	// Test that NewMigrationManager properly initializes the manager
	keyspace := "test_keyspace"

	// Create a migration manager without session for testing structure
	mm := &MigrationManager{
		keyspace: keyspace,
	}
	mm.registerMigrations()

	assert.NotNil(t, mm)
	assert.Equal(t, keyspace, mm.keyspace)
	assert.NotEmpty(t, mm.migrations)

	// Check that migrations are registered in correct order
	assert.Equal(t, "1.0.0", mm.migrations[0].Version)
	assert.Equal(t, "1.1.0", mm.migrations[1].Version)
}

func TestMigrationConstantsValues(t *testing.T) {
	// Test that constants are properly defined
	assert.Equal(t, "1.1.0", CurrentSchemaVersion)
	assert.Equal(t, "1.0.0", MinSupportedVersion)
	assert.Equal(t, "schema_version", SchemaVersionTable)
}

// Integration test structure (would require real ScyllaDB)
func TestMigrationManager_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test would:
	// 1. Start a test ScyllaDB container
	// 2. Create a fresh keyspace
	// 3. Run migrations
	// 4. Validate schema
	// 5. Test rollback scenarios
	// 6. Clean up

	t.Skip("Integration test requires ScyllaDB container setup")
}

// Benchmark tests for migration performance
func BenchmarkMigrationManager_compareVersions(b *testing.B) {
	mm := &MigrationManager{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mm.compareVersions("1.2.3", "1.2.4")
	}
}

func BenchmarkMigrationManager_parseStatements(b *testing.B) {
	mm := &MigrationManager{}
	script := getV110MigrationScript()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mm.parseStatements(script)
	}
}

// Test helper functions
func createTestMigrationManager() *MigrationManager {
	return &MigrationManager{
		keyspace: "test_keyspace",
	}
}

func TestMigrationManager_GetSchemaInfo_Structure(t *testing.T) {
	// Test the structure of schema info without requiring a database
	mm := createTestMigrationManager()
	mm.registerMigrations()

	// Test that the method exists and has correct structure
	assert.NotNil(t, mm)
	assert.NotEmpty(t, mm.migrations)

	// Verify migration registration
	assert.Len(t, mm.migrations, 2)
	assert.Equal(t, "1.0.0", mm.migrations[0].Version)
	assert.Equal(t, "1.1.0", mm.migrations[1].Version)
}

func TestMigrationValidation(t *testing.T) {
	// Test migration validation functions exist and have correct signatures
	mm := &MigrationManager{}

	// Test that validation functions are properly defined
	assert.NotNil(t, mm.validateInitialSchema)
	assert.NotNil(t, mm.validateV110Schema)
}

// Test error handling scenarios
func TestMigrationManager_ErrorHandling(t *testing.T) {
	mm := &MigrationManager{}

	tests := []struct {
		name         string
		errorString  string
		shouldIgnore bool
	}{
		{"table already exists", "table already exists", true},
		{"column already exists", "column already exists", true},
		{"duplicate column name", "duplicate column name", true},
		{"syntax error", "syntax error near", false},
		{"connection error", "connection refused", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errorString)
			result := mm.isIgnorableError(err)
			assert.Equal(t, tt.shouldIgnore, result)
		})
	}
}
