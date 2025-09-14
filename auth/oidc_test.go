package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOIDCAuthenticator_Discovery(t *testing.T) {
	// Create mock OIDC server
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid_configuration" {
			discovery := OIDCDiscovery{
				Issuer:                server.URL,
				AuthorizationEndpoint: server.URL + "/auth",
				TokenEndpoint:         server.URL + "/token",
				UserInfoEndpoint:      server.URL + "/userinfo",
				JWKSUri:              server.URL + "/jwks",
				ScopesSupported:       []string{"openid", "profile", "email"},
				ResponseTypesSupported: []string{"code"},
			}
			
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(discovery)
			return
		}
		
		http.NotFound(w, r)
	}))
	defer server.Close()
	
	// Test successful discovery
	auth, err := NewOIDCAuthenticator(server.URL, "test-client", "test-secret")
	require.NoError(t, err)
	assert.NotNil(t, auth.discovery)
	assert.Equal(t, server.URL, auth.discovery.Issuer)
	assert.Equal(t, server.URL+"/userinfo", auth.discovery.UserInfoEndpoint)
}

func TestOIDCAuthenticator_Authenticate(t *testing.T) {
	// Create mock OIDC server
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid_configuration":
			discovery := OIDCDiscovery{
				Issuer:           server.URL,
				UserInfoEndpoint: server.URL + "/userinfo",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(discovery)
			
		case "/userinfo":
			// Check authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader != "Bearer valid-token" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			
			userInfo := OIDCUserInfo{
				Sub:               "user-123",
				Name:              "Test User",
				Email:             "test@example.com",
				PreferredUsername: "testuser",
				Roles:             []string{"admin", "user"},
			}
			
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(userInfo)
			
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	
	auth, err := NewOIDCAuthenticator(server.URL, "test-client", "test-secret")
	require.NoError(t, err)
	
	// Test successful authentication
	principal, err := auth.Authenticate(context.Background(), "valid-token")
	require.NoError(t, err)
	
	assert.Equal(t, "user-123", principal.ID)
	assert.Equal(t, "user", principal.Type)
	assert.Equal(t, AuthMethodOIDC, principal.Method)
	assert.Equal(t, []string{"admin", "user"}, principal.Roles)
	assert.Equal(t, "test@example.com", principal.Attrs["email"])
	assert.Equal(t, "Test User", principal.Attrs["name"])
	assert.Equal(t, "testuser", principal.Attrs["username"])
	
	// Test invalid token
	_, err = auth.Authenticate(context.Background(), "invalid-token")
	assert.Error(t, err)
}

func TestOIDCAuthenticator_ValidateToken(t *testing.T) {
	// Create mock OIDC server
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid_configuration":
			discovery := OIDCDiscovery{
				Issuer:           server.URL,
				UserInfoEndpoint: server.URL + "/userinfo",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(discovery)
			
		case "/userinfo":
			authHeader := r.Header.Get("Authorization")
			if authHeader == "Bearer valid-token" {
				userInfo := OIDCUserInfo{Sub: "user-123"}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(userInfo)
			} else {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			}
			
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	
	auth, err := NewOIDCAuthenticator(server.URL, "test-client", "test-secret")
	require.NoError(t, err)
	
	// Test valid token
	err = auth.ValidateToken(context.Background(), "valid-token")
	assert.NoError(t, err)
	
	// Test invalid token
	err = auth.ValidateToken(context.Background(), "invalid-token")
	assert.Error(t, err)
}

func TestOIDCAuthenticator_GetAuthorizationURL(t *testing.T) {
	// Create mock OIDC server
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		discovery := OIDCDiscovery{
			Issuer:                server.URL,
			AuthorizationEndpoint: server.URL + "/auth",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(discovery)
	}))
	defer server.Close()
	
	auth, err := NewOIDCAuthenticator(server.URL, "test-client", "test-secret")
	require.NoError(t, err)
	
	authURL, err := auth.GetAuthorizationURL("http://localhost/callback", "test-state", []string{"openid", "profile"})
	require.NoError(t, err)
	
	assert.Contains(t, authURL, server.URL+"/auth")
	assert.Contains(t, authURL, "client_id=test-client")
	assert.Contains(t, authURL, "redirect_uri=http%3A%2F%2Flocalhost%2Fcallback")
	assert.Contains(t, authURL, "state=test-state")
	assert.Contains(t, authURL, "scope=openid+profile")
}

