package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ipfs-cluster/ipfs-cluster/state/scyllastate"
)

// Version information
const (
	Version = "1.0.0"
	AppName = "scylla-state-cli"
)

// CLIApp represents the main CLI application
type CLIApp struct {
	config     *scyllastate.Config
	cliTools   *scyllastate.CLITools
	logger     *log.Logger
	verbose    bool
	configFile string
}

// SimpleLogger implements the Logger interface for CLI tools
type SimpleLogger struct {
	logger  *log.Logger
	verbose bool
}

func (l *SimpleLogger) Printf(format string, args ...interface{}) {
	l.logger.Printf(format, args...)
}

func (l *SimpleLogger) Errorf(format string, args ...interface{}) {
	l.logger.Printf("ERROR: "+format, args...)
}

func (l *SimpleLogger) Warnf(format string, args ...interface{}) {
	l.logger.Printf("WARN: "+format, args...)
}

func (l *SimpleLogger) Infof(format string, args ...interface{}) {
	l.logger.Printf("INFO: "+format, args...)
}

func (l *SimpleLogger) Debugf(format string, args ...interface{}) {
	if l.verbose {
		l.logger.Printf("DEBUG: "+format, args...)
	}
}

func main() {
	app := &CLIApp{
		logger: log.New(os.Stderr, "[scylla-state-cli] ", log.LstdFlags),
	}

	if err := app.run(); err != nil {
		app.logger.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func (app *CLIApp) run() error {
	// Parse global flags
	if err := app.parseGlobalFlags(); err != nil {
		return err
	}

	// Load configuration
	if err := app.loadConfig(); err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize CLI tools
	logger := &SimpleLogger{logger: app.logger, verbose: app.verbose}
	cliTools, err := scyllastate.NewCLITools(app.config, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize CLI tools: %w", err)
	}
	defer cliTools.Close()

	app.cliTools = cliTools

	// Get command and arguments
	args := flag.Args()
	if len(args) == 0 {
		app.printUsage()
		return nil
	}

	command := args[0]
	commandArgs := args[1:]

	// Handle special commands
	switch command {
	case "help", "--help", "-h":
		app.cliTools.PrintHelp()
		return nil
	case "version", "--version", "-v":
		fmt.Printf("%s version %s\n", AppName, Version)
		return nil
	}

	// Execute command
	return app.cliTools.RunCommand(command, commandArgs)
}

func (app *CLIApp) parseGlobalFlags() error {
	var showHelp, showVersion bool

	flag.BoolVar(&app.verbose, "verbose", false, "Enable verbose output")
	flag.BoolVar(&showHelp, "help", false, "Show help information")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.StringVar(&app.configFile, "config", "", "Configuration file path")

	flag.Usage = func() {
		app.printUsage()
	}

	flag.Parse()

	if showHelp {
		app.printUsage()
		os.Exit(0)
	}

	if showVersion {
		fmt.Printf("%s version %s\n", AppName, Version)
		os.Exit(0)
	}

	return nil
}

func (app *CLIApp) loadConfig() error {
	// Default configuration
	app.config = &scyllastate.Config{
		Hosts:                      []string{"127.0.0.1"},
		Port:                       9042,
		Keyspace:                   "ipfs_pins",
		Username:                   "",
		Password:                   "",
		NumConns:                   4,
		Timeout:                    30 * time.Second,
		ConnectTimeout:             10 * time.Second,
		Consistency:                "QUORUM",
		SerialConsistency:          "SERIAL",
		BatchSize:                  1000,
		BatchTimeout:               time.Second,
		MetricsEnabled:             false,
		TracingEnabled:             false,
		TLSEnabled:                 false,
		TokenAwareRouting:          true,
		DCAwareRouting:             true,
		LocalDC:                    "",
		PageSize:                   5000,
		PreparedStatementCacheSize: 1000,
		RetryPolicy: scyllastate.RetryPolicyConfig{
			NumRetries:    3,
			MinRetryDelay: 100 * time.Millisecond,
			MaxRetryDelay: 10 * time.Second,
		},
	}

	// Load from config file if specified
	if app.configFile != "" {
		if err := app.loadConfigFromFile(app.configFile); err != nil {
			return fmt.Errorf("failed to load config from file: %w", err)
		}
	} else {
		// Try to load from default locations
		defaultPaths := []string{
			"./scylla-state.json",
			"~/.config/ipfs-cluster/scylla-state.json",
			"/etc/ipfs-cluster/scylla-state.json",
		}

		for _, path := range defaultPaths {
			if app.fileExists(path) {
				if err := app.loadConfigFromFile(path); err != nil {
					app.logger.Printf("Warning: failed to load config from %s: %v", path, err)
				} else {
					app.logger.Printf("Loaded configuration from %s", path)
					break
				}
			}
		}
	}

	// Override with environment variables
	app.loadConfigFromEnv()

	return nil
}

func (app *CLIApp) loadConfigFromFile(filename string) error {
	// Expand home directory
	if strings.HasPrefix(filename, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		filename = filepath.Join(home, filename[2:])
	}

	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(app.config); err != nil {
		return fmt.Errorf("failed to decode config file: %w", err)
	}

	return nil
}

func (app *CLIApp) loadConfigFromEnv() {
	// Load configuration from environment variables
	if hosts := os.Getenv("SCYLLA_HOSTS"); hosts != "" {
		app.config.Hosts = strings.Split(hosts, ",")
	}

	if keyspace := os.Getenv("SCYLLA_KEYSPACE"); keyspace != "" {
		app.config.Keyspace = keyspace
	}

	if username := os.Getenv("SCYLLA_USERNAME"); username != "" {
		app.config.Username = username
	}

	if password := os.Getenv("SCYLLA_PASSWORD"); password != "" {
		app.config.Password = password
	}

	if consistency := os.Getenv("SCYLLA_CONSISTENCY"); consistency != "" {
		app.config.Consistency = consistency
	}

	if localDC := os.Getenv("SCYLLA_LOCAL_DC"); localDC != "" {
		app.config.LocalDC = localDC
	}

	// TLS configuration
	if os.Getenv("SCYLLA_TLS_ENABLED") == "true" {
		app.config.TLSEnabled = true
	}

	if certFile := os.Getenv("SCYLLA_TLS_CERT_FILE"); certFile != "" {
		app.config.TLSCertFile = certFile
	}

	if keyFile := os.Getenv("SCYLLA_TLS_KEY_FILE"); keyFile != "" {
		app.config.TLSKeyFile = keyFile
	}

	if caFile := os.Getenv("SCYLLA_TLS_CA_FILE"); caFile != "" {
		app.config.TLSCAFile = caFile
	}
}

func (app *CLIApp) fileExists(filename string) bool {
	// Expand home directory
	if strings.HasPrefix(filename, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		filename = filepath.Join(home, filename[2:])
	}

	_, err := os.Stat(filename)
	return err == nil
}

func (app *CLIApp) printUsage() {
	fmt.Printf("%s - ScyllaDB State Management CLI\n", AppName)
	fmt.Printf("Version: %s\n\n", Version)

	fmt.Printf("Usage: %s [global-options] <command> [command-options] [arguments]\n\n", AppName)

	fmt.Printf("Global Options:\n")
	fmt.Printf("  --config <file>    Configuration file path\n")
	fmt.Printf("  --verbose          Enable verbose output\n")
	fmt.Printf("  --help             Show this help message\n")
	fmt.Printf("  --version          Show version information\n\n")

	fmt.Printf("Commands:\n")
	fmt.Printf("  export             Export state data to file\n")
	fmt.Printf("  import             Import state data from file\n")
	fmt.Printf("  status             Show ScyllaDB state status\n")
	fmt.Printf("  health             Perform health check\n")
	fmt.Printf("  migrate            Migrate data from another backend\n")
	fmt.Printf("  validate           Validate state data integrity\n")
	fmt.Printf("  stats              Show detailed statistics\n")
	fmt.Printf("  cleanup            Clean up orphaned data\n")
	fmt.Printf("  backup             Create backup of state data\n")
	fmt.Printf("  restore            Restore from backup\n")
	fmt.Printf("  help               Show detailed command help\n\n")

	fmt.Printf("Configuration:\n")
	fmt.Printf("  Configuration can be provided via:\n")
	fmt.Printf("  1. Command line flag: --config <file>\n")
	fmt.Printf("  2. Default locations:\n")
	fmt.Printf("     - ./scylla-state.json\n")
	fmt.Printf("     - ~/.config/ipfs-cluster/scylla-state.json\n")
	fmt.Printf("     - /etc/ipfs-cluster/scylla-state.json\n")
	fmt.Printf("  3. Environment variables:\n")
	fmt.Printf("     - SCYLLA_HOSTS (comma-separated)\n")
	fmt.Printf("     - SCYLLA_KEYSPACE\n")
	fmt.Printf("     - SCYLLA_USERNAME\n")
	fmt.Printf("     - SCYLLA_PASSWORD\n")
	fmt.Printf("     - SCYLLA_CONSISTENCY\n")
	fmt.Printf("     - SCYLLA_LOCAL_DC\n")
	fmt.Printf("     - SCYLLA_TLS_ENABLED (true/false)\n")
	fmt.Printf("     - SCYLLA_TLS_CERT_FILE\n")
	fmt.Printf("     - SCYLLA_TLS_KEY_FILE\n")
	fmt.Printf("     - SCYLLA_TLS_CA_FILE\n\n")

	fmt.Printf("Examples:\n")
	fmt.Printf("  %s status --detailed\n", AppName)
	fmt.Printf("  %s export --format json /tmp/state-backup.json\n", AppName)
	fmt.Printf("  %s import --format protobuf /tmp/state-backup.pb\n", AppName)
	fmt.Printf("  %s health\n", AppName)
	fmt.Printf("  %s migrate dsstate /path/to/dsstate/config\n", AppName)
	fmt.Printf("  %s validate --sample-rate 0.1\n", AppName)
}

// Example configuration file generator
func generateExampleConfig() {
	config := &scyllastate.Config{
		Hosts:                      []string{"scylla1.example.com", "scylla2.example.com", "scylla3.example.com"},
		Port:                       9042,
		Keyspace:                   "ipfs_pins",
		Username:                   "cluster_user",
		Password:                   "secure_password",
		NumConns:                   10,
		Timeout:                    30 * time.Second,
		ConnectTimeout:             10 * time.Second,
		Consistency:                "QUORUM",
		SerialConsistency:          "SERIAL",
		BatchSize:                  1000,
		BatchTimeout:               time.Second,
		MetricsEnabled:             true,
		TracingEnabled:             false,
		TLSEnabled:                 true,
		TLSCertFile:                "/etc/ssl/certs/client.crt",
		TLSKeyFile:                 "/etc/ssl/private/client.key",
		TLSCAFile:                  "/etc/ssl/certs/ca.crt",
		TokenAwareRouting:          true,
		DCAwareRouting:             true,
		LocalDC:                    "datacenter1",
		PageSize:                   5000,
		PreparedStatementCacheSize: 1000,
		RetryPolicy: scyllastate.RetryPolicyConfig{
			NumRetries:    3,
			MinRetryDelay: 100 * time.Millisecond,
			MaxRetryDelay: 10 * time.Second,
		},
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	fmt.Printf("# Example ScyllaDB State Configuration\n")
	encoder.Encode(config)
}
