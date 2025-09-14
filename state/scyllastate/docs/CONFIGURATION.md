# ScyllaDB State Store Configuration Guide

This guide provides detailed configuration instructions for the ScyllaDB state store in various deployment scenarios.

## Table of Contents

- [Configuration Structure](#configuration-structure)
- [Basic Configuration](#basic-configuration)
- [Production Configuration](#production-configuration)
- [Multi-Datacenter Configuration](#multi-datacenter-configuration)
- [Security Configuration](#security-configuration)
- [Performance Tuning](#performance-tuning)
- [Environment-Specific Examples](#environment-specific-examples)
- [Configuration Validation](#configuration-validation)

## Configuration Structure

The ScyllaDB state store configuration is defined by the `Config` struct:

```go
type Config struct {
    // Connection Settings
    Hosts                        []string      `json:"hosts"`
    Port                         int           `json:"port"`
    Keyspace                     string        `json:"keyspace"`
    Username                     string        `json:"username"`
    Password                     string        `json:"password"`
    
    // Timeout Settings
    Timeout                      time.Duration `json:"timeout"`
    ConnectTimeout               time.Duration `json:"connect_timeout"`
    
    // Connection Pool Settings
    NumConns                     int           `json:"num_conns"`
    MaxWaitTime                  time.Duration `json:"max_wait_time"`
    
    // Consistency Settings
    Consistency                  string        `json:"consistency"`
    SerialConsistency            string        `json:"serial_consistency"`
    
    // Routing Settings
    TokenAwareRouting            bool          `json:"token_aware_routing"`
    DCAwareRouting               bool          `json:"dc_aware_routing"`
    LocalDC                      string        `json:"local_dc"`
    
    // Performance Settings
    PageSize                     int           `json:"page_size"`
    PreparedStatementCacheSize   int           `json:"prepared_statement_cache_size"`
    
    // TLS Settings
    TLS                          *TLSConfig    `json:"tls"`
    
    // Retry Policy Settings
    RetryPolicy                  *RetryPolicyConfig `json:"retry_policy"`
    
    // Monitoring Settings
    MetricsEnabled               bool          `json:"metrics_enabled"`
    TracingEnabled               bool          `json:"tracing_enabled"`
}
```

## Basic Configuration

### Minimal Configuration

For development or testing with a single ScyllaDB node:

```json
{
  "hosts": ["127.0.0.1"],
  "port": 9042,
  "keyspace": "ipfs_pins_dev",
  "consistency": "ONE"
}
```

```go
config := &scyllastate.Config{
    Hosts:       []string{"127.0.0.1"},
    Port:        9042,
    Keyspace:    "ipfs_pins_dev",
    Consistency: "ONE",
}
config.Default() // Apply default values
```

### Standard Configuration

For small production deployments:

```json
{
  "hosts": ["10.0.1.10", "10.0.1.11", "10.0.1.12"],
  "port": 9042,
  "keyspace": "ipfs_pins",
  "username": "ipfs_user",
  "password": "secure_password",
  "timeout": "10s",
  "connect_timeout": "5s",
  "num_conns": 4,
  "consistency": "QUORUM",
  "token_aware_routing": true,
  "page_size": 5000,
  "metrics_enabled": true
}
```

## Production Configuration

### High-Performance Configuration

Optimized for high-throughput production environments:

```json
{
  "hosts": [
    "scylla-node1.prod.example.com",
    "scylla-node2.prod.example.com", 
    "scylla-node3.prod.example.com",
    "scylla-node4.prod.example.com",
    "scylla-node5.prod.example.com",
    "scylla-node6.prod.example.com"
  ],
  "port": 9042,
  "keyspace": "ipfs_pins_prod",
  "username": "ipfs_cluster_prod",
  "password": "${SCYLLA_PASSWORD}",
  
  "timeout": "15s",
  "connect_timeout": "10s",
  "num_conns": 8,
  "max_wait_time": "2s",
  
  "consistency": "QUORUM",
  "serial_consistency": "SERIAL",
  
  "token_aware_routing": true,
  "dc_aware_routing": true,
  "local_dc": "datacenter1",
  
  "page_size": 10000,
  "prepared_statement_cache_size": 2000,
  
  "retry_policy": {
    "num_retries": 5,
    "min_retry_delay": "100ms",
    "max_retry_delay": "30s",
    "retryable_errors": ["timeout", "unavailable", "overloaded"]
  },
  
  "metrics_enabled": true,
  "tracing_enabled": false
}
```

### Enterprise Configuration

For large-scale enterprise deployments with security:

```json
{
  "hosts": [
    "scylla-dc1-node1.enterprise.com",
    "scylla-dc1-node2.enterprise.com",
    "scylla-dc1-node3.enterprise.com",
    "scylla-dc2-node1.enterprise.com",
    "scylla-dc2-node2.enterprise.com",
    "scylla-dc2-node3.enterprise.com"
  ],
  "port": 9142,
  "keyspace": "ipfs_pins_enterprise",
  "username": "ipfs_cluster_service",
  "password": "${SCYLLA_SERVICE_PASSWORD}",
  
  "timeout": "20s",
  "connect_timeout": "15s",
  "num_conns": 12,
  "max_wait_time": "5s",
  
  "consistency": "LOCAL_QUORUM",
  "serial_consistency": "LOCAL_SERIAL",
  
  "token_aware_routing": true,
  "dc_aware_routing": true,
  "local_dc": "datacenter1",
  
  "page_size": 15000,
  "prepared_statement_cache_size": 5000,
  
  "tls": {
    "enabled": true,
    "cert_file": "/etc/ssl/certs/scylla-client.pem",
    "key_file": "/etc/ssl/private/scylla-client-key.pem",
    "ca_file": "/etc/ssl/certs/scylla-ca.pem",
    "insecure_skip_verify": false,
    "server_name": "scylla.enterprise.com"
  },
  
  "retry_policy": {
    "num_retries": 7,
    "min_retry_delay": "50ms",
    "max_retry_delay": "60s",
    "retryable_errors": ["timeout", "unavailable", "overloaded", "read_timeout"]
  },
  
  "metrics_enabled": true,
  "tracing_enabled": true
}
```

## Multi-Datacenter Configuration

### Two-Datacenter Setup

Configuration for active-active multi-datacenter deployment:

```json
{
  "hosts": [
    "scylla-us-east-1.example.com",
    "scylla-us-east-2.example.com",
    "scylla-us-east-3.example.com",
    "scylla-us-west-1.example.com",
    "scylla-us-west-2.example.com",
    "scylla-us-west-3.example.com"
  ],
  "port": 9042,
  "keyspace": "ipfs_pins_multidc",
  
  "consistency": "LOCAL_QUORUM",
  "serial_consistency": "LOCAL_SERIAL",
  
  "dc_aware_routing": true,
  "local_dc": "us-east",
  "token_aware_routing": true,
  
  "timeout": "25s",
  "connect_timeout": "15s",
  "num_conns": 6,
  
  "retry_policy": {
    "num_retries": 5,
    "min_retry_delay": "200ms",
    "max_retry_delay": "45s"
  }
}
```

### Cross-Region Configuration

For global deployments across multiple regions:

```json
{
  "hosts": [
    "scylla-us-east.global.com",
    "scylla-us-west.global.com", 
    "scylla-eu-west.global.com",
    "scylla-ap-southeast.global.com"
  ],
  "port": 9042,
  "keyspace": "ipfs_pins_global",
  
  "consistency": "LOCAL_QUORUM",
  "serial_consistency": "LOCAL_SERIAL",
  
  "dc_aware_routing": true,
  "local_dc": "us-east",
  "token_aware_routing": true,
  
  "timeout": "60s",
  "connect_timeout": "30s",
  "num_conns": 4,
  
  "retry_policy": {
    "num_retries": 8,
    "min_retry_delay": "500ms",
    "max_retry_delay": "120s"
  }
}
```

## Security Configuration

### TLS Configuration

#### Basic TLS Setup

```json
{
  "tls": {
    "enabled": true,
    "insecure_skip_verify": false
  }
}
```

#### Mutual TLS (mTLS) Setup

```json
{
  "tls": {
    "enabled": true,
    "cert_file": "/etc/ipfs-cluster/ssl/client.crt",
    "key_file": "/etc/ipfs-cluster/ssl/client.key",
    "ca_file": "/etc/ipfs-cluster/ssl/ca.crt",
    "insecure_skip_verify": false,
    "server_name": "scylla-cluster.internal"
  }
}
```

#### TLS with Custom Cipher Suites

```json
{
  "tls": {
    "enabled": true,
    "cert_file": "/etc/ssl/certs/client.pem",
    "key_file": "/etc/ssl/private/client-key.pem",
    "ca_file": "/etc/ssl/certs/ca.pem",
    "min_version": "1.2",
    "max_version": "1.3",
    "cipher_suites": [
      "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
      "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"
    ]
  }
}
```

### Authentication Configuration

#### Username/Password Authentication

```json
{
  "username": "ipfs_cluster_user",
  "password": "${SCYLLA_PASSWORD}",
  "auth_provider": "password"
}
```

#### LDAP Authentication

```json
{
  "username": "cn=ipfs-cluster,ou=services,dc=example,dc=com",
  "password": "${LDAP_PASSWORD}",
  "auth_provider": "ldap"
}
```

## Performance Tuning

### High-Throughput Configuration

Optimized for maximum throughput:

```json
{
  "num_conns": 16,
  "max_wait_time": "1s",
  "timeout": "5s",
  "connect_timeout": "3s",
  
  "page_size": 20000,
  "prepared_statement_cache_size": 10000,
  
  "consistency": "ONE",
  "token_aware_routing": true,
  
  "retry_policy": {
    "num_retries": 3,
    "min_retry_delay": "50ms",
    "max_retry_delay": "5s"
  }
}
```

### Low-Latency Configuration

Optimized for minimal latency:

```json
{
  "num_conns": 8,
  "max_wait_time": "500ms",
  "timeout": "2s",
  "connect_timeout": "1s",
  
  "page_size": 1000,
  "prepared_statement_cache_size": 5000,
  
  "consistency": "ONE",
  "token_aware_routing": true,
  "dc_aware_routing": true,
  
  "retry_policy": {
    "num_retries": 2,
    "min_retry_delay": "10ms",
    "max_retry_delay": "1s"
  }
}
```

### Balanced Configuration

Good balance between throughput and latency:

```json
{
  "num_conns": 6,
  "max_wait_time": "2s",
  "timeout": "10s",
  "connect_timeout": "5s",
  
  "page_size": 5000,
  "prepared_statement_cache_size": 2000,
  
  "consistency": "QUORUM",
  "token_aware_routing": true,
  
  "retry_policy": {
    "num_retries": 4,
    "min_retry_delay": "100ms",
    "max_retry_delay": "15s"
  }
}
```

## Environment-Specific Examples

### Development Environment

```json
{
  "hosts": ["localhost"],
  "port": 9042,
  "keyspace": "ipfs_pins_dev",
  "consistency": "ONE",
  "timeout": "30s",
  "num_conns": 2,
  "metrics_enabled": true,
  "tracing_enabled": true
}
```

### Testing Environment

```json
{
  "hosts": ["scylla-test1", "scylla-test2"],
  "port": 9042,
  "keyspace": "ipfs_pins_test",
  "consistency": "QUORUM",
  "timeout": "15s",
  "num_conns": 3,
  "token_aware_routing": true,
  "metrics_enabled": true,
  "retry_policy": {
    "num_retries": 3,
    "min_retry_delay": "100ms",
    "max_retry_delay": "10s"
  }
}
```

### Staging Environment

```json
{
  "hosts": [
    "scylla-staging1.internal",
    "scylla-staging2.internal",
    "scylla-staging3.internal"
  ],
  "port": 9042,
  "keyspace": "ipfs_pins_staging",
  "username": "ipfs_staging",
  "password": "${SCYLLA_STAGING_PASSWORD}",
  
  "consistency": "QUORUM",
  "timeout": "20s",
  "connect_timeout": "10s",
  "num_conns": 4,
  
  "token_aware_routing": true,
  "page_size": 7500,
  
  "tls": {
    "enabled": true,
    "insecure_skip_verify": true
  },
  
  "metrics_enabled": true,
  "tracing_enabled": false
}
```

### Production Environment

```json
{
  "hosts": [
    "scylla-prod1.company.com",
    "scylla-prod2.company.com",
    "scylla-prod3.company.com",
    "scylla-prod4.company.com",
    "scylla-prod5.company.com",
    "scylla-prod6.company.com"
  ],
  "port": 9142,
  "keyspace": "ipfs_pins_production",
  "username": "ipfs_cluster_prod",
  "password": "${SCYLLA_PROD_PASSWORD}",
  
  "timeout": "30s",
  "connect_timeout": "15s",
  "num_conns": 8,
  "max_wait_time": "3s",
  
  "consistency": "QUORUM",
  "serial_consistency": "SERIAL",
  
  "token_aware_routing": true,
  "dc_aware_routing": true,
  "local_dc": "datacenter1",
  
  "page_size": 10000,
  "prepared_statement_cache_size": 3000,
  
  "tls": {
    "enabled": true,
    "cert_file": "/etc/ssl/certs/ipfs-cluster-prod.pem",
    "key_file": "/etc/ssl/private/ipfs-cluster-prod-key.pem",
    "ca_file": "/etc/ssl/certs/company-ca.pem",
    "insecure_skip_verify": false,
    "server_name": "scylla-cluster.company.com"
  },
  
  "retry_policy": {
    "num_retries": 6,
    "min_retry_delay": "100ms",
    "max_retry_delay": "45s",
    "retryable_errors": ["timeout", "unavailable", "overloaded"]
  },
  
  "metrics_enabled": true,
  "tracing_enabled": false
}
```

## Configuration Validation

### Validation Function

```go
func validateConfig(config *scyllastate.Config) error {
    if len(config.Hosts) == 0 {
        return fmt.Errorf("at least one host must be specified")
    }
    
    if config.Port <= 0 || config.Port > 65535 {
        return fmt.Errorf("invalid port: %d", config.Port)
    }
    
    if config.Keyspace == "" {
        return fmt.Errorf("keyspace must be specified")
    }
    
    validConsistencies := []string{"ONE", "TWO", "THREE", "QUORUM", "ALL", "LOCAL_QUORUM", "EACH_QUORUM", "LOCAL_ONE"}
    if !contains(validConsistencies, config.Consistency) {
        return fmt.Errorf("invalid consistency level: %s", config.Consistency)
    }
    
    if config.NumConns <= 0 {
        return fmt.Errorf("num_conns must be positive")
    }
    
    if config.Timeout <= 0 {
        return fmt.Errorf("timeout must be positive")
    }
    
    return nil
}
```

### Configuration Testing

```go
func testConfiguration(config *scyllastate.Config) error {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // Test connection
    state, err := scyllastate.New(ctx, config)
    if err != nil {
        return fmt.Errorf("failed to create state store: %w", err)
    }
    defer state.Close()
    
    // Test basic operations
    testPin := createTestPin()
    
    // Test write
    err = state.Add(ctx, testPin)
    if err != nil {
        return fmt.Errorf("failed to add test pin: %w", err)
    }
    
    // Test read
    _, err = state.Get(ctx, testPin.Cid)
    if err != nil {
        return fmt.Errorf("failed to get test pin: %w", err)
    }
    
    // Cleanup
    state.Rm(ctx, testPin.Cid)
    
    return nil
}
```

### Environment Variable Substitution

```go
func loadConfigWithEnvSubstitution(filename string) (*scyllastate.Config, error) {
    data, err := ioutil.ReadFile(filename)
    if err != nil {
        return nil, err
    }
    
    // Substitute environment variables
    configStr := os.ExpandEnv(string(data))
    
    config := &scyllastate.Config{}
    err = json.Unmarshal([]byte(configStr), config)
    if err != nil {
        return nil, err
    }
    
    // Apply defaults and validate
    config.Default()
    err = validateConfig(config)
    if err != nil {
        return nil, err
    }
    
    return config, nil
}
```

### Configuration Best Practices

1. **Use Environment Variables**: Store sensitive information like passwords in environment variables
2. **Validate Configuration**: Always validate configuration before using it
3. **Test Configuration**: Test configuration in non-production environments first
4. **Monitor Performance**: Adjust configuration based on performance metrics
5. **Document Changes**: Keep track of configuration changes and their impact
6. **Use Appropriate Consistency**: Choose consistency levels based on your requirements
7. **Enable Monitoring**: Always enable metrics in production environments
8. **Secure Connections**: Use TLS in production environments
9. **Plan for Failures**: Configure appropriate retry policies
10. **Regular Reviews**: Regularly review and optimize configuration

For more detailed information about specific configuration options, see the [main README](../README.md) and [performance tuning guide](./PERFORMANCE.md).