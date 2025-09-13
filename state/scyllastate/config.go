package scyllastate

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gocql/gocql"
	"github.com/ipfs-cluster/ipfs-cluster/config"
)

const (
	// DefaultConfigKey is the key used to identify this component in the configuration
	DefaultConfigKey = "scylladb"

	// Default values
	DefaultPort              = 9042
	DefaultKeyspace          = "ipfs_pins"
	DefaultNumConns          = 10
	DefaultTimeout           = 30 * time.Second
	DefaultConnectTimeout    = 10 * time.Second
	DefaultConsistency       = "QUORUM"
	DefaultSerialConsistency = "SERIAL"
	DefaultBatchSize         = 1000
	DefaultBatchTimeout      = 1 * time.Second
	DefaultRetryNumRetries   = 3
	DefaultRetryMinDelay     = 100 * time.Millisecond
	DefaultRetryMaxDelay     = 10 * time.Second

	// Advanced connection pool defaults
	DefaultMaxConnectionsPerHost = 20
	DefaultMinConnectionsPerHost = 2
	DefaultMaxWaitTime           = 10 * time.Second
	DefaultKeepAlive             = 30 * time.Second

	// Query optimization defaults
	DefaultPreparedStatementCacheSize = 1000
	DefaultPageSize                   = 5000

	// Connection behavior defaults
	DefaultReconnectInterval    = 60 * time.Second
	DefaultMaxReconnectAttempts = 3
)

// Config holds configuration for ScyllaDB state store
type Config struct {
	config.Saver

	// Connection settings
	Hosts    []string `json:"hosts"`
	Port     int      `json:"port"`
	Keyspace string   `json:"keyspace"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`

	// TLS settings
	TLSEnabled            bool   `json:"tls_enabled"`
	TLSCertFile           string `json:"tls_cert_file,omitempty"`
	TLSKeyFile            string `json:"tls_key_file,omitempty"`
	TLSCAFile             string `json:"tls_ca_file,omitempty"`
	TLSInsecureSkipVerify bool   `json:"tls_insecure_skip_verify"`

	// Performance settings
	NumConns       int           `json:"num_conns"`
	Timeout        time.Duration `json:"timeout"`
	ConnectTimeout time.Duration `json:"connect_timeout"`

	// Consistency settings
	Consistency       string `json:"consistency"`
	SerialConsistency string `json:"serial_consistency"`

	// Retry policy settings
	RetryPolicy RetryPolicyConfig `json:"retry_policy"`

	// Batching settings
	BatchSize    int           `json:"batch_size"`
	BatchTimeout time.Duration `json:"batch_timeout"`

	// Monitoring settings
	MetricsEnabled bool `json:"metrics_enabled"`
	TracingEnabled bool `json:"tracing_enabled"`

	// Multi-DC settings
	LocalDC           string `json:"local_dc,omitempty"`
	DCAwareRouting    bool   `json:"dc_aware_routing"`
	TokenAwareRouting bool   `json:"token_aware_routing"`

	// Advanced connection pool settings
	MaxConnectionsPerHost int           `json:"max_connections_per_host"`
	MinConnectionsPerHost int           `json:"min_connections_per_host"`
	MaxWaitTime           time.Duration `json:"max_wait_time"`
	KeepAlive             time.Duration `json:"keep_alive"`

	// Query optimization settings
	PreparedStatementCacheSize int  `json:"prepared_statement_cache_size"`
	DisableSkipMetadata        bool `json:"disable_skip_metadata"`
	PageSize                   int  `json:"page_size"`

	// Connection behavior settings
	ReconnectInterval        time.Duration `json:"reconnect_interval"`
	MaxReconnectAttempts     int           `json:"max_reconnect_attempts"`
	IgnorePeerAddr           bool          `json:"ignore_peer_addr"`
	DisableInitialHostLookup bool          `json:"disable_initial_host_lookup"`
}

// configJSON represents the JSON serialization format
type configJSON struct {
	Hosts    []string `json:"hosts"`
	Port     int      `json:"port"`
	Keyspace string   `json:"keyspace"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`

	TLSEnabled            bool   `json:"tls_enabled"`
	TLSCertFile           string `json:"tls_cert_file,omitempty"`
	TLSKeyFile            string `json:"tls_key_file,omitempty"`
	TLSCAFile             string `json:"tls_ca_file,omitempty"`
	TLSInsecureSkipVerify bool   `json:"tls_insecure_skip_verify"`

	NumConns       int    `json:"num_conns"`
	Timeout        string `json:"timeout"`
	ConnectTimeout string `json:"connect_timeout"`

	Consistency       string `json:"consistency"`
	SerialConsistency string `json:"serial_consistency"`

	RetryPolicy retryPolicyConfigJSON `json:"retry_policy"`

	BatchSize    int    `json:"batch_size"`
	BatchTimeout string `json:"batch_timeout"`

	MetricsEnabled bool `json:"metrics_enabled"`
	TracingEnabled bool `json:"tracing_enabled"`

	LocalDC           string `json:"local_dc,omitempty"`
	DCAwareRouting    bool   `json:"dc_aware_routing"`
	TokenAwareRouting bool   `json:"token_aware_routing"`

	MaxConnectionsPerHost string `json:"max_connections_per_host"`
	MinConnectionsPerHost string `json:"min_connections_per_host"`
	MaxWaitTime           string `json:"max_wait_time"`
	KeepAlive             string `json:"keep_alive"`

	PreparedStatementCacheSize string `json:"prepared_statement_cache_size"`
	DisableSkipMetadata        bool   `json:"disable_skip_metadata"`
	PageSize                   string `json:"page_size"`

	ReconnectInterval        string `json:"reconnect_interval"`
	MaxReconnectAttempts     string `json:"max_reconnect_attempts"`
	IgnorePeerAddr           bool   `json:"ignore_peer_addr"`
	DisableInitialHostLookup bool   `json:"disable_initial_host_lookup"`
}

