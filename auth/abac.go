package auth

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ABACAuthorizer implements Attribute-Based Access Control
type ABACAuthorizer struct {
	policies map[string]*Policy
	mu       sync.RWMutex
}

// Policy represents an ABAC policy
type Policy struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Effect      Effect      `json:"effect"` // Allow or Deny
	Rules       []Rule      `json:"rules"`
	Priority    int         `json:"priority"` // Higher priority policies are evaluated first
	Enabled     bool        `json:"enabled"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// Effect represents the policy effect
type Effect string

const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

// Rule represents a condition in a policy
type Rule struct {
	Attribute string    `json:"attribute"` // e.g., "subject.role", "resource.type", "action.name"
	Operator  Operator  `json:"operator"`
	Value     string    `json:"value"`
	Values    []string  `json:"values,omitempty"` // For IN operator
}

// Operator represents comparison operators
type Operator string

const (
	OpEquals       Operator = "eq"
	OpNotEquals    Operator = "ne"
	OpIn           Operator = "in"
	OpNotIn        Operator = "not_in"
	OpContains     Operator = "contains"
	OpStartsWith   Operator = "starts_with"
	OpEndsWith     Operator = "ends_with"
	OpGreaterThan  Operator = "gt"
	OpLessThan     Operator = "lt"
	OpGreaterEqual Operator = "gte"
	OpLessEqual    Operator = "lte"
	OpRegex        Operator = "regex"
)

// EvaluationContext contains all attributes for policy evaluation
type EvaluationContext struct {
	Subject  map[string]string `json:"subject"`
	Resource map[string]string `json:"resource"`
	Action   map[string]string `json:"action"`
	Context  map[string]string `json:"context"`
}

// NewABACAuthorizer creates a new ABAC authorizer
func NewABACAuthorizer() *ABACAuthorizer {
	authorizer := &ABACAuthorizer{
		policies: make(map[string]*Policy),
	}
	
	// Initialize default policies
	authorizer.initializeDefaults()
	
	return authorizer
}

// Authorize evaluates policies to determine if access should be granted
func (a *ABACAuthorizer) Authorize(ctx context.Context, authCtx *AuthContext) error {
	if authCtx.Principal == nil {
		return fmt.Errorf("no principal provided")
	}
	
	// Build evaluation context
	evalCtx := a.buildEvaluationContext(authCtx)
	
	// Get applicable policies sorted by priority
	policies := a.getApplicablePolicies()
	
	// Evaluate policies in priority order
	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}
		
		if a.evaluatePolicy(policy, evalCtx) {
			if policy.Effect == EffectDeny {
				return fmt.Errorf("access denied by policy: %s", policy.Name)
			}
			// If effect is Allow, grant access
			return nil
		}
	}
	
	// Default deny if no policy explicitly allows
	return fmt.Errorf("access denied: no policy allows the requested action")
}

// GetPermissions returns permissions based on policy evaluation (simplified for ABAC)
func (a *ABACAuthorizer) GetPermissions(ctx context.Context, principal *Principal) ([]Permission, error) {
	// For ABAC, permissions are dynamic based on context
	// This is a simplified implementation that returns basic permissions
	permissions := []Permission{
		{
			Resource: ResourcePin,
			Actions:  []string{ActionPinGet, ActionPinList},
		},
	}
	
	// Check if user has admin role
	for _, role := range principal.Roles {
		if role == "admin" {
			permissions = append(permissions, Permission{
				Resource: "*",
				Actions:  []string{"*"},
			})
			break
		}
	}
	
	return permissions, nil
}

// buildEvaluationContext creates an evaluation context from the auth context
func (a *ABACAuthorizer) buildEvaluationContext(authCtx *AuthContext) *EvaluationContext {
	evalCtx := &EvaluationContext{
		Subject:  make(map[string]string),
		Resource: make(map[string]string),
		Action:   make(map[string]string),
		Context:  make(map[string]string),
	}
	
	// Subject attributes
	if authCtx.Principal != nil {
		evalCtx.Subject["id"] = authCtx.Principal.ID
		evalCtx.Subject["type"] = authCtx.Principal.Type
		evalCtx.Subject["tenant_id"] = authCtx.Principal.TenantID
		evalCtx.Subject["roles"] = strings.Join(authCtx.Principal.Roles, ",")
		
		// Add custom attributes
		for key, value := range authCtx.Principal.Attrs {
			evalCtx.Subject[key] = value
		}
	}
	
	// Resource attributes
	evalCtx.Resource["name"] = authCtx.Resource
	
	// Action attributes
	evalCtx.Action["name"] = authCtx.Action
	
	// Context attributes
	for key, value := range authCtx.Context {
		evalCtx.Context[key] = value
	}
	
	// Add time-based attributes
	now := time.Now()
	evalCtx.Context["time.hour"] = strconv.Itoa(now.Hour())
	evalCtx.Context["time.day_of_week"] = strconv.Itoa(int(now.Weekday()))
	evalCtx.Context["time.timestamp"] = strconv.FormatInt(now.Unix(), 10)
	
	return evalCtx
}

// getApplicablePolicies returns all policies sorted by priority (highest first)
func (a *ABACAuthorizer) getApplicablePolicies() []*Policy {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	policies := make([]*Policy, 0, len(a.policies))
	for _, policy := range a.policies {
		policies = append(policies, policy)
	}
	
	// Sort by priority (highest first)
	for i := 0; i < len(policies)-1; i++ {
		for j := i + 1; j < len(policies); j++ {
			if policies[i].Priority < policies[j].Priority {
				policies[i], policies[j] = policies[j], policies[i]
			}
		}
	}
	
	return policies
}

// evaluatePolicy evaluates a single policy against the evaluation context
func (a *ABACAuthorizer) evaluatePolicy(policy *Policy, evalCtx *EvaluationContext) bool {
	// All rules must be true for the policy to match
	for _, rule := range policy.Rules {
		if !a.evaluateRule(rule, evalCtx) {
			return false
		}
	}
	
	return true
}

// evaluateRule evaluates a single rule against the evaluation context
func (a *ABACAuthorizer) evaluateRule(rule Rule, evalCtx *EvaluationContext) bool {
	// Get attribute value
	attributeValue := a.getAttributeValue(rule.Attribute, evalCtx)
	
	// Evaluate based on operator
	switch rule.Operator {
	case OpEquals:
		return attributeValue == rule.Value
	case OpNotEquals:
		return attributeValue != rule.Value
	case OpIn:
		for _, value := range rule.Values {
			if attributeValue == value {
				return true
			}
		}
		return false
	case OpNotIn:
		for _, value := range rule.Values {
			if attributeValue == value {
				return false
			}
		}
		return true
	case OpContains:
		return strings.Contains(attributeValue, rule.Value)
	case OpStartsWith:
		return strings.HasPrefix(attributeValue, rule.Value)
	case OpEndsWith:
		return strings.HasSuffix(attributeValue, rule.Value)
	case OpGreaterThan:
		return a.compareNumeric(attributeValue, rule.Value) > 0
	case OpLessThan:
		return a.compareNumeric(attributeValue, rule.Value) < 0
	case OpGreaterEqual:
		return a.compareNumeric(attributeValue, rule.Value) >= 0
	case OpLessEqual:
		return a.compareNumeric(attributeValue, rule.Value) <= 0
	default:
		logger.Warnf("Unknown operator: %s", rule.Operator)
		return false
	}
}

// getAttributeValue retrieves an attribute value from the evaluation context
func (a *ABACAuthorizer) getAttributeValue(attribute string, evalCtx *EvaluationContext) string {
	parts := strings.SplitN(attribute, ".", 2)
	if len(parts) != 2 {
		return ""
	}
	
	category := parts[0]
	key := parts[1]
	
	switch category {
	case "subject":
		return evalCtx.Subject[key]
	case "resource":
		return evalCtx.Resource[key]
	case "action":
		return evalCtx.Action[key]
	case "context":
		return evalCtx.Context[key]
	default:
		return ""
	}
}

// compareNumeric compares two string values as numbers
func (a *ABACAuthorizer) compareNumeric(a1, a2 string) int {
	n1, err1 := strconv.ParseFloat(a1, 64)
	n2, err2 := strconv.ParseFloat(a2, 64)
	
	if err1 != nil || err2 != nil {
		// Fall back to string comparison
		if a1 < a2 {
			return -1
		} else if a1 > a2 {
			return 1
		}
		return 0
	}
	
	if n1 < n2 {
		return -1
	} else if n1 > n2 {
		return 1
	}
	return 0
}

// AddPolicy adds or updates a policy
func (a *ABACAuthorizer) AddPolicy(policy *Policy) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	policy.UpdatedAt = time.Now()
	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = policy.UpdatedAt
	}
	
	a.policies[policy.ID] = policy
	logger.Infof("Added ABAC policy: %s", policy.Name)
}

// RemovePolicy removes a policy
func (a *ABACAuthorizer) RemovePolicy(policyID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	delete(a.policies, policyID)
	logger.Infof("Removed ABAC policy: %s", policyID)
}

// GetPolicy retrieves a policy by ID
func (a *ABACAuthorizer) GetPolicy(policyID string) (*Policy, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	policy, exists := a.policies[policyID]
	if exists {
		// Return a copy to prevent external modification
		policyCopy := *policy
		return &policyCopy, true
	}
	
	return nil, false
}

// ListPolicies returns all policies
func (a *ABACAuthorizer) ListPolicies() []*Policy {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	policies := make([]*Policy, 0, len(a.policies))
	for _, policy := range a.policies {
		policyCopy := *policy
		policies = append(policies, &policyCopy)
	}
	
	return policies
}

// initializeDefaults sets up default ABAC policies
func (a *ABACAuthorizer) initializeDefaults() {
	now := time.Now()
	
	// Default policies
	policies := []*Policy{
		{
			ID:          "admin-full-access",
			Name:        "Admin Full Access",
			Description: "Administrators have full access to all resources",
			Effect:      EffectAllow,
			Priority:    100,
			Enabled:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
			Rules: []Rule{
				{
					Attribute: "subject.roles",
					Operator:  OpContains,
					Value:     "admin",
				},
			},
		},
		{
			ID:          "user-pin-access",
			Name:        "User Pin Access",
			Description: "Users can manage pins in their tenant",
			Effect:      EffectAllow,
			Priority:    50,
			Enabled:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
			Rules: []Rule{
				{
					Attribute: "subject.roles",
					Operator:  OpContains,
					Value:     "user",
				},
				{
					Attribute: "resource.name",
					Operator:  OpEquals,
					Value:     ResourcePin,
				},
				{
					Attribute: "action.name",
					Operator:  OpIn,
					Values:    []string{ActionPinAdd, ActionPinRemove, ActionPinGet, ActionPinList},
				},
			},
		},
		{
			ID:          "viewer-read-access",
			Name:        "Viewer Read Access",
			Description: "Viewers have read-only access",
			Effect:      EffectAllow,
			Priority:    25,
			Enabled:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
			Rules: []Rule{
				{
					Attribute: "subject.roles",
					Operator:  OpContains,
					Value:     "viewer",
				},
				{
					Attribute: "action.name",
					Operator:  OpIn,
					Values:    []string{ActionPinGet, ActionPinList, ActionStatus, ActionHealth, ActionMetrics},
				},
			},
		},
		{
			ID:          "business-hours-only",
			Name:        "Business Hours Only",
			Description: "Restrict access to business hours (9 AM - 5 PM)",
			Effect:      EffectDeny,
			Priority:    75,
			Enabled:     false, // Disabled by default
			CreatedAt:   now,
			UpdatedAt:   now,
			Rules: []Rule{
				{
					Attribute: "time.hour",
					Operator:  OpLessThan,
					Value:     "9",
				},
			},
		},
	}
	
	// Add policies
	for _, policy := range policies {
		a.policies[policy.ID] = policy
	}
	
	logger.Info("Initialized default ABAC policies")
}