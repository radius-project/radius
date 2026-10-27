// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
)

// PolicyActor contains authenticated identity and server-verified provenance.
// Handlers must not deserialize this from an untrusted request body.
type PolicyActor struct {
	Identity      Identity
	Management    api.AssignmentManagement
	RequestID     string
	CorrelationID string
	Recovery      bool
	TakeOwnership bool
}

// PolicyMutation changes exactly one logical resource. IDs, scope, role type,
// creator and ownership are set or checked by the common write path.
type PolicyMutation struct {
	Target     string
	Delete     bool
	Role       *api.RoleDefinitionProperties
	Assignment *api.RoleAssignmentProperties
	Settings   *api.AuthorizationSettingsProperties
}

// SettingsChangeGuard supplies the future bootstrap SubjectAccessReview,
// component enforcement and rollout/history checks. Without it settings changes
// are unavailable; this layer does not implement or bypass those checks.
type SettingsChangeGuard interface {
	ValidateSettingsChange(context.Context, PolicyActor, api.AuthorizationSettingsProperties, api.AuthorizationSettingsProperties) error
}

const settingsID = "/providers/radius.core/authorizationsettings/default"

func permanentAdministrators(policy Policy) int {
	count := 0
	for _, a := range policy.Assignments {
		p := a.Properties
		if p == nil || p.RoleDefinitionID == nil || p.Principal == nil || p.Principal.Type == nil {
			continue
		}
		scope, err := extensionScope(*a.ID, "roleAssignments")
		if err != nil || scope != "/" || !strings.EqualFold(*p.RoleDefinitionID, builtInID("radius-administrator")) {
			continue
		}
		if p.ManagedBy != nil && p.ManagedBy.Kind != nil && *p.ManagedBy.Kind == api.AssignmentManagementKindBootstrap {
			continue
		}
		if *p.Principal.Type == api.PrincipalTypeUser || *p.Principal.Type == api.PrincipalTypeGroup {
			count++
		}
	}
	return count
}

func principalDisplay(p api.Principal) string {
	return string(*p.Type) + ":" + *p.Issuer + ":" + *p.Subject
}

func validateManagement(management api.AssignmentManagement, bootstrap bool) error {
	if management.Kind == nil {
		return fmt.Errorf("verified assignment management kind is required")
	}
	switch *management.Kind {
	case api.AssignmentManagementKindBootstrap:
		if !bootstrap {
			return fmt.Errorf("ManagedByConflict: bootstrap assignments require initialization")
		}
	case api.AssignmentManagementKindGitOps:
		if management.Source == nil || *management.Source == "" {
			return fmt.Errorf("verified GitOps source is required")
		}
	case api.AssignmentManagementKindImperative, api.AssignmentManagementKindDeployment:
	default:
		return fmt.Errorf("unknown assignment management kind")
	}
	return nil
}

func (s *PolicySnapshot) allows(identity Identity, permission, target string) error {
	decisions, err := s.Evaluate(identity, []Check{{permission, target}})
	if err != nil {
		return err
	}
	if !decisions[0].Allowed {
		return fmt.Errorf("AuthorizationFailed: %s at %s", permission, target)
	}
	return nil
}

func (s *PolicySnapshot) accessAdministrator(identity Identity, scope string) bool {
	principal, err := key(&identity.Principal)
	if err != nil {
		return false
	}
	principals := map[principalKey]bool{principal: true}
	for _, group := range identity.Groups {
		k, err := key(&group)
		if err != nil || k.typ != api.PrincipalTypeGroup || k.issuer != principal.issuer {
			return false
		}
		principals[k] = true
	}
	for _, assignment := range s.evaluator.assignments {
		if principals[assignment.principal] && within(assignment.scope, scope) &&
			assignment.role.builtin && slices.Contains([]string{"access-administrator", "radius-administrator"}, assignment.role.name) {
			return true
		}
	}
	return false
}

