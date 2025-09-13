# C4 Code Diagram - Detailed Explanation

## Overview
The Code Diagram (`c4-code.puml`) represents the lowest level of the C4 model, showing the actual implementation files, classes, and methods that realize Task 2.1 and Task 2.2 requirements.

## File-Level Implementation Analysis

### Main Implementation Files

#### config.go - Core Configuration Implementation
```plantuml
Component(config_go, "config.go", "Go file", "Main configuration implementation")
```

**Real Implementation Details:**
- **File Size**: ~1000+ lines
- **Location**: `state/scyllastate/config.go`
- **Primary Responsibility**: Complete Task 2.1 and Task 2.2 implementation

**File Structure**:
```go
// Package declaration and imports
package scyllastate

import (
    "crypto/tls"
    "crypto/x509"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "strings"
    "time"
    
    "github.com/gocql/gocql"
    "github.com/ipfs-cluster/ipfs-cluster/config"
)

// Constants (Task 2.1)
const (
    DefaultConfigKey         = "scylladb"
    DefaultPort              = 9042
    DefaultKeyspace          = "ipfs_pins"
    // ... more constants
)

// Main Config struct (Task 2.1)
type Config struct {
    config.Saver
    // ... all configuration fields
}

// JSON helper structs (Task 2.1)
type configJSON struct {
    // ... JSON serialization fields
}

// All methods implementation
// ... 40+ methods
```

#### validate_config.go - Advanced Validation
```plantuml
Component(validation_go, "validate_config.go", "Go file", "Advanced validation implementation")
```

**Real Implementation Details:**
- **File Size**: ~500+ lines  
- **Location**: `state/scyllastate/validate_config.go`
- **Primary Responsibility**: Extended validation system for Task 2.1

**File Structure**:
```go
package scyllastate

import (
    "fmt"
    "net"
    "strings"
    "time"
)

// Validation level enumeration
type ValidationLevel int

const (
    ValidationBasic ValidationLevel = iota
    ValidationStrict
    ValidationDevelopment
)

// Validation options struct
type ValidationOptions struct {
    Level                ValidationLevel
    AllowInsecureTLS     bool
    AllowWeakConsistency bool
    MinHosts             int
    MaxHosts             int
    RequireAuth          bool
}

// Factory methods for validation options
func DefaultValidationOptions() *ValidationOptions { /* ... */ }
func StrictValidationOptions() *ValidationOptions { /* ... */ }
func DevelopmentValidationOptions() *ValidationOptions { /* ... */ }

// Advanced validation methods
func (cfg *Config) ValidateWithOptions(opts *ValidationOptions) error { /* ... */ }
func (cfg *Config) validateBasic(opts *ValidationOptions) error { /* ... */ }
func (cfg *Config) validateStrict(opts *ValidationOptions) error { /* ... */ }
// ... more validation methods
```

### Test Implementation Files

#### validate_config_test.go - Validation Tests
```plantuml
Component(validate_test_go, "validate_config_test.go", "Go file", "Validation tests")
```

**Real Implementation Details:**
- **File Size**: ~400+ lines
- **Location**: `state/scyllastate/validate_config_test.go`
- **Test Coverage**: All validation methods and options

**Test Structure**:
```go
package scyllastate

import (
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// Test validation options
func TestValidationOptions(t *testing.T) {
    t.Run("DefaultValidationOptions", func(t *testing.T) {
        opts := DefaultValidationOptions()
        assert.Equal(t, ValidationBasic, opts.Level)
        // ... more assertions
    })
    // ... more subtests
}

// Test validation with options
func TestConfig_ValidateWithOptions(t *testing.T) {
    tests := []struct {
        name    string
        setup   func(*Config)
        opts    *ValidationOptions
        wantErr bool
        errMsg  string
    }{
        // ... test cases
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ... test implementation
        })
    }
}

// More test functions...
```

#### tls_auth_test.go - TLS and Authentication Tests  
```plantuml
Component(tls_auth_test_go, "tls_auth_test.go", "Go file", "TLS and auth tests")
```

**Real Implementation Details:**
- **File Size**: 1270+ lines
- **Location**: `state/scyllastate/tls_auth_test.go`
- **Unique Features**: Automatic certificate generation, comprehensive TLS testing

