package auth

import (
	"context"
	"fmt"
	"strings"
)

// PolicyEngine interface for different authorization engines
type PolicyEngine interface {
	Authorizer
	GetName() string
}

// PolicyEngineType represents the type of policy engine
type PolicyEngineType string

const (
	PolicyEngineRBAC PolicyEngineType = "rbac"
	PolicyEngineABAC PolicyEngineType = "abac"
)

// MultiTenantAuthorizer wraps another authorizer to provide tenant isolation
type MultiTenantAuthorizer struct {
	underlying      Authorizer
	tenantIsolation bool
}

// NewMultiTenantAuthorizer creates a new multi-tenant authorizer wrapper
func NewMultiTenantAuthorizer(underlying Authorizer, tenantIsolation bool) *MultiTenantAuthorizer {
	return &MultiTenantAuthorizer{
		underlying:      underlying,
		tenantIsolation: tenantIsolation,
	}
}

// Authorize checks authorization with tenant isolation if enabled
func (m *MultiTenantAuthorizer) Authorize(ctx context.Context, authCtx *AuthContext) error {
	// Add tenant isolation checks if enabled
	if m.tenantIsolation && authCtx.Principal != nil {
		// Add tenant context for resource access
		if authCtx.Context == nil {
			authCtx.Context = make(map[string]string)
		}
		authCtx.Context["tenant_id"] = authCtx.Principal.TenantID
		
		// Ensure users can only access resources in their tenant
		if resourceTenant, exists := authCtx.Context["resource_tenant"]; exists {
			if resourceTenant != authCtx.Principal.TenantID && !m.hasAdminRole(authCtx.Principal) {
				return fmt.Errorf("access denied: resource belongs to different tenant")
			}
		}
	}
	
	return m.underlying.Authorize(ctx, authCtx)
}

// GetPermissions delegates to the underlying authorizer
func (m *MultiTenantAuthorizer) GetPermissions(ctx context.Context, principal *Principal) ([]Permission, error) {
	permissions, err := m.underlying.GetPermissions(ctx, principal)
	if err != nil {
		return nil, err
	}
	
	// Add tenant context to permissions if tenant isolation is enabled
	if m.tenantIsolation && principal != nil {
		for i := range permissions {
			if permissions[i].Context == nil {
				permissions[i].Context = make(map[string]string)
			}
			permissions[i].Context["tenant_id"] = principal.TenantID
		}
	}
	
	return permissions, nil
}

// hasAdminRole checks if the principal has admin role
func (m *MultiTenantAuthorizer) hasAdminRole(principal *Principal) bool {
	for _, role := range principal.Roles {
		if role == "admin" || role == "system" {
			return true
		}
	}
	return false
}

// CreatePolicyEngine creates a policy engine based on the specified type
func CreatePolicyEngine(engineType PolicyEngineType, multiTenant bool) (Authorizer, error) {
	var engine Authorizer
	
	switch engineType {
	case PolicyEngineRBAC:
		engine = NewRBACAuthorizer()
	case PolicyEngineABAC:
		engine = NewABACAuthorizer()
	default:
		return nil, fmt.Errorf("unsupported policy engine type: %s", engineType)
	}
	
	// Wrap with multi-tenant support if requested
	if multiTenant {
		engine = NewMultiTenantAuthorizer(engine, true)
	}
	
	return engine, nil
}

// RBACPolicyEngine implements PolicyEngine for RBAC
type RBACPolicyEngine struct {
	*RBACAuthorizer
}

// NewRBACPolicyEngine creates a new RBAC policy engine
func NewRBACPolicyEngine() *RBACPolicyEngine {
	return &RBACPolicyEngine{
		RBACAuthorizer: NewRBACAuthorizer(),
	}
}

// GetName returns the engine name
func (r *RBACPolicyEngine) GetName() string {
	return "RBAC"
}

// ABACPolicyEngine implements PolicyEngine for ABAC
type ABACPolicyEngine struct {
	*ABACAuthorizer
}

