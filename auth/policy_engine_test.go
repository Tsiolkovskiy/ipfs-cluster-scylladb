package auth

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePolicyEngine(t *testing.T) {
	tests := []struct {
		name        string
		engineType  PolicyEngineType
		multiTenant bool
		expectError bool
	}{
		{
			name:        "RBAC engine",
			engineType:  PolicyEngineRBAC,
			multiTenant: false,
			expectError: false,
		},
		{
			name:        "ABAC engine",
			engineType:  PolicyEngineABAC,
			multiTenant: false,
			expectError: false,
		},
		{
			name:        "RBAC with multi-tenant",
			engineType:  PolicyEngineRBAC,
			multiTenant: true,
			expectError: false,
		},
		{
			name:        "invalid engine type",
			engineType:  "invalid",
			multiTenant: false,
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := CreatePolicyEngine(tt.engineType, tt.multiTenant)
			
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, engine)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, engine)
			}
		})
	}
}

func TestRBACPolicyEngine(t *testing.T) {
	engine := NewRBACPolicyEngine()
	
	assert.Equal(t, "RBAC", engine.GetName())
	assert.NotNil(t, engine.RBACAuthorizer)
}

func TestABACPolicyEngine(t *testing.T) {
	engine := NewABACPolicyEngine()
	
	assert.Equal(t, "ABAC", engine.GetName())
	assert.NotNil(t, engine.ABACAuthorizer)
}

func TestMultiTenantAuthorizer(t *testing.T) {
	// Create underlying RBAC authorizer
	rbac := NewRBACAuthorizer()
	
	// Wrap with multi-tenant support
	multiTenant := NewMultiTenantAuthorizer(rbac, true)
	
	// Create principals from different tenants
	tenant1Principal := &Principal{
		ID:       "user1",
		Type:     "user",
		TenantID: "tenant1",
		Roles:    []string{"user"},
	}
	

	
	adminPrincipal := &Principal{
		ID:       "admin",
		Type:     "user",
		TenantID: "tenant1",
		Roles:    []string{"admin"},
	}
	
	// Test same tenant access
	authCtx := &AuthContext{
		Principal: tenant1Principal,
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
		Context: map[string]string{
			"resource_tenant": "tenant1",
		},
	}
	
	err := multiTenant.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
	
	// Test cross-tenant access (should be denied)
	authCtx.Context["resource_tenant"] = "tenant2"
	err = multiTenant.Authorize(context.Background(), authCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "different tenant")
	
	// Test admin cross-tenant access (should be allowed)
	authCtx.Principal = adminPrincipal
	err = multiTenant.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
}

func TestMultiTenantAuthorizer_GetPermissions(t *testing.T) {
	rbac := NewRBACAuthorizer()
	multiTenant := NewMultiTenantAuthorizer(rbac, true)
	
	principal := &Principal{
		ID:       "user1",
		Type:     "user",
		TenantID: "tenant1",
		Roles:    []string{"user"},
	}
	
	permissions, err := multiTenant.GetPermissions(context.Background(), principal)
	require.NoError(t, err)
	
	// Check that tenant context is added to permissions
	for _, perm := range permissions {
		if perm.Context != nil {
			assert.Equal(t, "tenant1", perm.Context["tenant_id"])
		}
	}
}

func TestCompositeAuthorizer_FirstAllow(t *testing.T) {
	// Create two authorizers
	rbac := NewRBACAuthorizer()
	abac := NewABACAuthorizer()
	
	// Create composite with first-allow strategy
	composite := NewCompositeAuthorizer([]Authorizer{rbac, abac}, StrategyFirstAllow)
	
	// Create principal that would be denied by RBAC but allowed by ABAC
	principal := &Principal{
		ID:       "test-user",
		Type:     "user",
		TenantID: "default",
		Roles:    []string{"admin"}, // Admin role exists in both systems
	}
	
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
	}
	
	// Should be allowed since admin role allows access in both systems
	err := composite.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
}

func TestCompositeAuthorizer_AllAllow(t *testing.T) {
	// Create two authorizers
	rbac := NewRBACAuthorizer()
	abac := NewABACAuthorizer()
	
	// Create composite with all-allow strategy
	composite := NewCompositeAuthorizer([]Authorizer{rbac, abac}, StrategyAllAllow)
	
	// Create principal with admin role (should be allowed by both)
	principal := &Principal{
		ID:       "admin-user",
		Type:     "user",
		TenantID: "default",
		Roles:    []string{"admin"},
	}
	
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
	}
	
	// Should be allowed since both authorizers allow admin access
	err := composite.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
	
	// Test with principal that might be denied by one authorizer
	principal.Roles = []string{"unknown-role"}
	err = composite.Authorize(context.Background(), authCtx)
	assert.Error(t, err) // Should be denied since at least one authorizer denies
}

func TestCompositeAuthorizer_Majority(t *testing.T) {
	// Create three mock authorizers for majority testing
	allow1 := &MockAuthorizer{err: nil}
	allow2 := &MockAuthorizer{err: nil}
	deny1 := &MockAuthorizer{err: fmt.Errorf("denied")}
	
	// Create composite with majority strategy (2 allow, 1 deny = allow)
	composite := NewCompositeAuthorizer([]Authorizer{allow1, allow2, deny1}, StrategyMajority)
	
	principal := &Principal{
		ID:       "test-user",
		Type:     "user",
		TenantID: "default",
	}
	
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
	}
	
	// Should be allowed (2 allow > 1 deny)
	err := composite.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
	
	// Test with majority deny (1 allow, 2 deny = deny)
	composite = NewCompositeAuthorizer([]Authorizer{allow1, deny1, &MockAuthorizer{err: fmt.Errorf("denied")}}, StrategyMajority)
	err = composite.Authorize(context.Background(), authCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "majority of authorizers denied")
}

func TestCompositeAuthorizer_GetPermissions(t *testing.T) {
	// Create authorizers with different permissions
	rbac := NewRBACAuthorizer()
	abac := NewABACAuthorizer()
	
	composite := NewCompositeAuthorizer([]Authorizer{rbac, abac}, StrategyFirstAllow)
	
	principal := &Principal{
		ID:       "test-user",
		Type:     "user",
		TenantID: "default",
		Roles:    []string{"user"},
	}
	
	permissions, err := composite.GetPermissions(context.Background(), principal)
	require.NoError(t, err)
	
	// Should have permissions from both authorizers (deduplicated)
	assert.Greater(t, len(permissions), 0)
}

func TestCompositeAuthorizer_NoAuthorizers(t *testing.T) {
	composite := NewCompositeAuthorizer([]Authorizer{}, StrategyFirstAllow)
	
	principal := &Principal{
		ID:       "test-user",
		Type:     "user",
		TenantID: "default",
	}
	
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
	}
	
	err := composite.Authorize(context.Background(), authCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no authorizers configured")
}

func TestCompositeAuthorizer_UnsupportedStrategy(t *testing.T) {
	rbac := NewRBACAuthorizer()
	composite := NewCompositeAuthorizer([]Authorizer{rbac}, "unsupported")
	
	principal := &Principal{
		ID:       "test-user",
		Type:     "user",
		TenantID: "default",
	}
	
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
	}
	
	err := composite.Authorize(context.Background(), authCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported composite strategy")
}