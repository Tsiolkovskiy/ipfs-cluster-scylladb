# C4 Component Diagram - Detailed Explanation

## Overview
The Component Diagram (`c4-component.puml`) provides a detailed view of the ScyllaDB State Store package components, showing how Task 2.1 and Task 2.2 requirements are implemented as discrete, interacting components.

## Component Analysis by Task

### Task 2.1 Components (Configuration Structure and Validation)

#### Config Struct
```plantuml
Component(config_struct, "Config Struct", "Go struct", "Main configuration structure with all fields")
```

**Real Implementation Mapping:**
- **File**: `state/scyllastate/config.go`
- **Lines**: ~50-150 (struct definition)
- **Actual Code**:
  ```go
  // Main configuration structure (Task 2.1)
  type Config struct {
      config.Saver
      
      // Connection settings
      Hosts    []string `json:"hosts"`
      Port     int      `json:"port"`
      Keyspace string   `json:"keyspace"`
      Username string   `json:"username,omitempty"`
      Password string   `json:"password,omitempty"`
      
      // TLS settings (Task 2.2 integration)
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
  }
  ```

**Component Responsibilities:**
- Central data structure for all configuration
- JSON tags for serialization support
- Integration point for all other components
- Implements config.Saver interface

#### JSON Marshaling
```plantuml
Component(json_marshal, "JSON Marshaling", "Go methods", "LoadJSON/ToJSON/ToDisplayJSON methods")
```

**Real Implementation Mapping:**
- **File**: `state/scyllastate/config.go`
- **Lines**: ~400-600 (JSON methods)
- **Key Methods**:
  ```go
  // JSON Marshaling Component (Task 2.1)
  
  // LoadJSON loads configuration from JSON bytes
  func (cfg *Config) LoadJSON(raw []byte) error {
      jcfg := &configJSON{}
      err := json.Unmarshal(raw, jcfg)
      if err != nil {
          return fmt.Errorf("error unmarshaling ScyllaDB config: %w", err)
      }
      
      // Copy basic fields
      cfg.Hosts = jcfg.Hosts
      cfg.Port = jcfg.Port
      cfg.Keyspace = jcfg.Keyspace
      cfg.Username = jcfg.Username
      cfg.Password = jcfg.Password
      
      // Parse duration fields (special handling)
      if jcfg.Timeout != "" {
          timeout, err := time.ParseDuration(jcfg.Timeout)
          if err != nil {
              return fmt.Errorf("invalid timeout duration: %w", err)
          }
          cfg.Timeout = timeout
      }
      
      // ... more duration parsing
      return nil
  }
  
  // ToJSON returns the JSON representation
  func (cfg *Config) ToJSON() ([]byte, error) {
      jcfg := &configJSON{
          Hosts:    cfg.Hosts,
          Port:     cfg.Port,
          Keyspace: cfg.Keyspace,
          Username: cfg.Username,
          Password: cfg.Password,
          Timeout:  cfg.Timeout.String(), // Duration to string
          // ... more field mapping
      }
      return config.DefaultJSONMarshal(jcfg)
  }
  
  // ToDisplayJSON returns safe JSON (hides passwords)
  func (cfg *Config) ToDisplayJSON() ([]byte, error) {
      jcfg := &configJSON{
          // ... copy all fields
          Password: "<hidden>", // Security: hide password
      }
      if cfg.Password != "" {
          jcfg.Password = "<hidden>"
      }
      return config.DefaultJSONMarshal(jcfg)
  }
  ```

**Supporting Structures**:
```go
// JSON helper structures (Task 2.1)
type configJSON struct {
    Hosts    []string `json:"hosts"`
    Port     int      `json:"port"`
    Keyspace string   `json:"keyspace"`
    Username string   `json:"username,omitempty"`
    Password string   `json:"password,omitempty"`
    
    // Duration fields as strings for JSON
    Timeout        string `json:"timeout"`
    ConnectTimeout string `json:"connect_timeout"`
    BatchTimeout   string `json:"batch_timeout"`
    
    RetryPolicy retryPolicyConfigJSON `json:"retry_policy"`
    // ... more fields
}

type retryPolicyConfigJSON struct {
    NumRetries    int    `json:"num_retries"`
    MinRetryDelay string `json:"min_retry_delay"`
    MaxRetryDelay string `json:"max_retry_delay"`
}
```

