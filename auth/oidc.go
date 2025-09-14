package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OIDCAuthenticator implements OpenID Connect authentication
type OIDCAuthenticator struct {
	issuer       string
	clientID     string
	clientSecret string
	discovery    *OIDCDiscovery
	httpClient   *http.Client
}

// OIDCDiscovery represents OIDC discovery document
type OIDCDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSUri               string `json:"jwks_uri"`
	ScopesSupported       []string `json:"scopes_supported"`
	ResponseTypesSupported []string `json:"response_types_supported"`
}

// OIDCTokenResponse represents the response from token endpoint
type OIDCTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// OIDCUserInfo represents user information from userinfo endpoint
type OIDCUserInfo struct {
	Sub               string `json:"sub"`
	Name              string `json:"name,omitempty"`
	GivenName         string `json:"given_name,omitempty"`
	FamilyName        string `json:"family_name,omitempty"`
	MiddleName        string `json:"middle_name,omitempty"`
	Nickname          string `json:"nickname,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Profile           string `json:"profile,omitempty"`
	Picture           string `json:"picture,omitempty"`
	Website           string `json:"website,omitempty"`
	Email             string `json:"email,omitempty"`
	EmailVerified     bool   `json:"email_verified,omitempty"`
	Gender            string `json:"gender,omitempty"`
	Birthdate         string `json:"birthdate,omitempty"`
	Zoneinfo          string `json:"zoneinfo,omitempty"`
	Locale            string `json:"locale,omitempty"`
	PhoneNumber       string `json:"phone_number,omitempty"`
	PhoneNumberVerified bool `json:"phone_number_verified,omitempty"`
	Address           *OIDCAddress `json:"address,omitempty"`
	UpdatedAt         int64  `json:"updated_at,omitempty"`
	
	// Custom claims
	Groups []string `json:"groups,omitempty"`
	Roles  []string `json:"roles,omitempty"`
}

// OIDCAddress represents address information
type OIDCAddress struct {
	Formatted     string `json:"formatted,omitempty"`
	StreetAddress string `json:"street_address,omitempty"`
	Locality      string `json:"locality,omitempty"`
	Region        string `json:"region,omitempty"`
	PostalCode    string `json:"postal_code,omitempty"`
	Country       string `json:"country,omitempty"`
}

// NewOIDCAuthenticator creates a new OIDC authenticator
func NewOIDCAuthenticator(issuer, clientID, clientSecret string) (*OIDCAuthenticator, error) {
	authenticator := &OIDCAuthenticator{
		issuer:       issuer,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient: &http.Client{
			Timeout: time.Second * 30,
		},
	}
	
	// Discover OIDC endpoints
	if err := authenticator.discover(); err != nil {
		return nil, fmt.Errorf("OIDC discovery failed: %w", err)
	}
	
	return authenticator, nil
}

// Authenticate validates an OIDC access token and returns a Principal
func (o *OIDCAuthenticator) Authenticate(ctx context.Context, token string) (*Principal, error) {
	// Get user info using the access token
	userInfo, err := o.getUserInfo(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	
	// Map OIDC user info to Principal
	principal := &Principal{
		ID:       userInfo.Sub,
		Type:     "user",
		TenantID: "default", // Could be mapped from custom claims
		Method:   AuthMethodOIDC,
		IssuedAt: time.Now(),
		Attrs:    make(map[string]string),
	}
	
	// Map user attributes
	if userInfo.Email != "" {
		principal.Attrs["email"] = userInfo.Email
	}
	if userInfo.Name != "" {
		principal.Attrs["name"] = userInfo.Name
	}
	if userInfo.PreferredUsername != "" {
		principal.Attrs["username"] = userInfo.PreferredUsername
	}
	
	// Map roles from OIDC claims
	if len(userInfo.Roles) > 0 {
		principal.Roles = userInfo.Roles
	} else if len(userInfo.Groups) > 0 {
		// Map groups to roles if no explicit roles
		principal.Roles = userInfo.Groups
	} else {
		// Default role
		principal.Roles = []string{"user"}
	}
	
	return principal, nil
}

// GetMethod returns the authentication method
func (o *OIDCAuthenticator) GetMethod() AuthMethod {
	return AuthMethodOIDC
}

// ValidateToken validates an OIDC access token
func (o *OIDCAuthenticator) ValidateToken(ctx context.Context, token string) error {
	_, err := o.getUserInfo(ctx, token)
	return err
}

// discover performs OIDC discovery to get endpoints
func (o *OIDCAuthenticator) discover() error {
	discoveryURL := strings.TrimSuffix(o.issuer, "/") + "/.well-known/openid_configuration"
	
	resp, err := o.httpClient.Get(discoveryURL)
	if err != nil {
		return fmt.Errorf("failed to fetch discovery document: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discovery request failed with status: %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read discovery response: %w", err)
	}
	
	var discovery OIDCDiscovery
	if err := json.Unmarshal(body, &discovery); err != nil {
		return fmt.Errorf("failed to parse discovery document: %w", err)
	}
	
	o.discovery = &discovery
	return nil
}

// getUserInfo retrieves user information using access token
func (o *OIDCAuthenticator) getUserInfo(ctx context.Context, accessToken string) (*OIDCUserInfo, error) {
	if o.discovery == nil || o.discovery.UserInfoEndpoint == "" {
		return nil, fmt.Errorf("userinfo endpoint not available")
	}
	
	req, err := http.NewRequestWithContext(ctx, "GET", o.discovery.UserInfoEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create userinfo request: %w", err)
	}
	
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo request failed with status: %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read userinfo response: %w", err)
	}
	
	var userInfo OIDCUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse userinfo response: %w", err)
	}
	
	return &userInfo, nil
}

// ExchangeCodeForToken exchanges authorization code for access token
func (o *OIDCAuthenticator) ExchangeCodeForToken(ctx context.Context, code, redirectURI string) (*OIDCTokenResponse, error) {
	if o.discovery == nil || o.discovery.TokenEndpoint == "" {
		return nil, fmt.Errorf("token endpoint not available")
	}
	
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", o.clientID)
	data.Set("client_secret", o.clientSecret)
	
	req, err := http.NewRequestWithContext(ctx, "POST", o.discovery.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	var tokenResponse OIDCTokenResponse
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	
	return &tokenResponse, nil
}

// GetAuthorizationURL generates the authorization URL for OIDC flow
func (o *OIDCAuthenticator) GetAuthorizationURL(redirectURI, state string, scopes []string) (string, error) {
	if o.discovery == nil || o.discovery.AuthorizationEndpoint == "" {
		return "", fmt.Errorf("authorization endpoint not available")
	}
	
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}
	
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", o.clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", strings.Join(scopes, " "))
	params.Set("state", state)
	
	authURL := o.discovery.AuthorizationEndpoint + "?" + params.Encode()
	return authURL, nil
}

// RefreshToken refreshes an access token using refresh token
func (o *OIDCAuthenticator) RefreshToken(ctx context.Context, refreshToken string) (*OIDCTokenResponse, error) {
	if o.discovery == nil || o.discovery.TokenEndpoint == "" {
		return nil, fmt.Errorf("token endpoint not available")
	}
	
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", o.clientID)
	data.Set("client_secret", o.clientSecret)
	
	req, err := http.NewRequestWithContext(ctx, "POST", o.discovery.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh request failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	var tokenResponse OIDCTokenResponse
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}
	
	return &tokenResponse, nil
}