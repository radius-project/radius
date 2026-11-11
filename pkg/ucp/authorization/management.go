// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
)

// policyMutationError distinguishes rejected policy input from storage failures.
// Only the former may be returned to an untrusted management client.
type policyMutationError struct{ error }

func (e *policyMutationError) Unwrap() error { return e.error }

var errManagementNotFound = errors.New("authorization resource not found")

type policyAccessError struct {
	permission, target string
}

func (e *policyAccessError) Error() string {
	return fmt.Sprintf("AuthorizationFailed: %s at %s", e.permission, e.target)
}

type managementTarget struct {
	id, scope, kind, name string
}

func (target managementTarget) permission(operation string) string {
	return "Radius.Core/" + target.kind + "/" + operation
}

func (target managementTarget) authorizationTarget() string {
	if target.name == "" {
		return target.scope
	}
	return target.id
}

func roleResource(resource api.RoleDefinitionResource) api.RoleDefinitionResource {
	resource.Name = to.Ptr((*resource.ID)[strings.LastIndex(*resource.ID, "/")+1:])
	resource.Type = to.Ptr("Radius.Core/roleDefinitions")
	return resource
}

func assignmentResource(resource api.RoleAssignmentResource) api.RoleAssignmentResource {
	resource.Name = to.Ptr((*resource.ID)[strings.LastIndex(*resource.ID, "/")+1:])
	resource.Type = to.Ptr("Radius.Core/roleAssignments")
	return resource
}

func managementResource(snapshot *PolicySnapshot, target managementTarget) (any, error) {
	policy, err := snapshot.Policy()
	if err != nil {
		return nil, err
	}
	if target.kind == "roleDefinitions" {
		for _, resource := range append(snapshot.evaluator.catalog.BuiltInRoles(), policy.CustomRoles...) {
			if strings.EqualFold(*resource.ID, target.id) {
				return roleResource(resource), nil
			}
		}
	} else {
		for _, resource := range policy.Assignments {
			if strings.EqualFold(*resource.ID, target.id) {
				return assignmentResource(resource), nil
			}
		}
	}
	return nil, errManagementNotFound
}

func (r *PolicyRuntime) readManagement(identity Identity, target managementTarget, descendants bool) (any, error) {
	var result any
	err := r.inspectPolicy(func(snapshot *PolicySnapshot) error {
		// Check before looking up the resource, including when it does not exist.
		if err := snapshot.allows(identity, target.permission("read"), target.authorizationTarget()); err != nil {
			return &policyMutationError{err}
		}
		if target.name != "" {
			var err error
			result, err = managementResource(snapshot, target)
			return err
		}
		policy, err := snapshot.Policy()
		if err != nil {
			return err
		}
		if target.kind == "roleDefinitions" {
			values := []*api.RoleDefinitionResource{}
			for _, resource := range append(snapshot.evaluator.catalog.BuiltInRoles(), policy.CustomRoles...) {
				scope, err := extensionScope(*resource.ID, target.kind)
				if err != nil {
					return err
				}
				if scope == target.scope {
					values = append(values, to.Ptr(roleResource(resource)))
				}
			}
			slices.SortFunc(values, func(a, b *api.RoleDefinitionResource) int { return strings.Compare(*a.ID, *b.ID) })
			result = api.AuthorizationListRoleDefinitionResource{Value: values}
			return nil
		}
		values := []*api.RoleAssignmentResource{}
		for _, resource := range policy.Assignments {
			scope := strings.ToLower(*resource.Properties.Scope)
			if scope == target.scope || descendants && within(target.scope, scope) {
				values = append(values, to.Ptr(assignmentResource(resource)))
			}
		}
		slices.SortFunc(values, func(a, b *api.RoleAssignmentResource) int { return strings.Compare(*a.ID, *b.ID) })
		result = api.AuthorizationListRoleAssignmentResource{Value: values}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *PolicyRuntime) authorizeManagement(identity Identity, target managementTarget, operation string) error {
	return r.inspectPolicy(func(snapshot *PolicySnapshot) error {
		if err := snapshot.allows(identity, target.permission(operation), target.authorizationTarget()); err != nil {
			return &policyMutationError{err}
		}
		return nil
	})
}

func validateManagementPrincipal(principal *api.Principal) error {
	if _, err := key(principal); err != nil {
		return err
	}
	for _, field := range []struct {
		name  string
		value *string
		limit int
	}{
		{"issuer", principal.Issuer, maxPrincipalIssuerLength},
		{"subject", principal.Subject, maxPrincipalSubjectLength},
		{"displayName", principal.DisplayName, maxPrincipalDisplayNameLength},
	} {
		if field.value != nil {
			if err := validateAccessString(field.name, *field.value, field.limit); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateManagementScope(scope, kind string) error {
	if err := validateAccessString("scope", scope, maxAccessScopeLength); err != nil {
		return err
	}
	if err := ValidateScope(scope); err != nil {
		return err
	}
	if kind == "accessChanges" && scope != "/" {
		return fmt.Errorf("ScopeNotAssignable: access history requires installation scope")
	}
	if kind == "roleDefinitions" && strings.Contains(scope, "/providers/") {
		return fmt.Errorf("ScopeNotAssignable: role definitions require installation, plane or resource-group scope")
	}
	return nil
}
