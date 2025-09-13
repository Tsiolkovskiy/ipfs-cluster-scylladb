package scyllastate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gocql/gocql"
)

// MigrationCLI provides command-line interface for migration operations
type MigrationCLI struct {
	session  *gocql.Session
	keyspace string
	manager  *MigrationManager
}

// NewMigrationCLI creates a new migration CLI instance
func NewMigrationCLI(session *gocql.Session, keyspace string) *MigrationCLI {
	return &MigrationCLI{
		session:  session,
		keyspace: keyspace,
		manager:  NewMigrationManager(session, keyspace),
	}
}

// CLICommand represents a CLI command
type CLICommand struct {
	Name        string
	Description string
	Handler     func(args []string) error
}

// GetCommands returns available CLI commands
func (cli *MigrationCLI) GetCommands() []CLICommand {
	return []CLICommand{
		{
			Name:        "status",
			Description: "Show current migration status",
			Handler:     cli.statusCommand,
		},
		{
			Name:        "migrate",
			Description: "Apply pending migrations",
			Handler:     cli.migrateCommand,
		},
		{
			Name:        "validate",
			Description: "Validate schema structure",
			Handler:     cli.validateCommand,
		},
		{
			Name:        "history",
			Description: "Show migration history",
			Handler:     cli.historyCommand,
		},
		{
			Name:        "info",
			Description: "Show detailed schema information",
			Handler:     cli.infoCommand,
		},
		{
			Name:        "report",
			Description: "Generate comprehensive migration report",
			Handler:     cli.reportCommand,
		},
		{
			Name:        "health",
			Description: "Perform migration system health check",
			Handler:     cli.healthCommand,
		},
		{
			Name:        "recommendations",
			Description: "Get migration and optimization recommendations",
			Handler:     cli.recommendationsCommand,
		},
	}
}

// statusCommand shows current migration status
func (cli *MigrationCLI) statusCommand(args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, err := GetMigrationStatus(ctx, cli.session, cli.keyspace)
	if err != nil {
		return fmt.Errorf("failed to get migration status: %w", err)
	}

	fmt.Printf("Migration Status for keyspace '%s':\n", cli.keyspace)
	fmt.Printf("  Current Version: %s\n", status.CurrentVersion)
	fmt.Printf("  Target Version:  %s\n", status.TargetVersion)
	fmt.Printf("  Schema Valid:    %t\n", status.SchemaValid)
	fmt.Printf("  Migrations Needed: %t\n", status.MigrationsNeeded)

	if len(status.PendingMigrations) > 0 {
		fmt.Printf("  Pending Migrations:\n")
		for _, migration := range status.PendingMigrations {
			fmt.Printf("    - %s\n", migration)
		}
	}

	if !status.LastMigration.IsZero() {
		fmt.Printf("  Last Migration: %s\n", status.LastMigration.Format(time.RFC3339))
	}

	return nil
}

// migrateCommand applies pending migrations
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
		} else {
			fmt.Printf("  No migrations were needed\n")
		}

		if result.ValidationResult != nil && !result.ValidationResult.Valid {
			fmt.Printf("  ⚠ Schema validation issues found:\n")
			for _, issue := range result.ValidationResult.Issues {
				fmt.Printf("    - %s\n", issue)
			}
		}
	} else {
		fmt.Printf("✗ Migration failed: %s\n", result.Error)
		return fmt.Errorf("migration failed")
	}

	return nil
}

// validateCommand validates schema structure
func (cli *MigrationCLI) validateCommand(args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Printf("Validating schema for keyspace '%s'...\n", cli.keyspace)

	validationResult, err := ValidateSchemaStructure(ctx, cli.manager)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if validationResult.Valid {
		fmt.Printf("✓ Schema validation passed\n")
	} else {
		fmt.Printf("✗ Schema validation failed\n")

		if len(validationResult.MissingTables) > 0 {
			fmt.Printf("  Missing Tables:\n")
			for _, table := range validationResult.MissingTables {
				fmt.Printf("    - %s\n", table)
			}
		}

		if len(validationResult.MissingColumns) > 0 {
			fmt.Printf("  Missing Columns:\n")
			for _, column := range validationResult.MissingColumns {
				fmt.Printf("    - %s\n", column)
			}
		}

		if len(validationResult.Issues) > 0 {
			fmt.Printf("  Issues:\n")
			for _, issue := range validationResult.Issues {
				fmt.Printf("    - %s\n", issue)
			}
		}
	}

	return nil
}

// historyCommand shows migration history
func (cli *MigrationCLI) historyCommand(args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	history, err := cli.manager.GetMigrationHistory(ctx)
	if err != nil {
		return fmt.Errorf("failed to get migration history: %w", err)
	}

	fmt.Printf("Migration History for keyspace '%s':\n", cli.keyspace)

	if len(history) == 0 {
		fmt.Printf("  No migrations have been applied\n")
		return nil
	}

	for _, entry := range history {
		fmt.Printf("  Version: %s\n", entry.Version)
		fmt.Printf("    Applied: %s\n", entry.AppliedAt.Format(time.RFC3339))
		if entry.Comment != "" {
			fmt.Printf("    Comment: %s\n", entry.Comment)
		}
		fmt.Printf("\n")
	}

	return nil
}

// infoCommand shows detailed schema information
func (cli *MigrationCLI) infoCommand(args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := cli.manager.GetSchemaInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get schema info: %w", err)
	}

	fmt.Printf("Schema Information for keyspace '%s':\n", cli.keyspace)

	// Pretty print the info
	jsonData, err := json.MarshalIndent(info, "  ", "  ")
	if err != nil {
		return fmt.Errorf("failed to format schema info: %w", err)
	}

	fmt.Printf("  %s\n", string(jsonData))

	return nil
}

