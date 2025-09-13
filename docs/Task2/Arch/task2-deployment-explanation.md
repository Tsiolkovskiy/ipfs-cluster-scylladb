# Task 2 Deployment Diagram - Detailed Explanation

## Overview
The Deployment Diagram (`task2-deployment.puml`) illustrates how Task 2.1 (Configuration Structure) and Task 2.2 (TLS and Authentication) are deployed across different environments, showing the practical application of the implementation in real-world scenarios.

## Environment Analysis

### Development Environment

#### Developer Workstation
```plantuml
node "Developer Workstation" as DevWorkstation {
    component "IPFS Cluster Node" as DevCluster
    component "ScyllaDB Config" as DevConfig
    component "TLS Handler" as DevTLS
}
```

**Real Implementation Mapping:**

**Development Configuration Setup:**
```go
// Developer workstation configuration (Task 2.1)
func setupDevelopmentConfig() *Config {
    cfg := &Config{}
    cfg.Default()
    
    // Development-specific settings
    cfg.Hosts = []string{"localhost"}
    cfg.Port = 9042
    cfg.Keyspace = "ipfs_pins_dev"
    
    // Relaxed TLS settings for development (Task 2.2)
    cfg.TLSEnabled = true
    cfg.TLSInsecureSkipVerify = true // Only for development!
    cfg.Username = "dev_user"
    cfg.Password = "dev_password"
    
    // Performance settings optimized for development
    cfg.NumConns = 5
    cfg.Timeout = 10 * time.Second
    cfg.ConnectTimeout = 5 * time.Second
    cfg.Consistency = "ONE" // Faster for development
    
    // Validate with development options
    opts := DevelopmentValidationOptions()
    if err := cfg.ValidateWithOptions(opts); err != nil {
        log.Fatalf("Development validation failed: %v", err)
    }
    
    return cfg
}
```

**Development TLS Handler:**
```go
// Development TLS configuration (Task 2.2)
func (cfg *Config) setupDevelopmentTLS() error {
    // Create self-signed certificates for development
    if cfg.TLSCertFile == "" {
        cfg.TLSCertFile = "./dev-certs/client.crt"
        cfg.TLSKeyFile = "./dev-certs/client.key"
        cfg.TLSCAFile = "./dev-certs/ca.crt"
    }
    
    // Allow insecure TLS for development
    cfg.TLSInsecureSkipVerify = true
    
    // Create TLS config
    tlsConfig, err := cfg.CreateTLSConfig()
    if err != nil {
        return fmt.Errorf("failed to create development TLS config: %w", err)
    }
    
    // Log TLS info for debugging
    tlsInfo := cfg.GetTLSInfo()
    log.Printf("Development TLS enabled: %t", tlsInfo["enabled"])
    log.Printf("Development TLS insecure: %t", tlsInfo["insecure_skip_verify"])
    
    return nil
}
```

#### Local ScyllaDB
```plantuml
database "Local ScyllaDB" as DevScyllaDB
```

**Development Database Configuration:**
```bash
# Local ScyllaDB setup for development
docker run -d \
  --name scylla-dev \
  -p 9042:9042 \
  -e SCYLLA_CLUSTER_NAME=dev-cluster \
  -e SCYLLA_DC=datacenter1 \
  -e SCYLLA_RACK=rack1 \
  scylladb/scylla:latest \
  --smp 1 \
  --memory 1G \
  --overprovisioned 1
```

**Development Connection:**
```go
// Connect to local ScyllaDB in development
func connectDevelopment(cfg *Config) (*gocql.Session, error) {
    // Create cluster configuration
    cluster, err := cfg.CreateClusterConfig()
    if err != nil {
        return nil, fmt.Errorf("failed to create cluster config: %w", err)
    }
    
    // Development-specific settings
    cluster.ProtoVersion = 4
    cluster.DisableInitialHostLookup = true
    
    // Create session
    session, err := cluster.CreateSession()
    if err != nil {
        return nil, fmt.Errorf("failed to create development session: %w", err)
    }
    
    log.Println("Connected to local ScyllaDB for development")
    return session, nil
}
```

### Production Environment

#### Kubernetes Cluster
```plantuml
package "Kubernetes Cluster" {
    node "IPFS Cluster Pod 1" as ProdPod1
    node "IPFS Cluster Pod 2" as ProdPod2
    node "IPFS Cluster Pod 3" as ProdPod3
}
```

**Production Kubernetes Deployment:**

**ConfigMap for Production Configuration:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: scylladb-config
  namespace: ipfs-cluster
