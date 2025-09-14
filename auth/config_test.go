package auth

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_JSONSerialization(t *testing.T) {
	config := &Config{
		Enabled:            true,
		DefaultTenant:      "test-tenant",
		SessionTimeout:     time.Hour * 2,
		JWTEnabled:         true,
		JWTSecret:          "test-secret",
		JWTIssuer:          "test-issuer",
		JWTAudience:        "test-audience",
		APIKeyEnabled:      true,
		APIKeyTTL:          time.Hour * 48,
		DIDEnabled:         true,
		Web3Enabled:        true,
		OIDCEnabled:        true,
		OIDCIssuer:         "https://auth.example.com",
		OIDCClientID:       "test-client-id",
		OIDCClientSecret:   "test-client-secret",
		AuthzEnabled:       true,
		PolicyEngine:       "abac",
		AuditEnabled:       true,
		AuditBackend:       "scylladb",
		AuditLogPath:       "/custom/audit.log",
		MultiTenantEnabled: true,
	}
	
	// Test marshaling
	data, err := json.Marshal(config)
	require.NoError(t, err)
	
	// Test unmarshaling
	var decoded Config
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	
	// Verify all fields
	assert.Equal(t, config.Enabled, decoded.Enabled)
	assert.Equal(t, config.DefaultTenant, decoded.DefaultTenant)
	assert.Equal(t, config.SessionTimeout, decoded.SessionTimeout)
	assert.Equal(t, config.JWTEnabled, decoded.JWTEnabled)
	assert.Equal(t, config.JWTSecret, decoded.JWTSecret)
	assert.Equal(t, config.JWTIssuer, decoded.JWTIssuer)
	assert.Equal(t, config.JWTAudience, decoded.JWTAudience)
	assert.Equal(t, config.APIKeyEnabled, decoded.APIKeyEnabled)
	assert.Equal(t, config.APIKeyTTL, decoded.APIKeyTTL)
	assert.Equal(t, config.DIDEnabled, decoded.DIDEnabled)
	assert.Equal(t, config.Web3Enabled, decoded.Web3Enabled)
	assert.Equal(t, config.OIDCEnabled, decoded.OIDCEnabled)
	assert.Equal(t, config.OIDCIssuer, decoded.OIDCIssuer)
	assert.Equal(t, config.OIDCClientID, decoded.OIDCClientID)
	assert.Equal(t, config.OIDCClientSecret, decoded.OIDCClientSecret)
	assert.Equal(t, config.AuthzEnabled, decoded.AuthzEnabled)
	assert.Equal(t, config.PolicyEngine, decoded.PolicyEngine)
	assert.Equal(t, config.AuditEnabled, decoded.AuditEnabled)
	assert.Equal(t, config.AuditBackend, decoded.AuditBackend)
	assert.Equal(t, config.AuditLogPath, decoded.AuditLogPath)
	assert.Equal(t, config.MultiTenantEnabled, decoded.MultiTenantEnabled)
}

func TestConfig_Validation(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectValid bool
	}{
		{
			name:        "default config should be valid",
			config:      DefaultConfig(),
			expectValid: true,
		},
		{
			name: "enabled config with JWT should be valid",
			config: &Config{
				Enabled:     true,
				JWTEnabled:  true,
				JWTSecret:   "test-secret",
				JWTIssuer:   "test-issuer",
				JWTAudience: "test-audience",
			},
			expectValid: true,
		},
		{
			name: "enabled config with API key should be valid",
			config: &Config{
				Enabled:       true,
				APIKeyEnabled: true,
				APIKeyTTL:     time.Hour * 24,
			},
			expectValid: true,
		},
		{
			name: "enabled config with OIDC should be valid",
			config: &Config{
				Enabled:          true,
				OIDCEnabled:      true,
				OIDCIssuer:       "https://auth.example.com",
				OIDCClientID:     "client-id",
				OIDCClientSecret: "client-secret",
			},
			expectValid: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For now, we don't have validation logic implemented
			// This test serves as a placeholder for future validation
			assert.NotNil(t, tt.config)
		})
	}
}

func TestAuthMethod_String(t *testing.T) {
	tests := []struct {
		method   AuthMethod
		expected string
	}{
		{AuthMethodJWT, "jwt"},
		{AuthMethodAPIKey, "apikey"},
		{AuthMethodDID, "did"},
		{AuthMethodOIDC, "oidc"},
		{AuthMethodWeb3, "web3"},
	}
	
	for _, tt := range tests {
		t.Run(string(tt.method), func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.method))
		})
	}
}

func TestAccessResult_String(t *testing.T) {
	tests := []struct {
		result   AccessResult
		expected string
	}{
		{AccessGranted, "granted"},
		{AccessDenied, "denied"},
		{AccessError, "error"},
	}
	
	for _, tt := range tests {
		t.Run(string(tt.result), func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.result))
		})
	}
}

func TestAuthResult_String(t *testing.T) {
	tests := []struct {
		result   AuthResult
		expected string
	}{
		{AuthSuccess, "success"},
		{AuthFailure, "failure"},
		{AuthError, "error"},
	}
	
	for _, tt := range tests {
		t.Run(string(tt.result), func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.result))
		})
	}
}

func TestPrincipal_JSONSerialization(t *testing.T) {
	principal := &Principal{
		ID:       "user-123",
		Type:     "user",
		TenantID: "tenant-456",
		Roles:    []string{"admin", "user"},
		Attrs: map[string]string{
			"department": "engineering",
			"level":      "senior",
		},
		Method:    AuthMethodJWT,
		IssuedAt:  time.Now(),
		ExpiresAt: &time.Time{},
	}
	
	// Set expiration time
	expTime := time.Now().Add(time.Hour)
	principal.ExpiresAt = &expTime
	
	// Test marshaling
	data, err := json.Marshal(principal)
	require.NoError(t, err)
	
	// Test unmarshaling
	var decoded Principal
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	
	// Verify all fields
	assert.Equal(t, principal.ID, decoded.ID)
	assert.Equal(t, principal.Type, decoded.Type)
	assert.Equal(t, principal.TenantID, decoded.TenantID)
	assert.Equal(t, principal.Roles, decoded.Roles)
	assert.Equal(t, principal.Attrs, decoded.Attrs)
	assert.Equal(t, principal.Method, decoded.Method)
	assert.True(t, principal.IssuedAt.Equal(decoded.IssuedAt))
	assert.True(t, principal.ExpiresAt.Equal(*decoded.ExpiresAt))
}

func TestPermission_JSONSerialization(t *testing.T) {
	permission := &Permission{
		Resource: ResourcePin,
		Actions:  []string{ActionPinAdd, ActionPinRemove},
		Context: map[string]string{
			"tenant": "test-tenant",
			"scope":  "read-write",
		},
	}
	
	// Test marshaling
	data, err := json.Marshal(permission)
	require.NoError(t, err)
	
	// Test unmarshaling
	var decoded Permission
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	
	// Verify all fields
	assert.Equal(t, permission.Resource, decoded.Resource)
	assert.Equal(t, permission.Actions, decoded.Actions)
	assert.Equal(t, permission.Context, decoded.Context)
}