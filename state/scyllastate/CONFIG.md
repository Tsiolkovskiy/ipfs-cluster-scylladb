# ScyllaDB State Store Configuration

This document describes the configuration options for the ScyllaDB state store implementation.

## Configuration Structure

The ScyllaDB state store configuration is defined in the `Config` struct and supports the following categories of settings:

### Connection Settings

- `hosts` ([]string): List of ScyllaDB host addresses
- `port` (int): Port number for ScyllaDB connections (default: 9042)
- `keyspace` (string): Keyspace name for storing pin metadata (default: "ipfs_pins")
- `username` (string): Username for authentication (optional)
- `password` (string): Password for authentication (optional)

### TLS Settings

- `tls_enabled` (bool): Enable TLS encryption (default: false)
- `tls_cert_file` (string): Path to client certificate file (optional)
- `tls_key_file` (string): Path to client private key file (optional)
- `tls_ca_file` (string): Path to CA certificate file (optional)
- `tls_insecure_skip_verify` (bool): Skip TLS certificate verification (default: false)

### Performance Settings

- `num_conns` (int): Number of connections per host (default: 10)
- `timeout` (duration): Query timeout (default: 30s)
- `connect_timeout` (duration): Connection timeout (default: 10s)

### Consistency Settings

- `consistency` (string): Read/write consistency level (default: "QUORUM")
- `serial_consistency` (string): Serial consistency level (default: "SERIAL")

### Retry Policy Settings

- `retry_policy.num_retries` (int): Maximum number of retries (default: 3)
- `retry_policy.min_retry_delay` (duration): Minimum delay between retries (default: 100ms)
- `retry_policy.max_retry_delay` (duration): Maximum delay between retries (default: 10s)

### Batching Settings

- `batch_size` (int): Maximum number of operations per batch (default: 1000)
- `batch_timeout` (duration): Timeout for batch operations (default: 1s)

### Monitoring Settings

- `metrics_enabled` (bool): Enable Prometheus metrics (default: true)
- `tracing_enabled` (bool): Enable query tracing (default: false)

### Multi-Datacenter Settings

- `local_dc` (string): Local datacenter name (optional)
- `dc_aware_routing` (bool): Enable datacenter-aware routing (default: false)
- `token_aware_routing` (bool): Enable token-aware routing (default: false)

## Configuration Methods

### Default Configuration

```go
cfg := &Config{}
err := cfg.Default()
```

Sets all configuration values to their defaults.

### Environment Variable Override

```go
err := cfg.ApplyEnvVars()
```

Applies environment variable overrides:

- `SCYLLADB_HOSTS`: Comma-separated list of hosts
- `SCYLLADB_KEYSPACE`: Keyspace name
- `SCYLLADB_USERNAME`: Username
- `SCYLLADB_PASSWORD`: Password
- `SCYLLADB_TLS_ENABLED`: Enable TLS (true/false)
- `SCYLLADB_TLS_CERT_FILE`: TLS certificate file path
- `SCYLLADB_TLS_KEY_FILE`: TLS key file path
- `SCYLLADB_TLS_CA_FILE`: TLS CA file path
- `SCYLLADB_LOCAL_DC`: Local datacenter name

### Validation

#### Basic Validation

```go
err := cfg.Validate()
```

Performs basic configuration validation.

#### Advanced Validation

```go
opts := StrictValidationOptions()
err := cfg.ValidateWithOptions(opts)
```

Validation levels:
- `ValidationBasic`: Basic validation (default)
- `ValidationStrict`: Strict validation for production
- `ValidationDevelopment`: Relaxed validation for development

#### Production Validation

```go
err := cfg.ValidateProduction()
```

Validates configuration for production use with strict requirements:
- TLS must be enabled
- Authentication credentials must be provided
- TLS certificate verification cannot be disabled
- Strong consistency levels required
- Minimum of 3 hosts recommended

### JSON Serialization

#### Load from JSON

```go
jsonData := []byte(`{"hosts": ["localhost"], "port": 9042}`)
err := cfg.LoadJSON(jsonData)
```

