package auth

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWeb3Authenticator(t *testing.T) {
	resolver := NewMemoryWeb3Resolver()
	auth := NewWeb3Authenticator(1, "0x1234567890123456789012345678901234567890", resolver)
	
	assert.Equal(t, AuthMethodWeb3, auth.GetMethod())
	assert.Equal(t, int64(1), auth.chainID)
}

func TestWeb3Authenticator_Authenticate(t *testing.T) {
	resolver := NewMemoryWeb3Resolver()
	auth := NewWeb3Authenticator(1, "0x1234567890123456789012345678901234567890", resolver)
	
	// Register test identity
	testAddress := "0x742d35cc6634c0532925a3b8d0c9c0c8c8c8c8c8"
	identity := &Web3Identity{
		Address:    testAddress,
		ENSName:    "test.eth",
		Attributes: map[string]string{"department": "engineering"},
		Roles:      []string{"admin", "user"},
		TenantID:   "test-tenant",
		Verified:   true,
	}
	resolver.RegisterIdentity(testAddress, identity)
	
	// Create signed message
	signedMsg := &Web3SignedMessage{
		Address:   testAddress,
		Message:   "Sign in to IPFS Cluster",
		Signature: "0x" + "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef12",
		Timestamp: time.Now().Unix(),
		ChainID:   1,
		Nonce:     "test-nonce",
	}
	
	signedMsgJSON, err := json.Marshal(signedMsg)
	require.NoError(t, err)
	
	// Test successful authentication
	principal, err := auth.Authenticate(context.Background(), string(signedMsgJSON))
	require.NoError(t, err)
	
	assert.Equal(t, testAddress, principal.ID)
	assert.Equal(t, "user", principal.Type)
	assert.Equal(t, "test-tenant", principal.TenantID)
	assert.Equal(t, []string{"admin", "user"}, principal.Roles)
	assert.Equal(t, AuthMethodWeb3, principal.Method)
	assert.Equal(t, "engineering", principal.Attrs["department"])
	assert.Equal(t, "test.eth", principal.Attrs["ens_name"])
	assert.Equal(t, "1", principal.Attrs["chain_id"])
}

func TestWeb3Authenticator_ExpiredTimestamp(t *testing.T) {
	resolver := NewMemoryWeb3Resolver()
	auth := NewWeb3Authenticator(1, "0x1234567890123456789012345678901234567890", resolver)
	
	// Create signed message with old timestamp
	signedMsg := &Web3SignedMessage{
		Address:   "0x742d35cc6634c0532925a3b8d0c9c0c8c8c8c8c8",
		Message:   "Sign in to IPFS Cluster",
		Signature: "0x" + "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef12",
		Timestamp: time.Now().Unix() - 400, // 400 seconds ago (> 5 minutes)
		ChainID:   1,
	}
	
	signedMsgJSON, err := json.Marshal(signedMsg)
	require.NoError(t, err)
	
	_, err = auth.Authenticate(context.Background(), string(signedMsgJSON))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timestamp out of range")
}

func TestWeb3Authenticator_InvalidChainID(t *testing.T) {
	resolver := NewMemoryWeb3Resolver()
	auth := NewWeb3Authenticator(1, "0x1234567890123456789012345678901234567890", resolver)
	
	// Create signed message with wrong chain ID
	signedMsg := &Web3SignedMessage{
		Address:   "0x742d35cc6634c0532925a3b8d0c9c0c8c8c8c8c8",
		Message:   "Sign in to IPFS Cluster",
		Signature: "0x" + "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef12",
		Timestamp: time.Now().Unix(),
		ChainID:   137, // Polygon instead of Ethereum
	}
	
	signedMsgJSON, err := json.Marshal(signedMsg)
	require.NoError(t, err)
	
	_, err = auth.Authenticate(context.Background(), string(signedMsgJSON))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid chain ID")
}

