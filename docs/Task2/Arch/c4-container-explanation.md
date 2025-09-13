# C4 Container Diagram - Detailed Explanation

## Overview
The Container Diagram (`c4-container.puml`) breaks down the IPFS Cluster system into its major containers and shows how Task 2.1 and Task 2.2 implementations are distributed across these containers.

## Container Analysis

### 1. IPFS Cluster System Boundary

#### IPFS Cluster Node
```plantuml
Container(cluster_node, "IPFS Cluster Node", "Go", "Cluster node with state store")
```

**Real Implementation Mapping:**
- **Technology**: Go application that imports the ScyllaDB state store
- **Integration Point**: Uses the state store as a plugin/component
- **Code Usage**:
  ```go
  import "github.com/ipfs-cluster/ipfs-cluster/state/scyllastate"
  
  // Cluster node initializes state store
  stateStore := &scyllastate.StateStore{
      config: cfg,
  }
  ```

**Responsibilities:**
- Manages IPFS cluster operations
- Delegates state persistence to ScyllaDB State Store
- Handles cluster consensus and coordination

#### Configuration Management
```plantuml
Container(config_mgmt, "Configuration Management", "Go", "Handles ScyllaDB configuration")
```

**Real Implementation Mapping:**
- **Primary File**: `state/scyllastate/config.go` (Task 2.1)
- **Supporting Files**: `validate_config.go`, environment handling
- **Key Components**:
  ```go
  // Configuration Management Container Implementation
  type Config struct {
      // Task 2.1: Connection fields
      Hosts    []string `json:"hosts"`
      Port     int      `json:"port"`
      Keyspace string   `json:"keyspace"`
      
      // Task 2.2: Security fields
      TLSEnabled  bool   `json:"tls_enabled"`
      Username    string `json:"username,omitempty"`
      Password    string `json:"password,omitempty"`
      
      // Task 2.1: Performance fields
      NumConns       int           `json:"num_conns"`
      Timeout        time.Duration `json:"timeout"`
      ConnectTimeout time.Duration `json:"connect_timeout"`
  }
  ```

**Container Responsibilities:**
1. **Configuration Structure** (Task 2.1):
   - Defines all configuration fields
   - Provides default values via `Default()` method
   - Handles JSON serialization/deserialization

2. **Validation Management** (Task 2.1):
   - Basic validation via `Validate()` method
   - Advanced validation via `ValidateWithOptions()`
   - Production validation via `ValidateProduction()`

3. **Environment Integration**:
   - Processes environment variables via `ApplyEnvVars()`
   - Supports runtime configuration overrides

#### ScyllaDB State Store
```plantuml
Container(state_store, "ScyllaDB State Store", "Go", "State persistence layer")
```

**Real Implementation Mapping:**
- **Core Files**: All files in `state/scyllastate/` package
- **Key Methods**: Connection creation, TLS setup, authentication
- **Implementation Structure**:
  ```go
  // State Store Container Implementation
  type StateStore struct {
      config  *Config
      session *gocql.Session
  }
  
  func (s *StateStore) Initialize() error {
      // Task 2.2: Create secure cluster configuration
      cluster, err := s.config.CreateSecureClusterConfig()
      if err != nil {
          return fmt.Errorf("failed to create secure config: %w", err)
      }
      
      // Establish session
      s.session, err = cluster.CreateSession()
      return err
  }
  ```

**Container Responsibilities:**
1. **Connection Management** (Task 2.1 + 2.2):
   - Creates gocql cluster configuration
   - Manages connection pooling and timeouts
   - Handles connection lifecycle

2. **Security Implementation** (Task 2.2):
   - TLS configuration and certificate handling
   - Authentication setup (username/password)
   - Security validation and assessment

3. **State Operations**:
   - Provides CRUD operations for cluster state
   - Handles data serialization/deserialization
   - Manages transactions and consistency