data:
  config.json: |
    {
      "hosts": ["scylla-node-1.scylla.svc.cluster.local", "scylla-node-2.scylla.svc.cluster.local", "scylla-node-3.scylla.svc.cluster.local"],
      "port": 9042,
      "keyspace": "ipfs_cluster_prod",
      "tls_enabled": true,
      "consistency": "QUORUM",
      "serial_consistency": "SERIAL",
      "num_conns": 20,
      "timeout": "30s",
      "connect_timeout": "10s",
      "batch_size": 1000,
      "metrics_enabled": true,
      "dc_aware_routing": true,
      "token_aware_routing": true,
      "local_dc": "datacenter1"
    }
```

**Secret for Production Credentials:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: scylladb-credentials
  namespace: ipfs-cluster
type: Opaque
data:
  username: <base64-encoded-username>
  password: <base64-encoded-password>
  tls-cert: <base64-encoded-client-cert>
  tls-key: <base64-encoded-client-key>
  ca-cert: <base64-encoded-ca-cert>
```

**Production Pod Configuration:**
```go
// Production configuration setup (Task 2.1 + 2.2)
func setupProductionConfig() *Config {
    cfg := &Config{}
    cfg.Default()
    
    // Load configuration from ConfigMap
    configData, err := os.ReadFile("/etc/config/config.json")
    if err != nil {
        log.Fatalf("Failed to read config: %v", err)
    }
    
    if err := cfg.LoadJSON(configData); err != nil {
        log.Fatalf("Failed to load JSON config: %v", err)
    }
    
    // Apply environment variables from Secret
    if err := cfg.ApplyEnvVars(); err != nil {
        log.Fatalf("Failed to apply env vars: %v", err)
    }
    
    // Production TLS configuration (Task 2.2)
    cfg.TLSEnabled = true
    cfg.TLSCertFile = "/etc/ssl/certs/client.crt"
    cfg.TLSKeyFile = "/etc/ssl/private/client.key"
    cfg.TLSCAFile = "/etc/ssl/certs/ca.crt"
    cfg.TLSInsecureSkipVerify = false // Must be false in production
    
    // Validate with strict production requirements
    if err := cfg.ValidateProduction(); err != nil {
        log.Fatalf("Production validation failed: %v", err)
    }
    
    // Security assessment
    level, score, issues := cfg.GetSecurityLevel()
    log.Printf("Security Level: %s (Score: %d/100)", level, score)
    if len(issues) > 0 {
        log.Printf("Security Issues: %v", issues)
        if level == "Critical" || level == "Poor" {
            log.Fatalf("Security level too low for production: %s", level)
        }
    }
    
    return cfg
}
```

**Production Deployment Manifest:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ipfs-cluster
  namespace: ipfs-cluster
spec:
  replicas: 3
  selector:
    matchLabels:
      app: ipfs-cluster
  template:
    metadata:
      labels:
        app: ipfs-cluster
    spec:
      containers:
      - name: ipfs-cluster
        image: ipfs/ipfs-cluster:latest
        env:
        - name: SCYLLADB_USERNAME
          valueFrom:
            secretKeyRef:
              name: scylladb-credentials
              key: username
        - name: SCYLLADB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: scylladb-credentials
              key: password
        - name: SCYLLADB_TLS_ENABLED
          value: "true"
        - name: SCYLLADB_TLS_CERT_FILE
          value: "/etc/ssl/certs/client.crt"
        - name: SCYLLADB_TLS_KEY_FILE
          value: "/etc/ssl/private/client.key"
        - name: SCYLLADB_TLS_CA_FILE
          value: "/etc/ssl/certs/ca.crt"
        - name: SCYLLADB_LOCAL_DC
          value: "datacenter1"
        volumeMounts:
        - name: config
          mountPath: /etc/config
        - name: tls-certs
          mountPath: /etc/ssl/certs
          readOnly: true
        - name: tls-private
          mountPath: /etc/ssl/private
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: scylladb-config
      - name: tls-certs
        secret:
          secretName: scylladb-credentials
          items:
          - key: tls-cert
            path: client.crt
          - key: ca-cert
            path: ca.crt
      - name: tls-private
        secret:
          secretName: scylladb-credentials
          items:
          - key: tls-key
            path: client.key
```

#### ScyllaDB Cluster
```plantuml
package "ScyllaDB Cluster" {
    database "ScyllaDB Node 1\n(DC1)" as ScyllaDB1
    database "ScyllaDB Node 2\n(DC1)" as ScyllaDB2
    database "ScyllaDB Node 3\n(DC2)" as ScyllaDB3
}
```

**Production ScyllaDB Configuration:**

**ScyllaDB Cluster Setup:**
```yaml
apiVersion: v1
kind: Service
metadata:
  name: scylla-cluster
  namespace: scylla
