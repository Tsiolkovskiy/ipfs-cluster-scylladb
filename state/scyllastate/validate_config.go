package scyllastate

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// ValidationLevel represents the level of validation to perform
type ValidationLevel int

const (
	// ValidationBasic performs basic validation (default)
	ValidationBasic ValidationLevel = iota
	// ValidationStrict performs strict validation for production environments
	ValidationStrict
	// ValidationDevelopment performs relaxed validation for development
	ValidationDevelopment
)

// ValidationOptions configures validation behavior
type ValidationOptions struct {
	Level                ValidationLevel
	AllowInsecureTLS     bool
	AllowWeakConsistency bool
	MinHosts             int
	MaxHosts             int
	RequireAuth          bool
}

// DefaultValidationOptions returns default validation options
func DefaultValidationOptions() *ValidationOptions {
	return &ValidationOptions{
		Level:                ValidationBasic,
		AllowInsecureTLS:     false,
		AllowWeakConsistency: false,
		MinHosts:             1,
		MaxHosts:             100,
		RequireAuth:          false,
	}
}

// StrictValidationOptions returns strict validation options for production
func StrictValidationOptions() *ValidationOptions {
	return &ValidationOptions{
		Level:                ValidationStrict,
		AllowInsecureTLS:     false,
		AllowWeakConsistency: false,
		MinHosts:             3,
		MaxHosts:             50,
		RequireAuth:          true,
	}
}

// DevelopmentValidationOptions returns relaxed validation options for development
func DevelopmentValidationOptions() *ValidationOptions {
	return &ValidationOptions{
		Level:                ValidationDevelopment,
		AllowInsecureTLS:     true,
		AllowWeakConsistency: true,
		MinHosts:             1,
		MaxHosts:             10,
		RequireAuth:          false,
	}
}

// ValidateWithOptions validates the configuration with custom validation options
func (cfg *Config) ValidateWithOptions(opts *ValidationOptions) error {
	if opts == nil {
		opts = DefaultValidationOptions()
	}

	// Basic validation first
	if err := cfg.validateBasic(opts); err != nil {
		return err
	}

	// Level-specific validation
	switch opts.Level {
	case ValidationStrict:
		return cfg.validateStrict(opts)
	case ValidationDevelopment:
		return cfg.validateDevelopment(opts)
	default:
		return nil
	}
}

// validateBasic performs basic configuration validation
func (cfg *Config) validateBasic(opts *ValidationOptions) error {
	// Validate hosts
	if err := cfg.validateHosts(opts); err != nil {
		return err
	}

	// Validate port
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be between 1 and 65535)", cfg.Port)
	}

	// Validate keyspace
	if err := cfg.validateKeyspace(); err != nil {
		return err
	}

	// Validate timeouts
	if err := cfg.validateTimeouts(); err != nil {
		return err
	}

	// Validate consistency levels
	if err := cfg.validateConsistencyLevels(opts); err != nil {
		return err
	}

	// Validate retry policy
	if err := cfg.validateRetryPolicy(); err != nil {
		return err
	}

	// Validate batch settings
	if err := cfg.validateBatchSettings(); err != nil {
		return err
	}

	// Validate TLS settings
	if err := cfg.validateTLSSettings(opts); err != nil {
		return err
	}

	return nil
}

// validateStrict performs strict validation for production environments
func (cfg *Config) validateStrict(opts *ValidationOptions) error {
	// Require TLS in strict mode
	if !cfg.TLSEnabled {
		return fmt.Errorf("TLS must be enabled in strict validation mode")
	}

	// Require authentication
	if opts.RequireAuth && (cfg.Username == "" || cfg.Password == "") {
		return fmt.Errorf("authentication credentials are required in strict validation mode")
	}

	// Disallow insecure TLS
	if cfg.TLSInsecureSkipVerify && !opts.AllowInsecureTLS {
		return fmt.Errorf("insecure TLS verification is not allowed in strict validation mode")
	}

	// Validate consistency levels for production
	weakConsistencyLevels := []string{"ANY", "ONE"}
	for _, weak := range weakConsistencyLevels {
		if cfg.Consistency == weak && !opts.AllowWeakConsistency {
			return fmt.Errorf("consistency level %s is not recommended for production", weak)
		}
	}

	// Validate connection pool size for production
	if cfg.NumConns < 5 {
		return fmt.Errorf("num_conns should be at least 5 for production (got %d)", cfg.NumConns)
	}

	// Validate timeout values for production
	if cfg.Timeout < 10*time.Second {
		return fmt.Errorf("timeout should be at least 10s for production (got %v)", cfg.Timeout)
	}

	return nil
}