**Key Test Features**:
```go
package scyllastate

import (
    "crypto/rand"
    "crypto/rsa"
    "crypto/tls"
    "crypto/x509"
    "crypto/x509/pkix"
    "encoding/pem"
    "math/big"
    "os"
    "path/filepath"
    "testing"
    "time"
    
    "github.com/gocql/gocql"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// Test helper for creating real certificates
func createTestCertificates(t *testing.T, tempDir string) (certFile, keyFile, caFile string) {
    // Generate CA private key
    caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
    require.NoError(t, err)
    
    // Create CA certificate template
    caTemplate := x509.Certificate{
        SerialNumber: big.NewInt(1),
        Subject: pkix.Name{
            Organization: []string{"Test CA"},
            Country:      []string{"US"},
        },
        NotBefore:             time.Now(),
        NotAfter:              time.Now().Add(365 * 24 * time.Hour),
        IsCA:                  true,
        ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
        KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
        BasicConstraintsValid: true,
    }
    
    // ... certificate generation and file writing
    return certFile, keyFile, caFile
}

// Comprehensive TLS tests
func TestConfig_CreateTLSConfig(t *testing.T) {
    // ... extensive TLS testing with real certificates
}

func TestConfig_ValidateTLSCertificates(t *testing.T) {
    // ... certificate validation testing
}

// ... 15+ test functions covering all TLS and auth aspects
```

### Example and Documentation Files

#### example_tls_auth.go - Usage Examples
```plantuml
Component(example_go, "example_tls_auth.go", "Go file", "Usage examples")
```

**Real Implementation Details:**
- **File Size**: ~200+ lines
- **Location**: `state/scyllastate/example_tls_auth.go`
- **Purpose**: Practical examples for developers

**Example Structure**:
```go
package scyllastate

import (
    "fmt"
    "log"
)

// ExampleTLSConfiguration demonstrates TLS setup for ScyllaDB
func ExampleTLSConfiguration() {
    // Create a new configuration
    cfg := &Config{}
    cfg.Default()
    
    // Configure TLS settings (Task 2.2)
    cfg.TLSEnabled = true
    cfg.TLSCertFile = "/path/to/client.crt"
    cfg.TLSKeyFile = "/path/to/client.key"
    cfg.TLSCAFile = "/path/to/ca.crt"
    cfg.TLSInsecureSkipVerify = false
    
    // Configure authentication (Task 2.2)
    cfg.Username = "scylla_user"
    cfg.Password = "secure_password"
    
    // Configure hosts for production (Task 2.1)
    cfg.Hosts = []string{
        "scylla-node-1.example.com",
        "scylla-node-2.example.com",
        "scylla-node-3.example.com",
    }
    
    // Set strong consistency for production (Task 2.1)
    cfg.Consistency = "QUORUM"
    cfg.SerialConsistency = "SERIAL"
    
    // Validate the configuration
    if err := cfg.ValidateProduction(); err != nil {
        log.Fatalf("Production validation failed: %v", err)
    }
    
    // Get security assessment (Task 2.2)
    level, score, issues := cfg.GetSecurityLevel()
    fmt.Printf("Security Level: %s (Score: %d/100)\n", level, score)
    if len(issues) > 0 {
        fmt.Println("Security Issues:")
        for _, issue := range issues {
            fmt.Printf("  - %s\n", issue)
        }
    }
    
    // Create a secure cluster configuration (Task 2.2)
    cluster, err := cfg.CreateSecureClusterConfig()
    if err != nil {
        log.Fatalf("Failed to create secure cluster config: %v", err)
    }
    
    fmt.Printf("Cluster configured with %d hosts\n", len(cluster.Hosts))
    fmt.Printf("TLS enabled: %t\n", cluster.SslOpts != nil)
    fmt.Printf("Authentication enabled: %t\n", cluster.Authenticator != nil)
}

// ExampleDevelopmentConfiguration demonstrates development setup
func ExampleDevelopmentConfiguration() {
    cfg := &Config{}
    cfg.Default()
    
    // Development settings - less secure but easier to set up
    cfg.TLSEnabled = true
    cfg.TLSInsecureSkipVerify = true // Only for development!
    cfg.Username = "dev_user"
    cfg.Password = "dev_password"
    cfg.Consistency = "ONE" // Faster for development
    
    // Validate with development options
    opts := DevelopmentValidationOptions()
    if err := cfg.ValidateWithOptions(opts); err != nil {
        log.Fatalf("Development validation failed: %v", err)
    }
    
    // Get TLS and auth info
    tlsInfo := cfg.GetTLSInfo()
    authInfo := cfg.GetAuthInfo()
    
    fmt.Printf("TLS enabled: %t\n", tlsInfo["enabled"])
    fmt.Printf("TLS insecure: %t\n", tlsInfo["insecure_skip_verify"])
    fmt.Printf("Auth enabled: %t\n", authInfo["auth_enabled"])
}

// More examples for Multi-DC, Environment variables, etc.
```