#### Basic Validation
```plantuml
Component(validation_basic, "Basic Validation", "Go methods", "Validate() and field-specific validation")
```

**Real Implementation Mapping:**
- **File**: `state/scyllastate/config.go`
- **Lines**: ~200-400 (validation methods)
- **Key Methods**:
  ```go
  // Basic Validation Component (Task 2.1)
  
  // Main validation method
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
      
      // Validate consistency levels
      if err := cfg.validateConsistency(); err != nil {
          return err
      }
      
      // Validate retry policy
      if err := cfg.validateRetryPolicy(); err != nil {
          return err
      }
      
      // Validate TLS settings (Task 2.2 integration)
      if err := cfg.validateTLS(); err != nil {
          return err
      }
      
      return nil
  }
  
  // Field-specific validation methods
  func (cfg *Config) validateConsistency() error {
      validConsistency := map[string]bool{
          "ANY": true, "ONE": true, "TWO": true, "THREE": true,
          "QUORUM": true, "ALL": true, "LOCAL_QUORUM": true,
          "EACH_QUORUM": true, "LOCAL_ONE": true,
      }
      
      if !validConsistency[cfg.Consistency] {
          return fmt.Errorf("invalid consistency level: %s", cfg.Consistency)
      }
      
      validSerialConsistency := map[string]bool{
          "SERIAL": true, "LOCAL_SERIAL": true,
      }
      
      if !validSerialConsistency[cfg.SerialConsistency] {
          return fmt.Errorf("invalid serial consistency level: %s", cfg.SerialConsistency)
      }
      
      return nil
  }
  
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
  ```

#### Default Values
```plantuml
Component(defaults, "Default Values", "Go methods", "Default() method and constants")
```

**Real Implementation Mapping:**
- **File**: `state/scyllastate/config.go`
- **Lines**: ~20-50 (constants), ~150-200 (Default method)
- **Implementation**:
  ```go
  // Default Values Component (Task 2.1)
  
  // Constants for default values
  const (
      DefaultConfigKey         = "scylladb"
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
  )
  
  // Default method sets all default values
  func (cfg *Config) Default() error {
      cfg.Hosts = []string{"127.0.0.1"}
      cfg.Port = DefaultPort
      cfg.Keyspace = DefaultKeyspace
      cfg.Username = ""
      cfg.Password = ""
      
      // TLS defaults (Task 2.2 integration)
      cfg.TLSEnabled = false
      cfg.TLSCertFile = ""
      cfg.TLSKeyFile = ""
      cfg.TLSCAFile = ""
      cfg.TLSInsecureSkipVerify = false
      
      // Performance defaults
      cfg.NumConns = DefaultNumConns
      cfg.Timeout = DefaultTimeout
      cfg.ConnectTimeout = DefaultConnectTimeout
      
      // Consistency defaults
      cfg.Consistency = DefaultConsistency
      cfg.SerialConsistency = DefaultSerialConsistency
      
      // Retry policy defaults
      cfg.RetryPolicy = RetryPolicyConfig{
          NumRetries:    DefaultRetryNumRetries,
          MinRetryDelay: DefaultRetryMinDelay,
          MaxRetryDelay: DefaultRetryMaxDelay,
      }
      
      // Batch defaults
      cfg.BatchSize = DefaultBatchSize
      cfg.BatchTimeout = DefaultBatchTimeout
      
      // Monitoring defaults
      cfg.MetricsEnabled = true
      cfg.TracingEnabled = false
      
      // Multi-DC defaults
      cfg.LocalDC = ""
      cfg.DCAwareRouting = false
      cfg.TokenAwareRouting = false
      
      return nil
  }
  ```