// retryPolicyConfigJSON represents retry policy in JSON format
type retryPolicyConfigJSON struct {
	NumRetries    int    `json:"num_retries"`
	MinRetryDelay string `json:"min_retry_delay"`
	MaxRetryDelay string `json:"max_retry_delay"`
}

// ConfigKey returns the key used to identify this component in the configuration
func (cfg *Config) ConfigKey() string {
	return DefaultConfigKey
}

// Default sets default values for the configuration
func (cfg *Config) Default() error {
	cfg.Hosts = []string{"127.0.0.1"}
	cfg.Port = DefaultPort
	cfg.Keyspace = DefaultKeyspace
	cfg.Username = ""
	cfg.Password = ""

	cfg.TLSEnabled = false
	cfg.TLSCertFile = ""
	cfg.TLSKeyFile = ""
	cfg.TLSCAFile = ""
	cfg.TLSInsecureSkipVerify = false

	cfg.NumConns = DefaultNumConns
	cfg.Timeout = DefaultTimeout
	cfg.ConnectTimeout = DefaultConnectTimeout

	cfg.Consistency = DefaultConsistency
	cfg.SerialConsistency = DefaultSerialConsistency

	cfg.RetryPolicy = RetryPolicyConfig{
		NumRetries:    DefaultRetryNumRetries,
		MinRetryDelay: DefaultRetryMinDelay,
		MaxRetryDelay: DefaultRetryMaxDelay,
	}

	cfg.BatchSize = DefaultBatchSize
	cfg.BatchTimeout = DefaultBatchTimeout

	cfg.MetricsEnabled = true
	cfg.TracingEnabled = false

	cfg.LocalDC = ""
	cfg.DCAwareRouting = false
	cfg.TokenAwareRouting = false

	// Advanced connection pool settings
	cfg.MaxConnectionsPerHost = DefaultMaxConnectionsPerHost
	cfg.MinConnectionsPerHost = DefaultMinConnectionsPerHost
	cfg.MaxWaitTime = DefaultMaxWaitTime
	cfg.KeepAlive = DefaultKeepAlive

	// Query optimization settings
	cfg.PreparedStatementCacheSize = DefaultPreparedStatementCacheSize
	cfg.DisableSkipMetadata = false
	cfg.PageSize = DefaultPageSize

	// Connection behavior settings
	cfg.ReconnectInterval = DefaultReconnectInterval
	cfg.MaxReconnectAttempts = DefaultMaxReconnectAttempts
	cfg.IgnorePeerAddr = false
	cfg.DisableInitialHostLookup = false

	return nil
}

// ApplyEnvVars applies environment variable overrides
func (cfg *Config) ApplyEnvVars() error {
	// SCYLLADB_HOSTS - comma-separated list of hosts
	if hosts := os.Getenv("SCYLLADB_HOSTS"); hosts != "" {
		cfg.Hosts = strings.Split(hosts, ",")
		for i := range cfg.Hosts {
			cfg.Hosts[i] = strings.TrimSpace(cfg.Hosts[i])
		}
	}

	// SCYLLADB_KEYSPACE
	if keyspace := os.Getenv("SCYLLADB_KEYSPACE"); keyspace != "" {
		cfg.Keyspace = keyspace
	}

	// SCYLLADB_USERNAME
	if username := os.Getenv("SCYLLADB_USERNAME"); username != "" {
		cfg.Username = username
	}

	// SCYLLADB_PASSWORD
	if password := os.Getenv("SCYLLADB_PASSWORD"); password != "" {
		cfg.Password = password
	}

	// SCYLLADB_TLS_ENABLED
	if tlsEnabled := os.Getenv("SCYLLADB_TLS_ENABLED"); tlsEnabled != "" {
		cfg.TLSEnabled = strings.ToLower(tlsEnabled) == "true"
	}

	// SCYLLADB_TLS_CERT_FILE
	if certFile := os.Getenv("SCYLLADB_TLS_CERT_FILE"); certFile != "" {
		cfg.TLSCertFile = certFile
	}

	// SCYLLADB_TLS_KEY_FILE
	if keyFile := os.Getenv("SCYLLADB_TLS_KEY_FILE"); keyFile != "" {
		cfg.TLSKeyFile = keyFile
	}

	// SCYLLADB_TLS_CA_FILE
	if caFile := os.Getenv("SCYLLADB_TLS_CA_FILE"); caFile != "" {
		cfg.TLSCAFile = caFile
	}

	// SCYLLADB_LOCAL_DC
	if localDC := os.Getenv("SCYLLADB_LOCAL_DC"); localDC != "" {
		cfg.LocalDC = localDC
		cfg.DCAwareRouting = true // Enable DC-aware routing when local DC is set
	}

	return nil
}

