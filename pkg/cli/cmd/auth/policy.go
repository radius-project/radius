// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/google/uuid"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
)

func stringPointers(values []string) []*string {
	result := make([]*string, len(values))
	for i := range values {
		result[i] = &values[i]
	}
	return result
}

func (r *runner) runRole(ctx context.Context, client policyClient) error {
	options := columns("NAME", ".name", "TYPE", ".properties.roleType", "PERMISSIONS", ".properties.permissions", "ASSIGNABLE SCOPES", ".properties.assignableScopes")
	if r.verb == "list" {
		values, err := client.roles(ctx, r.scope)
		if err != nil {
			return err
		}
		return r.write(values, options)
	}
	if r.verb == "show" {
		value, err := client.role(ctx, r.scope, r.name)
		if err != nil {
			return err
		}
		return r.write(value, options)
	}
	var value api.RoleDefinitionResource
	if r.verb == "create" {
		_, err := client.role(ctx, r.scope, r.name)
		var responseError *azcore.ResponseError
		if err == nil {
			return fmt.Errorf("role already exists; use 'rad auth role update'")
		}
		if !errors.As(err, &responseError) || responseError.StatusCode != 404 {
			return err
		}
		value.Properties = &api.RoleDefinitionProperties{Permissions: stringPointers(r.permissions)}
		if len(r.assignableScopes) == 0 {
			r.assignableScopes = []string{r.scope}
		}
	} else {
		var err error
		value, err = client.role(ctx, r.scope, r.name)
		if err != nil {
			return err
		}
		if value.Properties == nil {
			return fmt.Errorf("server returned a role without properties")
		}
		if value.Properties.RoleType != nil && *value.Properties.RoleType == api.RoleTypeBuiltIn {
			return fmt.Errorf("built-in roles cannot be changed or deleted")
		}
	}
	if r.cmd.Flags().Changed("permission") {
		value.Properties.Permissions = stringPointers(r.permissions)
	}
	if len(r.assignableScopes) > 0 {
		value.Properties.AssignableScopes = stringPointers(r.assignableScopes)
	}
	if r.cmd.Flags().Changed("description") {
		value.Properties.Description = &r.description
	}
	if r.cmd.Flags().Changed("display-name") {
		value.Properties.DisplayName = &r.displayName
	}
	if err := r.confirm(fmt.Sprintf("%s role %s at scope %s. Permissions: %s. Assignable scopes: %s. Changes affect every assignment using this role.",
		r.verb, r.name, r.scope, permissionSummary(value.Properties.Permissions), permissionSummary(value.Properties.AssignableScopes))); err != nil {
		return err
	}
	if r.verb == "delete" {
		if err := client.deleteRole(ctx, r.scope, r.name); err != nil {
			return err
		}
		return r.write(struct {
			Deleted string `json:"deleted"`
		}{resourceID(r.scope, "roleDefinitions", r.name)}, columns("DELETED", ".deleted"))
	}
	// Server-owned fields must not be sent back as caller input.
	result, err := client.putRole(ctx, r.scope, r.name, api.RoleDefinitionResource{
		Properties: &api.RoleDefinitionProperties{
			Permissions: value.Properties.Permissions, AssignableScopes: value.Properties.AssignableScopes,
			Description: value.Properties.Description, DisplayName: value.Properties.DisplayName,
		},
	})
	if err != nil {
		return err
	}
	return r.write(result, options)
}

