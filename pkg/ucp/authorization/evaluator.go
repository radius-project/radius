// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"fmt"
	"slices"
	"strings"

	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
)

// Identity contains an already authenticated principal and verified group
// identities. The evaluator does not trust HTTP headers or resolve membership.
type Identity struct {
	Principal api.Principal
	Groups    []api.Principal
}

// Check is one independent permission requirement on a concrete target.
type Check struct {
	Permission string
	Target     string
}

// Decision is independent of enforcement mode. Grant IDs are sensitive policy
// data: API consumers must redact them unless the caller can read assignments.
type Decision struct {
	Check
	Allowed          bool
	AssignmentID     string
	RoleDefinitionID string
}

type role struct {
	id, name    string
	permissions map[string]bool
	scopes      []string
	builtin     bool
}

type assignment struct {
	id, scope string
	role      role
	principal principalKey
}

type principalKey struct {
	typ             api.PrincipalType
	issuer, subject string
}

// Evaluator is an immutable validated policy snapshot with no I/O or cache.
type Evaluator struct {
	catalog     *Catalog
	assignments []assignment
}

// NewEvaluator validates and copies custom roles and assignments. Built-ins
// always come from this release, never from caller-supplied roleType or names.
// Write safety, ownership and policy generations belong to the policy store.
func NewEvaluator(catalog *Catalog, custom []api.RoleDefinitionResource, assignments []api.RoleAssignmentResource) (*Evaluator, error) {
	if catalog == nil {
		return nil, fmt.Errorf("catalog is required")
	}
	roles := map[string]role{}
	for _, definition := range catalog.BuiltInRoles() {
		compiled, err := compileRole(catalog, definition, true)
		if err != nil {
			return nil, err
		}
		roles[compiled.id] = compiled
	}
	for _, definition := range custom {
		compiled, err := compileRole(catalog, definition, false)
		if err != nil {
			return nil, err
		}
		if _, exists := roles[compiled.id]; exists {
			return nil, fmt.Errorf("duplicate role definition %q", compiled.id)
		}
		roles[compiled.id] = compiled
	}
	evaluator := &Evaluator{catalog: catalog}
	ids := map[string]bool{}
	for _, value := range assignments {
		if value.ID == nil || value.Properties == nil || value.Properties.RoleDefinitionID == nil {
			return nil, fmt.Errorf("assignment ID, properties and roleDefinitionId are required")
		}
		id, err := canonicalTarget(*value.ID)
		if err != nil {
			return nil, err
		}
		if ids[id] {
			return nil, fmt.Errorf("duplicate assignment %q", id)
		}
		ids[id] = true
		scope, err := extensionScope(id, "roleAssignments")
		if err != nil {
			return nil, err
		}
		if value.Properties.Scope != nil && !strings.EqualFold(*value.Properties.Scope, scope) {
			return nil, fmt.Errorf("assignment scope does not match ID %q", id)
		}
		definition, exists := roles[strings.ToLower(*value.Properties.RoleDefinitionID)]
		if !exists {
			return nil, fmt.Errorf("assignment references unknown role %q", *value.Properties.RoleDefinitionID)
		}
		if err := validateAssignmentScope(definition, scope); err != nil {
			return nil, err
		}
		principal, err := key(value.Properties.Principal)
		if err != nil {
			return nil, err
		}
		evaluator.assignments = append(evaluator.assignments, assignment{id, scope, definition, principal})
	}
	// Explanations are deterministic regardless of database enumeration order.
	slices.SortFunc(evaluator.assignments, func(a, b assignment) int { return strings.Compare(a.id, b.id) })
	return evaluator, nil
}

func compileRole(catalog *Catalog, definition api.RoleDefinitionResource, builtin bool) (role, error) {
	if definition.ID == nil || definition.Properties == nil {
		return role{}, fmt.Errorf("role ID and properties are required")
	}
	id, err := canonicalTarget(*definition.ID)
	if err != nil {
		return role{}, err
	}
	scope, err := extensionScope(id, "roleDefinitions")
	if err != nil {
		return role{}, err
	}
	if scope != "/" {
		parsed, err := canonicalTarget(scope)
		if err != nil || strings.Contains(parsed, "/providers/") {
			return role{}, fmt.Errorf("custom roles require installation, plane or resource group scope")
		}
	}
	if !builtin && definition.Properties.RoleType != nil && *definition.Properties.RoleType != api.RoleTypeCustom {
		return role{}, fmt.Errorf("BuiltInRoleImmutable: custom input cannot declare a built-in role")
	}
	name := id[strings.LastIndex(id, "/")+1:]
	if !builtin && scope == "/" && containsFold([]string{
		"radius-administrator", "access-administrator", "platform-administrator", "application-developer",
		"environment-deployer", "recipe-pack-administrator", "resource-type-administrator", "reader", "auditor",
	}, name) {
		return role{}, fmt.Errorf("BuiltInRoleImmutable: %s", id)
	}
	permissions := make([]string, 0, len(definition.Properties.Permissions))
	for _, permission := range definition.Properties.Permissions {
		if permission == nil {
			return role{}, fmt.Errorf("permission cannot be null")
		}
		permissions = append(permissions, *permission)
	}
	if err := catalog.ValidatePermissions(permissions); err != nil {
		return role{}, err
	}
	result := role{id: id, name: name, builtin: builtin, permissions: map[string]bool{}}
	for _, permission := range permissions {
		result.permissions[strings.ToLower(permission)] = true
	}
	for _, allowed := range definition.Properties.AssignableScopes {
		if allowed == nil {
			return role{}, fmt.Errorf("assignable scope cannot be null")
		}
		if err := ValidateScope(*allowed); err != nil {
			return role{}, err
		}
		result.scopes = append(result.scopes, strings.ToLower(*allowed))
	}
	if len(result.scopes) == 0 {
		result.scopes = []string{scope}
	}
	return result, nil
}

