# IPFS Cluster Authentication and Authorization

This package provides authentication and authorization services for IPFS Cluster, supporting multiple authentication methods and flexible authorization policies.

## Features

### Authentication Methods
- **JWT Tokens**: JSON Web Token authentication with signature validation
- **API Keys**: Long-lived API key authentication with hashing and rotation
- **DID Signatures**: Decentralized Identity (DID) signature-based authentication
- **OIDC**: OpenID Connect integration with external identity providers
- **Web3**: Web3 wallet integration (MetaMask, WalletConnect)

### Authorization
- **RBAC**: Role-Based Access Control
- **ABAC**: Attribute-Based Access Control
- **Policy Engine**: Flexible policy evaluation system
- **Multi-tenancy**: Tenant-based data isolation

### Audit Logging
- **File Logger**: JSON-structured audit logs to file
- **Syslog Logger**: System log integration
- **Memory Logger**: In-memory logging for testing
- **ScyllaDB Logger**: Database-backed audit logging (planned)

## Architecture

### Core Components

#### AuthService
The main service that orchestrates authentication and authorization:

```go
type AuthService struct {
    config        *Config
    authenticators map[AuthMethod]Authenticator
    authorizer     Authorizer
    auditLogger    AuditLogger
}
```

#### Interfaces

**Authenticator**: Handles different authentication methods
```go
type Authenticator interface {
    Authenticate(ctx context.Context, token string) (*Principal, error)
    GetMethod() AuthMethod
    ValidateToken(ctx context.Context, token string) error
}
```

**Authorizer**: Makes authorization decisions
```go
type Authorizer interface {
    Authorize(ctx context.Context, authCtx *AuthContext) error
    GetPermissions(ctx context.Context, principal *Principal) ([]Permission, error)
}
```

**AuditLogger**: Records security events
```go
type AuditLogger interface {
    LogAccess(ctx context.Context, event *AccessEvent) error
    LogAuth(ctx context.Context, event *AuthEvent) error
}
```

### Data Models

#### Principal
Represents an authenticated entity:
```go
type Principal struct {
    ID       string            `json:"id"`
    Type     string            `json:"type"` // user, service, system
    TenantID string            `json:"tenant_id"`
    Roles    []string          `json:"roles"`
    Attrs    map[string]string `json:"attributes"`
    Method   AuthMethod        `json:"auth_method"`
    IssuedAt time.Time         `json:"issued_at"`
    ExpiresAt *time.Time       `json:"expires_at,omitempty"`
}
```

#### Permission
Defines access permissions:
```go
type Permission struct {
    Resource string            `json:"resource"`
    Actions  []string          `json:"actions"`
    Context  map[string]string `json:"context,omitempty"`
}
```

## Configuration

### Basic Configuration
```json
{
  "enabled": true,
  "default_tenant": "default",
  "session_timeout": "24h",
  "jwt_enabled": true,
  "jwt_secret": "your-secret-key",
  "jwt_issuer": "ipfs-cluster",
  "jwt_audience": "ipfs-cluster-api",
  "apikey_enabled": true,
  "apikey_ttl": "720h",
  "authz_enabled": true,
  "policy_engine": "rbac",
  "audit_enabled": true,
  "audit_backend": "file",
  "audit_log_path": "/var/log/ipfs-cluster/audit.log"
}
```

### Multi-tenant Configuration
```json
{
  "multi_tenant_enabled": true,
  "default_tenant": "public",
  "policy_engine": "abac"
}
```

### OIDC Configuration
```json
{
  "oidc_enabled": true,
  "oidc_issuer": "https://auth.example.com",
  "oidc_client_id": "ipfs-cluster",
  "oidc_client_secret": "client-secret"
}
```

## Usage

### Basic Setup
```go
// Create configuration
config := auth.DefaultConfig()
config.Enabled = true
config.JWTSecret = "your-secret-key"

// Create auth service
authService, err := auth.NewAuthService(ctx, config)
if err != nil {
    log.Fatal(err)
}

// Register authenticators (implemented in subtasks 12.2-12.4)
// authService.RegisterAuthenticator(jwtAuth)
// authService.RegisterAuthenticator(apiKeyAuth)
```

### Authentication
```go
// Authenticate a JWT token
principal, err := authService.Authenticate(ctx, auth.AuthMethodJWT, token)
if err != nil {
    return fmt.Errorf("authentication failed: %w", err)
}
```

### Authorization
```go
// Check authorization
authCtx := &auth.AuthContext{
    Principal: principal,
    Resource:  auth.ResourcePin,
    Action:    auth.ActionPinAdd,
    Context:   map[string]string{"cid": "QmExample"},
}

err := authService.Authorize(ctx, authCtx)
if err != nil {
    return fmt.Errorf("access denied: %w", err)
}
```

## Actions and Resources

### Supported Actions
- `pin:add` - Add new pins
- `pin:remove` - Remove existing pins
- `pin:list` - List pins
- `pin:get` - Get pin details
- `pin:update` - Update pin metadata
- `status:get` - Get cluster status
- `health:get` - Get health information
- `metrics:get` - Get metrics
- `admin:*` - Administrative operations

### Supported Resources
- `pin` - Pin operations
- `cluster` - Cluster operations
- `metrics` - Metrics access
- `health` - Health checks
- `admin` - Administrative functions

## Audit Events

### Access Events
Logged for every authorization check:
```json
{
  "type": "access",
  "timestamp": "2024-01-01T12:00:00Z",
  "principal": {
    "id": "user-123",
    "type": "user",
    "tenant_id": "default",
    "roles": ["user"],
    "method": "jwt"
  },
  "resource": "pin",
  "action": "pin:add",
  "result": "granted",
  "request_id": "req-456",
  "client_ip": "192.168.1.1",
  "context": {
    "cid": "QmExample"
  }
}
```

### Authentication Events
Logged for every authentication attempt:
```json
{
  "type": "auth",
  "timestamp": "2024-01-01T12:00:00Z",
  "method": "jwt",
  "result": "success",
  "principal": {
    "id": "user-123",
    "type": "user",
    "tenant_id": "default"
  },
  "client_ip": "192.168.1.1"
}
```

## Testing

Run the test suite:
```bash
go test -v ./auth/...
```

The package includes comprehensive tests for:
- Configuration serialization/deserialization
- AuthService functionality
- Audit logging (file and memory backends)
- Concurrent access patterns
- Error handling

## Implementation Status

### ✅ Completed (Task 12.1)
- Core interfaces and data models
- AuthService structure and basic functionality
- Audit logging system (file, syslog placeholder, memory)
- Configuration management
- Comprehensive test suite

### 🔄 Planned (Subsequent Tasks)
- **Task 12.2**: JWT, API Key, and DID authenticators
- **Task 12.3**: RBAC/ABAC policy engine and authorization
- **Task 12.4**: OIDC and Web3 wallet integration

## Security Considerations

1. **Token Storage**: Ensure JWT secrets and API keys are stored securely
2. **Audit Logs**: Protect audit logs from tampering and unauthorized access
3. **Multi-tenancy**: Ensure proper tenant isolation in authorization policies
4. **Rate Limiting**: Implement rate limiting for authentication attempts
5. **Secure Defaults**: Authentication is disabled by default for backward compatibility

## Dependencies

- `github.com/ipfs/go-log/v2` - Structured logging
- `github.com/stretchr/testify` - Testing framework

Additional dependencies will be added in subsequent tasks for JWT handling, cryptographic operations, and external integrations.