## Class-Level Implementation Analysis

### Core Classes

#### Config Struct - Main Configuration Class
```plantuml
Component(config_struct_class, "Config", "Go struct", "Main configuration structure")
```

**Complete Implementation**:
```go
// Config holds configuration for ScyllaDB state store (Task 2.1 + 2.2)
type Config struct {
    config.Saver
    
    // Connection settings (Task 2.1)
    Hosts    []string `json:"hosts"`
    Port     int      `json:"port"`
    Keyspace string   `json:"keyspace"`
    Username string   `json:"username,omitempty"`
    Password string   `json:"password,omitempty"`
    
    // TLS settings (Task 2.2)
    TLSEnabled            bool   `json:"tls_enabled"`
    TLSCertFile           string `json:"tls_cert_file,omitempty"`
    TLSKeyFile            string `json:"tls_key_file,omitempty"`
    TLSCAFile             string `json:"tls_ca_file,omitempty"`
    TLSInsecureSkipVerify bool   `json:"tls_insecure_skip_verify"`
    
    // Performance settings (Task 2.1)
    NumConns       int           `json:"num_conns"`
    Timeout        time.Duration `json:"timeout"`
    ConnectTimeout time.Duration `json:"connect_timeout"`
    
    // Consistency settings (Task 2.1)
    Consistency       string `json:"consistency"`
    SerialConsistency string `json:"serial_consistency"`
    
    // Retry policy settings (Task 2.1)
    RetryPolicy RetryPolicyConfig `json:"retry_policy"`
    
    // Batching settings (Task 2.1)
    BatchSize    int           `json:"batch_size"`
    BatchTimeout time.Duration `json:"batch_timeout"`
    
    // Monitoring settings (Task 2.1)
    MetricsEnabled bool `json:"metrics_enabled"`
    TracingEnabled bool `json:"tracing_enabled"`
    
    // Multi-DC settings (Task 2.1)
    LocalDC           string `json:"local_dc,omitempty"`
    DCAwareRouting    bool   `json:"dc_aware_routing"`
    TokenAwareRouting bool   `json:"token_aware_routing"`
}
```

**Method Categories**:

1. **Basic Methods (Task 2.1)**:
   ```go
   func (cfg *Config) ConfigKey() string
   func (cfg *Config) Default() error
   func (cfg *Config) ApplyEnvVars() error
   func (cfg *Config) Validate() error
   ```

2. **JSON Methods (Task 2.1)**:
   ```go
   func (cfg *Config) LoadJSON(raw []byte) error
   func (cfg *Config) ToJSON() ([]byte, error)
   func (cfg *Config) ToDisplayJSON() ([]byte, error)
   ```

3. **TLS Methods (Task 2.2)**:
   ```go
   func (cfg *Config) CreateTLSConfig() (*tls.Config, error)
   func (cfg *Config) ValidateTLSCertificates() error
   func (cfg *Config) GetTLSInfo() map[string]interface{}
   ```

4. **Authentication Methods (Task 2.2)**:
   ```go
   func (cfg *Config) CreateAuthenticator() gocql.Authenticator
   func (cfg *Config) GetAuthInfo() map[string]interface{}
   ```

5. **Secure Connection Methods (Task 2.2)**:
   ```go
   func (cfg *Config) CreateClusterConfig() (*gocql.ClusterConfig, error)
   func (cfg *Config) CreateSecureClusterConfig() (*gocql.ClusterConfig, error)
   func (cfg *Config) ValidateProduction() error
   func (cfg *Config) GetSecurityLevel() (string, int, []string)
   ```

#### RetryPolicyConfig - Retry Configuration
```plantuml
Component(retry_policy_class, "RetryPolicyConfig", "Go struct", "Retry policy configuration")
```

**Implementation**:
```go
// RetryPolicyConfig configures retry behavior (Task 2.1)
type RetryPolicyConfig struct {
    NumRetries    int           `json:"num_retries"`
    MinRetryDelay time.Duration `json:"min_retry_delay"`
    MaxRetryDelay time.Duration `json:"max_retry_delay"`
}
```

#### JSON Helper Classes
```plantuml
Component(config_json_class, "configJSON", "Go struct", "JSON serialization helper")
```