// Validate checks the configuration for errors
func (cfg *Config) Validate() error {
	if len(cfg.Hosts) == 0 {
		return errors.New("at least one host must be specified")
	}

	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("invalid port: %d", cfg.Port)
	}

	if cfg.Keyspace == "" {
		return errors.New("keyspace cannot be empty")
	}

	if cfg.NumConns <= 0 {
		return fmt.Errorf("num_conns must be positive, got: %d", cfg.NumConns)
	}

	if cfg.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive, got: %v", cfg.Timeout)
	}

	if cfg.ConnectTimeout <= 0 {
		return fmt.Errorf("connect_timeout must be positive, got: %v", cfg.ConnectTimeout)
	}

	// Validate consistency levels
	if err := cfg.validateConsistency(); err != nil {
		return err
	}

	// Validate retry policy
	if err := cfg.validateRetryPolicy(); err != nil {
		return err
	}

	// Validate batch settings
	if cfg.BatchSize <= 0 {
		return fmt.Errorf("batch_size must be positive, got: %d", cfg.BatchSize)
	}

	if cfg.BatchTimeout <= 0 {
		return fmt.Errorf("batch_timeout must be positive, got: %v", cfg.BatchTimeout)
	}

	// Validate TLS settings
	if err := cfg.validateTLS(); err != nil {
		return err
	}

	// Validate connection pool settings
	if err := cfg.validateConnectionPool(); err != nil {
		return err
	}

	// Validate query optimization settings
	if err := cfg.validateQueryOptimization(); err != nil {
		return err
	}

	return nil
}

// validateConsistency validates consistency level settings
func (cfg *Config) validateConsistency() error {
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

// validateRetryPolicy validates retry policy settings
func (cfg *Config) validateRetryPolicy() error {
	if cfg.RetryPolicy.NumRetries < 0 {
		return fmt.Errorf("retry num_retries must be non-negative, got: %d", cfg.RetryPolicy.NumRetries)
	}

	if cfg.RetryPolicy.MinRetryDelay <= 0 {
		return fmt.Errorf("retry min_retry_delay must be positive, got: %v", cfg.RetryPolicy.MinRetryDelay)
	}

	if cfg.RetryPolicy.MaxRetryDelay <= 0 {
		return fmt.Errorf("retry max_retry_delay must be positive, got: %v", cfg.RetryPolicy.MaxRetryDelay)
	}

	if cfg.RetryPolicy.MinRetryDelay > cfg.RetryPolicy.MaxRetryDelay {
		return fmt.Errorf("retry min_retry_delay (%v) cannot be greater than max_retry_delay (%v)",
			cfg.RetryPolicy.MinRetryDelay, cfg.RetryPolicy.MaxRetryDelay)
	}

	return nil
}

// validateTLS validates TLS configuration
func (cfg *Config) validateTLS() error {
	if !cfg.TLSEnabled {
		return nil
	}

	// If TLS is enabled and cert/key files are specified, validate they exist
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		if _, err := os.Stat(cfg.TLSCertFile); os.IsNotExist(err) {
			return fmt.Errorf("TLS cert file does not exist: %s", cfg.TLSCertFile)
		}

		if _, err := os.Stat(cfg.TLSKeyFile); os.IsNotExist(err) {
			return fmt.Errorf("TLS key file does not exist: %s", cfg.TLSKeyFile)
		}

		// Try to load the certificate to validate it
		if _, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil {
			return fmt.Errorf("invalid TLS certificate/key pair: %w", err)
		}
	}

	// If CA file is specified, validate it exists and is readable
	if cfg.TLSCAFile != "" {
		if _, err := os.Stat(cfg.TLSCAFile); os.IsNotExist(err) {
			return fmt.Errorf("TLS CA file does not exist: %s", cfg.TLSCAFile)
		}

		// Try to read and parse the CA file
		caCert, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return fmt.Errorf("cannot read TLS CA file: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return fmt.Errorf("invalid TLS CA certificate in file: %s", cfg.TLSCAFile)
		}
	}

	return nil
}

// validateConnectionPool validates connection pool settings
func (cfg *Config) validateConnectionPool() error {
	if cfg.MaxConnectionsPerHost <= 0 {
		return fmt.Errorf("max_connections_per_host must be positive, got: %d", cfg.MaxConnectionsPerHost)
	}

	if cfg.MinConnectionsPerHost <= 0 {
		return fmt.Errorf("min_connections_per_host must be positive, got: %d", cfg.MinConnectionsPerHost)
	}

	if cfg.MinConnectionsPerHost > cfg.MaxConnectionsPerHost {
		return fmt.Errorf("min_connections_per_host (%d) cannot be greater than max_connections_per_host (%d)",
			cfg.MinConnectionsPerHost, cfg.MaxConnectionsPerHost)
	}

	if cfg.MaxWaitTime <= 0 {
		return fmt.Errorf("max_wait_time must be positive, got: %v", cfg.MaxWaitTime)
	}

	if cfg.KeepAlive <= 0 {
		return fmt.Errorf("keep_alive must be positive, got: %v", cfg.KeepAlive)
	}

	if cfg.ReconnectInterval <= 0 {
		return fmt.Errorf("reconnect_interval must be positive, got: %v", cfg.ReconnectInterval)
	}

	if cfg.MaxReconnectAttempts < 0 {
		return fmt.Errorf("max_reconnect_attempts must be non-negative, got: %d", cfg.MaxReconnectAttempts)
	}

	return nil
}