### 2. ScyllaDB Cluster Boundary

#### ScyllaDB Nodes
```plantuml
ContainerDb(scylla_node1, "ScyllaDB Node 1", "ScyllaDB", "Primary database node")
ContainerDb(scylla_node2, "ScyllaDB Node 2", "ScyllaDB", "Replica database node")
ContainerDb(scylla_node3, "ScyllaDB Node 3", "ScyllaDB", "Replica database node")
```

**Real Implementation Mapping:**
- **Configuration**: Via `Hosts []string` field in Config
- **Connection Code**:
  ```go
  // Multi-node cluster configuration
  cfg.Hosts = []string{
      "scylla-node-1.example.com",
      "scylla-node-2.example.com", 
      "scylla-node-3.example.com",
  }
  
  // gocql cluster setup
  cluster := gocql.NewCluster(cfg.Hosts...)
  
  // Task 2.1: Performance configuration
  cluster.NumConns = cfg.NumConns
  cluster.Timeout = cfg.Timeout
  cluster.Consistency = cfg.GetConsistency()
  
  // Task 2.1: Multi-DC support
  if cfg.DCAwareRouting && cfg.LocalDC != "" {
      policy := gocql.DCAwareRoundRobinPolicy(cfg.LocalDC)
      if cfg.TokenAwareRouting {
          cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(policy)
      }
  }
  ```

**Database Characteristics:**
- **High Availability**: Multiple nodes for redundancy
- **Consistency**: Configurable via `Consistency` field (Task 2.1)
- **Performance**: Optimized connection pooling and timeouts
- **Security**: TLS encryption and authentication required

### 3. External Systems

#### Certificate Authority
```plantuml
System_Ext(cert_authority, "Certificate Authority", "Issues TLS certificates")
```

**Real Implementation Mapping:**
- **Certificate Validation**: Via `ValidateTLSCertificates()` method (Task 2.2)
- **Certificate Loading**: Via `CreateTLSConfig()` method
- **Implementation Code**:
  ```go
  // Certificate Authority integration (Task 2.2)
  func (cfg *Config) ValidateTLSCertificates() error {
      if !cfg.TLSEnabled {
          return nil
      }
      
      // Validate client certificate from CA
      if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
          cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
          if err != nil {
              return fmt.Errorf("invalid certificate from CA: %w", err)
          }
          
          // Check certificate expiry
          if len(cert.Certificate) > 0 {
              x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
              if err != nil {
                  return fmt.Errorf("failed to parse CA-issued certificate: %w", err)
              }
              
              if time.Now().After(x509Cert.NotAfter) {
                  return fmt.Errorf("CA-issued certificate has expired")
              }
          }
      }
      
      // Validate CA certificate
      if cfg.TLSCAFile != "" {
          caCert, err := os.ReadFile(cfg.TLSCAFile)
          if err != nil {
              return fmt.Errorf("cannot read CA certificate: %w", err)
          }
          
          caCertPool := x509.NewCertPool()
          if !caCertPool.AppendCertsFromPEM(caCert) {
              return fmt.Errorf("invalid CA certificate format")
          }
      }
      
      return nil
  }
  ```

#### Environment Variables
```plantuml
System_Ext(env_config, "Environment Variables", "Runtime configuration")
```

**Real Implementation Mapping:**
- **Environment Processing**: Via `ApplyEnvVars()` method (Task 2.1)
- **Supported Variables**: All major configuration fields
- **Implementation Code**:
  ```go
  // Environment Variables integration (Task 2.1)
  func (cfg *Config) ApplyEnvVars() error {
      // Connection settings
      if hosts := os.Getenv("SCYLLADB_HOSTS"); hosts != "" {
          cfg.Hosts = strings.Split(hosts, ",")
      }
      
      if keyspace := os.Getenv("SCYLLADB_KEYSPACE"); keyspace != "" {
          cfg.Keyspace = keyspace
      }
      
      // Task 2.2: Security settings
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
          cfg.DCAwareRouting = true
      }
      
      return nil
  }
  ```