func TestWeb3Authenticator_InvalidFormat(t *testing.T) {
	resolver := NewMemoryWeb3Resolver()
	auth := NewWeb3Authenticator(1, "0x1234567890123456789012345678901234567890", resolver)
	
	tests := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"invalid JSON", "invalid-json"},
		{"missing fields", `{"address": "0x123"}`},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auth.Authenticate(context.Background(), tt.token)
			assert.Error(t, err)
		})
	}
}

func TestWeb3Authenticator_ValidateToken(t *testing.T) {
	resolver := NewMemoryWeb3Resolver()
	auth := NewWeb3Authenticator(1, "0x1234567890123456789012345678901234567890", resolver)
	
	signedMsg := &Web3SignedMessage{
		Address:   "0x742d35cc6634c0532925a3b8d0c9c0c8c8c8c8c8",
		Message:   "Sign in to IPFS Cluster",
		Signature: "0x" + "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef12",
		Timestamp: time.Now().Unix(),
		ChainID:   1,
	}
	
	signedMsgJSON, err := json.Marshal(signedMsg)
	require.NoError(t, err)
	
	// ValidateToken should have the same behavior as Authenticate
	err = auth.ValidateToken(context.Background(), string(signedMsgJSON))
	assert.NoError(t, err)
}

func TestMemoryWeb3Resolver(t *testing.T) {
	resolver := NewMemoryWeb3Resolver()
	
	testAddress := "0x742d35cc6634c0532925a3b8d0c9c0c8c8c8c8c8"
	identity := &Web3Identity{
		Address:    testAddress,
		ENSName:    "test.eth",
		Attributes: map[string]string{"level": "senior"},
		Roles:      []string{"developer"},
		TenantID:   "dev-team",
		Verified:   true,
	}
	
	// Register identity
	resolver.RegisterIdentity(testAddress, identity)
	
	// Resolve identity
	resolved, err := resolver.ResolveAddress(context.Background(), testAddress)
	require.NoError(t, err)
	assert.Equal(t, testAddress, resolved.Address)
	assert.Equal(t, "test.eth", resolved.ENSName)
	assert.Equal(t, []string{"developer"}, resolved.Roles)
	assert.Equal(t, "dev-team", resolved.TenantID)
	
	// Test case insensitive address resolution
	resolved, err = resolver.ResolveAddress(context.Background(), strings.ToUpper(testAddress))
	require.NoError(t, err)
	assert.Equal(t, testAddress, resolved.Address)
	
	// Test unknown address (should return default identity)
	unknown, err := resolver.ResolveAddress(context.Background(), "0x0000000000000000000000000000000000000000")
	require.NoError(t, err)
	assert.Equal(t, "0x0000000000000000000000000000000000000000", unknown.Address)
	assert.Equal(t, []string{"user"}, unknown.Roles)
	assert.Equal(t, "default", unknown.TenantID)
	assert.False(t, unknown.Verified)
}

func TestMemoryWeb3Resolver_VerifySignature(t *testing.T) {
	resolver := NewMemoryWeb3Resolver()
	
	// Test valid signature format
	err := resolver.VerifySignature(context.Background(), 
		"Sign in to IPFS Cluster",
		"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef12",
		"0x742d35cc6634c0532925a3b8d0c9c0c8c8c8c8c8")
	assert.NoError(t, err)
	
	// Test invalid signature format
	err = resolver.VerifySignature(context.Background(), 
		"message",
		"invalid-signature",
		"0x742d35cc6634c0532925a3b8d0c9c0c8c8c8c8c8")
	assert.Error(t, err)
	
	// Test invalid signature length
	err = resolver.VerifySignature(context.Background(), 
		"message",
		"0x1234",
		"0x742d35cc6634c0532925a3b8d0c9c0c8c8c8c8c8")
	assert.Error(t, err)
	
	// Test invalid address format
	err = resolver.VerifySignature(context.Background(), 
		"message",
		"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef12",
		"invalid-address")
	assert.Error(t, err)
}

func TestEthereumSignatureVerifier(t *testing.T) {
	verifier := NewEthereumSignatureVerifier()
	
	// Test signature verification (will fail with placeholder implementation)
	err := verifier.VerifyPersonalSign(
		"Sign in to IPFS Cluster",
		"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef12",
		"0x742d35cc6634c0532925a3b8d0c9c0c8c8c8c8c8")
	
	// Should fail with placeholder implementation
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "signature recovery not implemented")
}