// validateQueryOptimization validates query optimization settings
func (cfg *Config) validateQueryOptimization() error {
	if cfg.PreparedStatementCacheSize <= 0 {
		return fmt.Errorf("prepared_statement_cache_size must be positive, got: %d", cfg.PreparedStatementCacheSize)
	}

	if cfg.PageSize <= 0 {
		return fmt.Errorf("page_size must be positive, got: %d", cfg.PageSize)
	}

	// Warn about very large page sizes that might cause memory issues
	if cfg.PageSize > 100000 {
		// In a real implementation, this might be logged as a warning
		// For now, we'll allow it but could add a warning mechanism
	}

	return nil
}

// LoadJSON loads configuration from JSON bytes
func (cfg *Config) LoadJSON(raw []byte) error {
	jcfg := &configJSON{}
	err := json.Unmarshal(raw, jcfg)
	if err != nil {
		return fmt.Errorf("error unmarshaling ScyllaDB config: %w", err)
	}

	cfg.Hosts = jcfg.Hosts
	cfg.Port = jcfg.Port
	cfg.Keyspace = jcfg.Keyspace
	cfg.Username = jcfg.Username
	cfg.Password = jcfg.Password

	cfg.TLSEnabled = jcfg.TLSEnabled
	cfg.TLSCertFile = jcfg.TLSCertFile
	cfg.TLSKeyFile = jcfg.TLSKeyFile
	cfg.TLSCAFile = jcfg.TLSCAFile
	cfg.TLSInsecureSkipVerify = jcfg.TLSInsecureSkipVerify

	cfg.NumConns = jcfg.NumConns
	cfg.Consistency = jcfg.Consistency
	cfg.SerialConsistency = jcfg.SerialConsistency

	cfg.RetryPolicy.NumRetries = jcfg.RetryPolicy.NumRetries

	cfg.BatchSize = jcfg.BatchSize
	cfg.MetricsEnabled = jcfg.MetricsEnabled
	cfg.TracingEnabled = jcfg.TracingEnabled

	cfg.LocalDC = jcfg.LocalDC
	cfg.DCAwareRouting = jcfg.DCAwareRouting
	cfg.TokenAwareRouting = jcfg.TokenAwareRouting

	cfg.DisableSkipMetadata = jcfg.DisableSkipMetadata
	cfg.IgnorePeerAddr = jcfg.IgnorePeerAddr
	cfg.DisableInitialHostLookup = jcfg.DisableInitialHostLookup

	// Parse duration fields
	if jcfg.Timeout != "" {
		timeout, err := time.ParseDuration(jcfg.Timeout)
		if err != nil {
			return fmt.Errorf("invalid timeout duration: %w", err)
		}
		cfg.Timeout = timeout
	}

	if jcfg.ConnectTimeout != "" {
		connectTimeout, err := time.ParseDuration(jcfg.ConnectTimeout)
		if err != nil {
			return fmt.Errorf("invalid connect_timeout duration: %w", err)
		}
		cfg.ConnectTimeout = connectTimeout
	}

	if jcfg.BatchTimeout != "" {
		batchTimeout, err := time.ParseDuration(jcfg.BatchTimeout)
		if err != nil {
			return fmt.Errorf("invalid batch_timeout duration: %w", err)
		}
		cfg.BatchTimeout = batchTimeout
	}

	if jcfg.RetryPolicy.MinRetryDelay != "" {
		minDelay, err := time.ParseDuration(jcfg.RetryPolicy.MinRetryDelay)
		if err != nil {
			return fmt.Errorf("invalid retry min_retry_delay duration: %w", err)
		}
		cfg.RetryPolicy.MinRetryDelay = minDelay
	}

	if jcfg.RetryPolicy.MaxRetryDelay != "" {
		maxDelay, err := time.ParseDuration(jcfg.RetryPolicy.MaxRetryDelay)
		if err != nil {
			return fmt.Errorf("invalid retry max_retry_delay duration: %w", err)
		}
		cfg.RetryPolicy.MaxRetryDelay = maxDelay
	}

	// Parse new connection pool fields
	if jcfg.MaxConnectionsPerHost != "" {
		maxConns, err := strconv.Atoi(jcfg.MaxConnectionsPerHost)
		if err != nil {
			return fmt.Errorf("invalid max_connections_per_host: %w", err)
		}
		cfg.MaxConnectionsPerHost = maxConns
	}

	if jcfg.MinConnectionsPerHost != "" {
		minConns, err := strconv.Atoi(jcfg.MinConnectionsPerHost)
		if err != nil {
			return fmt.Errorf("invalid min_connections_per_host: %w", err)
		}
		cfg.MinConnectionsPerHost = minConns
	}

	if jcfg.MaxWaitTime != "" {
		maxWait, err := time.ParseDuration(jcfg.MaxWaitTime)
		if err != nil {
			return fmt.Errorf("invalid max_wait_time duration: %w", err)
		}
		cfg.MaxWaitTime = maxWait
	}

	if jcfg.KeepAlive != "" {
		keepAlive, err := time.ParseDuration(jcfg.KeepAlive)
		if err != nil {
			return fmt.Errorf("invalid keep_alive duration: %w", err)
		}
		cfg.KeepAlive = keepAlive
	}

	// Parse query optimization fields
	if jcfg.PreparedStatementCacheSize != "" {
		cacheSize, err := strconv.Atoi(jcfg.PreparedStatementCacheSize)
		if err != nil {
			return fmt.Errorf("invalid prepared_statement_cache_size: %w", err)
		}
		cfg.PreparedStatementCacheSize = cacheSize
	}

	if jcfg.PageSize != "" {
		pageSize, err := strconv.Atoi(jcfg.PageSize)
		if err != nil {
			return fmt.Errorf("invalid page_size: %w", err)
		}
		cfg.PageSize = pageSize
	}

	// Parse connection behavior fields
	if jcfg.ReconnectInterval != "" {
		reconnectInterval, err := time.ParseDuration(jcfg.ReconnectInterval)
		if err != nil {
			return fmt.Errorf("invalid reconnect_interval duration: %w", err)
		}
		cfg.ReconnectInterval = reconnectInterval
	}

	if jcfg.MaxReconnectAttempts != "" {
		maxReconnect, err := strconv.Atoi(jcfg.MaxReconnectAttempts)
		if err != nil {
			return fmt.Errorf("invalid max_reconnect_attempts: %w", err)
		}
		cfg.MaxReconnectAttempts = maxReconnect
	}

	return nil
}

