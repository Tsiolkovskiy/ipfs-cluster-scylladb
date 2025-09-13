# Task 2 Sequence Diagram - Detailed Explanation

## Overview
The Sequence Diagram (`task2-sequence.puml`) illustrates the complete interaction flow for implementing Task 2.1 (Configuration Structure) and Task 2.2 (TLS and Authentication), showing how components collaborate to establish a secure ScyllaDB connection.

## Sequence Flow Analysis

### Phase 1: Task 2.1 - Configuration Setup

#### Step 1: Configuration Creation
```plantuml
Developer -> Config: Create new Config()
activate Config
Config -> Config: Default()
```

**Real Implementation Mapping:**
```go
// Developer creates new configuration
cfg := &Config{}

// Initialize with default values (Task 2.1)
err := cfg.Default()
if err != nil {
    return fmt.Errorf("failed to set defaults: %w", err)
}
```

**Default() Method Implementation:**
```go
func (cfg *Config) Default() error {
    cfg.Hosts = []string{"127.0.0.1"}
    cfg.Port = DefaultPort                    // 9042
    cfg.Keyspace = DefaultKeyspace           // "ipfs_pins"
    cfg.NumConns = DefaultNumConns           // 10
    cfg.Timeout = DefaultTimeout             // 30 * time.Second
    cfg.ConnectTimeout = DefaultConnectTimeout // 10 * time.Second
    cfg.Consistency = DefaultConsistency     // "QUORUM"
    cfg.SerialConsistency = DefaultSerialConsistency // "SERIAL"
    
    // TLS defaults (Task 2.2 integration)
    cfg.TLSEnabled = false
    cfg.TLSInsecureSkipVerify = false
    
    // Retry policy defaults
    cfg.RetryPolicy = RetryPolicyConfig{
        NumRetries:    DefaultRetryNumRetries,    // 3
        MinRetryDelay: DefaultRetryMinDelay,      // 100ms
        MaxRetryDelay: DefaultRetryMaxDelay,      // 10s
    }
    
    // Batch defaults
    cfg.BatchSize = DefaultBatchSize          // 1000
    cfg.BatchTimeout = DefaultBatchTimeout    // 1s
    
    // Monitoring defaults
    cfg.MetricsEnabled = true
    cfg.TracingEnabled = false
    
    return nil
}
```

#### Step 2: JSON Configuration Loading
```plantuml
Developer -> Config: LoadJSON(configData)
Config -> Config: Parse JSON fields
Config -> Config: Convert duration strings
```

**Real Implementation Mapping:**
```go
// Developer loads JSON configuration
configData := []byte(`{
    "hosts": ["scylla1.prod.com", "scylla2.prod.com", "scylla3.prod.com"],
    "port": 9042,
    "keyspace": "ipfs_cluster_state",
    "tls_enabled": true,
    "username": "cluster_user",
    "timeout": "30s",
    "connect_timeout": "10s",
    "consistency": "QUORUM",
    "num_conns": 15,
    "batch_size": 500
}`)

err := cfg.LoadJSON(configData)
if err != nil {
    return fmt.Errorf("failed to load JSON config: %w", err)
}
```

**LoadJSON() Method Implementation:**
```go
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
    cfg.TLSEnabled = jcfg.TLSEnabled
    
    // Parse duration fields (special handling for JSON)
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
    
    // ... more field parsing
    return nil
}
```

#### Step 3: Environment Variable Application
```plantuml
Developer -> Config: ApplyEnvVars()
Config -> Config: Override with environment variables
```

**Real Implementation Mapping:**
```go
// Developer applies environment variable overrides
err := cfg.ApplyEnvVars()
if err != nil {
    return fmt.Errorf("failed to apply environment variables: %w", err)
}
```

