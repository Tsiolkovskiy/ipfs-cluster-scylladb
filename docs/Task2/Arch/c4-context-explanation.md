# C4 Context Diagram - Detailed Explanation

## Overview
The Context Diagram (`c4-context.puml`) represents the highest level view of the ScyllaDB State Store system, showing how it fits into the broader ecosystem and who interacts with it.

## Diagram Elements Analysis

### 1. Actors (People)

#### Developer
```plantuml
Person(developer, "Developer", "Configures and uses ScyllaDB state store")
```

**Real Implementation Mapping:**
- **Code Location**: `state/scyllastate/example_tls_auth.go`
- **Actual Usage**:
  ```go
  // Developer creates and configures the state store
  cfg := &Config{}
  cfg.Default()
  cfg.TLSEnabled = true
  cfg.Username = "scylla_user"
  cfg.Password = "secure_password"
  ```

**Developer Interactions:**
1. **Configuration Creation** (Task 2.1):
   - Uses `Config` struct from `config.go`
   - Calls `Default()` method for initial setup
   - Modifies fields for specific requirements

2. **JSON Configuration** (Task 2.1):
   - Uses `LoadJSON()` method to load from files
   - Uses `ToJSON()` for serialization
   - Example: Loading from `scylladb-config.json`

3. **Validation** (Task 2.1):
   - Calls `Validate()` for basic validation
   - Uses `ValidateWithOptions()` for advanced validation
   - Chooses validation level: Basic, Strict, or Development

#### DevOps Engineer
```plantuml
Person(devops, "DevOps Engineer", "Deploys and manages ScyllaDB infrastructure")
```

**Real Implementation Mapping:**
- **Code Location**: Environment variable handling in `config.go`
- **Actual Usage**:
  ```go
  // DevOps sets environment variables
  // SCYLLADB_HOSTS=node1.prod.com,node2.prod.com,node3.prod.com
  // SCYLLADB_TLS_ENABLED=true
  // SCYLLADB_USERNAME=prod_user
  
  cfg.ApplyEnvVars() // Applies environment overrides
  ```

**DevOps Interactions:**
1. **Environment Configuration**:
   - Sets `SCYLLADB_*` environment variables
   - Uses `ApplyEnvVars()` method for runtime configuration
   - Manages secrets through environment variables

2. **Security Management** (Task 2.2):
   - Manages TLS certificates via `SCYLLADB_TLS_CERT_FILE`
   - Handles CA certificates via `SCYLLADB_TLS_CA_FILE`
   - Configures authentication credentials securely

3. **Production Validation**:
   - Uses `ValidateProduction()` method
   - Ensures strict security requirements
   - Monitors security level with `GetSecurityLevel()`

### 2. Systems

#### IPFS Cluster (Internal System)
```plantuml
System(ipfs_cluster, "IPFS Cluster", "Distributed IPFS cluster system")
```

**Real Implementation Mapping:**
- **Integration Point**: The IPFS Cluster uses the ScyllaDB state store as a plugin
- **Code Usage**:
  ```go
  // IPFS Cluster initializes the state store
  stateStore := &ScyllaDBStateStore{
      config: cfg,
  }
  
  // Uses the state store for persistence
  err := stateStore.Store(ctx, key, value)
  ```

**System Characteristics:**
- **Distributed**: Multiple cluster nodes can use the same ScyllaDB backend
- **State Persistence**: Stores cluster state, pin information, and metadata
- **High Availability**: Relies on ScyllaDB's replication for data durability

#### ScyllaDB Cluster (External System)
```plantuml
System_Ext(scylladb, "ScyllaDB Cluster", "High-performance NoSQL database cluster")
```

**Real Implementation Mapping:**
- **Connection Creation**: Via `CreateClusterConfig()` method
- **Actual Connection Code**:
  ```go
  // Creates gocql.ClusterConfig
  cluster, err := cfg.CreateClusterConfig()
  if err != nil {
      return fmt.Errorf("failed to create cluster config: %w", err)
  }
  
  // Establishes session
  session, err := cluster.CreateSession()
  ```

**External System Properties:**
- **Multi-node**: Configured via `Hosts []string` field
- **TLS Encryption**: Enabled via `TLSEnabled bool` field
- **Authentication**: Configured via `Username/Password` fields
- **High Availability**: Supported via consistency levels and replication

### 3. Relationships

#### Developer → IPFS Cluster
```plantuml
Rel(developer, ipfs_cluster, "Configures state store")
```

**Implementation Details:**
- **Configuration Flow**:
  1. Developer creates `Config` struct (Task 2.1)
  2. Sets connection parameters: `Hosts`, `Port`, `Keyspace`
  3. Configures TLS settings: `TLSEnabled`, certificate files (Task 2.2)
  4. Sets authentication: `Username`, `Password` (Task 2.2)
  5. Validates configuration with appropriate level

**Code Example**:
```go
// Developer configuration workflow
cfg := &Config{}
cfg.Default()

// Task 2.1: Basic configuration
cfg.Hosts = []string{"scylla1.prod.com", "scylla2.prod.com"}
cfg.Port = 9042
cfg.Keyspace = "ipfs_cluster_state"

// Task 2.2: Security configuration
cfg.TLSEnabled = true
cfg.TLSCertFile = "/etc/ssl/certs/client.crt"
cfg.TLSKeyFile = "/etc/ssl/private/client.key"
cfg.Username = "cluster_user"
cfg.Password = "secure_password"

// Validation
if err := cfg.Validate(); err != nil {
    log.Fatalf("Configuration validation failed: %v", err)
}
```

