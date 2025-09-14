package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// JWTAuthenticator implements JWT token authentication
type JWTAuthenticator struct {
	secret   []byte
	issuer   string
	audience string
}

// JWTClaims represents JWT token claims
type JWTClaims struct {
	Subject   string            `json:"sub"`
	Issuer    string            `json:"iss"`
	Audience  string            `json:"aud"`
	ExpiresAt int64             `json:"exp"`
	IssuedAt  int64             `json:"iat"`
	NotBefore int64             `json:"nbf,omitempty"`
	JTI       string            `json:"jti,omitempty"`
	Type      string            `json:"type,omitempty"`
	TenantID  string            `json:"tenant_id,omitempty"`
	Roles     []string          `json:"roles,omitempty"`
	Attrs     map[string]string `json:"attrs,omitempty"`
}

// NewJWTAuthenticator creates a new JWT authenticator
func NewJWTAuthenticator(secret, issuer, audience string) *JWTAuthenticator {
	return &JWTAuthenticator{
		secret:   []byte(secret),
		issuer:   issuer,
		audience: audience,
	}
}

// Authenticate validates a JWT token and returns a Principal
func (j *JWTAuthenticator) Authenticate(ctx context.Context, token string) (*Principal, error) {
	claims, err := j.validateToken(token)
	if err != nil {
		return nil, fmt.Errorf("invalid JWT token: %w", err)
	}
	
	// Check expiration
	if claims.ExpiresAt > 0 && time.Now().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("token expired")
	}
	
	// Check not before
	if claims.NotBefore > 0 && time.Now().Unix() < claims.NotBefore {
		return nil, fmt.Errorf("token not yet valid")
	}
	
	// Check issuer
	if j.issuer != "" && claims.Issuer != j.issuer {
		return nil, fmt.Errorf("invalid issuer: expected %s, got %s", j.issuer, claims.Issuer)
	}
	
	// Check audience
	if j.audience != "" && claims.Audience != j.audience {
		return nil, fmt.Errorf("invalid audience: expected %s, got %s", j.audience, claims.Audience)
	}
	
	principal := &Principal{
		ID:       claims.Subject,
		Type:     claims.Type,
		TenantID: claims.TenantID,
		Roles:    claims.Roles,
		Attrs:    claims.Attrs,
		Method:   AuthMethodJWT,
		IssuedAt: time.Unix(claims.IssuedAt, 0),
	}
	
	if claims.ExpiresAt > 0 {
		expTime := time.Unix(claims.ExpiresAt, 0)
		principal.ExpiresAt = &expTime
	}
	
	// Set defaults
	if principal.Type == "" {
		principal.Type = "user"
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
func (j *JWTAuthenticator) GetMethod() AuthMethod {
	return AuthMethodJWT
}

// ValidateToken validates a JWT token without full authentication
func (j *JWTAuthenticator) ValidateToken(ctx context.Context, token string) error {
	_, err := j.validateToken(token)
	return err
}

// validateToken validates the JWT token signature and structure
func (j *JWTAuthenticator) validateToken(token string) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}
	
	// Decode header
	headerData, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid header encoding: %w", err)
	}
	
	var header map[string]interface{}
	if err := json.Unmarshal(headerData, &header); err != nil {
		return nil, fmt.Errorf("invalid header JSON: %w", err)
	}
	
	// Check algorithm
	alg, ok := header["alg"].(string)
	if !ok || alg != "HS256" {
		return nil, fmt.Errorf("unsupported algorithm: %v", alg)
	}
	
	// Verify signature
	message := parts[0] + "." + parts[1]
	expectedSignature := j.sign(message)
	
	actualSignature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding: %w", err)
	}
	
	if !hmac.Equal(expectedSignature, actualSignature) {
		return nil, fmt.Errorf("invalid signature")
	}
	
	// Decode payload
	payloadData, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid payload encoding: %w", err)
	}
	
	var claims JWTClaims
	if err := json.Unmarshal(payloadData, &claims); err != nil {
		return nil, fmt.Errorf("invalid payload JSON: %w", err)
	}
	
	return &claims, nil
}

// sign creates HMAC-SHA256 signature for the message
func (j *JWTAuthenticator) sign(message string) []byte {
	h := hmac.New(sha256.New, j.secret)
	h.Write([]byte(message))
	return h.Sum(nil)
}

// CreateToken creates a new JWT token (utility method for testing/admin)
func (j *JWTAuthenticator) CreateToken(principal *Principal, duration time.Duration) (string, error) {
	now := time.Now()
	
	claims := JWTClaims{
		Subject:   principal.ID,
		Issuer:    j.issuer,
		Audience:  j.audience,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(duration).Unix(),
		Type:      principal.Type,
		TenantID:  principal.TenantID,
		Roles:     principal.Roles,
		Attrs:     principal.Attrs,
	}
	
	// Create header
	header := map[string]interface{}{
		"alg": "HS256",
		"typ": "JWT",
	}
	
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal header: %w", err)
	}
	
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}
	
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	
	message := headerB64 + "." + payloadB64
	signature := j.sign(message)
	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)
	
	return message + "." + signatureB64, nil
}