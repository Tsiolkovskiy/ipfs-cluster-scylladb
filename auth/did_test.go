package auth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDIDAuthenticator(t *testing.T) {
	resolver := NewMemoryDIDResolver()
	auth := NewDIDAuthenticator(resolver)
	
	assert.Equal(t, AuthMethodDID, auth.GetMethod())
}

func TestDIDAuthenticator_ValidateSignedMessage(t *testing.T) {
	resolver := NewMemoryDIDResolver()
	auth := NewDIDAuthenticator(resolver)
	
	// Register a test DID document
	did := "did:example:123"
	didDoc := &DIDDocument{
		ID:      did,
		Context: []string{"https://www.w3.org/ns/did/v1"},
		VerificationMethod: []VerificationMethod{
			{
				ID:           did + "#key1",
				Type:         "EcdsaSecp256k1VerificationKey2019",
				Controller:   did,
				PublicKeyHex: "04" + "1234567890abcdef" + "fedcba0987654321", // Mock public key
			},
		},
		Authentication: []string{did + "#key1"},
	}
	resolver.RegisterDID(did, didDoc)
	
	// Create a signed message (this would normally be created by a client)
	signedMsg := &SignedMessage{
		DID:       did,
		Timestamp: time.Now().Unix(),
		Nonce:     "test-nonce",
		Message:   "authenticate",
		Signature: "mock-signature", // In real implementation, this would be a valid signature
		Type:      "did",
	}
	
	signedMsgJSON, err := json.Marshal(signedMsg)
	require.NoError(t, err)
	
	// Note: This test will fail signature verification since we're using mock data
	// In a real implementation, you would use proper cryptographic signatures
	_, err = auth.Authenticate(context.Background(), string(signedMsgJSON))
	assert.Error(t, err) // Expected to fail with mock signature
}

func TestDIDAuthenticator_EthereumSignature(t *testing.T) {
	resolver := NewMemoryDIDResolver()
	auth := NewDIDAuthenticator(resolver)
	
	// Create an Ethereum DID signed message
	signedMsg := &SignedMessage{
		DID:       "did:ethr:0x1234567890123456789012345678901234567890",
		Timestamp: time.Now().Unix(),
		Nonce:     "test-nonce",
		Message:   "authenticate",
		Signature: "0x" + "1234567890abcdef" + "fedcba0987654321" + "12", // Mock signature (65 bytes)
		Type:      "ethereum",
	}
	
	signedMsgJSON, err := json.Marshal(signedMsg)
	require.NoError(t, err)
	
	// Note: This test will fail signature verification since we're using mock data
	_, err = auth.Authenticate(context.Background(), string(signedMsgJSON))
	assert.Error(t, err) // Expected to fail with mock signature
}

func TestDIDAuthenticator_InvalidFormat(t *testing.T) {
	resolver := NewMemoryDIDResolver()
	auth := NewDIDAuthenticator(resolver)
	
	tests := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"invalid JSON", "invalid-json"},
		{"missing fields", `{"did": "did:example:123"}`},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auth.Authenticate(context.Background(), tt.token)
			assert.Error(t, err)
		})
	}
}

func TestDIDAuthenticator_ExpiredTimestamp(t *testing.T) {
	resolver := NewMemoryDIDResolver()
	auth := NewDIDAuthenticator(resolver)
	
	// Create a signed message with old timestamp
	signedMsg := &SignedMessage{
		DID:       "did:example:123",
		Timestamp: time.Now().Unix() - 400, // 400 seconds ago (> 5 minutes)
		Nonce:     "test-nonce",
		Message:   "authenticate",
		Signature: "mock-signature",
		Type:      "did",
	}
	
	signedMsgJSON, err := json.Marshal(signedMsg)
	require.NoError(t, err)
	
	_, err = auth.Authenticate(context.Background(), string(signedMsgJSON))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timestamp out of range")
}

func TestDIDAuthenticator_UnsupportedSignatureType(t *testing.T) {
	resolver := NewMemoryDIDResolver()
	auth := NewDIDAuthenticator(resolver)
	
	signedMsg := &SignedMessage{
		DID:       "did:example:123",
		Timestamp: time.Now().Unix(),
		Nonce:     "test-nonce",
		Message:   "authenticate",
		Signature: "mock-signature",
		Type:      "unsupported",
	}
	
	signedMsgJSON, err := json.Marshal(signedMsg)
	require.NoError(t, err)
	
	_, err = auth.Authenticate(context.Background(), string(signedMsgJSON))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported signature type")
}

