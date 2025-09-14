package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthenticator(t *testing.T) {
	store := NewMemoryAPIKeyStore()
	auth := NewAPIKeyAuthenticator(store)
	
	assert.Equal(t, AuthMethodAPIKey, auth.GetMethod())
}

func TestAPIKeyAuthenticator_CreateAndAuthenticate(t *testing.T) {
	store := NewMemoryAPIKeyStore()
	auth := NewAPIKeyAuthenticator(store)
	
	// Create API key request
	req := &CreateAPIKeyRequest{
		Name:        "test-key",
		PrincipalID: "user-123",
		Type:        "service",
		TenantID:    "test-tenant",
		Roles:       []string{"admin", "user"},
		Attrs: map[string]string{
			"service": "test-service",
		},
		TTL: time.Hour * 24,
	}
	
	// Create API key
	resp, err := auth.CreateAPIKey(context.Background(), req)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.ID)
	assert.NotEmpty(t, resp.Token)
	assert.NotNil(t, resp.ExpiresAt)
	assert.True(t, resp.Token[:3] == "ak_")
	
	// Authenticate with the token
	principal, err := auth.Authenticate(context.Background(), resp.Token)
	require.NoError(t, err)
	
	assert.Equal(t, req.PrincipalID, principal.ID)
	assert.Equal(t, req.Type, principal.Type)
	assert.Equal(t, req.TenantID, principal.TenantID)
	assert.Equal(t, req.Roles, principal.Roles)
	assert.Equal(t, req.Attrs, principal.Attrs)
	assert.Equal(t, AuthMethodAPIKey, principal.Method)
	assert.NotNil(t, principal.ExpiresAt)
}

func TestAPIKeyAuthenticator_ExpiredKey(t *testing.T) {
	store := NewMemoryAPIKeyStore()
	auth := NewAPIKeyAuthenticator(store)
	
	// Create API key with very short TTL
	req := &CreateAPIKeyRequest{
		Name:        "test-key",
		PrincipalID: "user-123",
		TTL:         time.Millisecond,
	}
	
	resp, err := auth.CreateAPIKey(context.Background(), req)
	require.NoError(t, err)
	
	// Wait for key to expire
	time.Sleep(time.Millisecond * 10)
	
	// Try to authenticate with expired key
	_, err = auth.Authenticate(context.Background(), resp.Token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key expired")
}

func TestAPIKeyAuthenticator_InactiveKey(t *testing.T) {
	store := NewMemoryAPIKeyStore()
	auth := NewAPIKeyAuthenticator(store)
	
	// Create API key
	req := &CreateAPIKeyRequest{
		Name:        "test-key",
		PrincipalID: "user-123",
	}
	
	resp, err := auth.CreateAPIKey(context.Background(), req)
	require.NoError(t, err)
	
	// Decode the token to get the key string for hashing
	keyData := resp.Token[3:] // Remove "ak_" prefix
	decoded, err := base64.StdEncoding.DecodeString(keyData)
	require.NoError(t, err)
	hashedKey := auth.hashKey(string(decoded))
	
	// Deactivate the key
	apiKey, err := store.GetAPIKey(context.Background(), hashedKey)
	require.NoError(t, err)
	apiKey.IsActive = false
	err = store.UpdateAPIKey(context.Background(), apiKey)
	require.NoError(t, err)
	
	// Try to authenticate with inactive key
	_, err = auth.Authenticate(context.Background(), resp.Token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key is inactive")
}

func TestAPIKeyAuthenticator_InvalidFormat(t *testing.T) {
	store := NewMemoryAPIKeyStore()
	auth := NewAPIKeyAuthenticator(store)
	
	tests := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"wrong prefix", "wrong_prefix"},
		{"invalid base64", "ak_invalid-base64"},
		{"not found", "ak_dGVzdC1rZXk="},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auth.Authenticate(context.Background(), tt.token)
			assert.Error(t, err)
		})
	}
}