**ApplyEnvVars() Method Implementation:**
```go
func (cfg *Config) ApplyEnvVars() error {
    // Connection settings override
    if hosts := os.Getenv("SCYLLADB_HOSTS"); hosts != "" {
        cfg.Hosts = strings.Split(hosts, ",")
        for i := range cfg.Hosts {
            cfg.Hosts[i] = strings.TrimSpace(cfg.Hosts[i])
        }
    }
    
    if keyspace := os.Getenv("SCYLLADB_KEYSPACE"); keyspace != "" {
        cfg.Keyspace = keyspace
    }
    
    // Task 2.2: Security settings override
    if username := os.Getenv("SCYLLADB_USERNAME"); username != "" {
        cfg.Username = username
    }
    
    if password := os.Getenv("SCYLLADB_PASSWORD"); password != "" {
        cfg.Password = password
    }
    
    if tlsEnabled := os.Getenv("SCYLLADB_TLS_ENABLED"); tlsEnabled != "" {
        cfg.TLSEnabled = strings.ToLower(tlsEnabled) == "true"
    }
    
    if certFile := os.Getenv("SCYLLADB_TLS_CERT_FILE"); certFile != "" {
        cfg.TLSCertFile = certFile
    }
    
    if keyFile := os.Getenv("SCYLLADB_TLS_KEY_FILE"); keyFile != "" {
        cfg.TLSKeyFile = keyFile
    }
    
    if caFile := os.Getenv("SCYLLADB_TLS_CA_FILE"); caFile != "" {
        cfg.TLSCAFile = caFile
    }
    
    // Multi-DC settings
    if localDC := os.Getenv("SCYLLADB_LOCAL_DC"); localDC != "" {
        cfg.LocalDC = localDC
        cfg.DCAwareRouting = true // Enable DC-aware routing when local DC is set
    }
    
    return nil
}
```

#### Step 4: Configuration Validation
```plantuml
Developer -> Validation: ValidateWithOptions(opts)
activate Validation
Validation -> Validation: validateBasic()
Validation -> Validation: validateHosts()
Validation -> Validation: validateKeyspace()
Validation -> Validation: validateTimeouts()
```

**Real Implementation Mapping:**
```go
// Developer chooses validation level based on environment
var opts *ValidationOptions
switch environment {
case "production":
    opts = StrictValidationOptions()
case "development":
    opts = DevelopmentValidationOptions()
default:
    opts = DefaultValidationOptions()
}

// Perform validation
err := cfg.ValidateWithOptions(opts)
if err != nil {
    return fmt.Errorf("configuration validation failed: %w", err)
}
```

**Validation Flow Implementation:**
```go
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

func (cfg *Config) validateBasic(opts *ValidationOptions) error {
    // Validate hosts
    if err := cfg.validateHosts(opts); err != nil {
        return err
    }
    
    // Validate keyspace
    if err := cfg.validateKeyspace(); err != nil {
        return err
    }
    
    // Validate timeouts
    if err := cfg.validateTimeouts(); err != nil {
        return err
    }
    
    return nil
}

func (cfg *Config) validateStrict(opts *ValidationOptions) error {
    // Production requirements (Task 2.2 integration)
    if !cfg.TLSEnabled {
        return fmt.Errorf("TLS must be enabled in strict validation mode")
    }
    
    if opts.RequireAuth && (cfg.Username == "" || cfg.Password == "") {
        return fmt.Errorf("authentication credentials are required in strict validation mode")
    }
    
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
    
    // Validate minimum hosts for HA
    if len(cfg.Hosts) < opts.MinHosts {
        return fmt.Errorf("at least %d hosts are required for production", opts.MinHosts)
    }
    
    return nil
}
```

### Phase 2: Task 2.2 - TLS and Authentication Setup

#### Step 5: TLS Configuration Creation
```plantuml
Developer -> TLS: CreateTLSConfig()
activate TLS
TLS -> TLS: Check TLSEnabled
alt TLS Enabled
    TLS -> TLS: Load client certificate
    TLS -> TLS: Load CA certificate
    TLS -> TLS: ValidateTLSCertificates()
    TLS -> TLS: Check certificate expiry
end
TLS --> Config: *tls.Config
deactivate TLS
```

**Real Implementation Mapping:**
```go
// Developer creates TLS configuration
tlsConfig, err := cfg.CreateTLSConfig()
if err != nil {
    return fmt.Errorf("failed to create TLS config: %w", err)
}
```

**CreateTLSConfig() Method Implementation:**
```go
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
```

