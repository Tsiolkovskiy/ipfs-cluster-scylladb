# TLS and Authentication Configuration for ScyllaDB State Store

This document describes the TLS and authentication features implemented for the ScyllaDB state store in IPFS-Cluster.

## Overview

The ScyllaDB state store supports comprehensive security features including:

- **TLS Encryption**: Secure communication between IPFS-Cluster and ScyllaDB
- **Client Certificate Authentication**: Mutual TLS authentication using X.509 certificates
- **Username/Password Authentication**: Traditional credential-based authentication
- **Certificate Authority Validation**: Custom CA certificate support
- **Production Security Validation**: Strict security requirements for production environments

## Configuration Options

### TLS Configuration

```json
{
  "scylladb": {
    "tls_enabled": true,
    "tls_cert_file": "/path/to/client.crt",
    "tls_key_file": "/path/to/client.key",
    "tls_ca_file": "/path/to/ca.crt",
    "tls_insecure_skip_verify": false
  }
}
```

#### TLS Fields

- `tls_enabled`: Enable/disable TLS encryption (default: false)
- `tls_cert_file`: Path to client certificate file (PEM format)
- `tls_key_file`: Path to client private key file (PEM format)
- `tls_ca_file`: Path to Certificate Authority certificate file (PEM format)
- `tls_insecure_skip_verify`: Skip certificate verification (default: false, **never use in production**)

### Authentication Configuration

```json
{
  "scylladb": {
    "username": "cluster_user",
    "password": "secure_password"
  }
}
```

#### Authentication Fields

- `username`: ScyllaDB username for authentication
- `password`: ScyllaDB password for authentication

### Environment Variables

All configuration options can be overridden using environment variables:

```bash
export SCYLLADB_TLS_ENABLED="true"
export SCYLLADB_TLS_CERT_FILE="/etc/ssl/certs/client.crt"
export SCYLLADB_TLS_KEY_FILE="/etc/ssl/private/client.key"
export SCYLLADB_TLS_CA_FILE="/etc/ssl/certs/ca.crt"
export SCYLLADB_USERNAME="cluster_user"
export SCYLLADB_PASSWORD="secure_password"
```

## Security Levels

The configuration system provides security assessment with different levels:

### Excellent (90-100 points)
- TLS enabled with certificate verification
- Client certificate authentication configured
- Strong consistency level (QUORUM, ALL, LOCAL_QUORUM, EACH_QUORUM)
- Multiple hosts for high availability (3+)

### Good (70-89 points)
- TLS enabled with certificate verification
- Username/password authentication
- Strong consistency level
- Multiple hosts

### Fair (50-69 points)
- TLS enabled but may have some security issues
- Authentication configured
- May have weak consistency or insufficient hosts

### Poor (30-49 points)
- Some security features missing
- May have TLS disabled or weak authentication

### Critical (0-29 points)
- Major security issues
- TLS disabled and/or no authentication
- Weak consistency levels

## Usage Examples

### Production Configuration

```go
cfg := &Config{}
cfg.Default()

// Security settings
cfg.TLSEnabled = true
cfg.TLSCertFile = "/etc/ssl/certs/scylla-client.crt"
cfg.TLSKeyFile = "/etc/ssl/private/scylla-client.key"
cfg.TLSCAFile = "/etc/ssl/certs/scylla-ca.crt"
cfg.TLSInsecureSkipVerify = false

// Authentication
cfg.Username = "cluster_service"
cfg.Password = "production_password"

// High availability
cfg.Hosts = []string{
    "scylla-node-1.prod.com",
    "scylla-node-2.prod.com", 
    "scylla-node-3.prod.com",
}

// Strong consistency
cfg.Consistency = "QUORUM"

// Validate for production
if err := cfg.ValidateProduction(); err != nil {
    log.Fatalf("Production validation failed: %v", err)
}

// Create secure cluster configuration
cluster, err := cfg.CreateSecureClusterConfig()
if err != nil {
    log.Fatalf("Failed to create secure cluster: %v", err)
}
```

### Development Configuration

```go
cfg := &Config{}
cfg.Default()

// Relaxed security for development
cfg.TLSEnabled = true
cfg.TLSInsecureSkipVerify = true // Only for development!
cfg.Username = "dev_user"
cfg.Password = "dev_password"

// Validate with development options
opts := DevelopmentValidationOptions()
if err := cfg.ValidateWithOptions(opts); err != nil {
    log.Fatalf("Development validation failed: %v", err)
}
```

### Multi-Datacenter Configuration

```go
cfg := &Config{}
cfg.Default()

// Multi-DC hosts
cfg.Hosts = []string{
    "dc1-scylla-1.example.com",
    "dc1-scylla-2.example.com",
    "dc2-scylla-1.example.com", 
    "dc2-scylla-2.example.com",
}

// DC-aware routing
cfg.DCAwareRouting = true
cfg.LocalDC = "datacenter1"
cfg.TokenAwareRouting = true

// Use LOCAL_QUORUM for multi-DC
cfg.Consistency = "LOCAL_QUORUM"

// Security
cfg.TLSEnabled = true
cfg.Username = "cluster_user"
cfg.Password = "secure_password"
```

