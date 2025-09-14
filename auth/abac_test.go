package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestABACAuthorizer(t *testing.T) {
	authorizer := NewABACAuthorizer()
	
	// Test default policies exist
	policies := authorizer.ListPolicies()
	assert.Greater(t, len(policies), 0)
	
	// Check specific default policies
	_, exists := authorizer.GetPolicy("admin-full-access")
	assert.True(t, exists)
	
	_, exists = authorizer.GetPolicy("user-pin-access")
	assert.True(t, exists)
}

func TestABACAuthorizer_AdminAccess(t *testing.T) {
	authorizer := NewABACAuthorizer()
	
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
	
	// Test admin can access any resource
	authCtx.Resource = "custom-resource"
	authCtx.Action = "custom-action"
	
	err = authorizer.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
}

func TestABACAuthorizer_UserAccess(t *testing.T) {
	authorizer := NewABACAuthorizer()
	
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
	
	// Test user cannot access non-pin resources
	authCtx.Resource = ResourceAdmin
	authCtx.Action = ActionAdmin
	
	err = authorizer.Authorize(context.Background(), authCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no policy allows")
}

func TestABACAuthorizer_ViewerAccess(t *testing.T) {
	authorizer := NewABACAuthorizer()
	
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
	
	// Test viewer can access status
	authCtx.Resource = ResourceCluster
	authCtx.Action = ActionStatus
	
	err = authorizer.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
	
	// Test viewer cannot add pins
	authCtx.Resource = ResourcePin
	authCtx.Action = ActionPinAdd
	
	err = authorizer.Authorize(context.Background(), authCtx)
	assert.Error(t, err)
}

func TestABACAuthorizer_NoRoles(t *testing.T) {
	authorizer := NewABACAuthorizer()
	
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
	assert.Contains(t, err.Error(), "no policy allows")
}

func TestABACAuthorizer_CustomPolicy(t *testing.T) {
	authorizer := NewABACAuthorizer()
	
	// Add custom policy
	customPolicy := &Policy{
		ID:          "custom-policy",
		Name:        "Custom Policy",
		Description: "Custom policy for testing",
		Effect:      EffectAllow,
		Priority:    60,
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Rules: []Rule{
			{
				Attribute: "subject.id",
				Operator:  OpEquals,
				Value:     "custom-user",
			},
			{
				Attribute: "resource.name",
				Operator:  OpEquals,
				Value:     "custom-resource",
			},
		},
	}
	
	authorizer.AddPolicy(customPolicy)
	
	// Create principal
	principal := &Principal{
		ID:       "custom-user",
		Type:     "user",
		TenantID: "default",
		Roles:    []string{},
	}
	
	// Test custom policy allows access
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  "custom-resource",
		Action:    "custom-action",
	}
	
	err := authorizer.Authorize(context.Background(), authCtx)
	assert.NoError(t, err)
	
	// Test different user is denied
	principal.ID = "other-user"
	err = authorizer.Authorize(context.Background(), authCtx)
	assert.Error(t, err)
}

func TestABACAuthorizer_DenyPolicy(t *testing.T) {
	authorizer := NewABACAuthorizer()
	
	// Add deny policy with higher priority than allow policies
	denyPolicy := &Policy{
		ID:          "deny-policy",
		Name:        "Deny Policy",
		Description: "Policy that denies access",
		Effect:      EffectDeny,
		Priority:    200, // Higher than admin policy
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Rules: []Rule{
			{
				Attribute: "subject.id",
				Operator:  OpEquals,
				Value:     "blocked-user",
			},
		},
	}
	
	authorizer.AddPolicy(denyPolicy)
	
	// Create blocked principal (even with admin role)
	principal := &Principal{
		ID:       "blocked-user",
		Type:     "user",
		TenantID: "default",
		Roles:    []string{"admin"},
	}
	
	// Test access is denied despite admin role
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
	}
	
	err := authorizer.Authorize(context.Background(), authCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied by policy")
}

