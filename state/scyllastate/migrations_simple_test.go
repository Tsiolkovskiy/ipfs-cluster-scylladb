package scyllastate

import (
	"testing"
)

// Simple test to verify migration system compiles
func TestMigrationSystemCompiles(t *testing.T) {
	// Test that migration scripts are available
	script := getInitialSchemaScript()
	if script == "" {
		t.Error("Initial schema script should not be empty")
	}

	script = getV110MigrationScript()
	if script == "" {
		t.Error("V1.1.0 migration script should not be empty")
	}

	// Test that migration manager can be created
	mm := &MigrationManager{
		keyspace: "test",
	}
	mm.registerMigrations()

	if len(mm.migrations) == 0 {
		t.Error("Migrations should be registered")
	}

	// Test version comparison
	result := mm.compareVersions("1.0.0", "1.1.0")
	if result != -1 {
		t.Errorf("Expected 1.0.0 < 1.1.0, got %d", result)
	}

	result = mm.compareVersions("1.1.0", "1.0.0")
	if result != 1 {
		t.Errorf("Expected 1.1.0 > 1.0.0, got %d", result)
	}

	result = mm.compareVersions("1.1.0", "1.1.0")
	if result != 0 {
		t.Errorf("Expected 1.1.0 == 1.1.0, got %d", result)
	}
}

func TestMigrationConstantsNotEmpty(t *testing.T) {
	if CurrentSchemaVersion == "" {
		t.Error("CurrentSchemaVersion should not be empty")
	}

	if MinSupportedVersion == "" {
		t.Error("MinSupportedVersion should not be empty")
	}

	if SchemaVersionTable == "" {
		t.Error("SchemaVersionTable should not be empty")
	}
}

func TestMigrationScriptValidation(t *testing.T) {
	// Test initial schema script
	script := getInitialSchemaScript()
	if script == "" {
		t.Error("Initial schema script should not be empty")
	}

	// Should contain keyspace creation
	if !contains(script, "CREATE KEYSPACE") {
		t.Error("Initial schema should contain keyspace creation")
	}

	// Should contain main tables
	if !contains(script, "pins_by_cid") {
		t.Error("Initial schema should contain pins_by_cid table")
	}

	// Test v1.1.0 migration script
	v110Script := getV110MigrationScript()
	if v110Script == "" {
		t.Error("V1.1.0 migration script should not be empty")
	}

	// Should contain ALTER TABLE statements
	if !contains(v110Script, "ALTER TABLE") {
		t.Error("V1.1.0 migration should contain ALTER TABLE statements")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsAt(s, substr)))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