## Certificate Management

### Generating Test Certificates

For development and testing, you can generate self-signed certificates:

```bash
# Generate CA private key
openssl genrsa -out ca-key.pem 4096

# Generate CA certificate
openssl req -new -x509 -days 365 -key ca-key.pem -sha256 -out ca.pem -subj "/C=US/ST=CA/L=San Francisco/O=Test CA/CN=Test CA"

# Generate client private key
openssl genrsa -out client-key.pem 4096

# Generate client certificate signing request
openssl req -subj '/CN=client' -new -key client-key.pem -out client.csr

# Generate client certificate signed by CA
openssl x509 -req -days 365 -in client.csr -CA ca.pem -CAkey ca-key.pem -out client.pem -extensions v3_req -CAcreateserial
```

### Certificate Validation

The system automatically validates certificates for:

- File existence and readability
- Valid certificate/key pair matching
- Certificate expiration (warns if expiring within 30 days)
- CA certificate validity

## Security Best Practices

### Production Deployment

1. **Always enable TLS** in production environments
2. **Use strong passwords** for authentication
3. **Configure client certificates** for mutual TLS authentication
4. **Use proper CA certificates** - never skip certificate verification
5. **Use strong consistency levels** (QUORUM, LOCAL_QUORUM, ALL)
6. **Deploy multiple hosts** for high availability (minimum 3)
7. **Regularly rotate certificates** before expiration
8. **Use environment variables** for sensitive configuration

### Certificate Security

1. **Protect private keys** with appropriate file permissions (600)
2. **Store certificates securely** in protected directories
3. **Use separate certificates** for each service/environment
4. **Monitor certificate expiration** and set up renewal processes
5. **Validate certificate chains** in production

### Network Security

1. **Use firewalls** to restrict access to ScyllaDB ports
2. **Enable DC-aware routing** for multi-datacenter deployments
3. **Use token-aware routing** for optimal performance
4. **Monitor connection metrics** for security anomalies

## Troubleshooting

### Common TLS Issues

**Certificate verification failed:**
- Check that CA certificate is correctly configured
- Verify certificate chain is complete
- Ensure certificate is not expired

**Connection refused:**
- Verify TLS is enabled on ScyllaDB server
- Check firewall rules and port accessibility
- Confirm certificate/key file permissions

**Authentication failed:**
- Verify username/password are correct
- Check ScyllaDB user permissions
- Ensure authentication is enabled on server

### Debugging Commands

```bash
# Test TLS connection
openssl s_client -connect scylla-host:9042 -cert client.pem -key client-key.pem -CAfile ca.pem

# Verify certificate
openssl x509 -in client.pem -text -noout

# Check certificate expiration
openssl x509 -in client.pem -checkend 86400

# Validate certificate/key pair
openssl x509 -noout -modulus -in client.pem | openssl md5
openssl rsa -noout -modulus -in client-key.pem | openssl md5
```

## API Reference

### Configuration Methods

- `CreateTLSConfig() (*tls.Config, error)` - Creates TLS configuration
- `CreateAuthenticator() gocql.Authenticator` - Creates authentication handler
- `CreateClusterConfig() (*gocql.ClusterConfig, error)` - Creates cluster configuration
- `CreateSecureClusterConfig() (*gocql.ClusterConfig, error)` - Creates secure cluster configuration
- `ValidateProduction() error` - Validates configuration for production use
- `ValidateTLSCertificates() error` - Validates TLS certificate files
- `GetSecurityLevel() (string, int, []string)` - Assesses security configuration
- `GetTLSInfo() map[string]interface{}` - Returns TLS configuration details
- `GetAuthInfo() map[string]interface{}` - Returns authentication configuration details

### Validation Options

- `DefaultValidationOptions()` - Standard validation rules
- `StrictValidationOptions()` - Production validation rules  
- `DevelopmentValidationOptions()` - Relaxed validation for development

## Integration with IPFS-Cluster

The TLS and authentication features integrate seamlessly with IPFS-Cluster's configuration system:

```json
{
  "state": {
    "datastore": "scylladb",
    "scylladb": {
      "hosts": ["scylla1.example.com", "scylla2.example.com", "scylla3.example.com"],
      "keyspace": "ipfs_pins",
      "tls_enabled": true,
      "tls_cert_file": "/etc/ssl/certs/client.crt",
      "tls_key_file": "/etc/ssl/private/client.key", 
      "tls_ca_file": "/etc/ssl/certs/ca.crt",
      "username": "cluster_user",
      "password": "secure_password",
      "consistency": "QUORUM"
    }
  }
}
```

This configuration ensures secure, authenticated connections between IPFS-Cluster and ScyllaDB while maintaining high performance and availability.