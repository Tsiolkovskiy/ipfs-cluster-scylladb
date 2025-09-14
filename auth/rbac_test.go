package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRBACAuthorizer(t *testing.T) {
	authorizer := NewRBACAuthorizer()
	
	// Test default roles exist
	roles := authorizer.ListRoles()
	assert.Greater(t, len(roles), 0)
	
	// Check specific default roles
	_, exists := authorizer.GetRole("admin")
	assert.True(t, exists)
	
	_, exists = authorizer.GetRole("user")
	assert.True(t, exists)
	
	_, exists = authorizer.GetRole("viewer")
	assert.True(t, exists)
}

func TestRBACAuthorizer_AdminAccess(t *testing.T) {
	authorizer := NewRBACAuthorizer()
	
	// Create admin principal
	principal := &Principal{
		ID:       "admin-user",
		Type:     "user",
		TenantID: "default",
		Roles:    []string{"admin"},
	}
	
	// Test admin can access everything
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
	}
	
	err := authorizer.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
	
	// Test admin can access cluster operations
	authCtx.Resource = ResourceCluster
	authCtx.Action = ActionAdmin
	
	err = authorizer.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
}

func TestRBACAuthorizer_UserAccess(t *testing.T) {
	authorizer := NewRBACAuthorizer()
	
	// Create user principal
	principal := &Principal{
		ID:       "regular-user",
		Type:     "user",
		TenantID: "default",
		Roles:    []string{"user"},
	}
	
	// Test user can add pins
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
	}
	
	err := authorizer.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
	
	// Test user can read pins
	authCtx.Action = ActionPinGet
	err = authorizer.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
	
	// Test user cannot access admin functions
	authCtx.Resource = ResourceAdmin
	authCtx.Action = ActionAdmin
	
	err = authorizer.Authorize(context.Background(), authCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient permissions")
}

func TestRBACAuthorizer_ViewerAccess(t *testing.T) {
	authorizer := NewRBACAuthorizer()
	
	// Create viewer principal
	principal := &Principal{
		ID:       "viewer-user",
		Type:     "user",
		TenantID: "default",
		Roles:    []string{"viewer"},
	}
	
	// Test viewer can read pins
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  ResourcePin,
		Action:    ActionPinGet,
	}
	
	err := authorizer.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
	
	// Test viewer cannot add pins
	authCtx.Action = ActionPinAdd
	err = authorizer.Authorize(context.Background(), authCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient permissions")
}

func TestRBACAuthorizer_NoRoles(t *testing.T) {
	authorizer := NewRBACAuthorizer()
	
	// Create principal with no roles
	principal := &Principal{
		ID:       "no-roles-user",
		Type:     "user",
		TenantID: "default",
		Roles:    []string{},
	}
	
	// Test access is denied
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  ResourcePin,
		Action:    ActionPinGet,
	}
	
	err := authorizer.Authorize(context.Background(), authCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient permissions")
}

func TestRBACAuthorizer_CustomRole(t *testing.T) {
	authorizer := NewRBACAuthorizer()
	
	// Add custom role
	customRole := &Role{
		Name:        "custom",
		Description: "Custom role for testing",
		Permissions: []string{"pin.read"},
	}
	authorizer.AddRole(customRole)
	
	// Create principal with custom role
	principal := &Principal{
		ID:       "custom-user",
		Type:     "user",
		TenantID: "default",
		Roles:    []string{"custom"},
	}
	
	// Test custom role can read pins
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  ResourcePin,
		Action:    ActionPinGet,
	}
	
	err := authorizer.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
	
	// Test custom role cannot write pins
	authCtx.Action = ActionPinAdd
	err = authorizer.Authorize(context.Background(), authCtx)
	assert.Error(t, err)
}