func validateAssignmentScope(definition role, scope string) error {
	allowed := false
	for _, candidate := range definition.scopes {
		allowed = allowed || within(candidate, scope)
	}
	if !allowed {
		return fmt.Errorf("ScopeNotAssignable: role %s at %s", definition.id, scope)
	}
	if scope == "/" && definition.builtin && !containsFold([]string{
		"radius-administrator", "access-administrator", "platform-administrator",
		"recipe-pack-administrator", "resource-type-administrator", "reader", "auditor",
	}, definition.name) {
		return fmt.Errorf("ScopeNotAssignable: role %s at installation", definition.id)
	}
	if definition.builtin && definition.name == "resource-type-administrator" &&
		(scope != "/" && (strings.Contains(scope, "/resourcegroups/") || strings.Contains(scope, "/providers/"))) {
		return fmt.Errorf("ScopeNotAssignable: resource-type-administrator requires a plane or installation")
	}
	return nil
}

func key(value *api.Principal) (principalKey, error) {
	if value == nil || value.Type == nil || value.Issuer == nil || value.Subject == nil ||
		*value.Issuer == "" || *value.Subject == "" ||
		!slices.Contains([]api.PrincipalType{api.PrincipalTypeUser, api.PrincipalTypeGroup, api.PrincipalTypeWorkload}, *value.Type) ||
		*value.Subject == "system:anonymous" || *value.Subject == "system:unauthenticated" {
		return principalKey{}, fmt.Errorf("InvalidAuthenticationInfo: invalid or anonymous principal")
	}
	return principalKey{*value.Type, *value.Issuer, *value.Subject}, nil
}

// Evaluate requires every check independently. Missing grants are implicit
// denials; malformed identity, targets, or unknown permissions are explicit errors.
// It does not follow references, which must be supplied as additional checks.
func (e *Evaluator) Evaluate(identity Identity, checks []Check) ([]Decision, error) {
	principal, err := key(&identity.Principal)
	if err != nil {
		return nil, err
	}
	principals := map[principalKey]bool{principal: true}
	if principal.typ == api.PrincipalTypeGroup && len(identity.Groups) != 0 {
		return nil, fmt.Errorf("group inspection cannot include group membership")
	}
	for _, group := range identity.Groups {
		groupKey, err := key(&group)
		if err != nil {
			return nil, err
		}
		if groupKey.typ != api.PrincipalTypeGroup || groupKey.issuer != principal.issuer {
			return nil, fmt.Errorf("verified groups must be groups from the principal's issuer")
		}
		principals[groupKey] = true
	}
	if len(checks) == 0 {
		return nil, fmt.Errorf("at least one access check is required")
	}
	decisions := make([]Decision, 0, len(checks))
	for _, check := range checks {
		if err := e.catalog.ValidatePermissions([]string{check.Permission}); err != nil {
			return nil, err
		}
		target, err := canonicalTarget(check.Target)
		if err != nil {
			return nil, err
		}
		permission := strings.ToLower(check.Permission)
		decision := Decision{Check: check}
		for _, assignment := range e.assignments {
			if !principals[assignment.principal] || !within(assignment.scope, target) {
				continue
			}
			grants := assignment.role.permissions[permission]
			if assignment.role.builtin && assignment.role.name == "radius-administrator" {
				grants = true
			}
			if assignment.role.builtin && assignment.role.name == "application-developer" {
				for _, operation := range []string{"/read", "/write", "/delete", "/connect/action"} {
					if strings.HasSuffix(permission, operation) && e.catalog.application[strings.TrimSuffix(permission, operation)] {
						grants = true
					}
				}
			}
			if grants {
				decision.Allowed = true
				decision.AssignmentID = assignment.id
				decision.RoleDefinitionID = assignment.role.id
				break
			}
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}