func TestWalletConnectProvider(t *testing.T) {
	metadata := WalletConnectMetadata{
		Name:        "IPFS Cluster",
		Description: "Decentralized IPFS pinning service",
		URL:         "https://cluster.ipfs.io",
		Icons:       []string{"https://cluster.ipfs.io/icon.png"},
	}
	
	provider := NewWalletConnectProvider("test-project-id", metadata)
	
	assert.Equal(t, "test-project-id", provider.projectID)
	assert.Equal(t, metadata.Name, provider.metadata.Name)
	
	// Test session creation (placeholder implementation)
	session, err := provider.CreateSession(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), session.ChainID)
	assert.Equal(t, "placeholder-topic", session.Topic)
}

func TestMetaMaskProvider(t *testing.T) {
	provider := NewMetaMaskProvider()
	
	script := provider.GenerateConnectScript()
	assert.Contains(t, script, "connectMetaMask")
	assert.Contains(t, script, "signMessage")
	assert.Contains(t, script, "window.ethereum")
	assert.Contains(t, script, "eth_requestAccounts")
	assert.Contains(t, script, "personal_sign")
}

func TestGenerateAuthenticationMessage(t *testing.T) {
	address := "0x742d35cc6634c0532925a3b8d0c9c0c8c8c8c8c8"
	nonce := "test-nonce-123"
	timestamp := time.Now().Unix()
	domain := "cluster.ipfs.io"
	
	message := GenerateAuthenticationMessage(address, nonce, timestamp, domain)
	
	assert.Contains(t, message, address)
	assert.Contains(t, message, nonce)
	assert.Contains(t, message, domain)
	assert.Contains(t, message, "sign in with your Ethereum account")
	assert.Contains(t, message, time.Unix(timestamp, 0).Format(time.RFC3339))
}

func TestWeb3Identity_JSONSerialization(t *testing.T) {
	identity := &Web3Identity{
		Address: "0x742d35cc6634c0532925a3b8d0c9c0c8c8c8c8c8",
		ENSName: "test.eth",
		Attributes: map[string]string{
			"department": "engineering",
			"level":      "senior",
		},
		Roles:       []string{"admin", "developer"},
		TenantID:    "engineering-team",
		Verified:    true,
		LastUpdated: time.Now(),
	}
	
	// Test marshaling
	data, err := json.Marshal(identity)
	require.NoError(t, err)
	
	// Test unmarshaling
	var decoded Web3Identity
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	
	assert.Equal(t, identity.Address, decoded.Address)
	assert.Equal(t, identity.ENSName, decoded.ENSName)
	assert.Equal(t, identity.Attributes, decoded.Attributes)
	assert.Equal(t, identity.Roles, decoded.Roles)
	assert.Equal(t, identity.TenantID, decoded.TenantID)
	assert.Equal(t, identity.Verified, decoded.Verified)
}

func TestWeb3SignedMessage_JSONSerialization(t *testing.T) {
	signedMsg := &Web3SignedMessage{
		Address:   "0x742d35cc6634c0532925a3b8d0c9c0c8c8c8c8c8",
		Message:   "Sign in to IPFS Cluster",
		Signature: "0x1234567890abcdef",
		Timestamp: time.Now().Unix(),
		ChainID:   1,
		Nonce:     "test-nonce",
	}
	
	// Test marshaling
	data, err := json.Marshal(signedMsg)
	require.NoError(t, err)
	
	// Test unmarshaling
	var decoded Web3SignedMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	
	assert.Equal(t, signedMsg.Address, decoded.Address)
	assert.Equal(t, signedMsg.Message, decoded.Message)
	assert.Equal(t, signedMsg.Signature, decoded.Signature)
	assert.Equal(t, signedMsg.Timestamp, decoded.Timestamp)
	assert.Equal(t, signedMsg.ChainID, decoded.ChainID)
	assert.Equal(t, signedMsg.Nonce, decoded.Nonce)
}