// ToJSON returns the JSON representation of the configuration
func (cfg *Config) ToJSON() ([]byte, error) {
	jcfg := &configJSON{
		Hosts:                 cfg.Hosts,
		Port:                  cfg.Port,
		Keyspace:              cfg.Keyspace,
		Username:              cfg.Username,
		Password:              cfg.Password,
		TLSEnabled:            cfg.TLSEnabled,
		TLSCertFile:           cfg.TLSCertFile,
		TLSKeyFile:            cfg.TLSKeyFile,
		TLSCAFile:             cfg.TLSCAFile,
		TLSInsecureSkipVerify: cfg.TLSInsecureSkipVerify,
		NumConns:              cfg.NumConns,
		Timeout:               cfg.Timeout.String(),
		ConnectTimeout:        cfg.ConnectTimeout.String(),
		Consistency:           cfg.Consistency,
		SerialConsistency:     cfg.SerialConsistency,
		RetryPolicy: retryPolicyConfigJSON{
			NumRetries:    cfg.RetryPolicy.NumRetries,
			MinRetryDelay: cfg.RetryPolicy.MinRetryDelay.String(),
			MaxRetryDelay: cfg.RetryPolicy.MaxRetryDelay.String(),
		},
		BatchSize:         cfg.BatchSize,
		BatchTimeout:      cfg.BatchTimeout.String(),
		MetricsEnabled:    cfg.MetricsEnabled,
		TracingEnabled:    cfg.TracingEnabled,
		LocalDC:           cfg.LocalDC,
		DCAwareRouting:    cfg.DCAwareRouting,
		TokenAwareRouting: cfg.TokenAwareRouting,

		MaxConnectionsPerHost:      strconv.Itoa(cfg.MaxConnectionsPerHost),
		MinConnectionsPerHost:      strconv.Itoa(cfg.MinConnectionsPerHost),
		MaxWaitTime:                cfg.MaxWaitTime.String(),
		KeepAlive:                  cfg.KeepAlive.String(),
		PreparedStatementCacheSize: strconv.Itoa(cfg.PreparedStatementCacheSize),
		DisableSkipMetadata:        cfg.DisableSkipMetadata,
		PageSize:                   strconv.Itoa(cfg.PageSize),
		ReconnectInterval:          cfg.ReconnectInterval.String(),
		MaxReconnectAttempts:       strconv.Itoa(cfg.MaxReconnectAttempts),
		IgnorePeerAddr:             cfg.IgnorePeerAddr,
		DisableInitialHostLookup:   cfg.DisableInitialHostLookup,
	}

	return config.DefaultJSONMarshal(jcfg)
}

// ToDisplayJSON returns the JSON representation for display (hiding sensitive fields)
func (cfg *Config) ToDisplayJSON() ([]byte, error) {
	jcfg := &configJSON{
		Hosts:                 cfg.Hosts,
		Port:                  cfg.Port,
		Keyspace:              cfg.Keyspace,
		Username:              cfg.Username,
		Password:              "<hidden>", // Hide password in display
		TLSEnabled:            cfg.TLSEnabled,
		TLSCertFile:           cfg.TLSCertFile,
		TLSKeyFile:            cfg.TLSKeyFile,
		TLSCAFile:             cfg.TLSCAFile,
		TLSInsecureSkipVerify: cfg.TLSInsecureSkipVerify,
		NumConns:              cfg.NumConns,
		Timeout:               cfg.Timeout.String(),
		ConnectTimeout:        cfg.ConnectTimeout.String(),
		Consistency:           cfg.Consistency,
		SerialConsistency:     cfg.SerialConsistency,
		RetryPolicy: retryPolicyConfigJSON{
			NumRetries:    cfg.RetryPolicy.NumRetries,
			MinRetryDelay: cfg.RetryPolicy.MinRetryDelay.String(),
			MaxRetryDelay: cfg.RetryPolicy.MaxRetryDelay.String(),
		},
		BatchSize:         cfg.BatchSize,
		BatchTimeout:      cfg.BatchTimeout.String(),
		MetricsEnabled:    cfg.MetricsEnabled,
		TracingEnabled:    cfg.TracingEnabled,
		LocalDC:           cfg.LocalDC,
		DCAwareRouting:    cfg.DCAwareRouting,
		TokenAwareRouting: cfg.TokenAwareRouting,
	}

	// Hide password if it's set
	if cfg.Password != "" {
		jcfg.Password = "<hidden>"
	}

	return config.DefaultJSONMarshal(jcfg)
}

