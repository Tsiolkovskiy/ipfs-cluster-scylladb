package auth

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Web3Authenticator implements Web3 wallet authentication
type Web3Authenticator struct {
	chainID         int64
	contractAddress string
	resolver        Web3Resolver
}

// Web3Resolver interface for resolving Web3 identities
type Web3Resolver interface {
	// ResolveAddress resolves an Ethereum address to identity information
	ResolveAddress(ctx context.Context, address string) (*Web3Identity, error)
	
	// VerifySignature verifies an Ethereum signature
	VerifySignature(ctx context.Context, message, signature, address string) error
}

// Web3Identity represents a Web3 identity
type Web3Identity struct {
	Address     string            `json:"address"`
	ENSName     string            `json:"ens_name,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	Roles       []string          `json:"roles,omitempty"`
	TenantID    string            `json:"tenant_id,omitempty"`
	Verified    bool              `json:"verified"`
	LastUpdated time.Time         `json:"last_updated"`
}

// Web3SignedMessage represents a signed message from a Web3 wallet
type Web3SignedMessage struct {
	Address   string    `json:"address"`
	Message   string    `json:"message"`
	Signature string    `json:"signature"`
	Timestamp int64     `json:"timestamp"`
	ChainID   int64     `json:"chain_id,omitempty"`
	Nonce     string    `json:"nonce,omitempty"`
}

// NewWeb3Authenticator creates a new Web3 authenticator
func NewWeb3Authenticator(chainID int64, contractAddress string, resolver Web3Resolver) *Web3Authenticator {
	return &Web3Authenticator{
		chainID:         chainID,
		contractAddress: contractAddress,
		resolver:        resolver,
	}
}

// Authenticate validates a Web3 signature and returns a Principal
func (w *Web3Authenticator) Authenticate(ctx context.Context, token string) (*Principal, error) {
	var signedMsg Web3SignedMessage
	if err := json.Unmarshal([]byte(token), &signedMsg); err != nil {
		return nil, fmt.Errorf("invalid Web3 signed message format: %w", err)
	}
	
	// Validate timestamp (must be within 5 minutes)
	now := time.Now().Unix()
	if now-signedMsg.Timestamp > 300 || signedMsg.Timestamp-now > 300 {
		return nil, fmt.Errorf("message timestamp out of range")
	}
	
	// Validate chain ID if specified
	if signedMsg.ChainID != 0 && signedMsg.ChainID != w.chainID {
		return nil, fmt.Errorf("invalid chain ID: expected %d, got %d", w.chainID, signedMsg.ChainID)
	}
	
	// Verify signature
	if err := w.resolver.VerifySignature(ctx, signedMsg.Message, signedMsg.Signature, signedMsg.Address); err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}
	
	// Resolve Web3 identity
	identity, err := w.resolver.ResolveAddress(ctx, signedMsg.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Web3 identity: %w", err)
	}
	
	// Create principal
	principal := &Principal{
		ID:       signedMsg.Address,
		Type:     "user",
		TenantID: identity.TenantID,
		Roles:    identity.Roles,
		Attrs:    identity.Attributes,
		Method:   AuthMethodWeb3,
		IssuedAt: time.Unix(signedMsg.Timestamp, 0),
	}
	
	// Set defaults
	if principal.TenantID == "" {
		principal.TenantID = "default"
	}
	if len(principal.Roles) == 0 {
		principal.Roles = []string{"user"}
	}
	if principal.Attrs == nil {
		principal.Attrs = make(map[string]string)
	}
	
	// Add Web3-specific attributes
	principal.Attrs["chain_id"] = fmt.Sprintf("%d", w.chainID)
	if identity.ENSName != "" {
		principal.Attrs["ens_name"] = identity.ENSName
	}
	
	return principal, nil
}

// GetMethod returns the authentication method
func (w *Web3Authenticator) GetMethod() AuthMethod {
	return AuthMethodWeb3
}

// ValidateToken validates a Web3 signature without full authentication
func (w *Web3Authenticator) ValidateToken(ctx context.Context, token string) error {
	_, err := w.Authenticate(ctx, token)
	return err
}

// MemoryWeb3Resolver implements Web3Resolver using in-memory storage
type MemoryWeb3Resolver struct {
	identities map[string]*Web3Identity
}

// NewMemoryWeb3Resolver creates a new in-memory Web3 resolver
func NewMemoryWeb3Resolver() *MemoryWeb3Resolver {
	return &MemoryWeb3Resolver{
		identities: make(map[string]*Web3Identity),
	}
}

// ResolveAddress resolves an Ethereum address to identity information
func (m *MemoryWeb3Resolver) ResolveAddress(ctx context.Context, address string) (*Web3Identity, error) {
	// Normalize address to lowercase
	address = strings.ToLower(address)
	
	identity, exists := m.identities[address]
	if !exists {
		// Return default identity for unknown addresses
		return &Web3Identity{
			Address:     address,
			Attributes:  make(map[string]string),
			Roles:       []string{"user"},
			TenantID:    "default",
			Verified:    false,
			LastUpdated: time.Now(),
		}, nil
	}
	
	return identity, nil
}

// VerifySignature verifies an Ethereum signature
func (m *MemoryWeb3Resolver) VerifySignature(ctx context.Context, message, signature, address string) error {
	// This is a simplified implementation
	// In a real implementation, you would use proper Ethereum signature verification
	
	// Basic validation
	if !strings.HasPrefix(signature, "0x") {
		return fmt.Errorf("invalid signature format")
	}
	
	sigBytes, err := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}
	
	if len(sigBytes) != 65 {
		return fmt.Errorf("invalid signature length: expected 65 bytes, got %d", len(sigBytes))
	}
	
	// In a real implementation, you would:
	// 1. Hash the message with Ethereum signed message prefix
	// 2. Recover the public key from the signature
	// 3. Derive the address from the public key
	// 4. Compare with the expected address
	
	// For now, just validate format
	if !strings.HasPrefix(address, "0x") || len(address) != 42 {
		return fmt.Errorf("invalid Ethereum address format")
	}
	
	return nil
}

// RegisterIdentity registers a Web3 identity (for testing/admin)
func (m *MemoryWeb3Resolver) RegisterIdentity(address string, identity *Web3Identity) {
	address = strings.ToLower(address)
	identity.Address = address
	identity.LastUpdated = time.Now()
	m.identities[address] = identity
}

// EthereumSignatureVerifier provides utilities for Ethereum signature verification
type EthereumSignatureVerifier struct{}

// NewEthereumSignatureVerifier creates a new Ethereum signature verifier
func NewEthereumSignatureVerifier() *EthereumSignatureVerifier {
	return &EthereumSignatureVerifier{}
}

// VerifyPersonalSign verifies an Ethereum personal_sign signature
func (e *EthereumSignatureVerifier) VerifyPersonalSign(message, signature, expectedAddress string) error {
	// Remove 0x prefix from signature
	sigHex := strings.TrimPrefix(signature, "0x")
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}
	
	if len(sigBytes) != 65 {
		return fmt.Errorf("invalid signature length")
	}
	
	// Extract r, s, v
	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:64])
	v := sigBytes[64]
	
	// Adjust v for recovery
	if v < 27 {
		v += 27
	}
	
	// Create Ethereum signed message hash
	messageHash := e.hashPersonalMessage(message)
	
	// Recover public key and derive address
	recoveredAddress, err := e.recoverAddress(messageHash, r, s, v-27)
	if err != nil {
		return fmt.Errorf("failed to recover address: %w", err)
	}
	
	// Compare addresses (case-insensitive)
	if !strings.EqualFold(recoveredAddress, expectedAddress) {
		return fmt.Errorf("signature verification failed: addresses don't match")
	}
	
	return nil
}

// hashPersonalMessage creates the hash for Ethereum personal_sign
func (e *EthereumSignatureVerifier) hashPersonalMessage(message string) []byte {
	// Ethereum signed message format: "\x19Ethereum Signed Message:\n" + len(message) + message
	prefix := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(message))
	fullMessage := prefix + message
	
	// In a real implementation, you would use Keccak256
	// For now, return a placeholder
	return []byte(fullMessage)
}

// recoverAddress recovers the Ethereum address from signature components
func (e *EthereumSignatureVerifier) recoverAddress(messageHash []byte, r, s *big.Int, recovery byte) (string, error) {
	// This is a placeholder implementation
	// In a real implementation, you would use secp256k1 curve operations
	// to recover the public key and derive the Ethereum address
	
	return "0x0000000000000000000000000000000000000000", fmt.Errorf("signature recovery not implemented")
}

// WalletConnectProvider provides WalletConnect integration
type WalletConnectProvider struct {
	projectID string
	metadata  WalletConnectMetadata
}

// WalletConnectMetadata represents WalletConnect app metadata
type WalletConnectMetadata struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	URL         string   `json:"url"`
	Icons       []string `json:"icons"`
}

// WalletConnectSession represents a WalletConnect session
type WalletConnectSession struct {
	Topic     string    `json:"topic"`
	Accounts  []string  `json:"accounts"`
	ChainID   int64     `json:"chain_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewWalletConnectProvider creates a new WalletConnect provider
func NewWalletConnectProvider(projectID string, metadata WalletConnectMetadata) *WalletConnectProvider {
	return &WalletConnectProvider{
		projectID: projectID,
		metadata:  metadata,
	}
}

// CreateSession creates a new WalletConnect session
func (w *WalletConnectProvider) CreateSession(ctx context.Context, chainID int64) (*WalletConnectSession, error) {
	// This is a placeholder implementation
	// In a real implementation, you would integrate with WalletConnect SDK
	
	session := &WalletConnectSession{
		Topic:     "placeholder-topic",
		Accounts:  []string{},
		ChainID:   chainID,
		ExpiresAt: time.Now().Add(time.Hour * 24),
	}
	
	return session, nil
}

// RequestSignature requests a signature from the connected wallet
func (w *WalletConnectProvider) RequestSignature(ctx context.Context, session *WalletConnectSession, message string) (string, error) {
	// This is a placeholder implementation
	// In a real implementation, you would send a signing request through WalletConnect
	
	return "0x" + strings.Repeat("00", 65), fmt.Errorf("WalletConnect signature request not implemented")
}

// MetaMaskProvider provides MetaMask integration utilities
type MetaMaskProvider struct{}

// NewMetaMaskProvider creates a new MetaMask provider
func NewMetaMaskProvider() *MetaMaskProvider {
	return &MetaMaskProvider{}
}

// GenerateConnectScript generates JavaScript for MetaMask connection
func (m *MetaMaskProvider) GenerateConnectScript() string {
	return `
// MetaMask connection script
async function connectMetaMask() {
    if (typeof window.ethereum !== 'undefined') {
        try {
            // Request account access
            const accounts = await window.ethereum.request({ 
                method: 'eth_requestAccounts' 
            });
            
            // Get chain ID
            const chainId = await window.ethereum.request({ 
                method: 'eth_chainId' 
            });
            
            return {
                address: accounts[0],
                chainId: parseInt(chainId, 16)
            };
        } catch (error) {
            throw new Error('User rejected connection: ' + error.message);
        }
    } else {
        throw new Error('MetaMask not detected');
    }
}

async function signMessage(message) {
    if (typeof window.ethereum !== 'undefined') {
        try {
            const accounts = await window.ethereum.request({ 
                method: 'eth_accounts' 
            });
            
            if (accounts.length === 0) {
                throw new Error('No accounts connected');
            }
            
            const signature = await window.ethereum.request({
                method: 'personal_sign',
                params: [message, accounts[0]]
            });
            
            return signature;
        } catch (error) {
            throw new Error('Signature failed: ' + error.message);
        }
    } else {
        throw new Error('MetaMask not detected');
    }
}
`
}

// GenerateAuthenticationMessage generates a message for Web3 authentication
func GenerateAuthenticationMessage(address string, nonce string, timestamp int64, domain string) string {
	return fmt.Sprintf(`%s wants you to sign in with your Ethereum account:
%s

Please sign this message to authenticate.

URI: %s
Version: 1
Chain ID: 1
Nonce: %s
Issued At: %s`,
		domain,
		address,
		domain,
		nonce,
		time.Unix(timestamp, 0).Format(time.RFC3339),
	)
}