// reportCommand generates comprehensive migration report
func (cli *MigrationCLI) reportCommand(args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Printf("Generating migration report for keyspace '%s'...\n", cli.keyspace)

	report, err := CreateMigrationReport(ctx, cli.session, cli.keyspace)
	if err != nil {
		return fmt.Errorf("failed to create migration report: %w", err)
	}

	// Output format based on args
	outputFormat := "json"
	if len(args) > 0 {
		outputFormat = args[0]
	}

	switch outputFormat {
	case "json":
		jsonData, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format report: %w", err)
		}
		fmt.Printf("%s\n", string(jsonData))

	case "summary":
		cli.printReportSummary(report)

	default:
		return fmt.Errorf("unsupported output format: %s (supported: json, summary)", outputFormat)
	}

	return nil
}

// printReportSummary prints a human-readable summary of the migration report
func (cli *MigrationCLI) printReportSummary(report map[string]interface{}) {
	fmt.Printf("Migration Report Summary:\n")

	if schemaInfo, ok := report["schema_info"].(map[string]interface{}); ok {
		if currentVersion, ok := schemaInfo["current_version"].(string); ok {
			fmt.Printf("  Current Version: %s\n", currentVersion)
		}
		if targetVersion, ok := schemaInfo["target_version"].(string); ok {
			fmt.Printf("  Target Version: %s\n", targetVersion)
		}
		if migrationsNeeded, ok := schemaInfo["migrations_needed"].(bool); ok {
			fmt.Printf("  Migrations Needed: %t\n", migrationsNeeded)
		}
	}

	if validation, ok := report["validation"].(*ValidationResult); ok {
		fmt.Printf("  Schema Valid: %t\n", validation.Valid)
		if !validation.Valid {
			fmt.Printf("  Issues: %d\n", len(validation.Issues))
		}
	}

	if tableStats, ok := report["table_statistics"].(map[string]interface{}); ok {
		if tableCount, ok := tableStats["table_count"].(int); ok {
			fmt.Printf("  Tables: %d\n", tableCount)
		}
	}

	if generatedAt, ok := report["generated_at"].(time.Time); ok {
		fmt.Printf("  Generated: %s\n", generatedAt.Format(time.RFC3339))
	}
}

// healthCommand performs migration system health check
func (cli *MigrationCLI) healthCommand(args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Printf("Performing health check for keyspace '%s'...\n", cli.keyspace)

	err := MigrationHealthCheck(ctx, cli.session, cli.keyspace)
	if err != nil {
		fmt.Printf("✗ Health check failed: %v\n", err)
		return err
	}

	fmt.Printf("✓ Migration system is healthy\n")
	return nil
}

// recommendationsCommand gets migration and optimization recommendations
func (cli *MigrationCLI) recommendationsCommand(args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	recommendations, err := GetMigrationRecommendations(ctx, cli.session, cli.keyspace)
	if err != nil {
		return fmt.Errorf("failed to get recommendations: %w", err)
	}

	fmt.Printf("Migration Recommendations for keyspace '%s':\n", cli.keyspace)

	if len(recommendations) == 0 {
		fmt.Printf("  No recommendations - system is up to date\n")
		return nil
	}

	for i, recommendation := range recommendations {
		fmt.Printf("  %d. %s\n", i+1, recommendation)
	}

	return nil
}

// RunCommand executes a CLI command
func (cli *MigrationCLI) RunCommand(commandName string, args []string) error {
	commands := cli.GetCommands()

	for _, cmd := range commands {
		if cmd.Name == commandName {
			return cmd.Handler(args)
		}
	}

	return fmt.Errorf("unknown command: %s", commandName)
}

// PrintHelp prints help information for all commands
func (cli *MigrationCLI) PrintHelp() {
	fmt.Printf("ScyllaDB Migration CLI for keyspace '%s'\n\n", cli.keyspace)
	fmt.Printf("Available commands:\n")

	commands := cli.GetCommands()
	for _, cmd := range commands {
		fmt.Printf("  %-15s %s\n", cmd.Name, cmd.Description)
	}

	fmt.Printf("\nUsage examples:\n")
	fmt.Printf("  migration-cli status\n")
	fmt.Printf("  migration-cli migrate\n")
	fmt.Printf("  migration-cli validate\n")
	fmt.Printf("  migration-cli report json\n")
	fmt.Printf("  migration-cli report summary\n")
}

// SaveReportToFile saves a migration report to a file
func (cli *MigrationCLI) SaveReportToFile(filename string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	report, err := CreateMigrationReport(ctx, cli.session, cli.keyspace)
	if err != nil {
		return fmt.Errorf("failed to create migration report: %w", err)
	}

	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format report: %w", err)
	}

	err = os.WriteFile(filename, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write report to file: %w", err)
	}

	fmt.Printf("Migration report saved to: %s\n", filename)
	return nil
}

// InteractiveMode runs the CLI in interactive mode
func (cli *MigrationCLI) InteractiveMode() error {
	fmt.Printf("ScyllaDB Migration CLI - Interactive Mode\n")
	fmt.Printf("Keyspace: %s\n\n", cli.keyspace)

	for {
		fmt.Printf("Enter command (or 'help' for available commands, 'quit' to exit): ")

		var input string
		fmt.Scanln(&input)

		switch input {
		case "quit", "exit", "q":
			fmt.Printf("Goodbye!\n")
			return nil
		case "help", "h":
			cli.PrintHelp()
		case "":
			continue
		default:
			err := cli.RunCommand(input, []string{})
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		}

		fmt.Printf("\n")
	}
}