#### Export to JSON

```go
jsonData, err := cfg.ToJSON()
```

#### Display JSON (hides sensitive data)

```go
displayJSON, err := cfg.ToDisplayJSON()
```

### Utility Methods

#### Get Effective Configuration

```go
effectiveCfg, err := cfg.GetEffectiveConfig()
```

Returns a copy of the configuration with environment variables applied.

#### Check Multi-DC Setup

```go
isMultiDC := cfg.IsMultiDC()
```

#### Get Connection String

```go
connStr := cfg.GetConnectionString()
```

Returns a human-readable connection string (without credentials).

#### Create Cluster Configuration

```go
cluster, err := cfg.CreateClusterConfig()
```

Creates a `gocql.ClusterConfig` from the configuration.

#### Validation Summary

```go
summary := cfg.GetValidationSummary()
```

Returns a detailed summary including security and performance scores.

## Example Configurations

### Development Configuration

```json
{
  "hosts": ["localhost"],
  "port": 9042,
  "keyspace": "ipfs_pins_dev",
  "tls_enabled": false,
  "consistency": "ONE",
  "num_conns": 2,
  "metrics_enabled": true
}
```

### Production Configuration

```json
{
  "hosts": ["scylla1.prod.com", "scylla2.prod.com", "scylla3.prod.com"],
  "port": 9042,
  "keyspace": "ipfs_pins",
  "username": "cluster_user",
  "password": "secure_password",
  "tls_enabled": true,
  "tls_cert_file": "/etc/ssl/certs/client.crt",
  "tls_key_file": "/etc/ssl/private/client.key",
  "tls_ca_file": "/etc/ssl/certs/ca.crt",
  "consistency": "QUORUM",
  "num_conns": 10,
  "timeout": "30s",
  "local_dc": "datacenter1",
  "dc_aware_routing": true,
  "token_aware_routing": true,
  "metrics_enabled": true
}
```

### Multi-Datacenter Configuration

```json
{
  "hosts": [
    "dc1-scylla1.example.com",
    "dc1-scylla2.example.com",
    "dc2-scylla1.example.com",
    "dc2-scylla2.example.com"
  ],
  "port": 9042,
  "keyspace": "ipfs_pins",
  "username": "cluster_user",
  "password": "secure_password",
  "tls_enabled": true,
  "consistency": "LOCAL_QUORUM",
  "local_dc": "datacenter1",
  "dc_aware_routing": true,
  "token_aware_routing": true,
  "num_conns": 15,
  "timeout": "45s"
}
```

## Consistency Levels

### Available Consistency Levels

- `ANY`: Weakest consistency, allows writes to succeed even if all replicas are down
- `ONE`: Requires response from at least one replica
- `TWO`: Requires response from at least two replicas
- `THREE`: Requires response from at least three replicas
- `QUORUM`: Requires response from majority of replicas
- `ALL`: Requires response from all replicas
- `LOCAL_QUORUM`: Requires response from majority of replicas in local datacenter
- `EACH_QUORUM`: Requires response from majority of replicas in each datacenter
- `LOCAL_ONE`: Requires response from at least one replica in local datacenter

### Serial Consistency Levels

- `SERIAL`: Global serial consistency
- `LOCAL_SERIAL`: Local datacenter serial consistency

## Best Practices

### Security

1. Always enable TLS in production
2. Use strong authentication credentials
3. Never disable TLS certificate verification in production
4. Use appropriate consistency levels (avoid ANY and ONE in production)

### Performance

1. Use token-aware routing for better performance
2. Configure appropriate connection pool sizes
3. Use LOCAL_QUORUM consistency in multi-DC setups
4. Enable metrics for monitoring
5. Tune batch sizes based on workload

### High Availability

1. Use at least 3 ScyllaDB nodes
2. Configure multiple datacenters for disaster recovery
3. Use appropriate replication factors
4. Monitor cluster health and performance

### Development

1. Use relaxed validation options for development
2. Consider using lower consistency levels for testing
3. Enable tracing for debugging
4. Use smaller connection pools for resource conservation