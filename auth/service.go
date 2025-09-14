package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

var logger = logging.Logger("auth")

// AuthService provides authentication and authorization services
type AuthService struct {
	config        *Config
	authenticators map[AuthMethod]Authenticator
	authorizer     Authorizer
	auditLogger    AuditLogger
	mu            sync.RWMutex
}

// Saver interface for configuration persistence
type Saver interface {
	Save() error
}

// Config holds configuration for the AuthService
type Config struct {
	Saver
	
	// General settings
	Enabled         bool          `json:"enabled"`
	DefaultTenant   string        `json:"default_tenant"`
	SessionTimeout  time.Duration `json:"session_timeout"`
	
	// JWT settings
	JWTEnabled      bool   `json:"jwt_enabled"`
	JWTSecret       string `json:"jwt_secret"`
	JWTIssuer       string `json:"jwt_issuer"`
	JWTAudience     string `json:"jwt_audience"`
	
	// API Key settings
	APIKeyEnabled   bool          `json:"apikey_enabled"`
	APIKeyTTL       time.Duration `json:"apikey_ttl"`
	
	// DID/Web3 settings
	DIDEnabled      bool `json:"did_enabled"`
	Web3Enabled     bool `json:"web3_enabled"`
	
	// OIDC settings
	OIDCEnabled     bool   `json:"oidc_enabled"`
	OIDCIssuer      string `json:"oidc_issuer"`
	OIDCClientID    string `json:"oidc_client_id"`
	OIDCClientSecret string `json:"oidc_client_secret"`
	
	// Authorization settings
	AuthzEnabled    bool   `json:"authz_enabled"`
	PolicyEngine    string `json:"policy_engine"` // rbac, abac, opa
	
	// Audit settings
	AuditEnabled    bool   `json:"audit_enabled"`
	AuditBackend    string `json:"audit_backend"` // file, syslog, scylladb
	AuditLogPath    string `json:"audit_log_path"`
	
	// Multi-tenancy settings
	MultiTenantEnabled bool `json:"multi_tenant_enabled"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:            false,
		DefaultTenant:      "default",
		SessionTimeout:     time.Hour * 24,
		JWTEnabled:         true,
		JWTIssuer:          "ipfs-cluster",
		JWTAudience:        "ipfs-cluster-api",
		APIKeyEnabled:      true,
		APIKeyTTL:          time.Hour * 24 * 30, // 30 days
		DIDEnabled:         false,
		Web3Enabled:        false,
		OIDCEnabled:        false,
		AuthzEnabled:       true,
		PolicyEngine:       "rbac",
		AuditEnabled:       true,
		AuditBackend:       "file",
		MultiTenantEnabled: false,
	}
}

// NewAuthService creates a new AuthService instance
func NewAuthService(ctx context.Context, config *Config) (*AuthService, error) {
	if !config.Enabled {
		return &AuthService{config: config}, nil
	}
	
	service := &AuthService{
		config:         config,
		authenticators: make(map[AuthMethod]Authenticator),
	}
	
	// Initialize authenticators based on configuration
	if err := service.initializeAuthenticators(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize authenticators: %w", err)
	}
	
	// Initialize authorizer
	if config.AuthzEnabled {
		if err := service.initializeAuthorizer(ctx); err != nil {
			return nil, fmt.Errorf("failed to initialize authorizer: %w", err)
		}
	}
	
	// Initialize audit logger
	if config.AuditEnabled {
		if err := service.initializeAuditLogger(ctx); err != nil {
			return nil, fmt.Errorf("failed to initialize audit logger: %w", err)
		}
	}
	
	logger.Info("AuthService initialized successfully")
	return service, nil
}

// Authenticate authenticates a token using the appropriate authenticator
func (s *AuthService) Authenticate(ctx context.Context, method AuthMethod, token string) (*Principal, error) {
	if !s.config.Enabled {
		return nil, fmt.Errorf("authentication is disabled")
	}
	
	s.mu.RLock()
	authenticator, exists := s.authenticators[method]
	s.mu.RUnlock()
	
	if !exists {
		err := fmt.Errorf("authenticator for method %s not found", method)
		s.logAuthEvent(ctx, method, AuthFailure, nil, err.Error())
		return nil, err
	}
	
	principal, err := authenticator.Authenticate(ctx, token)
	if err != nil {
		s.logAuthEvent(ctx, method, AuthFailure, nil, err.Error())
		return nil, fmt.Errorf("authentication failed: %w", err)
	}
	
	s.logAuthEvent(ctx, method, AuthSuccess, principal, "")
	return principal, nil
}

// Authorize checks if a principal is authorized to perform an action
func (s *AuthService) Authorize(ctx context.Context, authCtx *AuthContext) error {
	if !s.config.Enabled || !s.config.AuthzEnabled {
		return nil // Authorization disabled
	}
	
	if s.authorizer == nil {
		return fmt.Errorf("authorizer not initialized")
	}
	
	err := s.authorizer.Authorize(ctx, authCtx)
	
	result := AccessGranted
	errorMsg := ""
	if err != nil {
		result = AccessDenied
		errorMsg = err.Error()
	}
	
	s.logAccessEvent(ctx, &AccessEvent{
		Timestamp: time.Now(),
		Principal: authCtx.Principal,
		Resource:  authCtx.Resource,
		Action:    authCtx.Action,
		Result:    result,
		Error:     errorMsg,
		Context:   authCtx.Context,
	})
	
	return err
}

// ValidateToken validates a token without full authentication
func (s *AuthService) ValidateToken(ctx context.Context, method AuthMethod, token string) error {
	if !s.config.Enabled {
		return nil
	}
	
	s.mu.RLock()
	authenticator, exists := s.authenticators[method]
	s.mu.RUnlock()
	
	if !exists {
		return fmt.Errorf("authenticator for method %s not found", method)
	}
	
	return authenticator.ValidateToken(ctx, token)
}

// GetPermissions returns permissions for a principal
func (s *AuthService) GetPermissions(ctx context.Context, principal *Principal) ([]Permission, error) {
	if !s.config.Enabled || !s.config.AuthzEnabled || s.authorizer == nil {
		return nil, nil
	}
	
	return s.authorizer.GetPermissions(ctx, principal)
}

// RegisterAuthenticator registers a new authenticator
func (s *AuthService) RegisterAuthenticator(authenticator Authenticator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.authenticators[authenticator.GetMethod()] = authenticator
	logger.Infof("Registered authenticator for method: %s", authenticator.GetMethod())
}

// SetAuthorizer sets the authorizer
func (s *AuthService) SetAuthorizer(authorizer Authorizer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.authorizer = authorizer
	logger.Info("Authorizer set successfully")
}

// SetAuditLogger sets the audit logger
func (s *AuthService) SetAuditLogger(auditLogger AuditLogger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.auditLogger = auditLogger
	logger.Info("Audit logger set successfully")
}

// IsEnabled returns whether authentication is enabled
func (s *AuthService) IsEnabled() bool {
	return s.config.Enabled
}

// initializeAuthenticators initializes authenticators based on configuration
func (s *AuthService) initializeAuthenticators(ctx context.Context) error {
	// Initialize JWT authenticator
	if s.config.JWTEnabled {
		if s.config.JWTSecret == "" {
			return fmt.Errorf("JWT secret is required when JWT is enabled")
		}
		jwtAuth := NewJWTAuthenticator(s.config.JWTSecret, s.config.JWTIssuer, s.config.JWTAudience)
		s.RegisterAuthenticator(jwtAuth)
		logger.Info("JWT authenticator initialized")
	}
	
	// Initialize API Key authenticator
	if s.config.APIKeyEnabled {
		// Use in-memory store for now - in production this would be ScyllaDB or another persistent store
		apiKeyStore := NewMemoryAPIKeyStore()
		apiKeyAuth := NewAPIKeyAuthenticator(apiKeyStore)
		s.RegisterAuthenticator(apiKeyAuth)
		logger.Info("API Key authenticator initialized")
	}
	
	// Initialize DID authenticator
	if s.config.DIDEnabled || s.config.Web3Enabled {
		// Use in-memory resolver for now - in production this would resolve from blockchain/IPFS
		didResolver := NewMemoryDIDResolver()
		didAuth := NewDIDAuthenticator(didResolver)
		s.RegisterAuthenticator(didAuth)
		logger.Info("DID authenticator initialized")
	}
	
	// Initialize OIDC authenticator
	if s.config.OIDCEnabled {
		if s.config.OIDCIssuer == "" || s.config.OIDCClientID == "" {
			return fmt.Errorf("OIDC issuer and client ID are required when OIDC is enabled")
		}
		oidcAuth, err := NewOIDCAuthenticator(s.config.OIDCIssuer, s.config.OIDCClientID, s.config.OIDCClientSecret)
		if err != nil {
			return fmt.Errorf("failed to create OIDC authenticator: %w", err)
		}
		s.RegisterAuthenticator(oidcAuth)
		logger.Info("OIDC authenticator initialized")
	}
	
	// Initialize Web3 authenticator
	if s.config.Web3Enabled {
		// Use in-memory resolver for now - in production this would integrate with blockchain APIs
		web3Resolver := NewMemoryWeb3Resolver()
		web3Auth := NewWeb3Authenticator(1, "", web3Resolver) // Default to Ethereum mainnet
		s.RegisterAuthenticator(web3Auth)
		logger.Info("Web3 authenticator initialized")
	}
	
	return nil
}

// initializeAuthorizer initializes the authorizer based on configuration
func (s *AuthService) initializeAuthorizer(ctx context.Context) error {
	var engineType PolicyEngineType
	
	switch s.config.PolicyEngine {
	case "rbac":
		engineType = PolicyEngineRBAC
	case "abac":
		engineType = PolicyEngineABAC
	default:
		return fmt.Errorf("unsupported policy engine: %s", s.config.PolicyEngine)
	}
	
	// Create policy engine with multi-tenant support if enabled
	authorizer, err := CreatePolicyEngine(engineType, s.config.MultiTenantEnabled)
	if err != nil {
		return fmt.Errorf("failed to create policy engine: %w", err)
	}
	
	s.authorizer = authorizer
	logger.Infof("Initialized %s policy engine (multi-tenant: %v)", 
		s.config.PolicyEngine, s.config.MultiTenantEnabled)
	
	return nil
}

// initializeAuditLogger initializes the audit logger based on configuration
func (s *AuthService) initializeAuditLogger(ctx context.Context) error {
	// Audit logger will be implemented in this subtask
	switch s.config.AuditBackend {
	case "file":
		auditLogger, err := NewFileAuditLogger(s.config.AuditLogPath)
		if err != nil {
			return fmt.Errorf("failed to create file audit logger: %w", err)
		}
		s.auditLogger = auditLogger
	case "syslog":
		auditLogger, err := NewSyslogAuditLogger()
		if err != nil {
			return fmt.Errorf("failed to create syslog audit logger: %w", err)
		}
		s.auditLogger = auditLogger
	case "scylladb":
		// ScyllaDB audit logger will be implemented later
		logger.Warn("ScyllaDB audit logger not yet implemented, falling back to file logger")
		auditLogger, err := NewFileAuditLogger(s.config.AuditLogPath)
		if err != nil {
			return fmt.Errorf("failed to create file audit logger: %w", err)
		}
		s.auditLogger = auditLogger
	default:
		return fmt.Errorf("unknown audit backend: %s", s.config.AuditBackend)
	}
	
	return nil
}

// logAuthEvent logs an authentication event
func (s *AuthService) logAuthEvent(ctx context.Context, method AuthMethod, result AuthResult, principal *Principal, errorMsg string) {
	if s.auditLogger == nil {
		return
	}
	
	event := &AuthEvent{
		Timestamp: time.Now(),
		Method:    method,
		Result:    result,
		Principal: principal,
		Error:     errorMsg,
	}
	
	// Extract client info from context if available
	if clientIP, ok := ctx.Value("client_ip").(string); ok {
		event.ClientIP = clientIP
	}
	if userAgent, ok := ctx.Value("user_agent").(string); ok {
		event.UserAgent = userAgent
	}
	
	if err := s.auditLogger.LogAuth(ctx, event); err != nil {
		logger.Errorf("Failed to log auth event: %v", err)
	}
}

// logAccessEvent logs an access event
func (s *AuthService) logAccessEvent(ctx context.Context, event *AccessEvent) {
	if s.auditLogger == nil {
		return
	}
	
	// Extract additional context if available
	if requestID, ok := ctx.Value("request_id").(string); ok {
		event.RequestID = requestID
	}
	if clientIP, ok := ctx.Value("client_ip").(string); ok {
		event.ClientIP = clientIP
	}
	if userAgent, ok := ctx.Value("user_agent").(string); ok {
		event.UserAgent = userAgent
	}
	
	if err := s.auditLogger.LogAccess(ctx, event); err != nil {
		logger.Errorf("Failed to log access event: %v", err)
	}
}

// Shutdown gracefully shuts down the auth service
func (s *AuthService) Shutdown(ctx context.Context) error {
	logger.Info("Shutting down AuthService")
	
	// Close audit logger if it implements io.Closer
	if closer, ok := s.auditLogger.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			logger.Errorf("Error closing audit logger: %v", err)
		}
	}
	
	return nil
}