func (s *PolicySnapshot) mayGrant(identity Identity, definition role, scope string) error {
	if s.accessAdministrator(identity, scope) {
		return nil
	}
	permissions := definition.permissions
	if definition.builtin && definition.name == "application-developer" {
		// Runtime applicationResources are grants too, not just release lists.
		permissions = map[string]bool{}
		for _, permission := range s.evaluator.catalog.Permissions() {
			for _, operation := range []string{"/read", "/write", "/delete", "/connect/action"} {
				if definition.permissions[strings.ToLower(permission)] ||
					strings.HasSuffix(strings.ToLower(permission), operation) &&
						s.evaluator.catalog.application[strings.TrimSuffix(strings.ToLower(permission), operation)] {
					permissions[strings.ToLower(permission)] = true
				}
			}
		}
	}
	for permission := range permissions {
		if err := s.allows(identity, permission, scope); err != nil {
			return fmt.Errorf("PrivilegeEscalationDenied: cannot grant %s at %s: %w", permission, scope, err)
		}
	}
	return nil
}

func checkOwnership(actor PolicyActor, existing *api.RoleAssignmentProperties, deleting bool, current *PolicySnapshot, scope string) error {
	if existing.ManagedBy == nil || existing.ManagedBy.Kind == nil {
		return fmt.Errorf("ManagedByConflict: missing persisted ownership")
	}
	switch *existing.ManagedBy.Kind {
	case api.AssignmentManagementKindBootstrap:
		return fmt.Errorf("ManagedByConflict: bootstrap assignments require installation settings")
	case api.AssignmentManagementKindGitOps:
		sameSource := actor.Management.Kind != nil && *actor.Management.Kind == api.AssignmentManagementKindGitOps &&
			actor.Management.Source != nil && existing.ManagedBy.Source != nil &&
			*actor.Management.Source == *existing.ManagedBy.Source
		if !sameSource && !(deleting && actor.TakeOwnership && current.accessAdministrator(actor.Identity, scope)) {
			return fmt.Errorf("ManagedByConflict: assignment belongs to another GitOps source")
		}
	}
	return nil
}

func applyPolicyMutation(ctx context.Context, current *PolicySnapshot, actor PolicyActor, mutation PolicyMutation, guard SettingsChangeGuard) (Policy, *api.PolicyChange, bool, error) {
	return applyPolicyMutationWithAuthority(ctx, current, current, actor, mutation, guard)
}