func TestAPIKeyAuthenticator_RevokeKey(t *testing.T) {
	store := NewMemoryAPIKeyStore()
	auth := NewAPIKeyAuthenticator(store)
	
	// Create API key
	req := &CreateAPIKeyRequest{
		Name:        "test-key",
		PrincipalID: "user-123",
	}
	
	resp, err := auth.CreateAPIKey(context.Background(), req)
	require.NoError(t, err)
	
	// Verify key works
	_, err = auth.Authenticate(context.Background(), resp.Token)
	assert.NoError(t, err)
	
	// Revoke key
	err = auth.RevokeAPIKey(context.Background(), resp.ID)
	assert.NoError(t, err)
	
	// Verify key no longer works
	_, err = auth.Authenticate(context.Background(), resp.Token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key not found")
}

func TestAPIKeyAuthenticator_ListKeys(t *testing.T) {
	store := NewMemoryAPIKeyStore()
	auth := NewAPIKeyAuthenticator(store)
	
	principalID := "user-123"
	
	// Create multiple API keys
	for i := 0; i < 3; i++ {
		req := &CreateAPIKeyRequest{
			Name:        fmt.Sprintf("test-key-%d", i),
			PrincipalID: principalID,
		}
		
		_, err := auth.CreateAPIKey(context.Background(), req)
		require.NoError(t, err)
	}
	
	// List keys for principal
	keys, err := auth.ListAPIKeys(context.Background(), principalID)
	require.NoError(t, err)
	assert.Len(t, keys, 3)
	
	// Verify all keys belong to the principal
	for _, key := range keys {
		assert.Equal(t, principalID, key.PrincipalID)
	}
}

func TestAPIKeyAuthenticator_DefaultValues(t *testing.T) {
	store := NewMemoryAPIKeyStore()
	auth := NewAPIKeyAuthenticator(store)
	
	// Create API key with minimal fields
	req := &CreateAPIKeyRequest{
		Name:        "test-key",
		PrincipalID: "user-123",
	}
	
	resp, err := auth.CreateAPIKey(context.Background(), req)
	require.NoError(t, err)
	
	principal, err := auth.Authenticate(context.Background(), resp.Token)
	require.NoError(t, err)
	
	// Check defaults are applied
	assert.Equal(t, "service", principal.Type)
	assert.Equal(t, "default", principal.TenantID)
	assert.NotNil(t, principal.Attrs)
}

func TestMemoryAPIKeyStore(t *testing.T) {
	store := NewMemoryAPIKeyStore()
	
	apiKey := &APIKey{
		ID:          "key-123",
		Name:        "test-key",
		HashedKey:   "hashed-key",
		PrincipalID: "user-123",
		Type:        "service",
		TenantID:    "test-tenant",
		Roles:       []string{"user"},
		Attrs:       map[string]string{"service": "test"},
		CreatedAt:   time.Now(),
		IsActive:    true,
	}
	
	// Store API key
	err := store.StoreAPIKey(context.Background(), apiKey)
	assert.NoError(t, err)
	
	// Get API key
	retrieved, err := store.GetAPIKey(context.Background(), "hashed-key")
	require.NoError(t, err)
	assert.Equal(t, apiKey.ID, retrieved.ID)
	assert.Equal(t, apiKey.Name, retrieved.Name)
	assert.Equal(t, apiKey.PrincipalID, retrieved.PrincipalID)
	
	// Update API key
	apiKey.Name = "updated-name"
	err = store.UpdateAPIKey(context.Background(), apiKey)
	assert.NoError(t, err)
	
	retrieved, err = store.GetAPIKey(context.Background(), "hashed-key")
	require.NoError(t, err)
	assert.Equal(t, "updated-name", retrieved.Name)
	
	// Update last used
	now := time.Now()
	err = store.UpdateLastUsed(context.Background(), "key-123", now)
	assert.NoError(t, err)
	
	retrieved, err = store.GetAPIKey(context.Background(), "hashed-key")
	require.NoError(t, err)
	assert.NotNil(t, retrieved.LastUsedAt)
	assert.True(t, retrieved.LastUsedAt.Equal(now))
	
	// List API keys
	keys, err := store.ListAPIKeys(context.Background(), "user-123")
	require.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Equal(t, apiKey.ID, keys[0].ID)
	
	// Delete API key
	err = store.DeleteAPIKey(context.Background(), "key-123")
	assert.NoError(t, err)
	
	_, err = store.GetAPIKey(context.Background(), "hashed-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key not found")
}

func TestMemoryAPIKeyStore_NotFound(t *testing.T) {
	store := NewMemoryAPIKeyStore()
	
	// Try to get non-existent key
	_, err := store.GetAPIKey(context.Background(), "non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key not found")
	
	// Try to update non-existent key
	apiKey := &APIKey{ID: "key-123", HashedKey: "non-existent"}
	err = store.UpdateAPIKey(context.Background(), apiKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key not found")
	
	// Try to delete non-existent key
	err = store.DeleteAPIKey(context.Background(), "non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key not found")
	
	// Try to update last used for non-existent key
	err = store.UpdateLastUsed(context.Background(), "non-existent", time.Now())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key not found")
}