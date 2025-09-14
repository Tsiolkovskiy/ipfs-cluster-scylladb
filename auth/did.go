package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// DIDAuthenticator implements DID and Web3 signature authentication
type DIDAuthenticator struct {
	resolver DIDResolver
}

// DIDResolver interface for resolving DID documents
type DIDResolver interface {
	// ResolveDID resolves a DID to a DID document
	ResolveDID(ctx context.Context, did string) (*DIDDocument, error)
}

// DIDDocument represents a DID document
type DIDDocument struct {
	ID                 string                 `json:"id"`
	Context            []string               `json:"@context"`
	VerificationMethod []VerificationMethod   `json:"verificationMethod"`
	Authentication     []string               `json:"authentication"`
	Service            []Service              `json:"service,omitempty"`
}

// VerificationMethod represents a verification method in a DID document
type VerificationMethod struct {
	ID                 string `json:"id"`
	Type               string `json:"type"`
	Controller         string `json:"controller"`
	PublicKeyMultibase string `json:"publicKeyMultibase,omitempty"`
	PublicKeyHex       string `json:"publicKeyHex,omitempty"`
	EthereumAddress    string `json:"ethereumAddress,omitempty"`
}

// Service represents a service in a DID document
type Service struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	ServiceEndpoint string `json:"serviceEndpoint"`
}

// SignedMessage represents a signed message for authentication
type SignedMessage struct {
	DID       string    `json:"did"`
	Timestamp int64     `json:"timestamp"`
	Nonce     string    `json:"nonce"`
	Message   string    `json:"message"`
	Signature string    `json:"signature"`
	Type      string    `json:"type"` // "did" or "ethereum"
}

// NewDIDAuthenticator creates a new DID authenticator
func NewDIDAuthenticator(resolver DIDResolver) *DIDAuthenticator {
	return &DIDAuthenticator{
		resolver: resolver,
	}
}

// Authenticate validates a DID signature and returns a Principal
func (d *DIDAuthenticator) Authenticate(ctx context.Context, token string) (*Principal, error) {
	var signedMsg SignedMessage
	if err := json.Unmarshal([]byte(token), &signedMsg); err != nil {
		return nil, fmt.Errorf("invalid signed message format: %w", err)
	}
	
	// Check timestamp (must be within 5 minutes)
	now := time.Now().Unix()
	if now-signedMsg.Timestamp > 300 || signedMsg.Timestamp-now > 300 {
		return nil, fmt.Errorf("message timestamp out of range")
	}
	
	// Verify signature based on type
	var err error
	switch signedMsg.Type {
	case "did":
		err = d.verifyDIDSignature(ctx, &signedMsg)
	case "ethereum":
		err = d.verifyEthereumSignature(ctx, &signedMsg)
	default:
		return nil, fmt.Errorf("unsupported signature type: %s", signedMsg.Type)
	}
	
	if err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}
	
	// Extract principal information from DID
	principal := &Principal{
		ID:       signedMsg.DID,
		Type:     "user",
		TenantID: "default",
		Roles:    []string{"user"},
		Attrs:    make(map[string]string),
		Method:   AuthMethodDID,
		IssuedAt: time.Unix(signedMsg.Timestamp, 0),
	}
	
	// Add signature type as attribute
	principal.Attrs["signature_type"] = signedMsg.Type
	
	return principal, nil
}

// GetMethod returns the authentication method
func (d *DIDAuthenticator) GetMethod() AuthMethod {
	return AuthMethodDID
}

// ValidateToken validates a DID signature without full authentication
func (d *DIDAuthenticator) ValidateToken(ctx context.Context, token string) error {
	_, err := d.Authenticate(ctx, token)
	return err
}

// verifyDIDSignature verifies a DID signature
func (d *DIDAuthenticator) verifyDIDSignature(ctx context.Context, signedMsg *SignedMessage) error {
	// Resolve DID document
	didDoc, err := d.resolver.ResolveDID(ctx, signedMsg.DID)
	if err != nil {
		return fmt.Errorf("failed to resolve DID: %w", err)
	}
	
	// Find verification method
	var verificationMethod *VerificationMethod
	for _, vm := range didDoc.VerificationMethod {
		// Check if this verification method is in the authentication array
		for _, authMethod := range didDoc.Authentication {
			if vm.ID == authMethod || strings.HasSuffix(authMethod, "#"+vm.ID) {
				verificationMethod = &vm
				break
			}
		}
		if verificationMethod != nil {
			break
		}
	}
	
	if verificationMethod == nil {
		return fmt.Errorf("no suitable verification method found")
	}
	
	// Create message to verify
	message := fmt.Sprintf("%s:%d:%s:%s", signedMsg.DID, signedMsg.Timestamp, signedMsg.Nonce, signedMsg.Message)
	
	// Verify signature based on key type
	switch verificationMethod.Type {
	case "EcdsaSecp256k1VerificationKey2019":
		return d.verifyECDSASignature(message, signedMsg.Signature, verificationMethod.PublicKeyHex)
	default:
		return fmt.Errorf("unsupported verification method type: %s", verificationMethod.Type)
	}
}