#### DevOps → ScyllaDB Cluster
```plantuml
Rel(devops, scylladb, "Manages database cluster")
```

**Implementation Details:**
- **Infrastructure Management**: DevOps manages the ScyllaDB cluster infrastructure
- **Certificate Management**: Provides TLS certificates that the state store validates
- **Security Policies**: Enforces authentication and authorization policies

**Environment Configuration**:
```bash
# DevOps environment setup
export SCYLLADB_HOSTS="scylla1.prod.com,scylla2.prod.com,scylla3.prod.com"
export SCYLLADB_TLS_ENABLED="true"
export SCYLLADB_TLS_CERT_FILE="/etc/ssl/certs/scylla-client.crt"
export SCYLLADB_TLS_KEY_FILE="/etc/ssl/private/scylla-client.key"
export SCYLLADB_TLS_CA_FILE="/etc/ssl/certs/scylla-ca.crt"
export SCYLLADB_USERNAME="prod_cluster_user"
export SCYLLADB_PASSWORD="$(cat /etc/secrets/scylla-password)"
export SCYLLADB_LOCAL_DC="datacenter1"
```

#### IPFS Cluster → ScyllaDB Cluster
```plantuml
Rel(ipfs_cluster, scylladb, "Stores cluster state", "CQL/TLS")
```

**Implementation Details:**
- **Protocol**: Uses CQL (Cassandra Query Language) over TLS
- **Connection Method**: Via gocql driver configured by `CreateClusterConfig()`
- **Security**: TLS encryption with optional client certificate authentication

**Actual Connection Code**:
```go
// Connection establishment (from CreateClusterConfig)
cluster := gocql.NewCluster(cfg.Hosts...)
cluster.Port = cfg.Port
cluster.Keyspace = cfg.Keyspace

// Task 2.2: TLS Configuration
if cfg.TLSEnabled {
    tlsConfig, err := cfg.CreateTLSConfig()
    if err != nil {
        return nil, fmt.Errorf("failed to create TLS config: %w", err)
    }
    cluster.SslOpts = &gocql.SslOptions{
        Config: tlsConfig,
    }
}

// Task 2.2: Authentication
if auth := cfg.CreateAuthenticator(); auth != nil {
    cluster.Authenticator = auth
}

// Performance settings (Task 2.1)
cluster.NumConns = cfg.NumConns
cluster.Timeout = cfg.Timeout
cluster.ConnectTimeout = cfg.ConnectTimeout
cluster.Consistency = cfg.GetConsistency()
```

## Context Notes Analysis

### ScyllaDB Cluster Note
```plantuml
note right of scylladb
  - Multi-node cluster
  - TLS encryption
  - Authentication required
  - High availability
end note
```

**Implementation Mapping:**
- **Multi-node cluster**: Handled by `Hosts []string` field in Config
- **TLS encryption**: Implemented via `CreateTLSConfig()` method (Task 2.2)
- **Authentication required**: Enforced by `ValidateProduction()` method
- **High availability**: Supported via consistency levels and DC-aware routing

### IPFS Cluster Note
```plantuml
note left of ipfs_cluster
  - Uses ScyllaDB for state persistence
  - Configurable connection settings
  - Security validation
  - Environment-based config
end note
```

**Implementation Mapping:**
- **State persistence**: Through the state store interface
- **Configurable connection settings**: All fields in Config struct (Task 2.1)
- **Security validation**: Via `GetSecurityLevel()` and validation methods (Task 2.2)
- **Environment-based config**: Via `ApplyEnvVars()` method

## Bridge to Implementation

### From Context to Code Structure

1. **Developer Interaction** → **Configuration API**:
   - Context shows developer configuring the system
   - Implementation provides `Config` struct with 20+ fields
   - Validation ensures configuration correctness

2. **DevOps Management** → **Environment Integration**:
   - Context shows infrastructure management
   - Implementation provides environment variable support
   - Production validation ensures deployment readiness

3. **System Integration** → **Driver Integration**:
   - Context shows IPFS Cluster using ScyllaDB
   - Implementation provides gocql driver integration
   - Security features ensure safe communication

### Key Implementation Files Referenced

1. **`config.go`**: Main configuration structure and methods
2. **`validate_config.go`**: Advanced validation and security assessment
3. **`example_tls_auth.go`**: Usage examples for developers
4. **`tls_auth_test.go`**: Comprehensive testing of security features

### Security Implementation Bridge

The context diagram emphasizes security (TLS, authentication), which maps directly to:

- **Task 2.2 TLS Implementation**: `CreateTLSConfig()`, `ValidateTLSCertificates()`
- **Task 2.2 Authentication**: `CreateAuthenticator()`, username/password support
- **Security Assessment**: `GetSecurityLevel()` with 0-100 scoring system
- **Production Readiness**: `ValidateProduction()` with strict requirements

This context diagram serves as the foundation for understanding how the detailed Task 2 implementation fits into the broader system architecture and user workflows.