spec:
  clusterIP: None
  selector:
    app: scylla
  ports:
  - port: 9042
    name: cql
  - port: 7000
    name: intra-node
  - port: 7001
    name: tls-intra-node
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: scylla
  namespace: scylla
spec:
  serviceName: scylla-cluster
  replicas: 3
  selector:
    matchLabels:
      app: scylla
  template:
    metadata:
      labels:
        app: scylla
    spec:
      containers:
      - name: scylla
        image: scylladb/scylla:latest
        args:
        - --cluster-name=prod-cluster
        - --seeds=scylla-0.scylla-cluster.scylla.svc.cluster.local
        - --smp=4
        - --memory=8G
        - --api-address=0.0.0.0
        - --rpc-address=0.0.0.0
        - --listen-address=$(POD_IP)
        - --broadcast-address=$(POD_IP)
        - --broadcast-rpc-address=$(POD_IP)
        - --endpoint-snitch=GossipingPropertyFileSnitch
        - --client-encryption-options=enabled=true,optional=false,keyfile=/etc/ssl/private/server.key,certificate=/etc/ssl/certs/server.crt,truststore=/etc/ssl/certs/ca.crt,require_client_auth=true
        env:
        - name: POD_IP
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
        ports:
        - containerPort: 9042
          name: cql
        - containerPort: 7000
          name: intra-node
        - containerPort: 7001
          name: tls-intra-node
        volumeMounts:
        - name: data
          mountPath: /var/lib/scylla
        - name: tls-certs
          mountPath: /etc/ssl/certs
          readOnly: true
        - name: tls-private
          mountPath: /etc/ssl/private
          readOnly: true
      volumes:
      - name: tls-certs
        secret:
          secretName: scylla-tls-certs
      - name: tls-private
        secret:
          secretName: scylla-tls-private
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 100Gi
```

**Multi-DC Connection Configuration:**
```go
// Production multi-DC connection (Task 2.1 + 2.2)
func connectProductionMultiDC(cfg *Config) (*gocql.Session, error) {
    // Validate production requirements
    if err := cfg.ValidateProduction(); err != nil {
        return nil, fmt.Errorf("production validation failed: %w", err)
    }
    
    // Create secure cluster configuration
    cluster, err := cfg.CreateSecureClusterConfig()
    if err != nil {
        return nil, fmt.Errorf("failed to create secure cluster config: %w", err)
    }
    
    // Production-specific settings
    cluster.ProtoVersion = 4
    cluster.DisableInitialHostLookup = false
    cluster.IgnorePeerAddr = false
    
    // Multi-DC configuration
    if cfg.IsMultiDC() {
        log.Printf("Configuring for Multi-DC deployment in %s", cfg.LocalDC)
        
        // DC-aware routing with token awareness
        policy := gocql.DCAwareRoundRobinPolicy(cfg.LocalDC)
        if cfg.TokenAwareRouting {
            cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(policy)
        } else {
            cluster.PoolConfig.HostSelectionPolicy = policy
        }
    }
    
    // Create session with retry
    var session *gocql.Session
    for i := 0; i < 3; i++ {
        session, err = cluster.CreateSession()
        if err == nil {
            break
        }
        log.Printf("Failed to create session (attempt %d/3): %v", i+1, err)
        time.Sleep(time.Duration(i+1) * 5 * time.Second)
    }
    
    if err != nil {
        return nil, fmt.Errorf("failed to create production session after 3 attempts: %w", err)
    }
    
    log.Printf("Connected to ScyllaDB cluster: %s", cfg.GetConnectionString())
    return session, nil
}
```

### Security Infrastructure

#### Certificate Authority
```plantuml
component "Certificate Authority" as CA
```

**Certificate Authority Integration:**

**CA Certificate Management:**
```go
// Certificate Authority integration (Task 2.2)
type CertificateManager struct {
    caConfig *CAConfig
}

type CAConfig struct {
    CACertFile     string
    CAKeyFile      string
    CertValidityDays int
    KeySize        int
}

