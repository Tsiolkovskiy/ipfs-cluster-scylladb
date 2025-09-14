package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// RBACAuthorizer implements Role-Based Access Control
type RBACAuthorizer struct {
	roles       map[string]*Role
	permissions map[string]*Permission
	mu          sync.RWMutex
}

// Role represents a role with associated permissions
type Role struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"` // Permission IDs
	Inherits    []string `json:"inherits"`    // Other role names this role inherits from
}

// NewRBACAuthorizer creates a new RBAC authorizer with default roles
func NewRBACAuthorizer() *RBACAuthorizer {
	authorizer := &RBACAuthorizer{
		roles:       make(map[string]*Role),
		permissions: make(map[string]*Permission),
	}
	
	// Initialize default permissions and roles
	authorizer.initializeDefaults()
	
	return authorizer
}

// Authorize checks if the principal is authorized to perform the action on the resource
func (r *RBACAuthorizer) Authorize(ctx context.Context, authCtx *AuthContext) error {
	if authCtx.Principal == nil {
		return fmt.Errorf("no principal provided")
	}
	
	// Check if user has required permissions through their roles
	userPermissions, err := r.GetPermissions(ctx, authCtx.Principal)
	if err != nil {
		return fmt.Errorf("failed to get user permissions: %w", err)
	}
	
	// Check if any permission allows the requested action on the resource
	for _, perm := range userPermissions {
		if r.permissionMatches(perm, authCtx.Resource, authCtx.Action, authCtx.Context) {
			return nil // Access granted
		}
	}
	
	return fmt.Errorf("access denied: insufficient permissions for action %s on resource %s", 
		authCtx.Action, authCtx.Resource)
}

// GetPermissions returns all permissions for a principal based on their roles
func (r *RBACAuthorizer) GetPermissions(ctx context.Context, principal *Principal) ([]Permission, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	permissionSet := make(map[string]*Permission)
	
	// Collect permissions from all roles (including inherited roles)
	for _, roleName := range principal.Roles {
		r.collectRolePermissions(roleName, permissionSet, make(map[string]bool))
	}
	
	// Convert to slice
	permissions := make([]Permission, 0, len(permissionSet))
	for _, perm := range permissionSet {
		permissions = append(permissions, *perm)
	}
	
	return permissions, nil
}

// collectRolePermissions recursively collects permissions from a role and its inherited roles
func (r *RBACAuthorizer) collectRolePermissions(roleName string, permissionSet map[string]*Permission, visited map[string]bool) {
	// Prevent infinite recursion
	if visited[roleName] {
		return
	}
	visited[roleName] = true
	
	role, exists := r.roles[roleName]
	if !exists {
		logger.Warnf("Role not found: %s", roleName)
		return
	}
	
	// Add direct permissions
	for _, permID := range role.Permissions {
		if perm, exists := r.permissions[permID]; exists {
			permissionSet[permID] = perm
		}
	}
	
	// Add inherited permissions
	for _, inheritedRole := range role.Inherits {
		r.collectRolePermissions(inheritedRole, permissionSet, visited)
	}
}

// permissionMatches checks if a permission allows the requested action on the resource
func (r *RBACAuthorizer) permissionMatches(perm Permission, resource, action string, context map[string]string) bool {
	// Check resource match
	if !r.resourceMatches(perm.Resource, resource) {
		return false
	}
	
	// Check action match
	if !r.actionMatches(perm.Actions, action) {
		return false
	}
	
	// Check context constraints
	if !r.contextMatches(perm.Context, context) {
		return false
	}
	
	return true
}

// resourceMatches checks if the permission resource matches the requested resource
func (r *RBACAuthorizer) resourceMatches(permResource, requestedResource string) bool {
	// Exact match
	if permResource == requestedResource {
		return true
	}
	
	// Wildcard match
	if permResource == "*" {
		return true
	}
	
	// Prefix match (e.g., "pin.*" matches "pin" and "pin.metadata")
	if strings.HasSuffix(permResource, "*") {
		prefix := strings.TrimSuffix(permResource, "*")
		return strings.HasPrefix(requestedResource, prefix)
	}
	
	return false
}

// actionMatches checks if any of the permission actions matches the requested action
func (r *RBACAuthorizer) actionMatches(permActions []string, requestedAction string) bool {
	for _, action := range permActions {
		// Exact match
		if action == requestedAction {
			return true
		}
		
		// Wildcard match
		if action == "*" {
			return true
		}
		
		// Prefix match (e.g., "pin:*" matches "pin:add", "pin:remove")
		if strings.HasSuffix(action, "*") {
			prefix := strings.TrimSuffix(action, "*")
			if strings.HasPrefix(requestedAction, prefix) {
				return true
			}
		}
	}
	
	return false
}

