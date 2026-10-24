// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"slices"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
)

// BuiltInRoles returns fresh API definitions for the eight explicit,
// build-generated release lists and the current-catalog superuser. Runtime
// applicationResources grants are evaluated separately and cannot add actions.
func (c *Catalog) BuiltInRoles() []api.RoleDefinitionResource {
	names := make([]string, 0, len(c.roles))
	for name := range c.roles {
		names = append(names, name)
	}
	names = append(names, "radius-administrator")
	slices.Sort(names)
	result := make([]api.RoleDefinitionResource, 0, len(names))
	for _, name := range names {
		permissions := slices.Clone(c.roles[name])
		if name == "radius-administrator" {
			permissions = c.Permissions()
		}
		words := strings.Split(name, "-")
		for i, word := range words {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
		display := strings.Join(words, " ")
		result = append(result, api.RoleDefinitionResource{
			ID: to.Ptr(builtInID(name)), Name: to.Ptr(name), Type: to.Ptr("Radius.Core/roleDefinitions"),
			Properties: &api.RoleDefinitionProperties{
				DisplayName: to.Ptr(display), Description: to.Ptr(display),
				RoleType: to.Ptr(api.RoleTypeBuiltIn), Permissions: to.SliceOfPtrs(permissions...),
				AssignableScopes: to.SliceOfPtrs("/"),
			},
		})
	}
	return result
}

func builtInID(name string) string {
	return "/providers/radius.core/roledefinitions/" + strings.ToLower(name)
}