// validateDevelopment performs relaxed validation for development environments
func (cfg *Config) validateDevelopment(opts *ValidationOptions) error {
	// In development mode, we're more lenient
	// Just warn about potentially problematic settings

	if cfg.TLSEnabled && cfg.TLSInsecureSkipVerify {
		// This is allowed in development but we could log a warning
		// For now, just allow it
	}

	if cfg.Consistency == "ANY" || cfg.Consistency == "ONE" {
		// Weak consistency is allowed in development
	}

	return nil
}

// validateHosts validates the host configuration
func (cfg *Config) validateHosts(opts *ValidationOptions) error {
	if len(cfg.Hosts) == 0 {
		return fmt.Errorf("at least one host must be specified")
	}

	if len(cfg.Hosts) < opts.MinHosts {
		return fmt.Errorf("at least %d hosts are required (got %d)", opts.MinHosts, len(cfg.Hosts))
	}

	if len(cfg.Hosts) > opts.MaxHosts {
		return fmt.Errorf("at most %d hosts are allowed (got %d)", opts.MaxHosts, len(cfg.Hosts))
	}

	// Validate each host
	for i, host := range cfg.Hosts {
		if err := cfg.validateHost(host); err != nil {
			return fmt.Errorf("invalid host at index %d: %w", i, err)
		}
	}

	// Check for duplicate hosts
	hostSet := make(map[string]bool)
	for _, host := range cfg.Hosts {
		if hostSet[host] {
			return fmt.Errorf("duplicate host found: %s", host)
		}
		hostSet[host] = true
	}

	return nil
}

// validateHost validates a single host address
func (cfg *Config) validateHost(host string) error {
	if host == "" {
		return fmt.Errorf("host cannot be empty")
	}

	// Check if it's a valid IP address
	if ip := net.ParseIP(host); ip != nil {
		return nil
	}

	// Check if it's a valid hostname
	if err := cfg.validateHostname(host); err != nil {
		return err
	}

	return nil
}

// validateHostname validates a hostname
func (cfg *Config) validateHostname(hostname string) error {
	if len(hostname) > 253 {
		return fmt.Errorf("hostname too long: %d characters (max 253)", len(hostname))
	}

	if hostname[0] == '.' || hostname[len(hostname)-1] == '.' {
		return fmt.Errorf("hostname cannot start or end with a dot")
	}

	// Split into labels and validate each
	labels := strings.Split(hostname, ".")
	for _, label := range labels {
		if err := cfg.validateHostnameLabel(label); err != nil {
			return fmt.Errorf("invalid hostname label '%s': %w", label, err)
		}
	}

	return nil
}

// validateHostnameLabel validates a single hostname label
func (cfg *Config) validateHostnameLabel(label string) error {
	if len(label) == 0 {
		return fmt.Errorf("label cannot be empty")
	}

	if len(label) > 63 {
		return fmt.Errorf("label too long: %d characters (max 63)", len(label))
	}

	if label[0] == '-' || label[len(label)-1] == '-' {
		return fmt.Errorf("label cannot start or end with hyphen")
	}

	// Check characters
	for _, r := range label {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-') {
			return fmt.Errorf("invalid character in label: %c", r)
		}
	}

	return nil
}