### Task 2.2 Components (TLS and Authentication)

#### TLS Configuration
```plantuml
Component(tls_config, "TLS Configuration", "Go methods", "CreateTLSConfig() and certificate handling")
```

**Real Implementation Mapping:**
- **File**: `state/scyllastate/config.go`
- **Lines**: ~700-850 (TLS methods)
- **Key Methods**:
  ```go
  // TLS Configuration Component (Task 2.2)
  
  // CreateTLSConfig creates a TLS configuration from settings
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
  
  // GetTLSInfo returns information about TLS configuration
  func (cfg *Config) GetTLSInfo() map[string]interface{} {
      info := make(map[string]interface{})
      
      info["enabled"] = cfg.TLSEnabled
      info["insecure_skip_verify"] = cfg.TLSInsecureSkipVerify
      info["client_cert_configured"] = cfg.TLSCertFile != "" && cfg.TLSKeyFile != ""
      info["ca_configured"] = cfg.TLSCAFile != ""
      
      // Get certificate details if available
      if cfg.TLSEnabled && cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
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
      
      return info
  }
  ```

#### Authentication Handler
```plantuml
Component(auth_handler, "Authentication", "Go methods", "CreateAuthenticator() for username/password")
```

**Real Implementation Mapping:**
- **File**: `state/scyllastate/config.go`
- **Lines**: ~850-950 (auth methods)
- **Key Methods**:
  ```go
  // Authentication Component (Task 2.2)
  
  // CreateAuthenticator creates a gocql authenticator
  func (cfg *Config) CreateAuthenticator() gocql.Authenticator {
      if cfg.Username != "" && cfg.Password != "" {
          return gocql.PasswordAuthenticator{
              Username: cfg.Username,
              Password: cfg.Password,
          }
      }
      return nil
  }
  
  // GetAuthInfo returns information about authentication
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
  ```

#### Secure Connections
```plantuml
Component(secure_conn, "Secure Connections", "Go methods", "CreateSecureClusterConfig() with validation")
```

**Real Implementation Mapping:**
- **File**: `state/scyllastate/config.go`
- **Lines**: ~950-1100 (secure connection methods)
- **Key Methods**:
  ```go
  // Secure Connections Component (Task 2.2)
  
  // CreateClusterConfig creates a gocql.ClusterConfig
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
      
      // Configure host selection policy for multi-DC
      if cfg.DCAwareRouting && cfg.LocalDC != "" {
          policy := gocql.DCAwareRoundRobinPolicy(cfg.LocalDC)
          if cfg.TokenAwareRouting {
              cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(policy)
          } else {
              cluster.PoolConfig.HostSelectionPolicy = policy
          }
      } else if cfg.TokenAwareRouting {
          cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(gocql.RoundRobinHostPolicy())
      }
      
      return cluster, nil
  }
  
  // CreateSecureClusterConfig creates a secure configuration with validation
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
      cluster.DisableInitialHostLookup = false
      cluster.IgnorePeerAddr = false
      
      return cluster, nil
  }
  ```

#### Certificate Validator
```plantuml
Component(cert_validator, "Certificate Validator", "Go methods", "ValidateTLSCertificates() and expiry checks")
```

**Real Implementation Mapping:**
- **File**: `state/scyllastate/config.go`
- **Lines**: ~1100-1300 (certificate validation)
- **Key Methods**:
  ```go
  // Certificate Validator Component (Task 2.2)
  
  // ValidateTLSCertificates validates TLS certificates and their expiry
  func (cfg *Config) ValidateTLSCertificates() error {
      if !cfg.TLSEnabled {
          return nil
      }
      
      // Validate client certificates
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
              
              // Check if certificate expires soon (within 30 days)
              if now.Add(30 * 24 * time.Hour).After(x509Cert.NotAfter) {
                  // Could log a warning here in a real implementation
              }
          }
      }
      
      // Validate CA certificate
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
  ```

### Advanced Validation Components (Task 2.1 Extension)

