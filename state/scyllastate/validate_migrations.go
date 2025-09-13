package scyllastate

import (
	"fmt"
	"strings"
)

// ValidateMigrationSystem performs basic validation of the migration system
func ValidateMigrationSystem() error {
	// Test that migration scripts are available
	script := getInitialSchemaScript()
	if script == "" {
		return fmt.Errorf("initial schema script is empty")
	}

	if !strings.Contains(script, "CREATE KEYSPACE") {
		return fmt.Errorf("initial schema script missing keyspace creation")
	}

	if !strings.Contains(script, "pins_by_cid") {
		return fmt.Errorf("initial schema script missing pins_by_cid table")
	}

	// Test v1.1.0 migration script
	v110Script := getV110MigrationScript()
	if v110Script == "" {
		return fmt.Errorf("v1.1.0 migration script is empty")
	}

	if !strings.Contains(v110Script, "ALTER TABLE") {
		return fmt.Errorf("v1.1.0 migration script missing ALTER TABLE statements")
	}

	// Test migration manager creation
	mm := &MigrationManager{
		keyspace: "test",
	}
	mm.registerMigrations()

	if len(mm.migrations) == 0 {
		return fmt.Errorf("no migrations registered")
	}

	// Test version comparison logic
	if mm.compareVersions("1.0.0", "1.1.0") != -1 {
		return fmt.Errorf("version comparison failed: 1.0.0 should be less than 1.1.0")
	}

	if mm.compareVersions("1.1.0", "1.0.0") != 1 {
		return fmt.Errorf("version comparison failed: 1.1.0 should be greater than 1.0.0")
	}

	if mm.compareVersions("1.1.0", "1.1.0") != 0 {
		return fmt.Errorf("version comparison failed: 1.1.0 should equal 1.1.0")
	}

	// Test constants
	if CurrentSchemaVersion == "" {
		return fmt.Errorf("CurrentSchemaVersion is empty")
	}

	if MinSupportedVersion == "" {
		return fmt.Errorf("MinSupportedVersion is empty")
	}

	if SchemaVersionTable == "" {
		return fmt.Errorf("SchemaVersionTable is empty")
	}

	// Test pending migrations logic
	pending := mm.getPendingMigrations("0.0.0")
	if len(pending) != 2 {
		return fmt.Errorf("expected 2 pending migrations from 0.0.0, got %d", len(pending))
	}

	pending = mm.getPendingMigrations("1.0.0")
	if len(pending) != 1 {
		return fmt.Errorf("expected 1 pending migration from 1.0.0, got %d", len(pending))
	}

	pending = mm.getPendingMigrations("1.1.0")
	if len(pending) != 0 {
		return fmt.Errorf("expected 0 pending migrations from 1.1.0, got %d", len(pending))
	}

	// Test version support
	if !mm.isVersionSupported("1.0.0") {
		return fmt.Errorf("version 1.0.0 should be supported")
	}

	if !mm.isVersionSupported("1.1.0") {
		return fmt.Errorf("version 1.1.0 should be supported")
	}

	if mm.isVersionSupported("0.9.0") {
		return fmt.Errorf("version 0.9.0 should not be supported")
	}

	fmt.Println("✓ Migration system validation passed")
	return nil
}

// ValidateMigrationScripts validates the content of migration scripts
func ValidateMigrationScripts() error {
	// Validate initial schema script
	script := getInitialSchemaScript()

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
		if !strings.Contains(script, table) {
			return fmt.Errorf("initial schema missing table: %s", table)
		}
	}

	// Validate v1.1.0 migration script
	v110Script := getV110MigrationScript()

	expectedAlters := []string{
		"ALTER TABLE ipfs_pins.pins_by_cid ADD version int",
		"ALTER TABLE ipfs_pins.pins_by_cid ADD checksum text",
		"ALTER TABLE ipfs_pins.pins_by_cid ADD priority tinyint",
	}

	for _, alter := range expectedAlters {
		if !strings.Contains(v110Script, alter) {
			return fmt.Errorf("v1.1.0 migration missing: %s", alter)
		}
	}

	expectedTables := []string{
		"partition_stats",
		"performance_metrics",
		"batch_operations",
	}

	for _, table := range expectedTables {
		if !strings.Contains(v110Script, table) {
			return fmt.Errorf("v1.1.0 migration missing table: %s", table)
		}
	}

	fmt.Println("✓ Migration scripts validation passed")
	return nil
}

// ValidateMigrationUtils validates migration utility functions
func ValidateMigrationUtils() error {
	// Test default migration config
	config := DefaultMigrationConfig()
	if config == nil {
		return fmt.Errorf("default migration config is nil")
	}

	if !config.AutoMigrate {
		return fmt.Errorf("auto migrate should be enabled by default")
	}

	if !config.ValidateSchema {
		return fmt.Errorf("validate schema should be enabled by default")
	}

	if config.AllowDowngrade {
		return fmt.Errorf("allow downgrade should be disabled by default")
	}

	fmt.Println("✓ Migration utilities validation passed")
	return nil
}

// RunAllValidations runs all migration system validations
func RunAllValidations() error {
	fmt.Println("Validating migration system...")

	if err := ValidateMigrationSystem(); err != nil {
		return fmt.Errorf("migration system validation failed: %w", err)
	}

	if err := ValidateMigrationScripts(); err != nil {
		return fmt.Errorf("migration scripts validation failed: %w", err)
	}

	if err := ValidateMigrationUtils(); err != nil {
		return fmt.Errorf("migration utilities validation failed: %w", err)
	}

	fmt.Println("✓ All migration validations passed successfully")
	return nil
}