## Relationship Analysis

### Developer → Configuration Management
```plantuml
Rel(developer, config_mgmt, "Configures", "JSON/Environment")
```

**Implementation Bridge:**
- **JSON Configuration** (Task 2.1):
  ```go
  // Developer loads JSON configuration
  configData, err := os.ReadFile("scylladb-config.json")
  if err != nil {
      return fmt.Errorf("failed to read config: %w", err)
  }
  
  cfg := &Config{}
  cfg.Default()
  
  if err := cfg.LoadJSON(configData); err != nil {
      return fmt.Errorf("failed to load JSON config: %w", err)
  }
  ```

- **Environment Configuration**:
  ```go
  // Developer applies environment overrides
  if err := cfg.ApplyEnvVars(); err != nil {
      return fmt.Errorf("failed to apply env vars: %w", err)
  }
  ```

### DevOps → Environment Variables
```plantuml
Rel(devops, env_config, "Sets variables", "Shell/K8s")
```

**Implementation Bridge:**
- **Shell Environment**:
  ```bash
  # DevOps sets environment variables
  export SCYLLADB_HOSTS="prod1.scylla.com,prod2.scylla.com,prod3.scylla.com"
  export SCYLLADB_TLS_ENABLED="true"
  export SCYLLADB_TLS_CERT_FILE="/etc/ssl/certs/scylla-client.crt"
  export SCYLLADB_USERNAME="prod_user"
  export SCYLLADB_PASSWORD="$(vault kv get -field=password secret/scylla)"
  ```

- **Kubernetes ConfigMap/Secret**:
  ```yaml
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: scylladb-config
  data:
    SCYLLADB_HOSTS: "prod1.scylla.com,prod2.scylla.com,prod3.scylla.com"
    SCYLLADB_TLS_ENABLED: "true"
    SCYLLADB_LOCAL_DC: "datacenter1"
  ---
  apiVersion: v1
  kind: Secret
  metadata:
    name: scylladb-credentials
  data:
    SCYLLADB_USERNAME: <base64-encoded-username>
    SCYLLADB_PASSWORD: <base64-encoded-password>
  ```

### Configuration Management → State Store
```plantuml
Rel(config_mgmt, state_store, "Provides config", "Go structs")
```

**Implementation Bridge:**
- **Configuration Injection**:
  ```go
  // Configuration Management provides config to State Store
  cfg := &Config{}
  cfg.Default()
  cfg.LoadJSON(configData)
  cfg.ApplyEnvVars()
  
  // Validate configuration
  if err := cfg.ValidateWithOptions(StrictValidationOptions()); err != nil {
      return fmt.Errorf("configuration validation failed: %w", err)
  }
  
  // Inject into State Store
  stateStore := &StateStore{
      config: cfg,
  }
  ```

### State Store → ScyllaDB Nodes
```plantuml
Rel(state_store, scylla_node1, "Connects", "CQL/TLS")
```

**Implementation Bridge:**
- **Secure Connection Creation** (Task 2.2):
  ```go
  // State Store creates secure connections to ScyllaDB nodes
  func (s *StateStore) Connect() error {
      // Create secure cluster configuration
      cluster, err := s.config.CreateSecureClusterConfig()
      if err != nil {
          return fmt.Errorf("failed to create secure cluster config: %w", err)
      }
      
      // Establish session with all nodes
      s.session, err = cluster.CreateSession()
      if err != nil {
          return fmt.Errorf("failed to create session: %w", err)
      }
      
      return nil
  }
  ```