// validateKeyspace validates the keyspace name
func (cfg *Config) validateKeyspace() error {
	if cfg.Keyspace == "" {
		return fmt.Errorf("keyspace cannot be empty")
	}

	if len(cfg.Keyspace) > 48 {
		return fmt.Errorf("keyspace name too long: %d characters (max 48)", len(cfg.Keyspace))
	}

	// Keyspace names must start with a letter or underscore
	if !((cfg.Keyspace[0] >= 'a' && cfg.Keyspace[0] <= 'z') ||
		(cfg.Keyspace[0] >= 'A' && cfg.Keyspace[0] <= 'Z') ||
		cfg.Keyspace[0] == '_') {
		return fmt.Errorf("keyspace name must start with a letter or underscore")
	}

	// Check remaining characters
	for i, r := range cfg.Keyspace {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return fmt.Errorf("invalid character in keyspace name at position %d: %c", i, r)
		}
	}

	// Reserved keyspace names
	reservedKeyspaces := []string{"system", "system_schema", "system_auth", "system_distributed", "system_traces"}
	for _, reserved := range reservedKeyspaces {
		if strings.EqualFold(cfg.Keyspace, reserved) {
			return fmt.Errorf("keyspace name '%s' is reserved", cfg.Keyspace)
		}
	}

	return nil
}

// validateTimeouts validates timeout configurations
func (cfg *Config) validateTimeouts() error {
	if cfg.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive (got %v)", cfg.Timeout)
	}

	if cfg.ConnectTimeout <= 0 {
		return fmt.Errorf("connect_timeout must be positive (got %v)", cfg.ConnectTimeout)
	}

	if cfg.BatchTimeout <= 0 {
		return fmt.Errorf("batch_timeout must be positive (got %v)", cfg.BatchTimeout)
	}

	// Reasonable upper bounds
	maxTimeout := 5 * time.Minute
	if cfg.Timeout > maxTimeout {
		return fmt.Errorf("timeout too large: %v (max %v)", cfg.Timeout, maxTimeout)
	}

	if cfg.ConnectTimeout > maxTimeout {
		return fmt.Errorf("connect_timeout too large: %v (max %v)", cfg.ConnectTimeout, maxTimeout)
	}

	// Connect timeout should generally be less than or equal to operation timeout
	if cfg.ConnectTimeout > cfg.Timeout {
		return fmt.Errorf("connect_timeout (%v) should not be greater than timeout (%v)", cfg.ConnectTimeout, cfg.Timeout)
	}

	return nil
}

// validateConsistencyLevels validates consistency level settings
func (cfg *Config) validateConsistencyLevels(opts *ValidationOptions) error {
	validConsistency := map[string]bool{
		"ANY":          true,
		"ONE":          true,
		"TWO":          true,
		"THREE":        true,
		"QUORUM":       true,
		"ALL":          true,
		"LOCAL_QUORUM": true,
		"EACH_QUORUM":  true,
		"LOCAL_ONE":    true,
	}

	if !validConsistency[cfg.Consistency] {
		return fmt.Errorf("invalid consistency level: %s", cfg.Consistency)
	}

	validSerialConsistency := map[string]bool{
		"SERIAL":       true,
		"LOCAL_SERIAL": true,
	}

	if !validSerialConsistency[cfg.SerialConsistency] {
		return fmt.Errorf("invalid serial consistency level: %s", cfg.SerialConsistency)
	}

	return nil
}

// validateBatchSettings validates batch configuration
func (cfg *Config) validateBatchSettings() error {
	if cfg.BatchSize <= 0 {
		return fmt.Errorf("batch_size must be positive (got %d)", cfg.BatchSize)
	}

	if cfg.BatchSize > 10000 {
		return fmt.Errorf("batch_size too large: %d (max 10000)", cfg.BatchSize)
	}

	return nil
}