func TestOIDCAuthenticator_ExchangeCodeForToken(t *testing.T) {
	// Create mock OIDC server
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid_configuration":
			discovery := OIDCDiscovery{
				Issuer:        server.URL,
				TokenEndpoint: server.URL + "/token",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(discovery)
			
		case "/token":
			if r.Method != "POST" {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			
			err := r.ParseForm()
			if err != nil {
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}
			
			if r.Form.Get("grant_type") != "authorization_code" {
				http.Error(w, "Invalid grant type", http.StatusBadRequest)
				return
			}
			
			if r.Form.Get("code") != "valid-code" {
				http.Error(w, "Invalid code", http.StatusBadRequest)
				return
			}
			
			tokenResponse := OIDCTokenResponse{
				AccessToken:  "access-token-123",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				RefreshToken: "refresh-token-123",
				IDToken:      "id-token-123",
				Scope:        "openid profile email",
			}
			
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenResponse)
			
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	
	auth, err := NewOIDCAuthenticator(server.URL, "test-client", "test-secret")
	require.NoError(t, err)
	
	// Test successful token exchange
	tokenResponse, err := auth.ExchangeCodeForToken(context.Background(), "valid-code", "http://localhost/callback")
	require.NoError(t, err)
	
	assert.Equal(t, "access-token-123", tokenResponse.AccessToken)
	assert.Equal(t, "Bearer", tokenResponse.TokenType)
	assert.Equal(t, 3600, tokenResponse.ExpiresIn)
	assert.Equal(t, "refresh-token-123", tokenResponse.RefreshToken)
	
	// Test invalid code
	_, err = auth.ExchangeCodeForToken(context.Background(), "invalid-code", "http://localhost/callback")
	assert.Error(t, err)
}

func TestOIDCAuthenticator_RefreshToken(t *testing.T) {
	// Create mock OIDC server
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid_configuration":
			discovery := OIDCDiscovery{
				Issuer:        server.URL,
				TokenEndpoint: server.URL + "/token",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(discovery)
			
		case "/token":
			err := r.ParseForm()
			if err != nil {
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}
			
			if r.Form.Get("grant_type") != "refresh_token" {
				http.Error(w, "Invalid grant type", http.StatusBadRequest)
				return
			}
			
			if r.Form.Get("refresh_token") != "valid-refresh-token" {
				http.Error(w, "Invalid refresh token", http.StatusBadRequest)
				return
			}
			
			tokenResponse := OIDCTokenResponse{
				AccessToken: "new-access-token",
				TokenType:   "Bearer",
				ExpiresIn:   3600,
			}
			
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenResponse)
			
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	
	auth, err := NewOIDCAuthenticator(server.URL, "test-client", "test-secret")
	require.NoError(t, err)
	
	// Test successful token refresh
	tokenResponse, err := auth.RefreshToken(context.Background(), "valid-refresh-token")
	require.NoError(t, err)
	
	assert.Equal(t, "new-access-token", tokenResponse.AccessToken)
	assert.Equal(t, "Bearer", tokenResponse.TokenType)
	
	// Test invalid refresh token
	_, err = auth.RefreshToken(context.Background(), "invalid-refresh-token")
	assert.Error(t, err)
}

func TestOIDCAuthenticator_GetMethod(t *testing.T) {
	// Create minimal mock server for discovery
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		discovery := OIDCDiscovery{Issuer: server.URL}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(discovery)
	}))
	defer server.Close()
	
	auth, err := NewOIDCAuthenticator(server.URL, "test-client", "test-secret")
	require.NoError(t, err)
	
	assert.Equal(t, AuthMethodOIDC, auth.GetMethod())
}

func TestOIDCUserInfo_JSONSerialization(t *testing.T) {
	userInfo := &OIDCUserInfo{
		Sub:               "user-123",
		Name:              "Test User",
		Email:             "test@example.com",
		EmailVerified:     true,
		PreferredUsername: "testuser",
		Groups:            []string{"group1", "group2"},
		Roles:             []string{"admin", "user"},
		Address: &OIDCAddress{
			Formatted:     "123 Main St, City, State 12345",
			StreetAddress: "123 Main St",
			Locality:      "City",
			Region:        "State",
			PostalCode:    "12345",
			Country:       "US",
		},
	}
	
	// Test marshaling
	data, err := json.Marshal(userInfo)
	require.NoError(t, err)
	
	// Test unmarshaling
	var decoded OIDCUserInfo
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	
	assert.Equal(t, userInfo.Sub, decoded.Sub)
	assert.Equal(t, userInfo.Name, decoded.Name)
	assert.Equal(t, userInfo.Email, decoded.Email)
	assert.Equal(t, userInfo.EmailVerified, decoded.EmailVerified)
	assert.Equal(t, userInfo.Groups, decoded.Groups)
	assert.Equal(t, userInfo.Roles, decoded.Roles)
	assert.Equal(t, userInfo.Address.Formatted, decoded.Address.Formatted)
}