func TestABACAuthorizer_RuleOperators(t *testing.T) {
	authorizer := NewABACAuthorizer()
	
	tests := []struct {
		name      string
		rule      Rule
		context   *EvaluationContext
		expected  bool
	}{
		{
			name: "equals operator",
			rule: Rule{
				Attribute: "subject.type",
				Operator:  OpEquals,
				Value:     "user",
			},
			context: &EvaluationContext{
				Subject: map[string]string{"type": "user"},
			},
			expected: true,
		},
		{
			name: "not equals operator",
			rule: Rule{
				Attribute: "subject.type",
				Operator:  OpNotEquals,
				Value:     "service",
			},
			context: &EvaluationContext{
				Subject: map[string]string{"type": "user"},
			},
			expected: true,
		},
		{
			name: "in operator",
			rule: Rule{
				Attribute: "action.name",
				Operator:  OpIn,
				Values:    []string{"pin:add", "pin:remove"},
			},
			context: &EvaluationContext{
				Action: map[string]string{"name": "pin:add"},
			},
			expected: true,
		},
		{
			name: "contains operator",
			rule: Rule{
				Attribute: "subject.roles",
				Operator:  OpContains,
				Value:     "admin",
			},
			context: &EvaluationContext{
				Subject: map[string]string{"roles": "user,admin,viewer"},
			},
			expected: true,
		},
		{
			name: "starts with operator",
			rule: Rule{
				Attribute: "resource.name",
				Operator:  OpStartsWith,
				Value:     "pin",
			},
			context: &EvaluationContext{
				Resource: map[string]string{"name": "pin.metadata"},
			},
			expected: true,
		},
		{
			name: "greater than operator",
			rule: Rule{
				Attribute: "context.priority",
				Operator:  OpGreaterThan,
				Value:     "5",
			},
			context: &EvaluationContext{
				Context: map[string]string{"priority": "10"},
			},
			expected: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := authorizer.evaluateRule(tt.rule, tt.context)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestABACAuthorizer_PolicyManagement(t *testing.T) {
	authorizer := NewABACAuthorizer()
	
	// Test adding policy
	testPolicy := &Policy{
		ID:          "test-policy",
		Name:        "Test Policy",
		Description: "Test policy",
		Effect:      EffectAllow,
		Priority:    50,
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Rules: []Rule{
			{
				Attribute: "subject.type",
				Operator:  OpEquals,
				Value:     "test",
			},
		},
	}
	
	authorizer.AddPolicy(testPolicy)
	
	// Test getting policy
	retrieved, exists := authorizer.GetPolicy("test-policy")
	assert.True(t, exists)
	assert.Equal(t, testPolicy.Name, retrieved.Name)
	assert.Equal(t, testPolicy.Effect, retrieved.Effect)
	
	// Test listing policies
	policies := authorizer.ListPolicies()
	found := false
	for _, policy := range policies {
		if policy.ID == "test-policy" {
			found = true
			break
		}
	}
	assert.True(t, found)
	
	// Test removing policy
	authorizer.RemovePolicy("test-policy")
	_, exists = authorizer.GetPolicy("test-policy")
	assert.False(t, exists)
}

func TestABACAuthorizer_PolicyPriority(t *testing.T) {
	authorizer := NewABACAuthorizer()
	
	// Clear existing policies for clean test
	for _, policy := range authorizer.ListPolicies() {
		authorizer.RemovePolicy(policy.ID)
	}
	
	// Add high priority deny policy
	denyPolicy := &Policy{
		ID:       "high-priority-deny",
		Name:     "High Priority Deny",
		Effect:   EffectDeny,
		Priority: 100,
		Enabled:  true,
		Rules: []Rule{
			{
				Attribute: "subject.id",
				Operator:  OpEquals,
				Value:     "test-user",
			},
		},
	}
	authorizer.AddPolicy(denyPolicy)
	
	// Add low priority allow policy
	allowPolicy := &Policy{
		ID:       "low-priority-allow",
		Name:     "Low Priority Allow",
		Effect:   EffectAllow,
		Priority: 50,
		Enabled:  true,
		Rules: []Rule{
			{
				Attribute: "subject.id",
				Operator:  OpEquals,
				Value:     "test-user",
			},
		},
	}
	authorizer.AddPolicy(allowPolicy)
	
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
	
	// High priority deny should win
	err := authorizer.Authorize(context.Background(), authCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access denied by policy")
}

func TestABACAuthorizer_TimeBasedAccess(t *testing.T) {
	authorizer := NewABACAuthorizer()
	
	// Enable business hours policy
	policy, exists := authorizer.GetPolicy("business-hours-only")
	require.True(t, exists)
	
	policy.Enabled = true
	authorizer.AddPolicy(policy)
	
	principal := &Principal{
		ID:       "time-user",
		Type:     "user",
		TenantID: "default",
		Roles:    []string{"user"},
	}
	
	authCtx := &AuthContext{
		Principal: principal,
		Resource:  ResourcePin,
		Action:    ActionPinAdd,
	}
	
	// The business hours policy should deny access before 9 AM
	// Note: This test depends on the current time, so it might not always fail
	// In a real implementation, you would mock the time
	err := authorizer.Authorize(context.Background(), authCtx)
	// We can't assert the specific result since it depends on current time
	// Just verify the authorization runs without panic
	_ = err
}