// verifyEthereumSignature verifies an Ethereum signature
func (d *DIDAuthenticator) verifyEthereumSignature(ctx context.Context, signedMsg *SignedMessage) error {
	// For Ethereum signatures, the DID should be in the format did:ethr:0x...
	if !strings.HasPrefix(signedMsg.DID, "did:ethr:") {
		return fmt.Errorf("invalid Ethereum DID format")
	}
	
	address := strings.TrimPrefix(signedMsg.DID, "did:ethr:")
	
	// Create message to verify (Ethereum signed message format)
	message := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s:%d:%s:%s",
		len(fmt.Sprintf("%s:%d:%s:%s", signedMsg.DID, signedMsg.Timestamp, signedMsg.Nonce, signedMsg.Message)),
		signedMsg.DID, signedMsg.Timestamp, signedMsg.Nonce, signedMsg.Message)
	
	// Verify signature
	return d.verifyEthereumECDSASignature(message, signedMsg.Signature, address)
}

// verifyECDSASignature verifies an ECDSA signature
func (d *DIDAuthenticator) verifyECDSASignature(message, signature, publicKeyHex string) error {
	// Decode public key
	pubKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return fmt.Errorf("invalid public key hex: %w", err)
	}
	
	// Parse public key (assuming uncompressed format: 04 + x + y)
	if len(pubKeyBytes) != 65 || pubKeyBytes[0] != 0x04 {
		return fmt.Errorf("invalid public key format")
	}
	
	x := new(big.Int).SetBytes(pubKeyBytes[1:33])
	y := new(big.Int).SetBytes(pubKeyBytes[33:65])
	
	pubKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}
	
	// Decode signature
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}
	
	if len(sigBytes) != 64 {
		return fmt.Errorf("invalid signature length")
	}
	
	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])
	
	// Hash message
	hash := sha256.Sum256([]byte(message))
	
	// Verify signature
	if !ecdsa.Verify(pubKey, hash[:], r, s) {
		return fmt.Errorf("signature verification failed")
	}
	
	return nil
}

// verifyEthereumECDSASignature verifies an Ethereum ECDSA signature
func (d *DIDAuthenticator) verifyEthereumECDSASignature(message, signature, expectedAddress string) error {
	// Decode signature
	sigBytes, err := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
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
	
	// Hash message
	hash := sha256.Sum256([]byte(message))
	
	// Recover public key
	pubKey, err := d.recoverPublicKey(hash[:], r, s, v-27)
	if err != nil {
		return fmt.Errorf("failed to recover public key: %w", err)
	}
	
	// Derive Ethereum address from public key
	address := d.publicKeyToAddress(pubKey)
	
	// Compare addresses (case-insensitive)
	if !strings.EqualFold(address, expectedAddress) {
		return fmt.Errorf("signature does not match expected address")
	}
	
	return nil
}

// recoverPublicKey recovers the public key from signature
func (d *DIDAuthenticator) recoverPublicKey(hash []byte, r, s *big.Int, recovery byte) (*ecdsa.PublicKey, error) {
	// This is a simplified implementation
	// In a real implementation, you would use a proper secp256k1 library
	return nil, fmt.Errorf("public key recovery not implemented")
}

// publicKeyToAddress converts a public key to an Ethereum address
func (d *DIDAuthenticator) publicKeyToAddress(pubKey *ecdsa.PublicKey) string {
	// This is a simplified implementation
	// In a real implementation, you would use Keccak256 hash
	return "0x0000000000000000000000000000000000000000"
}

// MemoryDIDResolver implements DIDResolver using in-memory storage
type MemoryDIDResolver struct {
	documents map[string]*DIDDocument
}

// NewMemoryDIDResolver creates a new in-memory DID resolver
func NewMemoryDIDResolver() *MemoryDIDResolver {
	return &MemoryDIDResolver{
		documents: make(map[string]*DIDDocument),
	}
}

// ResolveDID resolves a DID to a DID document
func (m *MemoryDIDResolver) ResolveDID(ctx context.Context, did string) (*DIDDocument, error) {
	doc, exists := m.documents[did]
	if !exists {
		return nil, fmt.Errorf("DID not found: %s", did)
	}
	
	return doc, nil
}

// RegisterDID registers a DID document (for testing)
func (m *MemoryDIDResolver) RegisterDID(did string, doc *DIDDocument) {
	m.documents[did] = doc
}