- **Connection Details**:
  ```go
  // Detailed connection setup (from CreateClusterConfig)
  cluster := gocql.NewCluster(cfg.Hosts...) // All three nodes
  cluster.Port = cfg.Port
  cluster.Keyspace = cfg.Keyspace
  
  // Task 2.2: TLS setup
  if cfg.TLSEnabled {
      tlsConfig, err := cfg.CreateTLSConfig()
      if err != nil {
          return nil, fmt.Errorf("TLS config creation failed: %w", err)
      }
      cluster.SslOpts = &gocql.SslOptions{Config: tlsConfig}
  }
  
  // Task 2.2: Authentication
  if auth := cfg.CreateAuthenticator(); auth != nil {
      cluster.Authenticator = auth
  }
  
  // Task 2.1: Performance settings
  cluster.NumConns = cfg.NumConns
  cluster.Timeout = cfg.Timeout
  cluster.ConnectTimeout = cfg.ConnectTimeout
  cluster.Consistency = cfg.GetConsistency()
  ```

### Certificate Authority → State Store
```plantuml
Rel(cert_authority, state_store, "Provides certificates", "X.509/PEM")
```

**Implementation Bridge:**
- **Certificate Integration** (Task 2.2):
  ```go
  // State Store validates CA-provided certificates
  func (s *StateStore) ValidateCertificates() error {
      return s.config.ValidateTLSCertificates()
  }
  
  // Certificate loading from CA-provided files
  func (cfg *Config) CreateTLSConfig() (*tls.Config, error) {
      tlsConfig := &tls.Config{
          InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
      }
      
      // Load CA-issued client certificate
      if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
          cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
          if err != nil {
              return nil, fmt.Errorf("failed to load CA-issued certificate: %w", err)
          }
          tlsConfig.Certificates = []tls.Certificate{cert}
      }
      
      // Load CA certificate for server validation
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

## Container Note Analysis

### State Store Note
```plantuml
note right of state_store
  Task 2.1: Configuration Structure
  - Config struct with all fields
  - JSON marshaling/unmarshaling
  - Validation methods
  - Default values
  
  Task 2.2: TLS & Authentication
  - TLS configuration
  - Certificate handling
  - Username/password auth
  - Secure connections
end note
```

**Implementation Mapping:**

**Task 2.1 Features:**
- **Config struct**: 20+ fields in `config.go`
- **JSON marshaling**: `LoadJSON()`, `ToJSON()`, `ToDisplayJSON()` methods
- **Validation methods**: `Validate()`, `ValidateWithOptions()`, `ValidateProduction()`
- **Default values**: `Default()` method with comprehensive defaults

**Task 2.2 Features:**
- **TLS configuration**: `CreateTLSConfig()`, `ValidateTLSCertificates()` methods
- **Certificate handling**: X.509 certificate loading and validation
- **Username/password auth**: `CreateAuthenticator()` method
- **Secure connections**: `CreateSecureClusterConfig()` method

## Bridge to Implementation

### Container Responsibilities → Code Structure

1. **Configuration Management Container** → **`config.go` + `validate_config.go`**:
   - Handles all configuration aspects (Task 2.1)
   - Provides validation and security assessment
   - Manages environment integration

2. **State Store Container** → **Complete `scyllastate` package**:
   - Integrates all Task 2.1 and Task 2.2 features
   - Provides secure connection management
   - Handles state persistence operations

3. **External System Integration** → **Environment and Certificate Handling**:
   - Environment variables processed by `ApplyEnvVars()`
   - Certificates validated by `ValidateTLSCertificates()`
   - Security assessed by `GetSecurityLevel()`

### Key Implementation Files Referenced

1. **`config.go`**: Configuration Management container implementation
2. **`validate_config.go`**: Advanced validation for Configuration Management
3. **State Store files**: Complete implementation of State Store container
4. **`tls_auth_test.go`**: Testing of container interactions and security

This container diagram effectively bridges the high-level system architecture with the detailed implementation structure, showing how Task 2.1 and Task 2.2 features are distributed across containers and how they interact to provide a complete ScyllaDB state store solution.