// contextMatches checks if the permission context constraints are satisfied
func (r *RBACAuthorizer) contextMatches(permContext, requestContext map[string]string) bool {
	// If no context constraints, allow
	if len(permContext) == 0 {
		return true
	}
	
	// All permission context constraints must be satisfied
	for key, expectedValue := range permContext {
		actualValue, exists := requestContext[key]
		if !exists {
			return false
		}
		
		// Support wildcard matching
		if expectedValue == "*" {
			continue
		}
		
		if actualValue != expectedValue {
			return false
		}
	}
	
	return true
}

// AddRole adds or updates a role
func (r *RBACAuthorizer) AddRole(role *Role) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.roles[role.Name] = role
	logger.Infof("Added role: %s", role.Name)
}

// RemoveRole removes a role
func (r *RBACAuthorizer) RemoveRole(roleName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	delete(r.roles, roleName)
	logger.Infof("Removed role: %s", roleName)
}

// GetRole retrieves a role by name
func (r *RBACAuthorizer) GetRole(roleName string) (*Role, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	role, exists := r.roles[roleName]
	if exists {
		// Return a copy to prevent external modification
		roleCopy := *role
		return &roleCopy, true
	}
	
	return nil, false
}

// ListRoles returns all roles
func (r *RBACAuthorizer) ListRoles() []*Role {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	roles := make([]*Role, 0, len(r.roles))
	for _, role := range r.roles {
		roleCopy := *role
		roles = append(roles, &roleCopy)
	}
	
	return roles
}

// AddPermission adds or updates a permission
func (r *RBACAuthorizer) AddPermission(permID string, perm *Permission) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.permissions[permID] = perm
	logger.Infof("Added permission: %s", permID)
}

// RemovePermission removes a permission
func (r *RBACAuthorizer) RemovePermission(permID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	delete(r.permissions, permID)
	logger.Infof("Removed permission: %s", permID)
}

// GetPermission retrieves a permission by ID
func (r *RBACAuthorizer) GetPermission(permID string) (*Permission, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	perm, exists := r.permissions[permID]
	if exists {
		// Return a copy to prevent external modification
		permCopy := *perm
		return &permCopy, true
	}
	
	return nil, false
}

// ListPermissions returns all permissions
func (r *RBACAuthorizer) ListPermissions() map[string]*Permission {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	permissions := make(map[string]*Permission)
	for id, perm := range r.permissions {
		permCopy := *perm
		permissions[id] = &permCopy
	}
	
	return permissions
}

// initializeDefaults sets up default roles and permissions
func (r *RBACAuthorizer) initializeDefaults() {
	// Define default permissions
	permissions := map[string]*Permission{
		"pin.read": {
			Resource: ResourcePin,
			Actions:  []string{ActionPinGet, ActionPinList},
		},
		"pin.write": {
			Resource: ResourcePin,
			Actions:  []string{ActionPinAdd, ActionPinRemove, ActionPinUpdate},
		},
		"pin.admin": {
			Resource: ResourcePin,
			Actions:  []string{"*"},
		},
		"cluster.read": {
			Resource: ResourceCluster,
			Actions:  []string{ActionStatus, ActionHealth},
		},
		"cluster.admin": {
			Resource: ResourceCluster,
			Actions:  []string{"*"},
		},
		"metrics.read": {
			Resource: ResourceMetrics,
			Actions:  []string{ActionMetrics},
		},
		"admin.all": {
			Resource: "*",
			Actions:  []string{"*"},
		},
	}
	
	// Add permissions
	for id, perm := range permissions {
		r.permissions[id] = perm
	}
	
	// Define default roles
	roles := []*Role{
		{
			Name:        "viewer",
			Description: "Read-only access to pins and cluster status",
			Permissions: []string{"pin.read", "cluster.read", "metrics.read"},
		},
		{
			Name:        "user",
			Description: "Standard user with pin management capabilities",
			Permissions: []string{"pin.read", "pin.write", "cluster.read"},
			Inherits:    []string{"viewer"},
		},
		{
			Name:        "admin",
			Description: "Full administrative access",
			Permissions: []string{"admin.all"},
			Inherits:    []string{"user"},
		},
		{
			Name:        "service",
			Description: "Service account with limited pin access",
			Permissions: []string{"pin.read", "pin.write"},
		},
	}
	
	// Add roles
	for _, role := range roles {
		r.roles[role.Name] = role
	}
	
	logger.Info("Initialized default RBAC roles and permissions")
}