#### Advanced Validation
```plantuml
Component(validation_advanced, "Advanced Validation", "Go methods", "ValidateWithOptions() with multiple levels")
```

**Real Implementation Mapping:**
- **File**: `state/scyllastate/validate_config.go`
- **Lines**: ~70-200 (advanced validation methods)
- **Key Methods**:
  ```go
  // Advanced Validation Component (Task 2.1 Extension)
  
  // ValidateWithOptions validates with custom validation options
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
  
  // validateStrict performs strict validation for production
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
      
      return nil
  }
  ```

#### Validation Options
```plantuml
Component(validation_opts, "Validation Options", "Go structs", "ValidationOptions with different levels")
```

**Real Implementation Mapping:**
- **File**: `state/scyllastate/validate_config.go`
- **Lines**: ~20-70 (validation options)
- **Implementation**:
  ```go
  // Validation Options Component (Task 2.1 Extension)
  
  // ValidationLevel represents the level of validation to perform
  type ValidationLevel int
  
  const (
      ValidationBasic ValidationLevel = iota
      ValidationStrict
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
  
  // DevelopmentValidationOptions returns relaxed validation options
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
  ```

#### Security Assessment
```plantuml
Component(security_assessment, "Security Assessment", "Go methods", "GetSecurityLevel() with scoring")
```

**Real Implementation Mapping:**
- **File**: `state/scyllastate/config.go`
- **Lines**: ~1300-1500 (security assessment)
- **Key Methods**:
  ```go
  // Security Assessment Component (Task 2.2 Extension)
  
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
  ```

### Utility Components

#### Environment Variables
```plantuml
Component(env_vars, "Environment Variables", "Go methods", "ApplyEnvVars() for runtime config")
```

**Real Implementation Mapping:**
- **File**: `state/scyllastate/config.go`
- **Lines**: ~250-350 (environment variable handling)
- **Implementation**: (Already detailed in container explanation)

#### Information Methods
```plantuml
Component(info_methods, "Information Methods", "Go methods", "GetTLSInfo(), GetAuthInfo(), GetConnectionString()")
```

**Real Implementation Mapping:**
- **File**: `state/scyllastate/config.go`
- **Lines**: ~1500-1700 (information methods)
- **Key Methods**: (Already detailed in TLS and Auth components)

## Component Relationships

### Component Interaction Flow

1. **Config Struct** serves as the central hub for all other components
2. **JSON Marshaling** handles serialization/deserialization of Config Struct
3. **Basic Validation** validates Config Struct fields
4. **Default Values** initializes Config Struct with sensible defaults
5. **TLS Configuration** uses Config Struct TLS fields to create TLS configs
6. **Authentication** uses Config Struct credentials to create authenticators
7. **Secure Connections** combines TLS and Auth components with Config Struct
8. **Advanced Validation** extends Basic Validation with configurable options
9. **Security Assessment** analyzes Config Struct to provide security scoring

### External Integration

- **gocql Driver**: Receives configurations from Secure Connections component
- **crypto/tls**: Used by TLS Configuration component for certificate handling
- **crypto/x509**: Used by Certificate Validator for certificate parsing

## Bridge to Implementation

### Component Architecture → File Structure

1. **Task 2.1 Components** → **`config.go` + `validate_config.go`**:
   - Config Struct, JSON Marshaling, Basic Validation, Default Values
   - Advanced Validation, Validation Options

2. **Task 2.2 Components** → **`config.go` methods**:
   - TLS Configuration, Authentication, Secure Connections, Certificate Validator
   - Security Assessment

3. **Utility Components** → **Supporting methods**:
   - Environment Variables, Information Methods

### Testing Bridge

- **Component Testing**: Each component has dedicated tests in `validate_config_test.go` and `tls_auth_test.go`
- **Integration Testing**: Components are tested together in realistic scenarios
- **Security Testing**: Security Assessment component is thoroughly tested with various configurations

This component diagram effectively shows how the Task 2.1 and Task 2.2 requirements are implemented as discrete, interacting components that work together to provide a complete, secure, and configurable ScyllaDB state store solution.