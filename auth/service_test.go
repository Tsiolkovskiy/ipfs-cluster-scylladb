package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	
	assert.False(t, config.Enabled)
	assert.Equal(t, "default", config.DefaultTenant)
	assert.Equal(t, time.Hour*24, config.SessionTimeout)
	assert.True(t, config.JWTEnabled)
	assert.Equal(t, "ipfs-cluster", config.JWTIssuer)
	assert.Equal(t, "ipfs-cluster-api", config.JWTAudience)
	assert.True(t, config.APIKeyEnabled)
	assert.Equal(t, time.Hour*24*30, config.APIKeyTTL)
	assert.False(t, config.DIDEnabled)
	assert.False(t, config.Web3Enabled)
	assert.False(t, config.OIDCEnabled)
	assert.True(t, config.AuthzEnabled)
	assert.Equal(t, "rbac", config.PolicyEngine)
	assert.True(t, config.AuditEnabled)
	assert.Equal(t, "file", config.AuditBackend)
	assert.False(t, config.MultiTenantEnabled)
}

func TestNewAuthService_Disabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false
	
	service, err := NewAuthService(context.Background(), config)
	require.NoError(t, err)
	assert.NotNil(t, service)
	assert.False(t, service.IsEnabled())
}

func TestNewAuthService_Enabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.JWTSecret = "test-secret"
	config.AuditLogPath = "/tmp/test-audit.log"
	
	service, err := NewAuthService(context.Background(), config)
	require.NoError(t, err)
	assert.NotNil(t, service)
	assert.True(t, service.IsEnabled())
}

func TestAuthService_AuthenticateDisabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false
	
	service, err := NewAuthService(context.Background(), config)
	require.NoError(t, err)
	
	_, err = service.Authenticate(context.Background(), AuthMethodJWT, "test-token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authentication is disabled")
}

func TestAuthService_AuthenticateNoAuthenticator(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.JWTEnabled = false // Disable JWT to test missing authenticator
	config.APIKeyEnabled = false
	config.DIDEnabled = false
	config.AuditLogPath = "/tmp/test-audit.log"
	
	service, err := NewAuthService(context.Background(), config)
	require.NoError(t, err)
	
	_, err = service.Authenticate(context.Background(), AuthMethodJWT, "test-token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authenticator for method jwt not found")
}

func TestAuthService_RegisterAuthenticator(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.JWTSecret = "test-secret"
	config.AuditLogPath = "/tmp/test-audit.log"
	
	service, err := NewAuthService(context.Background(), config)
	require.NoError(t, err)
	
	// Create mock authenticator for a different method
	mockAuth := &MockAuthenticator{
		method: AuthMethodDID,
	}
	
	service.RegisterAuthenticator(mockAuth)
	
	// Verify authenticator is registered
	service.mu.RLock()
	_, exists := service.authenticators[AuthMethodDID]
	service.mu.RUnlock()
	
	assert.True(t, exists)
}

func TestAuthService_AuthorizeDisabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false
	
	service, err := NewAuthService(context.Background(), config)
	require.NoError(t, err)
	
	authCtx := &AuthContext{
		Principal: &Principal{ID: "test-user"},
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
	}
	
	err = service.Authorize(context.Background(), authCtx)
	assert.NoError(t, err) // Should pass when disabled
}

func TestAuthService_ValidateTokenDisabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false
	
	service, err := NewAuthService(context.Background(), config)
	require.NoError(t, err)
	
	err = service.ValidateToken(context.Background(), AuthMethodJWT, "test-token")
	assert.NoError(t, err) // Should pass when disabled
}

func TestAuthService_GetPermissionsDisabled(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = false
	
	service, err := NewAuthService(context.Background(), config)
	require.NoError(t, err)
	
	principal := &Principal{ID: "test-user"}
	permissions, err := service.GetPermissions(context.Background(), principal)
	assert.NoError(t, err)
	assert.Nil(t, permissions) // Should return nil when disabled
}

func TestAuthService_SetAuthorizer(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.JWTSecret = "test-secret"
	config.AuditLogPath = "/tmp/test-audit.log"
	
	service, err := NewAuthService(context.Background(), config)
	require.NoError(t, err)
	
	mockAuthorizer := &MockAuthorizer{}
	service.SetAuthorizer(mockAuthorizer)
	
	service.mu.RLock()
	assert.Equal(t, mockAuthorizer, service.authorizer)
	service.mu.RUnlock()
}

func TestAuthService_SetAuditLogger(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.JWTSecret = "test-secret"
	config.AuditLogPath = "/tmp/test-audit.log"
	
	service, err := NewAuthService(context.Background(), config)
	require.NoError(t, err)
	
	mockAuditLogger := NewMemoryAuditLogger()
	service.SetAuditLogger(mockAuditLogger)
	
	service.mu.RLock()
	assert.Equal(t, mockAuditLogger, service.auditLogger)
	service.mu.RUnlock()
}

// Mock implementations for testing

type MockAuthenticator struct {
	method AuthMethod
	principal *Principal
	err    error
}

func (m *MockAuthenticator) Authenticate(ctx context.Context, token string) (*Principal, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.principal, nil
}

func (m *MockAuthenticator) GetMethod() AuthMethod {
	return m.method
}

func (m *MockAuthenticator) ValidateToken(ctx context.Context, token string) error {
	return m.err
}

type MockAuthorizer struct {
	err         error
	permissions []Permission
}

func (m *MockAuthorizer) Authorize(ctx context.Context, authCtx *AuthContext) error {
	return m.err
}

func (m *MockAuthorizer) GetPermissions(ctx context.Context, principal *Principal) ([]Permission, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.permissions, nil
}