// NewABACPolicyEngine creates a new ABAC policy engine
func NewABACPolicyEngine() *ABACPolicyEngine {
	return &ABACPolicyEngine{
		ABACAuthorizer: NewABACAuthorizer(),
	}
}

// GetName returns the engine name
func (a *ABACPolicyEngine) GetName() string {
	return "ABAC"
}

// CompositeAuthorizer combines multiple authorizers with different strategies
type CompositeAuthorizer struct {
	authorizers []Authorizer
	strategy    CompositeStrategy
}

// CompositeStrategy defines how multiple authorizers are combined
type CompositeStrategy string

const (
	StrategyFirstAllow CompositeStrategy = "first_allow" // First authorizer that allows wins
	StrategyAllAllow   CompositeStrategy = "all_allow"   // All authorizers must allow
	StrategyMajority   CompositeStrategy = "majority"    // Majority of authorizers must allow
)

// NewCompositeAuthorizer creates a new composite authorizer
func NewCompositeAuthorizer(authorizers []Authorizer, strategy CompositeStrategy) *CompositeAuthorizer {
	return &CompositeAuthorizer{
		authorizers: authorizers,
		strategy:    strategy,
	}
}

// Authorize evaluates authorization using the composite strategy
func (c *CompositeAuthorizer) Authorize(ctx context.Context, authCtx *AuthContext) error {
	if len(c.authorizers) == 0 {
		return fmt.Errorf("no authorizers configured")
	}
	
	switch c.strategy {
	case StrategyFirstAllow:
		return c.authorizeFirstAllow(ctx, authCtx)
	case StrategyAllAllow:
		return c.authorizeAllAllow(ctx, authCtx)
	case StrategyMajority:
		return c.authorizeMajority(ctx, authCtx)
	default:
		return fmt.Errorf("unsupported composite strategy: %s", c.strategy)
	}
}

// authorizeFirstAllow allows if any authorizer allows
func (c *CompositeAuthorizer) authorizeFirstAllow(ctx context.Context, authCtx *AuthContext) error {
	var lastErr error
	
	for _, authorizer := range c.authorizers {
		err := authorizer.Authorize(ctx, authCtx)
		if err == nil {
			return nil // First allow wins
		}
		lastErr = err
	}
	
	return fmt.Errorf("access denied by all authorizers: %w", lastErr)
}

// authorizeAllAllow requires all authorizers to allow
func (c *CompositeAuthorizer) authorizeAllAllow(ctx context.Context, authCtx *AuthContext) error {
	for _, authorizer := range c.authorizers {
		if err := authorizer.Authorize(ctx, authCtx); err != nil {
			return fmt.Errorf("access denied by authorizer: %w", err)
		}
	}
	
	return nil // All allowed
}

// authorizeMajority requires majority of authorizers to allow
func (c *CompositeAuthorizer) authorizeMajority(ctx context.Context, authCtx *AuthContext) error {
	allowCount := 0
	denyCount := 0
	
	for _, authorizer := range c.authorizers {
		if err := authorizer.Authorize(ctx, authCtx); err == nil {
			allowCount++
		} else {
			denyCount++
		}
	}
	
	if allowCount > denyCount {
		return nil
	}
	
	return fmt.Errorf("access denied: majority of authorizers denied access (%d deny, %d allow)", 
		denyCount, allowCount)
}

// GetPermissions combines permissions from all authorizers
func (c *CompositeAuthorizer) GetPermissions(ctx context.Context, principal *Principal) ([]Permission, error) {
	allPermissions := make([]Permission, 0)
	permissionSet := make(map[string]bool)
	
	for _, authorizer := range c.authorizers {
		permissions, err := authorizer.GetPermissions(ctx, principal)
		if err != nil {
			logger.Warnf("Failed to get permissions from authorizer: %v", err)
			continue
		}
		
		// Deduplicate permissions
		for _, perm := range permissions {
			key := fmt.Sprintf("%s:%s", perm.Resource, strings.Join(perm.Actions, ","))
			if !permissionSet[key] {
				permissionSet[key] = true
				allPermissions = append(allPermissions, perm)
			}
		}
	}
	
	return allPermissions, nil
}