// CreateTLSConfig creates a TLS configuration from the config settings
func (cfg *Config) CreateTLSConfig() (*tls.Config, error) {
	if !cfg.TLSEnabled {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
	}

	// Load client certificate if specified
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate if specified
	if cfg.TLSCAFile != "" {
		caCert, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

// CreateAuthenticator creates a gocql authenticator from the config settings
func (cfg *Config) CreateAuthenticator() gocql.Authenticator {
	if cfg.Username != "" && cfg.Password != "" {
		return gocql.PasswordAuthenticator{
			Username: cfg.Username,
			Password: cfg.Password,
		}
	}
	return nil
}

// GetConsistency returns the gocql.Consistency value for the configured consistency level
func (cfg *Config) GetConsistency() gocql.Consistency {
	switch cfg.Consistency {
	case "ANY":
		return gocql.Any
	case "ONE":
		return gocql.One
	case "TWO":
		return gocql.Two
	case "THREE":
		return gocql.Three
	case "QUORUM":
		return gocql.Quorum
	case "ALL":
		return gocql.All
	case "LOCAL_QUORUM":
		return gocql.LocalQuorum
	case "EACH_QUORUM":
		return gocql.EachQuorum
	case "LOCAL_ONE":
		return gocql.LocalOne
	default:
		return gocql.Quorum // Default fallback
	}
}

// GetSerialConsistency returns the gocql.SerialConsistency value for the configured serial consistency level
func (cfg *Config) GetSerialConsistency() gocql.SerialConsistency {
	switch cfg.SerialConsistency {
	case "SERIAL":
		return gocql.Serial
	case "LOCAL_SERIAL":
		return gocql.LocalSerial
	default:
		return gocql.Serial // Default fallback
	}
}

// CreateClusterConfig creates a gocql.ClusterConfig from this configuration
func (cfg *Config) CreateClusterConfig() (*gocql.ClusterConfig, error) {
	cluster := gocql.NewCluster(cfg.Hosts...)
	cluster.Port = cfg.Port
	cluster.Keyspace = cfg.Keyspace
	cluster.NumConns = cfg.NumConns
	cluster.Timeout = cfg.Timeout
	cluster.ConnectTimeout = cfg.ConnectTimeout
	cluster.Consistency = cfg.GetConsistency()
	cluster.SerialConsistency = cfg.GetSerialConsistency()

	// Set authenticator if credentials are provided
	if auth := cfg.CreateAuthenticator(); auth != nil {
		cluster.Authenticator = auth
	}

	// Configure TLS if enabled
	if cfg.TLSEnabled {
		tlsConfig, err := cfg.CreateTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config: %w", err)
		}
		cluster.SslOpts = &gocql.SslOptions{
			Config: tlsConfig,
		}
	}

	// Configure advanced connection pool settings for optimal performance
	cfg.configureConnectionPool(cluster)

	// Configure host selection policy for multi-DC setups with token-aware and DC-aware routing
	cfg.configureHostSelectionPolicy(cluster)

	// Configure query optimization settings
	cfg.configureQueryOptimization(cluster)

	// Configure connection behavior settings
	cfg.configureConnectionBehavior(cluster)

	return cluster, nil
}

// configureConnectionPool sets up optimal connection pool settings
func (cfg *Config) configureConnectionPool(cluster *gocql.ClusterConfig) {
	// Configure connection pool settings for high performance
	cluster.PoolConfig.HostSelectionPolicy = gocql.RoundRobinHostPolicy() // Default, will be overridden by host selection policy

	// Set connection limits per host for optimal resource utilization
	// These settings help balance connection overhead vs parallelism
	if cfg.MaxConnectionsPerHost > 0 {
		// Note: gocql doesn't directly expose MaxConnectionsPerHost in PoolConfig
		// We use NumConns as the total connections and distribute across hosts
		cluster.NumConns = cfg.MaxConnectionsPerHost
	}

	// Configure timeouts for connection management
	cluster.ConnectTimeout = cfg.ConnectTimeout
	cluster.Timeout = cfg.Timeout

	// Configure keep-alive settings to maintain persistent connections
	// This helps reduce connection establishment overhead
	if cfg.KeepAlive > 0 {
		// Note: gocql uses TCP keep-alive internally, we ensure reasonable timeout
		if cluster.Timeout < cfg.KeepAlive {
			cluster.Timeout = cfg.KeepAlive
		}
	}

	// Configure reconnection behavior for resilience
	cluster.ReconnectInterval = cfg.ReconnectInterval

	// Disable initial host lookup if configured (useful for containerized environments)
	cluster.DisableInitialHostLookup = cfg.DisableInitialHostLookup

	// Configure peer address handling for NAT/proxy environments
	cluster.IgnorePeerAddr = cfg.IgnorePeerAddr
}

// configureHostSelectionPolicy sets up token-aware and DC-aware routing for optimal performance
func (cfg *Config) configureHostSelectionPolicy(cluster *gocql.ClusterConfig) {
	var basePolicy gocql.HostSelectionPolicy

	// Configure DC-aware routing for multi-datacenter deployments
	if cfg.DCAwareRouting && cfg.LocalDC != "" {
		// Use DC-aware policy to prefer local datacenter for reduced latency
		basePolicy = gocql.DCAwareRoundRobinPolicy(cfg.LocalDC)
	} else {
		// Use simple round-robin for single DC deployments
		basePolicy = gocql.RoundRobinHostPolicy()
	}

	// Wrap with token-aware policy for optimal data locality
	if cfg.TokenAwareRouting {
		// Token-aware routing ensures queries go to nodes that own the data
		// This significantly reduces network hops and improves performance
		cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(basePolicy)
	} else {
		cluster.PoolConfig.HostSelectionPolicy = basePolicy
	}
}

// configureQueryOptimization sets up query optimization settings
func (cfg *Config) configureQueryOptimization(cluster *gocql.ClusterConfig) {
	// Configure page size for efficient result streaming
	// Larger page sizes reduce round trips but increase memory usage
	cluster.PageSize = cfg.PageSize

	// Configure prepared statement caching
	// Note: gocql handles prepared statement caching internally
	// We ensure reasonable limits are set via configuration validation

	// Disable skip metadata if configured (useful for debugging)
	// When enabled, this can improve performance by skipping result metadata
	if !cfg.DisableSkipMetadata {
		// Note: gocql doesn't expose this directly, but we track it for future use
		// This setting would be used when creating individual queries
	}
}

// configureConnectionBehavior sets up connection behavior for reliability
func (cfg *Config) configureConnectionBehavior(cluster *gocql.ClusterConfig) {
	// Configure maximum reconnection attempts
	// This helps handle temporary network issues gracefully
	if cfg.MaxReconnectAttempts > 0 {
		// Note: gocql handles reconnection internally
		// We store this for use in our retry policy
		cluster.ReconnectInterval = cfg.ReconnectInterval
	}

	// Configure connection event handling
	// This helps with monitoring and debugging connection issues
	cluster.Events.DisableNodeStatusEvents = false
	cluster.Events.DisableTopologyEvents = false
	cluster.Events.DisableSchemaEvents = true // Schema events not needed for our use case
}

// ValidateProduction validates configuration for production use
func (cfg *Config) ValidateProduction() error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	// Production-specific validations
	if !cfg.TLSEnabled {
		return errors.New("TLS must be enabled in production")
	}

	if cfg.Username == "" || cfg.Password == "" {
		return errors.New("authentication credentials must be provided in production")
	}

	if cfg.TLSInsecureSkipVerify {
		return errors.New("TLS certificate verification cannot be disabled in production")
	}

	if cfg.Consistency == "ANY" || cfg.Consistency == "ONE" {
		return fmt.Errorf("consistency level %s is not recommended for production", cfg.Consistency)
	}

	if len(cfg.Hosts) < 3 {
		return errors.New("at least 3 hosts are recommended for production")
	}

	return nil
}