func (r *runner) runAssignment(ctx context.Context, client policyClient) error {
	options := columns("NAME", ".name", "PRINCIPAL", ".properties.principal.subject", "ISSUER", ".properties.principal.issuer",
		"ROLE", ".properties.roleDefinitionId", "SCOPE", ".properties.scope")
	if r.verb == "list" {
		scopes := []string{r.scope}
		if r.inherited {
			scopes = scopeAncestors(r.scope)
		}
		result := []*api.RoleAssignmentResource{}
		for _, scope := range scopes {
			values, err := client.assignments(ctx, scope)
			if err != nil {
				return fmt.Errorf("list assignments at %s: %w", scope, err)
			}
			for _, value := range values {
				if r.subject != nil && (value.Properties == nil || !principalEqual(r.subject, value.Properties.Principal)) {
					continue
				}
				result = append(result, value)
			}
		}
		return r.write(result, options)
	}
	if r.verb == "show" {
		value, err := client.assignment(ctx, r.scope, r.name)
		if err != nil {
			return err
		}
		return r.write(value, options)
	}
	var value api.RoleAssignmentResource
	if r.verb == "create" {
		if r.resolveRole {
			var err error
			r.role, err = client.resolveRole(ctx, r.scope, r.role)
			if err != nil {
				return err
			}
		}
		if r.name == "" {
			r.name = uuid.NewSHA1(uuid.NameSpaceURL, []byte(r.principal+"\n"+r.issuer+"\n"+strings.ToLower(r.role)+"\n"+strings.ToLower(r.scope))).String()
		}
		value.Properties = &api.RoleAssignmentProperties{Principal: r.subject, RoleDefinitionID: &r.role}
	} else {
		var err error
		value, err = client.assignment(ctx, r.scope, r.name)
		if err != nil {
			return err
		}
		if value.Properties == nil || value.Properties.Principal == nil || value.Properties.RoleDefinitionID == nil {
			return fmt.Errorf("server returned an assignment without a principal or role")
		}
	}
	if r.cmd.Flags().Changed("description") {
		value.Properties.Description = &r.description
	}
	if r.cmd.Flags().Changed("display-name") {
		value.Properties.Principal.DisplayName = &r.displayName
	}
	principal := value.Properties.Principal
	if principal == nil || principal.Type == nil || principal.Subject == nil || principal.Issuer == nil {
		return fmt.Errorf("assignment principal is missing type, subject or issuer")
	}
	if err := r.confirm(fmt.Sprintf("%s assignment %s: principal %s:%s, issuer %s, role %s, exact scope %s. Access inherits to descendants.",
		r.verb, r.name, *principal.Type, *principal.Subject, *principal.Issuer,
		*value.Properties.RoleDefinitionID, r.scope)); err != nil {
		return err
	}
	if r.verb == "delete" {
		if err := client.deleteAssignment(ctx, r.scope, r.name); err != nil {
			return err
		}
		return r.write(struct {
			Deleted string `json:"deleted"`
		}{resourceID(r.scope, "roleAssignments", r.name)}, columns("DELETED", ".deleted"))
	}
	result, err := client.putAssignment(ctx, r.scope, r.name, api.RoleAssignmentResource{
		Properties: &api.RoleAssignmentProperties{
			Principal: value.Properties.Principal, RoleDefinitionID: value.Properties.RoleDefinitionID,
			Description: value.Properties.Description,
		},
	})
	if err != nil {
		return err
	}
	return r.write(result, options)
}

func (c policyClient) resolveRole(ctx context.Context, scope, name string) (string, error) {
	matches := []string{}
	for _, ancestor := range scopeAncestors(scope) {
		if strings.Contains(strings.ToLower(ancestor), "/providers/") {
			continue
		}
		role, err := c.role(ctx, ancestor, name)
		var responseError *azcore.ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode == 404 {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("resolve role at %s (use a full role definition ID if ancestor roles are not readable): %w", ancestor, err)
		}
		if role.ID == nil {
			return "", fmt.Errorf("server returned a role without an ID")
		}
		matches = append(matches, *role.ID)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("role %q does not exist at scope %s or its ancestors", name, scope)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("role %q is ambiguous: %s; supply a full role definition ID", name, strings.Join(matches, ", "))
	}
	return matches[0], nil
}

func principalEqual(a, b *api.Principal) bool {
	return a != nil && b != nil && a.Type != nil && b.Type != nil && a.Issuer != nil && b.Issuer != nil &&
		a.Subject != nil && b.Subject != nil && *a.Type == *b.Type && *a.Issuer == *b.Issuer && *a.Subject == *b.Subject
}

func permissionSummary(permissions []*string) string {
	values := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		if permission != nil {
			values = append(values, *permission)
		}
	}
	return strings.Join(values, ", ")
}