// Generate client certificate from CA
func (cm *CertificateManager) GenerateClientCertificate(commonName string) (*tls.Certificate, error) {
    // Load CA certificate and key
    caCert, err := tls.LoadX509KeyPair(cm.caConfig.CACertFile, cm.caConfig.CAKeyFile)
    if err != nil {
        return nil, fmt.Errorf("failed to load CA certificate: %w", err)
    }
    
    // Parse CA certificate
    caCertParsed, err := x509.ParseCertificate(caCert.Certificate[0])
    if err != nil {
        return nil, fmt.Errorf("failed to parse CA certificate: %w", err)
    }
    
    // Generate client private key
    clientKey, err := rsa.GenerateKey(rand.Reader, cm.caConfig.KeySize)
    if err != nil {
        return nil, fmt.Errorf("failed to generate client key: %w", err)
    }
    
    // Create client certificate template
    template := x509.Certificate{
        SerialNumber: big.NewInt(time.Now().Unix()),
        Subject: pkix.Name{
            CommonName: commonName,
            Organization: []string{"IPFS Cluster"},
        },
        NotBefore:    time.Now(),
        NotAfter:     time.Now().Add(time.Duration(cm.caConfig.CertValidityDays) * 24 * time.Hour),
        KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
        ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
    }
    
    // Create client certificate
    clientCertDER, err := x509.CreateCertificate(rand.Reader, &template, caCertParsed, &clientKey.PublicKey, caCert.PrivateKey)
    if err != nil {
        return nil, fmt.Errorf("failed to create client certificate: %w", err)
    }
    
    // Create TLS certificate
    clientCert := tls.Certificate{
        Certificate: [][]byte{clientCertDER},
        PrivateKey:  clientKey,
    }
    
    return &clientCert, nil
}

// Validate certificate against CA
func (cm *CertificateManager) ValidateCertificate(certFile string) error {
    // Load certificate
    certPEM, err := os.ReadFile(certFile)
    if err != nil {
        return fmt.Errorf("failed to read certificate: %w", err)
    }
    
    block, _ := pem.Decode(certPEM)
    if block == nil {
        return fmt.Errorf("failed to decode certificate PEM")
    }
    
    cert, err := x509.ParseCertificate(block.Bytes)
    if err != nil {
        return fmt.Errorf("failed to parse certificate: %w", err)
    }
    
    // Load CA certificate
    caCertPEM, err := os.ReadFile(cm.caConfig.CACertFile)
    if err != nil {
        return fmt.Errorf("failed to read CA certificate: %w", err)
    }
    
    caCertPool := x509.NewCertPool()
    if !caCertPool.AppendCertsFromPEM(caCertPEM) {
        return fmt.Errorf("failed to parse CA certificate")
    }
    
    // Verify certificate
    opts := x509.VerifyOptions{
        Roots: caCertPool,
    }
    
    _, err = cert.Verify(opts)
    if err != nil {
        return fmt.Errorf("certificate verification failed: %w", err)
    }
    
    // Check expiration
    if time.Now().After(cert.NotAfter) {
        return fmt.Errorf("certificate has expired")
    }
    
    if time.Now().Add(30 * 24 * time.Hour).After(cert.NotAfter) {
        log.Printf("Warning: certificate expires soon (%v)", cert.NotAfter)
    }
    
    return nil
}
```

#### Secret Management
```plantuml
component "Secret Management" as Secrets
```

**Secret Management Integration:**

**Environment Variable Management:**
```go
// Secret management for production (Task 2.1 + 2.2)
type SecretManager struct {
    vaultClient *vault.Client
    secretPath  string
}

// Load secrets from Vault
func (sm *SecretManager) LoadSecrets(cfg *Config) error {
    // Read secrets from Vault
    secret, err := sm.vaultClient.Logical().Read(sm.secretPath)
    if err != nil {
        return fmt.Errorf("failed to read secrets from Vault: %w", err)
    }
    
    if secret == nil || secret.Data == nil {
        return fmt.Errorf("no secrets found at path: %s", sm.secretPath)
    }
    
    // Apply secrets to configuration
    if username, ok := secret.Data["username"].(string); ok {
        cfg.Username = username
    }
    
    if password, ok := secret.Data["password"].(string); ok {
        cfg.Password = password
    }
    
    if tlsCert, ok := secret.Data["tls_cert"].(string); ok {
        // Write certificate to temporary file
        certFile, err := sm.writeTempFile("client.crt", tlsCert)
        if err != nil {
            return fmt.Errorf("failed to write TLS cert: %w", err)
        }
        cfg.TLSCertFile = certFile
    }
    
    if tlsKey, ok := secret.Data["tls_key"].(string); ok {
        // Write key to temporary file
        keyFile, err := sm.writeTempFile("client.key", tlsKey)
        if err != nil {
            return fmt.Errorf("failed to write TLS key: %w", err)
        }
        cfg.TLSKeyFile = keyFile
    }
    
    if caCert, ok := secret.Data["ca_cert"].(string); ok {
        // Write CA cert to temporary file
        caFile, err := sm.writeTempFile("ca.crt", caCert)
        if err != nil {
            return fmt.Errorf("failed to write CA cert: %w", err)
        }
        cfg.TLSCAFile = caFile
    }
    
    return nil
}