// GetEffectiveConfig returns the configuration with environment variables applied
func (cfg *Config) GetEffectiveConfig() (*Config, error) {
	// Create a copy of the config
	effectiveCfg := *cfg

	// Apply environment variables
	if err := effectiveCfg.ApplyEnvVars(); err != nil {
		return nil, fmt.Errorf("failed to apply environment variables: %w", err)
	}

	return &effectiveCfg, nil
}

// IsMultiDC returns true if this configuration is set up for multi-datacenter deployment
func (cfg *Config) IsMultiDC() bool {
	return cfg.DCAwareRouting && cfg.LocalDC != ""
}

// GetConnectionString returns a human-readable connection string (without credentials)
func (cfg *Config) GetConnectionString() string {
	var parts []string

	for _, host := range cfg.Hosts {
		parts = append(parts, fmt.Sprintf("%s:%d", host, cfg.Port))
	}

	connStr := strings.Join(parts, ",")

	if cfg.TLSEnabled {
		connStr += " (TLS)"
	}

	if cfg.IsMultiDC() {
		connStr += fmt.Sprintf(" (DC: %s)", cfg.LocalDC)
	}

	return connStr
}

// ValidateTLSCertificates validates that TLS certificate files are readable and valid
func (cfg *Config) ValidateTLSCertificates() error {
	if !cfg.TLSEnabled {
		return nil
	}

	// If both cert and key files are specified, validate the certificate pair
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		// Check if files exist
		if _, err := os.Stat(cfg.TLSCertFile); os.IsNotExist(err) {
			return fmt.Errorf("TLS cert file does not exist: %s", cfg.TLSCertFile)
		}

		if _, err := os.Stat(cfg.TLSKeyFile); os.IsNotExist(err) {
			return fmt.Errorf("TLS key file does not exist: %s", cfg.TLSKeyFile)
		}

		// Try to load the certificate to validate it
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("invalid TLS certificate/key pair: %w", err)
		}

		// Validate certificate expiration
		if len(cert.Certificate) > 0 {
			x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
			if err != nil {
				return fmt.Errorf("failed to parse TLS certificate: %w", err)
			}

			now := time.Now()
			if now.Before(x509Cert.NotBefore) {
				return fmt.Errorf("TLS certificate is not yet valid (valid from %v)", x509Cert.NotBefore)
			}

			if now.After(x509Cert.NotAfter) {
				return fmt.Errorf("TLS certificate has expired (expired on %v)", x509Cert.NotAfter)
			}

			// Warn if certificate expires soon (within 30 days)
			if now.Add(30 * 24 * time.Hour).After(x509Cert.NotAfter) {
				// Note: In a real implementation, this might be logged as a warning
				// For now, we'll just validate that it's not expired
			}
		}
	}

	// If CA file is specified, validate it exists and is readable
	if cfg.TLSCAFile != "" {
		if _, err := os.Stat(cfg.TLSCAFile); os.IsNotExist(err) {
			return fmt.Errorf("TLS CA file does not exist: %s", cfg.TLSCAFile)
		}

		// Try to read and parse the CA file
		caCert, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return fmt.Errorf("cannot read TLS CA file: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return fmt.Errorf("invalid TLS CA certificate in file: %s", cfg.TLSCAFile)
		}
	}

	return nil
}