// validateTLSSettings validates TLS configuration
func (cfg *Config) validateTLSSettings(opts *ValidationOptions) error {
	if !cfg.TLSEnabled {
		return nil
	}

	// If TLS is enabled and cert/key files are specified, validate they exist
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		// File existence is checked in the main Validate() method
		// Here we just validate the configuration consistency
		if cfg.TLSCertFile == cfg.TLSKeyFile {
			return fmt.Errorf("TLS cert file and key file cannot be the same")
		}
	}

	// Validate that if one of cert/key is specified, both must be specified
	if (cfg.TLSCertFile != "") != (cfg.TLSKeyFile != "") {
		return fmt.Errorf("both TLS cert file and key file must be specified together")
	}

	return nil
}

// GetValidationSummary returns a summary of the configuration validation
func (cfg *Config) GetValidationSummary() map[string]interface{} {
	summary := make(map[string]interface{})

	summary["hosts_count"] = len(cfg.Hosts)
	summary["tls_enabled"] = cfg.TLSEnabled
	summary["auth_configured"] = cfg.Username != "" && cfg.Password != ""
	summary["multi_dc"] = cfg.IsMultiDC()
	summary["consistency_level"] = cfg.Consistency
	summary["serial_consistency_level"] = cfg.SerialConsistency
	summary["connection_pool_size"] = cfg.NumConns
	summary["timeout"] = cfg.Timeout.String()
	summary["connect_timeout"] = cfg.ConnectTimeout.String()
	summary["batch_size"] = cfg.BatchSize
	summary["metrics_enabled"] = cfg.MetricsEnabled
	summary["tracing_enabled"] = cfg.TracingEnabled

	// Security assessment
	securityScore := 0
	securityIssues := []string{}

	if cfg.TLSEnabled {
		securityScore += 30
	} else {
		securityIssues = append(securityIssues, "TLS not enabled")
	}

	if cfg.Username != "" && cfg.Password != "" {
		securityScore += 25
	} else {
		securityIssues = append(securityIssues, "No authentication configured")
	}

	if cfg.TLSEnabled && !cfg.TLSInsecureSkipVerify {
		securityScore += 20
	} else if cfg.TLSEnabled && cfg.TLSInsecureSkipVerify {
		securityIssues = append(securityIssues, "TLS certificate verification disabled")
	}

	if cfg.Consistency != "ANY" && cfg.Consistency != "ONE" {
		securityScore += 15
	} else {
		securityIssues = append(securityIssues, "Weak consistency level")
	}

	if len(cfg.Hosts) >= 3 {
		securityScore += 10
	} else {
		securityIssues = append(securityIssues, "Insufficient hosts for high availability")
	}

	summary["security_score"] = securityScore
	summary["security_issues"] = securityIssues

	// Performance assessment
	perfScore := 0
	perfIssues := []string{}

	if cfg.NumConns >= 5 {
		perfScore += 25
	} else {
		perfIssues = append(perfIssues, "Low connection pool size")
	}

	if cfg.Timeout >= 10*time.Second {
		perfScore += 25
	} else {
		perfIssues = append(perfIssues, "Short timeout may cause failures under load")
	}

	if cfg.BatchSize >= 100 && cfg.BatchSize <= 1000 {
		perfScore += 25
	} else if cfg.BatchSize < 100 {
		perfIssues = append(perfIssues, "Small batch size may reduce throughput")
	} else {
		perfIssues = append(perfIssues, "Large batch size may cause timeouts")
	}

	if cfg.TokenAwareRouting {
		perfScore += 25
	} else {
		perfIssues = append(perfIssues, "Token-aware routing not enabled")
	}

	summary["performance_score"] = perfScore
	summary["performance_issues"] = perfIssues

	return summary
}

// ValidateAndSummarize validates the configuration and returns a detailed summary
func (cfg *Config) ValidateAndSummarize(opts *ValidationOptions) (map[string]interface{}, error) {
	summary := cfg.GetValidationSummary()

	err := cfg.ValidateWithOptions(opts)
	if err != nil {
		summary["validation_error"] = err.Error()
		summary["validation_passed"] = false
	} else {
		summary["validation_passed"] = true
	}

	return summary, err
}