func applyPolicyMutationWithAuthority(ctx context.Context, current, authority *PolicySnapshot, actor PolicyActor, mutation PolicyMutation, guard SettingsChangeGuard) (Policy, *api.PolicyChange, bool, error) {
	fail := func(err error) (Policy, *api.PolicyChange, bool, error) { return Policy{}, nil, false, err }
	if _, err := key(&actor.Identity.Principal); err != nil {
		return fail(err)
	}
	if actor.RequestID == "" {
		return fail(fmt.Errorf("request ID is required for access history"))
	}
	if err := validateManagement(actor.Management, false); err != nil {
		return fail(err)
	}
	id, err := canonicalTarget(mutation.Target)
	if err != nil {
		return fail(err)
	}
	if actor.TakeOwnership && !mutation.Delete {
		return fail(fmt.Errorf("ManagedByConflict: ownership override is deletion-only"))
	}
	policy, err := current.Policy()
	if err != nil {
		return fail(err)
	}
	operation := api.PolicyChangeOperationCreate
	change := &api.PolicyChange{Target: to.Ptr(id), ManagedBy: &actor.Management, Operation: &operation}
	inputs := 0
	for _, present := range []bool{mutation.Role != nil, mutation.Assignment != nil, mutation.Settings != nil} {
		if present {
			inputs++
		}
	}
	if (!mutation.Delete && inputs != 1) || (mutation.Delete && inputs != 0) {
		return fail(fmt.Errorf("a mutation must delete or supply exactly one policy body"))
	}
	if mutation.Delete {
		operation = api.PolicyChangeOperationDelete
	}
	highImpact := actor.TakeOwnership
	switch {
	case strings.Contains(id, "/providers/radius.core/roledefinitions/"):
		scope, err := extensionScope(id, "roleDefinitions")
		if err != nil {
			return fail(err)
		}
		if definition, exists := current.roles[id]; exists && definition.builtin {
			return fail(fmt.Errorf("BuiltInRoleImmutable: %s", id))
		}
		permission := "Radius.Core/roleDefinitions/write"
		if mutation.Delete {
			permission = "Radius.Core/roleDefinitions/delete"
		}
		if err := authority.allows(actor.Identity, permission, id); err != nil {
			return fail(err)
		}
		index := slices.IndexFunc(policy.CustomRoles, func(r api.RoleDefinitionResource) bool { return strings.EqualFold(*r.ID, id) })
		if index >= 0 {
			change.Before = &api.PolicySnapshot{RoleDefinition: policy.CustomRoles[index].Properties}
		}
		if mutation.Delete {
			if index < 0 {
				return policy, nil, false, nil
			}
			for _, assignment := range policy.Assignments {
				if strings.EqualFold(*assignment.Properties.RoleDefinitionID, id) {
					return fail(fmt.Errorf("RoleDefinitionInUse: %s", id))
				}
			}
			policy.CustomRoles = slices.Delete(policy.CustomRoles, index, index+1)
		} else {
			if mutation.Role == nil {
				return fail(fmt.Errorf("role definition body is required"))
			}
			definition := api.RoleDefinitionResource{ID: to.Ptr(id), Properties: mutation.Role}
			compiled, err := compileRole(current.evaluator.catalog, definition, false)
			if err != nil {
				return fail(err)
			}
			definition.Properties.RoleType = to.Ptr(api.RoleTypeCustom)
			if len(definition.Properties.AssignableScopes) == 0 {
				definition.Properties.AssignableScopes = to.SliceOfPtrs(scope)
			}
			for _, assignable := range compiled.scopes {
				if err := authority.allows(actor.Identity, "Radius.Core/roleDefinitions/write", assignable); err != nil {
					return fail(err)
				}
				granted := compiled
				if old, exists := current.roles[id]; exists {
					previouslyAssignable := false
					for _, oldScope := range old.scopes {
						previouslyAssignable = previouslyAssignable || within(oldScope, assignable)
					}
					if previouslyAssignable {
						granted.permissions = map[string]bool{}
						for permission := range compiled.permissions {
							if !old.permissions[permission] {
								granted.permissions[permission] = true
							}
						}
					}
				}
				if err := authority.mayGrant(actor.Identity, granted, assignable); err != nil {
					return fail(err)
				}
			}
			change.After = &api.PolicySnapshot{RoleDefinition: definition.Properties}
			if index >= 0 {
				operation = api.PolicyChangeOperationUpdate
				policy.CustomRoles[index] = definition
			} else {
				policy.CustomRoles = append(policy.CustomRoles, definition)
			}
		}
	case strings.Contains(id, "/providers/radius.core/roleassignments/"):
		scope, err := extensionScope(id, "roleAssignments")
		if err != nil {
			return fail(err)
		}
		permission := "Radius.Core/roleAssignments/write"
		if mutation.Delete {
			permission = "Radius.Core/roleAssignments/delete"
		}
		if err := authority.allows(actor.Identity, permission, id); err != nil {
			return fail(err)
		}
		index := slices.IndexFunc(policy.Assignments, func(a api.RoleAssignmentResource) bool { return strings.EqualFold(*a.ID, id) })
		if index >= 0 {
			change.Before = &api.PolicySnapshot{RoleAssignment: policy.Assignments[index].Properties}
			if err := checkOwnership(actor, policy.Assignments[index].Properties, mutation.Delete, authority, scope); err != nil {
				return fail(err)
			}
		}
		if mutation.Delete {
			if index < 0 {
				return policy, nil, false, nil
			}
			policy.Assignments = slices.Delete(policy.Assignments, index, index+1)
		} else {
			if mutation.Assignment == nil || mutation.Assignment.RoleDefinitionID == nil {
				return fail(fmt.Errorf("assignment and role definition ID are required"))
			}
			p := mutation.Assignment
			principal, err := key(p.Principal)
			if err != nil {
				return fail(err)
			}
			roleID, err := canonicalTarget(*p.RoleDefinitionID)
			if err != nil {
				return fail(err)
			}
			if index >= 0 {
				old := policy.Assignments[index].Properties
				oldKey, err := key(old.Principal)
				if err != nil {
					return fail(err)
				}
				if !strings.EqualFold(*old.RoleDefinitionID, roleID) || oldKey != principal {
					return fail(fmt.Errorf("RoleAssignmentImmutable: %s", id))
				}
				if reflect.DeepEqual(old.Description, p.Description) &&
					reflect.DeepEqual(old.Principal.DisplayName, p.Principal.DisplayName) {
					return policy, nil, false, nil
				}
				updated, err := clonePolicyValue(*old)
				if err != nil {
					return fail(err)
				}
				updated.Description = p.Description
				updated.Principal.DisplayName = p.Principal.DisplayName
				policy.Assignments[index].Properties = &updated
				change.After = &api.PolicySnapshot{RoleAssignment: &updated}
				operation = api.PolicyChangeOperationUpdate
				break
			}
			definition, exists := current.roles[roleID]
			if !exists {
				return fail(fmt.Errorf("unknown role definition %s", roleID))
			}
			if err := validateAssignmentScope(definition, scope); err != nil {
				return fail(err)
			}
			if err := authority.mayGrant(actor.Identity, definition, scope); err != nil {
				return fail(err)
			}
			for _, existing := range current.evaluator.assignments {
				if existing.scope == scope && existing.principal == principal && existing.role.id == roleID {
					return fail(fmt.Errorf("RoleAssignmentImmutable: tuple already assigned as %s", existing.id))
				}
			}
			p.RoleDefinitionID, p.Scope = to.Ptr(roleID), to.Ptr(scope)
			p.ManagedBy, p.CreatedBy = &actor.Management, to.Ptr(principalDisplay(actor.Identity.Principal))
			policy.Assignments = append(policy.Assignments, api.RoleAssignmentResource{ID: to.Ptr(id), Properties: p})
			change.After = &api.PolicySnapshot{RoleAssignment: p}
			actorKey, err := key(&actor.Identity.Principal)
			if err != nil {
				return fail(err)
			}
			highImpact = principal == actorKey || definition.builtin &&
				slices.Contains([]string{"access-administrator", "radius-administrator"}, definition.name)
			for _, group := range actor.Identity.Groups {
				groupKey, err := key(&group)
				if err != nil {
					return fail(err)
				}
				highImpact = highImpact || principal == groupKey
			}
		}
	case id == settingsID:
		if mutation.Delete || mutation.Settings == nil {
			return fail(fmt.Errorf("authorization settings cannot be deleted"))
		}
		if *policy.Settings.Properties.Mode == api.UserAuthorizationModeEnforce {
			if err := authority.allows(actor.Identity, "Radius.Core/authorizationSettings/write", id); err != nil {
				return fail(err)
			}
		}
		if guard == nil {
			return fail(fmt.Errorf("AuthorizationUnavailable: settings transition guard is not installed"))
		}
		mutation.Settings.Generation = to.Ptr(policy.Generation)
		if err := guard.ValidateSettingsChange(ctx, actor, *policy.Settings.Properties, *mutation.Settings); err != nil {
			return fail(err)
		}
		operation = api.PolicyChangeOperationUpdate
		change.Before = &api.PolicySnapshot{AuthorizationSettings: policy.Settings.Properties}
		policy.Settings.Properties = mutation.Settings
		if mutation.Settings.Mode != nil && *mutation.Settings.Mode == api.UserAuthorizationModeEnforce {
			policy.Assignments = slices.DeleteFunc(policy.Assignments, func(assignment api.RoleAssignmentResource) bool {
				return assignment.Properties.ManagedBy != nil && assignment.Properties.ManagedBy.Kind != nil &&
					*assignment.Properties.ManagedBy.Kind == api.AssignmentManagementKindBootstrap
			})
		}
		change.After = &api.PolicySnapshot{AuthorizationSettings: mutation.Settings}
		highImpact = true
	default:
		return fail(fmt.Errorf("unsupported policy resource %s", id))
	}
	if reflect.DeepEqual(change.Before, change.After) {
		return policy, nil, false, nil
	}
	if permanentAdministrators(policy) == 0 {
		return fail(fmt.Errorf("LastAdministratorProtected: a permanent installation administrator is required"))
	}
	if len(policy.Assignments) > 10000 || len(policy.CustomRoles) > 1000 {
		return fail(fmt.Errorf("PolicyCapacityExceeded: maximum 10000 assignments and 1000 custom roles"))
	}
	if _, err := NewPolicySnapshot(current.evaluator.catalog, policy); err != nil {
		return fail(err)
	}
	return policy, change, highImpact, nil
}
