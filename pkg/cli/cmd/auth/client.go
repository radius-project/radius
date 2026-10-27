// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
)

func collectPages[T, V any](ctx context.Context, pager *runtime.Pager[T], values func(T) []V) ([]V, error) {
	result := []V{}
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, values(page)...)
	}
	return result, nil
}

type policyClient struct {
	factory *api.ClientFactory
}

func (c policyClient) roles(ctx context.Context, scope string) ([]*api.RoleDefinitionResource, error) {
	if scope == "/" {
		return collectPages(ctx, c.factory.NewInstallationRoleDefinitionsClient().NewListPager(nil),
			func(p api.InstallationRoleDefinitionsClientListResponse) []*api.RoleDefinitionResource {
				return p.Value
			})
	}
	return collectPages(ctx, c.factory.NewScopedRoleDefinitionsClient().NewListPager(scope, nil),
		func(p api.ScopedRoleDefinitionsClientListResponse) []*api.RoleDefinitionResource { return p.Value })
}

func (c policyClient) role(ctx context.Context, scope, name string) (api.RoleDefinitionResource, error) {
	if scope == "/" {
		value, err := c.factory.NewInstallationRoleDefinitionsClient().Get(ctx, name, nil)
		return value.RoleDefinitionResource, err
	}
	value, err := c.factory.NewScopedRoleDefinitionsClient().Get(ctx, scope, name, nil)
	return value.RoleDefinitionResource, err
}

func (c policyClient) putRole(ctx context.Context, scope, name string, value api.RoleDefinitionResource) (api.RoleDefinitionResource, error) {
	if scope == "/" {
		result, err := c.factory.NewInstallationRoleDefinitionsClient().CreateOrUpdate(ctx, name, value, nil)
		return result.RoleDefinitionResource, err
	}
	result, err := c.factory.NewScopedRoleDefinitionsClient().CreateOrUpdate(ctx, scope, name, value, nil)
	return result.RoleDefinitionResource, err
}

func (c policyClient) deleteRole(ctx context.Context, scope, name string) error {
	if scope == "/" {
		_, err := c.factory.NewInstallationRoleDefinitionsClient().Delete(ctx, name, nil)
		return err
	}
	_, err := c.factory.NewScopedRoleDefinitionsClient().Delete(ctx, scope, name, nil)
	return err
}

func (c policyClient) assignments(ctx context.Context, scope string) ([]*api.RoleAssignmentResource, error) {
	if scope == "/" {
		return collectPages(ctx, c.factory.NewInstallationRoleAssignmentsClient().NewListPager(nil),
			func(p api.InstallationRoleAssignmentsClientListResponse) []*api.RoleAssignmentResource {
				return p.Value
			})
	}
	return collectPages(ctx, c.factory.NewScopedRoleAssignmentsClient().NewListPager(scope, nil),
		func(p api.ScopedRoleAssignmentsClientListResponse) []*api.RoleAssignmentResource { return p.Value })
}

func (c policyClient) assignment(ctx context.Context, scope, name string) (api.RoleAssignmentResource, error) {
	if scope == "/" {
		value, err := c.factory.NewInstallationRoleAssignmentsClient().Get(ctx, name, nil)
		return value.RoleAssignmentResource, err
	}
	value, err := c.factory.NewScopedRoleAssignmentsClient().Get(ctx, scope, name, nil)
	return value.RoleAssignmentResource, err
}

func (c policyClient) putAssignment(ctx context.Context, scope, name string, value api.RoleAssignmentResource) (api.RoleAssignmentResource, error) {
	if scope == "/" {
		result, err := c.factory.NewInstallationRoleAssignmentsClient().CreateOrUpdate(ctx, name, value, nil)
		return result.RoleAssignmentResource, err
	}
	result, err := c.factory.NewScopedRoleAssignmentsClient().CreateOrUpdate(ctx, scope, name, value, nil)
	return result.RoleAssignmentResource, err
}

func (c policyClient) deleteAssignment(ctx context.Context, scope, name string) error {
	if scope == "/" {
		_, err := c.factory.NewInstallationRoleAssignmentsClient().Delete(ctx, name, nil)
		return err
	}
	_, err := c.factory.NewScopedRoleAssignmentsClient().Delete(ctx, scope, name, nil)
	return err
}

func scopeAncestors(scope string) []string {
	result := []string{scope}
	if index := strings.LastIndex(strings.ToLower(scope), "/providers/"); index >= 0 {
		scope = scope[:index]
		if scope == "" {
			return []string{"/"}
		}
		result = append(result, scope)
	}
	if index := strings.Index(strings.ToLower(scope), "/resourcegroups/"); index >= 0 {
		scope = scope[:index]
		result = append(result, scope)
	}
	if scope != "/" {
		result = append(result, "/")
	}
	return result
}

func resourceID(scope, kind, name string) string {
	return strings.TrimSuffix(scope, "/") + "/providers/Radius.Core/" + kind + "/" + name
}

func resourceName(value, kind string) (scope, name string, err error) {
	marker := "/providers/radius.core/" + strings.ToLower(kind) + "/"
	index := strings.LastIndex(strings.ToLower(value), marker)
	if index < 0 {
		if err := validateName(value); err != nil {
			return "", "", err
		}
		return "", value, nil
	}
	scope, name = value[:index], value[index+len(marker):]
	if scope == "" {
		scope = "/"
	}
	if err := validateScope(scope); err != nil {
		return "", "", err
	}
	if err := validateName(name); err != nil {
		return "", "", err
	}
	return scope, name, nil
}

func validateName(value string) error {
	if value == "" || value == "." || value == ".." {
		return fmt.Errorf("a nonempty resource name is required")
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || strings.ContainsRune("-_.", character) {
			continue
		}
		return fmt.Errorf("invalid resource name %q: use letters, numbers, '.', '_' or '-'", value)
	}
	return nil
}