**Certificate Validation Implementation:**
```go
func (cfg *Config) ValidateTLSCertificates() error {
    if !cfg.TLSEnabled {
        return nil
    }
    
    // Validate client certificates
    if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
        // Check file existence
        if _, err := os.Stat(cfg.TLSCertFile); os.IsNotExist(err) {
            return fmt.Errorf("TLS cert file does not exist: %s", cfg.TLSCertFile)
        }
        
        if _, err := os.Stat(cfg.TLSKeyFile); os.IsNotExist(err) {
            return fmt.Errorf("TLS key file does not exist: %s", cfg.TLSKeyFile)
        }
        
        // Validate certificate/key pair
        cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
        if err != nil {
            return fmt.Errorf("invalid TLS certificate/key pair: %w", err)
        }
        
        // Check certificate expiry
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
        }
    }
    
    return nil
}
```

#### Step 6: Authentication Setup
```plantuml
Developer -> Auth: CreateAuthenticator()
activate Auth
Auth -> Auth: Check credentials
alt Username and Password provided
    Auth -> Auth: Create PasswordAuthenticator
else No credentials
    Auth --> Config: nil
end
Auth --> Config: gocql.Authenticator
deactivate Auth
```

**Real Implementation Mapping:**
```go
// Developer creates authenticator
auth := cfg.CreateAuthenticator()
if auth != nil {
    fmt.Println("Authentication configured")
} else {
    fmt.Println("No authentication configured")
}
```

**CreateAuthenticator() Method Implementation:**
```go
func (cfg *Config) CreateAuthenticator() gocql.Authenticator {
    if cfg.Username != "" && cfg.Password != "" {
        return gocql.PasswordAuthenticator{
            Username: cfg.Username,
            Password: cfg.Password,
        }
    }
    return nil
}
```

### Phase 3: Secure Connection Creation

#### Step 7: Secure Cluster Configuration
```plantuml
Developer -> Config: CreateSecureClusterConfig()
Config -> Validation: Validate security requirements
Config -> TLS: ValidateTLSCertificates()
TLS --> Config: Validation result
Config -> Config: CreateClusterConfig()
Config -> GoCQL: NewCluster(hosts...)
activate GoCQL
Config -> GoCQL: Set basic parameters
Config -> GoCQL: Set authenticator
Config -> GoCQL: Set TLS options
alt Multi-DC Setup
    Config -> GoCQL: Configure host selection policy
end
GoCQL --> Config: *gocql.ClusterConfig
deactivate GoCQL
Config -> Config: Apply security settings
Config --> Developer: Secure cluster config
```

**Real Implementation Mapping:**
```go
// Developer creates secure cluster configuration
cluster, err := cfg.CreateSecureClusterConfig()
if err != nil {
    return fmt.Errorf("failed to create secure cluster config: %w", err)
}
```

