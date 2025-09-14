package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// APIKey represents an API key with metadata
type APIKey struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	HashedKey   string            `json:"hashed_key"`
	PrincipalID string            `json:"principal_id"`
	Type        string            `json:"type"`
	TenantID    string            `json:"tenant_id"`
	Roles       []string          `json:"roles"`
	Attrs       map[string]string `json:"attrs"`
	CreatedAt   time.Time         `json:"created_at"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time        `json:"last_used_at,omitempty"`
	IsActive    bool              `json:"is_active"`
}

// APIKeyStore interface for storing and retrieving API keys
type APIKeyStore interface {
	// StoreAPIKey stores an API key
	StoreAPIKey(ctx context.Context, apiKey *APIKey) error
	
	// GetAPIKey retrieves an API key by hashed key
	GetAPIKey(ctx context.Context, hashedKey string) (*APIKey, error)
	
	// ListAPIKeys lists API keys for a principal
	ListAPIKeys(ctx context.Context, principalID string) ([]*APIKey, error)
	
	// UpdateAPIKey updates an API key
	UpdateAPIKey(ctx context.Context, apiKey *APIKey) error
	
	// DeleteAPIKey deletes an API key
	DeleteAPIKey(ctx context.Context, keyID string) error
	
	// UpdateLastUsed updates the last used timestamp
	UpdateLastUsed(ctx context.Context, keyID string, timestamp time.Time) error
}

// MemoryAPIKeyStore implements APIKeyStore using in-memory storage
type MemoryAPIKeyStore struct {
	keys map[string]*APIKey // hashedKey -> APIKey
	mu   sync.RWMutex
}

// NewMemoryAPIKeyStore creates a new in-memory API key store
func NewMemoryAPIKeyStore() *MemoryAPIKeyStore {
	return &MemoryAPIKeyStore{
		keys: make(map[string]*APIKey),
	}
}

// StoreAPIKey stores an API key
func (m *MemoryAPIKeyStore) StoreAPIKey(ctx context.Context, apiKey *APIKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.keys[apiKey.HashedKey] = apiKey
	return nil
}

// GetAPIKey retrieves an API key by hashed key
func (m *MemoryAPIKeyStore) GetAPIKey(ctx context.Context, hashedKey string) (*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	apiKey, exists := m.keys[hashedKey]
	if !exists {
		return nil, fmt.Errorf("API key not found")
	}
	
	// Return a copy to prevent external modification
	keyCopy := *apiKey
	return &keyCopy, nil
}

// ListAPIKeys lists API keys for a principal
func (m *MemoryAPIKeyStore) ListAPIKeys(ctx context.Context, principalID string) ([]*APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var keys []*APIKey
	for _, apiKey := range m.keys {
		if apiKey.PrincipalID == principalID {
			keyCopy := *apiKey
			keys = append(keys, &keyCopy)
		}
	}
	
	return keys, nil
}

// UpdateAPIKey updates an API key
func (m *MemoryAPIKeyStore) UpdateAPIKey(ctx context.Context, apiKey *APIKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.keys[apiKey.HashedKey]; !exists {
		return fmt.Errorf("API key not found")
	}
	
	m.keys[apiKey.HashedKey] = apiKey
	return nil
}

// DeleteAPIKey deletes an API key
func (m *MemoryAPIKeyStore) DeleteAPIKey(ctx context.Context, keyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for hashedKey, apiKey := range m.keys {
		if apiKey.ID == keyID {
			delete(m.keys, hashedKey)
			return nil
		}
	}
	
	return fmt.Errorf("API key not found")
}

// UpdateLastUsed updates the last used timestamp
func (m *MemoryAPIKeyStore) UpdateLastUsed(ctx context.Context, keyID string, timestamp time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for _, apiKey := range m.keys {
		if apiKey.ID == keyID {
			apiKey.LastUsedAt = &timestamp
			return nil
		}
	}
	
	return fmt.Errorf("API key not found")
}

// APIKeyAuthenticator implements API key authentication
type APIKeyAuthenticator struct {
	store APIKeyStore
}

// NewAPIKeyAuthenticator creates a new API key authenticator
func NewAPIKeyAuthenticator(store APIKeyStore) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{
		store: store,
	}
}

// Authenticate validates an API key and returns a Principal
func (a *APIKeyAuthenticator) Authenticate(ctx context.Context, token string) (*Principal, error) {
	// API key format: "ak_" + base64(keyId:keySecret)
	if !strings.HasPrefix(token, "ak_") {
		return nil, fmt.Errorf("invalid API key format")
	}
	
	keyData := token[3:] // Remove "ak_" prefix
	
	// Decode the key
	decoded, err := base64.StdEncoding.DecodeString(keyData)
	if err != nil {
		return nil, fmt.Errorf("invalid API key encoding: %w", err)
	}
	
	// Hash the key for lookup
	hashedKey := a.hashKey(string(decoded))
	
	// Retrieve API key from store
	apiKey, err := a.store.GetAPIKey(ctx, hashedKey)
	if err != nil {
		return nil, fmt.Errorf("API key not found: %w", err)
	}
	
	// Check if key is active
	if !apiKey.IsActive {
		return nil, fmt.Errorf("API key is inactive")
	}
	
	// Check expiration
	if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
		return nil, fmt.Errorf("API key expired")
	}
	
	// Verify the key matches
	if subtle.ConstantTimeCompare([]byte(hashedKey), []byte(apiKey.HashedKey)) != 1 {
		return nil, fmt.Errorf("invalid API key")
	}
	
	// Update last used timestamp (async to avoid blocking)
	go func() {
		if err := a.store.UpdateLastUsed(context.Background(), apiKey.ID, time.Now()); err != nil {
			logger.Warnf("Failed to update API key last used timestamp: %v", err)
		}
	}()
	
	principal := &Principal{
		ID:       apiKey.PrincipalID,
		Type:     apiKey.Type,
		TenantID: apiKey.TenantID,
		Roles:    apiKey.Roles,
		Attrs:    apiKey.Attrs,
		Method:   AuthMethodAPIKey,
		IssuedAt: apiKey.CreatedAt,
	}
	
	if apiKey.ExpiresAt != nil {
		principal.ExpiresAt = apiKey.ExpiresAt
	}
	
	// Set defaults
	if principal.Type == "" {
		principal.Type = "service"
	}
	if principal.TenantID == "" {
		principal.TenantID = "default"
	}
	if principal.Attrs == nil {
		principal.Attrs = make(map[string]string)
	}
	
	return principal, nil
}

// GetMethod returns the authentication method
func (a *APIKeyAuthenticator) GetMethod() AuthMethod {
	return AuthMethodAPIKey
}

// ValidateToken validates an API key without full authentication
func (a *APIKeyAuthenticator) ValidateToken(ctx context.Context, token string) error {
	_, err := a.Authenticate(ctx, token)
	return err
}

// CreateAPIKey creates a new API key
func (a *APIKeyAuthenticator) CreateAPIKey(ctx context.Context, req *CreateAPIKeyRequest) (*CreateAPIKeyResponse, error) {
	// Generate random key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}
	
	// Generate key ID
	keyID := generateKeyID()
	
	// Create key string: keyId:keySecret
	keyString := keyID + ":" + hex.EncodeToString(keyBytes)
	
	// Hash the key for storage
	hashedKey := a.hashKey(keyString)
	
	// Create API key record
	apiKey := &APIKey{
		ID:          keyID,
		Name:        req.Name,
		HashedKey:   hashedKey,
		PrincipalID: req.PrincipalID,
		Type:        req.Type,
		TenantID:    req.TenantID,
		Roles:       req.Roles,
		Attrs:       req.Attrs,
		CreatedAt:   time.Now(),
		IsActive:    true,
	}
	
	if req.TTL > 0 {
		expTime := time.Now().Add(req.TTL)
		apiKey.ExpiresAt = &expTime
	}
	
	// Store the API key
	if err := a.store.StoreAPIKey(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("failed to store API key: %w", err)
	}
	
	// Create the token
	token := "ak_" + base64.StdEncoding.EncodeToString([]byte(keyString))
	
	return &CreateAPIKeyResponse{
		ID:        keyID,
		Token:     token,
		ExpiresAt: apiKey.ExpiresAt,
	}, nil
}

// RevokeAPIKey revokes an API key
func (a *APIKeyAuthenticator) RevokeAPIKey(ctx context.Context, keyID string) error {
	return a.store.DeleteAPIKey(ctx, keyID)
}

// ListAPIKeys lists API keys for a principal
func (a *APIKeyAuthenticator) ListAPIKeys(ctx context.Context, principalID string) ([]*APIKey, error) {
	return a.store.ListAPIKeys(ctx, principalID)
}

// hashKey creates a SHA256 hash of the key
func (a *APIKeyAuthenticator) hashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// generateKeyID generates a unique key ID
func generateKeyID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// CreateAPIKeyRequest represents a request to create an API key
type CreateAPIKeyRequest struct {
	Name        string            `json:"name"`
	PrincipalID string            `json:"principal_id"`
	Type        string            `json:"type"`
	TenantID    string            `json:"tenant_id"`
	Roles       []string          `json:"roles"`
	Attrs       map[string]string `json:"attrs"`
	TTL         time.Duration     `json:"ttl"`
}

// CreateAPIKeyResponse represents the response from creating an API key
type CreateAPIKeyResponse struct {
	ID        string     `json:"id"`
	Token     string     `json:"token"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}