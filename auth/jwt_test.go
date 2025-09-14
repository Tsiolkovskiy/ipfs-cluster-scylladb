package auth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTAuthenticator(t *testing.T) {
	secret := "test-secret-key"
	issuer := "test-issuer"
	audience := "test-audience"
	
	auth := NewJWTAuthenticator(secret, issuer, audience)
	
	assert.Equal(t, AuthMethodJWT, auth.GetMethod())
}

func TestJWTAuthenticator_CreateAndValidateToken(t *testing.T) {
	secret := "test-secret-key"
	issuer := "test-issuer"
	audience := "test-audience"
	
	auth := NewJWTAuthenticator(secret, issuer, audience)
	
	// Create a test principal
	principal := &Principal{
		ID:       "user-123",
		Type:     "user",
		TenantID: "test-tenant",
		Roles:    []string{"admin", "user"},
		Attrs: map[string]string{
			"department": "engineering",
			"level":      "senior",
		},
	}
	
	// Create token
	token, err := auth.CreateToken(principal, time.Hour)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	
	// Validate token
	err = auth.ValidateToken(context.Background(), token)
	assert.NoError(t, err)
	
	// Authenticate with token
	authenticatedPrincipal, err := auth.Authenticate(context.Background(), token)
	require.NoError(t, err)
	
	assert.Equal(t, principal.ID, authenticatedPrincipal.ID)
	assert.Equal(t, principal.Type, authenticatedPrincipal.Type)
	assert.Equal(t, principal.TenantID, authenticatedPrincipal.TenantID)
	assert.Equal(t, principal.Roles, authenticatedPrincipal.Roles)
	assert.Equal(t, principal.Attrs, authenticatedPrincipal.Attrs)
	assert.Equal(t, AuthMethodJWT, authenticatedPrincipal.Method)
	assert.NotNil(t, authenticatedPrincipal.ExpiresAt)
}

func TestJWTAuthenticator_ExpiredToken(t *testing.T) {
	secret := "test-secret-key"
	issuer := "test-issuer"
	audience := "test-audience"
	
	auth := NewJWTAuthenticator(secret, issuer, audience)
	
	principal := &Principal{
		ID:       "user-123",
		Type:     "user",
		TenantID: "test-tenant",
	}
	
	// Create token that's already expired (negative duration)
	token, err := auth.CreateToken(principal, -time.Hour)
	require.NoError(t, err)
	
	// Try to authenticate with expired token
	_, err = auth.Authenticate(context.Background(), token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token expired")
}

func TestJWTAuthenticator_InvalidSignature(t *testing.T) {
	secret := "test-secret-key"
	issuer := "test-issuer"
	audience := "test-audience"
	
	auth := NewJWTAuthenticator(secret, issuer, audience)
	
	// Create token with different secret
	wrongAuth := NewJWTAuthenticator("wrong-secret", issuer, audience)
	principal := &Principal{
		ID:       "user-123",
		Type:     "user",
		TenantID: "test-tenant",
	}
	
	token, err := wrongAuth.CreateToken(principal, time.Hour)
	require.NoError(t, err)
	
	// Try to authenticate with wrong secret
	_, err = auth.Authenticate(context.Background(), token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature")
}

func TestJWTAuthenticator_InvalidIssuer(t *testing.T) {
	secret := "test-secret-key"
	issuer := "test-issuer"
	audience := "test-audience"
	
	auth := NewJWTAuthenticator(secret, issuer, audience)
	
	// Create token with different issuer
	wrongAuth := NewJWTAuthenticator(secret, "wrong-issuer", audience)
	principal := &Principal{
		ID:       "user-123",
		Type:     "user",
		TenantID: "test-tenant",
	}
	
	token, err := wrongAuth.CreateToken(principal, time.Hour)
	require.NoError(t, err)
	
	// Try to authenticate with wrong issuer
	_, err = auth.Authenticate(context.Background(), token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid issuer")
}

func TestJWTAuthenticator_InvalidAudience(t *testing.T) {
	secret := "test-secret-key"
	issuer := "test-issuer"
	audience := "test-audience"
	
	auth := NewJWTAuthenticator(secret, issuer, audience)
	
	// Create token with different audience
	wrongAuth := NewJWTAuthenticator(secret, issuer, "wrong-audience")
	principal := &Principal{
		ID:       "user-123",
		Type:     "user",
		TenantID: "test-tenant",
	}
	
	token, err := wrongAuth.CreateToken(principal, time.Hour)
	require.NoError(t, err)
	
	// Try to authenticate with wrong audience
	_, err = auth.Authenticate(context.Background(), token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid audience")
}

func TestJWTAuthenticator_InvalidFormat(t *testing.T) {
	secret := "test-secret-key"
	issuer := "test-issuer"
	audience := "test-audience"
	
	auth := NewJWTAuthenticator(secret, issuer, audience)
	
	tests := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"invalid format", "invalid.token"},
		{"too many parts", "part1.part2.part3.part4"},
		{"invalid base64", "invalid-base64.invalid-base64.invalid-base64"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auth.Authenticate(context.Background(), tt.token)
			assert.Error(t, err)
		})
	}
}

func TestJWTAuthenticator_DefaultValues(t *testing.T) {
	secret := "test-secret-key"
	issuer := "test-issuer"
	audience := "test-audience"
	
	auth := NewJWTAuthenticator(secret, issuer, audience)
	
	// Create principal with minimal fields
	principal := &Principal{
		ID: "user-123",
	}
	
	token, err := auth.CreateToken(principal, time.Hour)
	require.NoError(t, err)
	
	authenticatedPrincipal, err := auth.Authenticate(context.Background(), token)
	require.NoError(t, err)
	
	// Check defaults are applied
	assert.Equal(t, "user", authenticatedPrincipal.Type)
	assert.Equal(t, "default", authenticatedPrincipal.TenantID)
	assert.NotNil(t, authenticatedPrincipal.Attrs)
}

func TestJWTClaims_JSONSerialization(t *testing.T) {
	claims := &JWTClaims{
		Subject:   "user-123",
		Issuer:    "test-issuer",
		Audience:  "test-audience",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		IssuedAt:  time.Now().Unix(),
		Type:      "user",
		TenantID:  "test-tenant",
		Roles:     []string{"admin", "user"},
		Attrs: map[string]string{
			"department": "engineering",
		},
	}
	
	// Test that claims can be marshaled and unmarshaled
	data, err := json.Marshal(claims)
	require.NoError(t, err)
	
	var decoded JWTClaims
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	
	assert.Equal(t, claims.Subject, decoded.Subject)
	assert.Equal(t, claims.Issuer, decoded.Issuer)
	assert.Equal(t, claims.Audience, decoded.Audience)
	assert.Equal(t, claims.Type, decoded.Type)
	assert.Equal(t, claims.TenantID, decoded.TenantID)
	assert.Equal(t, claims.Roles, decoded.Roles)
	assert.Equal(t, claims.Attrs, decoded.Attrs)
}