**Implementation**:
```go
// configJSON represents the JSON serialization format (Task 2.1)
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
    Timeout        string `json:"timeout"`        // Duration as string
    ConnectTimeout string `json:"connect_timeout"` // Duration as string
    
    Consistency       string `json:"consistency"`
    SerialConsistency string `json:"serial_consistency"`
    
    RetryPolicy retryPolicyConfigJSON `json:"retry_policy"`
    
    BatchSize    int    `json:"batch_size"`
    BatchTimeout string `json:"batch_timeout"` // Duration as string
    
    MetricsEnabled bool `json:"metrics_enabled"`
    TracingEnabled bool `json:"tracing_enabled"`
    
    LocalDC           string `json:"local_dc,omitempty"`
    DCAwareRouting    bool   `json:"dc_aware_routing"`
    TokenAwareRouting bool   `json:"token_aware_routing"`
}

// retryPolicyConfigJSON represents retry policy in JSON format
type retryPolicyConfigJSON struct {
    NumRetries    int    `json:"num_retries"`
    MinRetryDelay string `json:"min_retry_delay"` // Duration as string
    MaxRetryDelay string `json:"max_retry_delay"` // Duration as string
}
```

### Validation Classes

#### ValidationOptions - Validation Configuration
```plantuml
Component(validation_opts_class, "ValidationOptions", "Go struct", "Validation configuration")
```

**Implementation**:
```go
// ValidationOptions configures validation behavior (Task 2.1 Extension)
type ValidationOptions struct {
    Level                ValidationLevel
    AllowInsecureTLS     bool
    AllowWeakConsistency bool
    MinHosts             int
    MaxHosts             int
    RequireAuth          bool
}
```

#### ValidationLevel - Validation Level Enumeration
```plantuml
Component(validation_level_enum, "ValidationLevel", "Go type", "Validation level enumeration")
```

**Implementation**:
```go
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
```

## External Dependencies Integration

### gocql Package Integration
```plantuml
System_Ext(gocql_pkg, "github.com/gocql/gocql", "Go package", "Cassandra/ScyllaDB driver")
```

**Integration Points**:
```go
// gocql types used in implementation
import "github.com/gocql/gocql"

// Authentication integration (Task 2.2)
func (cfg *Config) CreateAuthenticator() gocql.Authenticator {
    if cfg.Username != "" && cfg.Password != "" {
        return gocql.PasswordAuthenticator{
            Username: cfg.Username,
            Password: cfg.Password,
        }
    }
    return nil
}

// Cluster configuration integration (Task 2.1 + 2.2)
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
    
    return cluster, nil
}

// Consistency level conversion
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
```

### Crypto Package Integration
```plantuml
System_Ext(tls_pkg, "crypto/tls", "Go package", "TLS implementation")
System_Ext(x509_pkg, "crypto/x509", "Go package", "X.509 certificate handling")
```

**Integration Points**:
```go
import (
    "crypto/tls"
    "crypto/x509"
)

// TLS configuration creation (Task 2.2)
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

// Certificate validation (Task 2.2)
func (cfg *Config) ValidateTLSCertificates() error {
    if !cfg.TLSEnabled {
        return nil
    }
    
    // Validate client certificates
    if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
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
        }
    }
    
    return nil
}
```

## Bridge to Implementation

### Code Structure → Task Requirements

1. **Task 2.1 Implementation**:
   - **Config Struct**: Complete configuration structure with all required fields
   - **JSON Methods**: Full serialization/deserialization with proper type handling
   - **Validation Methods**: Basic and advanced validation with multiple levels
   - **Default Values**: Comprehensive default configuration

2. **Task 2.2 Implementation**:
   - **TLS Methods**: Complete TLS configuration with certificate support
   - **Auth Methods**: Username/password and certificate-based authentication
   - **Security Methods**: Secure connection creation and security assessment
   - **Certificate Validation**: Full certificate lifecycle management

### Testing Implementation Bridge

1. **Unit Testing**: Each method has dedicated unit tests
2. **Integration Testing**: Components tested together in realistic scenarios
3. **Security Testing**: Comprehensive TLS and authentication testing
4. **Certificate Testing**: Real certificate generation and validation

### Documentation Bridge

1. **Usage Examples**: Practical examples for different deployment scenarios
2. **API Documentation**: Complete method documentation
3. **Configuration Guide**: Detailed configuration options explanation

This code diagram provides the most detailed view of how Task 2.1 and Task 2.2 requirements are implemented in actual Go code, showing the complete bridge from architectural design to working implementation.