func TestRBACAuthorizer_RoleInheritance(t *testing.T) {
	authorizer := NewRBACAuthorizer()
	
	// Test that user role inherits from viewer
	principal := &Principal{
		ID:       "user-with-inheritance",
		Type:     "user",
		TenantID: "default",
		Roles:    []string{"user"},
	}
	
	permissions, err := authorizer.GetPermissions(context.Background(), principal)
	require.NoError(t, err)
	
	// Should have both user and viewer permissions
	hasReadPerm := false
	hasWritePerm := false
	
	for _, perm := range permissions {
		if perm.Resource == ResourcePin {
			for _, action := range perm.Actions {
				if action == ActionPinGet {
					hasReadPerm = true
				}
				if action == ActionPinAdd {
					hasWritePerm = true
				}
			}
		}
	}
	
	assert.True(t, hasReadPerm, "Should have read permission from viewer role")
	assert.True(t, hasWritePerm, "Should have write permission from user role")
}

func TestRBACAuthorizer_WildcardMatching(t *testing.T) {
	authorizer := NewRBACAuthorizer()
	
	// Create admin principal (has wildcard permissions)
	principal := &Principal{
		ID:       "admin-user",
		Type:     "user",
		TenantID: "default",
		Roles:    []string{"admin"},
	}
	
	// Test wildcard resource matching
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  "custom-resource",
		Action:    "custom-action",
	}
	
	err := authorizer.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
}

func TestRBACAuthorizer_RoleManagement(t *testing.T) {
	authorizer := NewRBACAuthorizer()
	
	// Test adding role
	testRole := &Role{
		Name:        "test-role",
		Description: "Test role",
		Permissions: []string{"pin.read"},
	}
	
	authorizer.AddRole(testRole)
	
	// Test getting role
	retrieved, exists := authorizer.GetRole("test-role")
	assert.True(t, exists)
	assert.Equal(t, testRole.Name, retrieved.Name)
	assert.Equal(t, testRole.Description, retrieved.Description)
	
	// Test listing roles
	roles := authorizer.ListRoles()
	found := false
	for _, role := range roles {
		if role.Name == "test-role" {
			found = true
			break
		}
	}
	assert.True(t, found)
	
	// Test removing role
	authorizer.RemoveRole("test-role")
	_, exists = authorizer.GetRole("test-role")
	assert.False(t, exists)
}

func TestRBACAuthorizer_PermissionManagement(t *testing.T) {
	authorizer := NewRBACAuthorizer()
	
	// Test adding permission
	testPerm := &Permission{
		Resource: "test-resource",
		Actions:  []string{"test-action"},
	}
	
	authorizer.AddPermission("test-perm", testPerm)
	
	// Test getting permission
	retrieved, exists := authorizer.GetPermission("test-perm")
	assert.True(t, exists)
	assert.Equal(t, testPerm.Resource, retrieved.Resource)
	assert.Equal(t, testPerm.Actions, retrieved.Actions)
	
	// Test listing permissions
	permissions := authorizer.ListPermissions()
	_, exists = permissions["test-perm"]
	assert.True(t, exists)
	
	// Test removing permission
	authorizer.RemovePermission("test-perm")
	_, exists = authorizer.GetPermission("test-perm")
	assert.False(t, exists)
}

func TestRBACAuthorizer_ContextMatching(t *testing.T) {
	authorizer := NewRBACAuthorizer()
	
	// Add permission with context constraints
	contextPerm := &Permission{
		Resource: ResourcePin,
		Actions:  []string{ActionPinAdd},
		Context: map[string]string{
			"tenant": "test-tenant",
		},
	}
	authorizer.AddPermission("context-perm", contextPerm)
	
	// Add role with context permission
	contextRole := &Role{
		Name:        "context-role",
		Permissions: []string{"context-perm"},
	}
	authorizer.AddRole(contextRole)
	
	principal := &Principal{
		ID:       "context-user",
		Type:     "user",
		TenantID: "test-tenant",
		Roles:    []string{"context-role"},
	}
	
	// Test with matching context
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
		Context: map[string]string{
			"tenant": "test-tenant",
		},
	}
	
	err := authorizer.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
	
	// Test with non-matching context
	authCtx.Context["tenant"] = "other-tenant"
	err = authorizer.Authorize(context.Background(), authCtx)
	assert.Error(t, err)
}