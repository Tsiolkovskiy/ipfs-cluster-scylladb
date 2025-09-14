package auth

import (
	"context"
	"time"
)

// AuthMethod represents different authentication methods
type AuthMethod string

const (
	AuthMethodJWT    AuthMethod = "jwt"
	AuthMethodAPIKey AuthMethod = "apikey"
	AuthMethodDID    AuthMethod = "did"
	AuthMethodOIDC   AuthMethod = "oidc"
	AuthMethodWeb3   AuthMethod = "web3"
)

// Principal represents an authenticated entity
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

// AuthContext contains authentication and authorization information
type AuthContext struct {
	Principal *Principal        `json:"principal"`
	Resource  string            `json:"resource"`
	Action    string            `json:"action"`
	Context   map[string]string `json:"context"`
}

// Authenticator interface for different authentication methods
type Authenticator interface {
	// Authenticate validates credentials and returns a Principal
	Authenticate(ctx context.Context, token string) (*Principal, error)
	
	// GetMethod returns the authentication method this authenticator handles
	GetMethod() AuthMethod
	
	// ValidateToken checks if a token is valid without full authentication
	ValidateToken(ctx context.Context, token string) error
}

// Authorizer interface for authorization decisions
type Authorizer interface {
	// Authorize checks if the principal is authorized to perform the action on the resource
	Authorize(ctx context.Context, authCtx *AuthContext) error
	
	// GetPermissions returns the permissions for a principal
	GetPermissions(ctx context.Context, principal *Principal) ([]Permission, error)
}

// Permission represents a specific permission
type Permission struct {
	Resource string            `json:"resource"`
	Actions  []string          `json:"actions"`
	Context  map[string]string `json:"context,omitempty"`
}

// AuditLogger interface for recording access attempts
type AuditLogger interface {
	// LogAccess records an access attempt
	LogAccess(ctx context.Context, event *AccessEvent) error
	
	// LogAuth records an authentication event
	LogAuth(ctx context.Context, event *AuthEvent) error
}

// AccessEvent represents an access attempt
type AccessEvent struct {
	Timestamp   time.Time         `json:"timestamp"`
	Principal   *Principal        `json:"principal,omitempty"`
	Resource    string            `json:"resource"`
	Action      string            `json:"action"`
	Result      AccessResult      `json:"result"`
	Error       string            `json:"error,omitempty"`
	Context     map[string]string `json:"context,omitempty"`
	RequestID   string            `json:"request_id,omitempty"`
	ClientIP    string            `json:"client_ip,omitempty"`
	UserAgent   string            `json:"user_agent,omitempty"`
}

// AuthEvent represents an authentication event
type AuthEvent struct {
	Timestamp time.Time    `json:"timestamp"`
	Method    AuthMethod   `json:"method"`
	Result    AuthResult   `json:"result"`
	Principal *Principal   `json:"principal,omitempty"`
	Error     string       `json:"error,omitempty"`
	ClientIP  string       `json:"client_ip,omitempty"`
	UserAgent string       `json:"user_agent,omitempty"`
}

// AccessResult represents the result of an access attempt
type AccessResult string

const (
	AccessGranted AccessResult = "granted"
	AccessDenied  AccessResult = "denied"
	AccessError   AccessResult = "error"
)

// AuthResult represents the result of an authentication attempt
type AuthResult string

const (
	AuthSuccess AuthResult = "success"
	AuthFailure AuthResult = "failure"
	AuthError   AuthResult = "error"
)

// Actions for IPFS Cluster operations
const (
	ActionPinAdd    = "pin:add"
	ActionPinRemove = "pin:remove"
	ActionPinList   = "pin:list"
	ActionPinGet    = "pin:get"
	ActionPinUpdate = "pin:update"
	ActionStatus    = "status:get"
	ActionHealth    = "health:get"
	ActionMetrics   = "metrics:get"
	ActionAdmin     = "admin:*"
)

// Resources for IPFS Cluster
const (
	ResourcePin     = "pin"
	ResourceCluster = "cluster"
	ResourceMetrics = "metrics"
	ResourceHealth  = "health"
	ResourceAdmin   = "admin"
)