// Write secret to temporary file
func (sm *SecretManager) writeTempFile(filename, content string) (string, error) {
    tmpDir := "/tmp/scylla-certs"
    if err := os.MkdirAll(tmpDir, 0700); err != nil {
        return "", fmt.Errorf("failed to create temp dir: %w", err)
    }
    
    filePath := filepath.Join(tmpDir, filename)
    if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
        return "", fmt.Errorf("failed to write file: %w", err)
    }
    
    return filePath, nil
}
```

#### Security Assessment
```plantuml
component "Security Assessment" as SecurityAssessment
```

**Security Assessment Implementation:**
```go
// Security assessment for deployment (Task 2.2)
type DeploymentSecurityAssessment struct {
    config *Config
}

// Assess deployment security
func (dsa *DeploymentSecurityAssessment) AssessDeployment() (*SecurityReport, error) {
    report := &SecurityReport{
        Timestamp: time.Now(),
        Environment: os.Getenv("ENVIRONMENT"),
    }
    
    // Get basic security level
    level, score, issues := dsa.config.GetSecurityLevel()
    report.SecurityLevel = level
    report.SecurityScore = score
    report.SecurityIssues = issues
    
    // Additional deployment-specific checks
    if err := dsa.assessDeploymentSpecific(report); err != nil {
        return nil, fmt.Errorf("deployment assessment failed: %w", err)
    }
    
    return report, nil
}

// Deployment-specific security assessment
func (dsa *DeploymentSecurityAssessment) assessDeploymentSpecific(report *SecurityReport) error {
    // Check certificate expiration
    if dsa.config.TLSEnabled && dsa.config.TLSCertFile != "" {
        if err := dsa.checkCertificateExpiration(report); err != nil {
            return err
        }
    }
    
    // Check network security
    if err := dsa.checkNetworkSecurity(report); err != nil {
        return err
    }
    
    // Check access controls
    if err := dsa.checkAccessControls(report); err != nil {
        return err
    }
    
    return nil
}

type SecurityReport struct {
    Timestamp       time.Time
    Environment     string
    SecurityLevel   string
    SecurityScore   int
    SecurityIssues  []string
    CertExpiration  *time.Time
    NetworkSecurity map[string]bool
    AccessControls  map[string]bool
    Recommendations []string
}
```

## Deployment Configuration Notes

### Development Configuration Note
```plantuml
note right of DevConfig
  **Task 2.1 Config:**
  - Single host: localhost:9042
  - Basic validation
  - JSON configuration
  - Environment variables
end note

note right of DevTLS
  **Task 2.2 TLS:**
  - TLS enabled
  - InsecureSkipVerify: true
  - Self-signed certificates
  - Development validation
end note
```

**Development Implementation:**
- Single ScyllaDB node for simplicity
- Relaxed security settings for ease of development
- Self-signed certificates acceptable
- Development validation level allows insecure settings

### Production Configuration Note
```plantuml
note right of ProdConfig1
  **Task 2.1 Config:**
  - Multiple hosts
  - Strict validation
  - Environment variables
  - Production defaults
end note

note right of ProdTLS1
  **Task 2.2 TLS:**
  - TLS required
  - Client certificates
  - CA validation
  - Production validation
end note
```

**Production Implementation:**
- Multi-node ScyllaDB cluster for high availability
- Strict security requirements enforced
- CA-signed certificates required
- Production validation level enforces security

## Bridge to Implementation

### Deployment Architecture → Configuration Implementation

1. **Development Deployment** → **Relaxed Configuration**:
   - DevelopmentValidationOptions() allows insecure settings
   - Single host configuration acceptable
   - Self-signed certificates supported

2. **Production Deployment** → **Strict Configuration**:
   - StrictValidationOptions() enforces security requirements
   - Multi-host configuration required
   - CA-signed certificates mandatory

3. **Security Infrastructure** → **Security Implementation**:
   - Certificate Authority integration for certificate management
   - Secret Management for secure credential handling
   - Security Assessment for continuous security monitoring

### Environment-Specific Features

**Development Features:**
- Quick setup with minimal security
- Local development support
- Self-signed certificate generation
- Relaxed validation rules

**Production Features:**
- High availability with multiple nodes
- Strict security enforcement
- CA-signed certificate requirement
- Comprehensive security assessment
- Multi-DC support with proper routing

This deployment diagram effectively bridges the architectural design with real-world deployment scenarios, showing how Task 2.1 and Task 2.2 implementations adapt to different environments while maintaining security and functionality requirements.