func TestMemoryDIDResolver(t *testing.T) {
	resolver := NewMemoryDIDResolver()
	
	did := "did:example:123"
	didDoc := &DIDDocument{
		ID:      did,
		Context: []string{"https://www.w3.org/ns/did/v1"},
		VerificationMethod: []VerificationMethod{
			{
				ID:           did + "#key1",
				Type:         "EcdsaSecp256k1VerificationKey2019",
				Controller:   did,
				PublicKeyHex: "04abcdef1234567890",
			},
		},
		Authentication: []string{did + "#key1"},
		Service: []Service{
			{
				ID:              did + "#service1",
				Type:            "LinkedDomains",
				ServiceEndpoint: "https://example.com",
			},
		},
	}
	
	// Register DID document
	resolver.RegisterDID(did, didDoc)
	
	// Resolve DID
	resolved, err := resolver.ResolveDID(context.Background(), did)
	require.NoError(t, err)
	assert.Equal(t, didDoc.ID, resolved.ID)
	assert.Equal(t, didDoc.Context, resolved.Context)
	assert.Len(t, resolved.VerificationMethod, 1)
	assert.Equal(t, didDoc.VerificationMethod[0].ID, resolved.VerificationMethod[0].ID)
	assert.Len(t, resolved.Service, 1)
	assert.Equal(t, didDoc.Service[0].ID, resolved.Service[0].ID)
	
	// Try to resolve non-existent DID
	_, err = resolver.ResolveDID(context.Background(), "did:example:nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DID not found")
}

func TestDIDDocument_JSONSerialization(t *testing.T) {
	didDoc := &DIDDocument{
		ID:      "did:example:123",
		Context: []string{"https://www.w3.org/ns/did/v1"},
		VerificationMethod: []VerificationMethod{
			{
				ID:                 "did:example:123#key1",
				Type:               "EcdsaSecp256k1VerificationKey2019",
				Controller:         "did:example:123",
				PublicKeyHex:       "04abcdef1234567890",
				EthereumAddress:    "0x1234567890123456789012345678901234567890",
			},
		},
		Authentication: []string{"did:example:123#key1"},
		Service: []Service{
			{
				ID:              "did:example:123#service1",
				Type:            "LinkedDomains",
				ServiceEndpoint: "https://example.com",
			},
		},
	}
	
	// Test marshaling
	data, err := json.Marshal(didDoc)
	require.NoError(t, err)
	
	// Test unmarshaling
	var decoded DIDDocument
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	
	assert.Equal(t, didDoc.ID, decoded.ID)
	assert.Equal(t, didDoc.Context, decoded.Context)
	assert.Len(t, decoded.VerificationMethod, 1)
	assert.Equal(t, didDoc.VerificationMethod[0].ID, decoded.VerificationMethod[0].ID)
	assert.Equal(t, didDoc.VerificationMethod[0].Type, decoded.VerificationMethod[0].Type)
	assert.Equal(t, didDoc.VerificationMethod[0].PublicKeyHex, decoded.VerificationMethod[0].PublicKeyHex)
	assert.Equal(t, didDoc.VerificationMethod[0].EthereumAddress, decoded.VerificationMethod[0].EthereumAddress)
	assert.Equal(t, didDoc.Authentication, decoded.Authentication)
	assert.Len(t, decoded.Service, 1)
	assert.Equal(t, didDoc.Service[0].ID, decoded.Service[0].ID)
}

func TestSignedMessage_JSONSerialization(t *testing.T) {
	signedMsg := &SignedMessage{
		DID:       "did:example:123",
		Timestamp: time.Now().Unix(),
		Nonce:     "test-nonce-12345",
		Message:   "authenticate to ipfs-cluster",
		Signature: "0x1234567890abcdef",
		Type:      "ethereum",
	}
	
	// Test marshaling
	data, err := json.Marshal(signedMsg)
	require.NoError(t, err)
	
	// Test unmarshaling
	var decoded SignedMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	
	assert.Equal(t, signedMsg.DID, decoded.DID)
	assert.Equal(t, signedMsg.Timestamp, decoded.Timestamp)
	assert.Equal(t, signedMsg.Nonce, decoded.Nonce)
	assert.Equal(t, signedMsg.Message, decoded.Message)
	assert.Equal(t, signedMsg.Signature, decoded.Signature)
	assert.Equal(t, signedMsg.Type, decoded.Type)
}

func TestDIDAuthenticator_ValidateToken(t *testing.T) {
	resolver := NewMemoryDIDResolver()
	auth := NewDIDAuthenticator(resolver)
	
	signedMsg := &SignedMessage{
		DID:       "did:example:123",
		Timestamp: time.Now().Unix(),
		Nonce:     "test-nonce",
		Message:   "authenticate",
		Signature: "mock-signature",
		Type:      "did",
	}
	
	signedMsgJSON, err := json.Marshal(signedMsg)
	require.NoError(t, err)
	
	// ValidateToken should have the same behavior as Authenticate
	err = auth.ValidateToken(context.Background(), string(signedMsgJSON))
	assert.Error(t, err) // Expected to fail with mock data
}