// GetSecurityLevel returns a security assessment of the configuration
func (cfg *Config) GetSecurityLevel() (level string, score int, issues []string) {
	score = 0
	issues = []string{}

	// TLS Configuration (40 points max)
	if cfg.TLSEnabled {
		score += 20
		if !cfg.TLSInsecureSkipVerify {
			score += 10
		} else {
			issues = append(issues, "TLS certificate verification is disabled")
		}
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			score += 10
		} else {
			issues = append(issues, "Client certificate authentication not configured")
		}
	} else {
		issues = append(issues, "TLS encryption is disabled")
	}

	// Authentication (30 points max)
	if cfg.Username != "" && cfg.Password != "" {
		score += 30
	} else {
		issues = append(issues, "No authentication credentials configured")
	}

	// Consistency Level (20 points max)
	strongConsistency := []string{"QUORUM", "ALL", "LOCAL_QUORUM", "EACH_QUORUM"}
	isStrong := false
	for _, strong := range strongConsistency {
		if cfg.Consistency == strong {
			isStrong = true
			break
		}
	}
	if isStrong {
		score += 20
	} else {
		issues = append(issues, fmt.Sprintf("Weak consistency level: %s", cfg.Consistency))
	}

	// High Availability (10 points max)
	if len(cfg.Hosts) >= 3 {
		score += 10
	} else {
		issues = append(issues, "Insufficient hosts for high availability")
	}

	// Determine security level based on score
	switch {
	case score >= 90:
		level = "Excellent"
	case score >= 70:
		level = "Good"
	case score >= 50:
		level = "Fair"
	case score >= 30:
		level = "Poor"
	default:
		level = "Critical"
	}

	return level, score, issues
}

// CreateSecureClusterConfig creates a cluster configuration with security best practices
func (cfg *Config) CreateSecureClusterConfig() (*gocql.ClusterConfig, error) {
	// Validate security requirements
	if !cfg.TLSEnabled {
		return nil, fmt.Errorf("TLS must be enabled for secure connections")
	}

	if cfg.Username == "" || cfg.Password == "" {
		return nil, fmt.Errorf("authentication credentials are required for secure connections")
	}

	if cfg.TLSInsecureSkipVerify {
		return nil, fmt.Errorf("TLS certificate verification cannot be disabled for secure connections")
	}

	// Validate certificates
	if err := cfg.ValidateTLSCertificates(); err != nil {
		return nil, fmt.Errorf("TLS certificate validation failed: %w", err)
	}

	// Create the cluster configuration
	cluster, err := cfg.CreateClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster config: %w", err)
	}

	// Apply additional security settings
	cluster.DisableInitialHostLookup = false // Enable host discovery for better resilience
	cluster.IgnorePeerAddr = false           // Use peer addresses for better routing

	return cluster, nil
}

// GetTLSInfo returns information about the TLS configuration
func (cfg *Config) GetTLSInfo() map[string]interface{} {
	info := make(map[string]interface{})

	info["enabled"] = cfg.TLSEnabled
	info["insecure_skip_verify"] = cfg.TLSInsecureSkipVerify
	info["client_cert_configured"] = cfg.TLSCertFile != "" && cfg.TLSKeyFile != ""
	info["ca_configured"] = cfg.TLSCAFile != ""

	if cfg.TLSEnabled {
		// Try to get certificate information
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			if cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile); err == nil {
				if len(cert.Certificate) > 0 {
					if x509Cert, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
						info["cert_subject"] = x509Cert.Subject.String()
						info["cert_issuer"] = x509Cert.Issuer.String()
						info["cert_not_before"] = x509Cert.NotBefore
						info["cert_not_after"] = x509Cert.NotAfter
						info["cert_expired"] = time.Now().After(x509Cert.NotAfter)
						info["cert_expires_soon"] = time.Now().Add(30 * 24 * time.Hour).After(x509Cert.NotAfter)
					}
				}
			}
		}
	}

	return info
}

// GetAuthInfo returns information about the authentication configuration
func (cfg *Config) GetAuthInfo() map[string]interface{} {
	info := make(map[string]interface{})

	info["username_configured"] = cfg.Username != ""
	info["password_configured"] = cfg.Password != ""
	info["auth_enabled"] = cfg.Username != "" && cfg.Password != ""

	if cfg.Username != "" {
		info["username"] = cfg.Username
		// Never include the actual password in info
	}

	return info
}