**CreateSecureClusterConfig() Method Implementation:**
```go
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

**CreateClusterConfig() Method Implementation:**
```go
func (cfg *Config) CreateClusterConfig() (*gocql.ClusterConfig, error) {
    cluster := gocql.NewCluster(cfg.Hosts...)
    cluster.Port = cfg.Port
    cluster.Keyspace = cfg.Keyspace
    
    // Performance settings (Task 2.1)
    cluster.NumConns = cfg.NumConns
    cluster.Timeout = cfg.Timeout
    cluster.ConnectTimeout = cfg.ConnectTimeout
    cluster.Consistency = cfg.GetConsistency()
    cluster.SerialConsistency = cfg.GetSerialConsistency()
    
    // Set authenticator if credentials are provided (Task 2.2)
    if auth := cfg.CreateAuthenticator(); auth != nil {
        cluster.Authenticator = auth
    }
    
    // Configure TLS if enabled (Task 2.2)
    if cfg.TLSEnabled {
        tlsConfig, err := cfg.CreateTLSConfig()
        if err != nil {
            return nil, fmt.Errorf("failed to create TLS config: %w", err)
        }
        cluster.SslOpts = &gocql.SslOptions{
            Config: tlsConfig,
        }
    }
    
    // Configure host selection policy for multi-DC setups (Task 2.1)
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
```

### Phase 4: Security Assessment

#### Step 8: Security Level Assessment
```plantuml
Developer -> Config: GetSecurityLevel()
activate Config
Config -> Config: Calculate security score
Config -> Config: Identify security issues
Config --> Developer: (level, score, issues)
deactivate Config
```

**Real Implementation Mapping:**
```go
// Developer gets security assessment
level, score, issues := cfg.GetSecurityLevel()
fmt.Printf("Security Level: %s (Score: %d/100)\n", level, score)
if len(issues) > 0 {
    fmt.Println("Security Issues:")
    for _, issue := range issues {
        fmt.Printf("  - %s\n", issue)
    }
}
```

**GetSecurityLevel() Method Implementation:**
```go
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

### Phase 5: Connection to ScyllaDB

#### Step 9: Session Establishment
```plantuml
Cluster -> GoCQL: CreateSession(clusterConfig)
activate GoCQL
GoCQL -> ScyllaDB: Establish TLS connection
activate ScyllaDB
ScyllaDB -> GoCQL: TLS handshake
GoCQL -> ScyllaDB: Authenticate
ScyllaDB --> GoCQL: Authentication success
GoCQL --> Cluster: Session established
deactivate GoCQL
deactivate ScyllaDB
```

**Real Implementation Mapping:**
```go
// IPFS Cluster establishes session with ScyllaDB
session, err := cluster.CreateSession()
if err != nil {
    return fmt.Errorf("failed to create ScyllaDB session: %w", err)
}
defer session.Close()

fmt.Println("Successfully connected to ScyllaDB cluster")
```

**Connection Process:**
1. **TLS Handshake**: ScyllaDB validates client certificate (if provided)
2. **Server Certificate Validation**: Client validates server certificate using CA
3. **Authentication**: Username/password authentication over TLS connection
4. **Session Creation**: Established session ready for CQL operations

## Sequence Notes Analysis

### Task 2 Complete Note
```plantuml
note over Cluster, ScyllaDB
  **Task 2 Complete:**
  - Configuration structure with all fields ✓
  - JSON marshaling/unmarshaling ✓
  - Comprehensive validation ✓
  - TLS with certificate support ✓
  - Username/password authentication ✓
  - Secure connection methods ✓
  - Security assessment system ✓
end note
```

**Implementation Verification:**

1. **Configuration structure with all fields** ✓:
   - 20+ fields in Config struct covering all aspects
   - Connection, TLS, performance, consistency, monitoring fields

2. **JSON marshaling/unmarshaling** ✓:
   - `LoadJSON()`, `ToJSON()`, `ToDisplayJSON()` methods
   - Proper duration handling and type conversion

3. **Comprehensive validation** ✓:
   - Basic, Strict, and Development validation levels
   - Field-specific validation methods
   - Production-ready validation requirements

4. **TLS with certificate support** ✓:
   - Client certificate and CA certificate support
   - Certificate validation and expiry checking
   - Secure TLS configuration creation

5. **Username/password authentication** ✓:
   - gocql.PasswordAuthenticator integration
   - Secure credential handling
   - Environment variable support for secrets

6. **Secure connection methods** ✓:
   - `CreateSecureClusterConfig()` with validation
   - Security requirement enforcement
   - Production-ready connection creation

7. **Security assessment system** ✓:
   - 0-100 point scoring system
   - 5 security levels (Critical → Excellent)
   - Detailed security issue identification

## Bridge to Implementation

### Sequence Flow → Code Execution

1. **Configuration Phase** → **Task 2.1 Implementation**:
   - Each sequence step maps to specific methods in `config.go`
   - Validation flow implemented in `validate_config.go`
   - JSON handling with proper type conversion

2. **Security Phase** → **Task 2.2 Implementation**:
   - TLS configuration creation with certificate handling
   - Authentication setup with gocql integration
   - Security assessment with comprehensive scoring

3. **Connection Phase** → **Integration Implementation**:
   - gocql driver integration with all security features
   - Multi-DC support with proper routing policies
   - Production-ready connection establishment

### Testing Bridge

The sequence flow is thoroughly tested in:
- **`validate_config_test.go`**: Configuration and validation testing
- **`tls_auth_test.go`**: TLS and authentication testing with real certificates
- **Integration tests**: Complete sequence flow testing

This sequence diagram effectively bridges the architectural design with the actual implementation flow, showing how Task 2.1 and Task 2.2 requirements are realized through a series of method calls and component interactions that result in a secure, production-ready ScyllaDB connection.