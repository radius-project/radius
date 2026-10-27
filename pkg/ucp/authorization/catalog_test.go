// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/authorization/internal/cataloggen"
	"github.com/stretchr/testify/require"
)

func TestCatalogUpToDate(t *testing.T) {
	generated, err := cataloggen.Generate(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	data, err := json.MarshalIndent(generated, "", "  ")
	require.NoError(t, err)
	require.Equal(t, string(append(data, '\n')), string(releaseCatalog), "run make generate-rbac-catalog")
}

func TestCatalogAndReleaseRoles(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	roles := catalog.BuiltInRoles()
	require.Len(t, roles, 9)
	for _, role := range roles {
		t.Run(*role.Name, func(t *testing.T) {
			permissions := []string{}
			for _, permission := range role.Properties.Permissions {
				permissions = append(permissions, *permission)
			}
			require.NoError(t, catalog.ValidatePermissions(permissions))
		})
	}
	for _, permission := range []string{"Radius.Core/*", "Radius.Core/applications/deploy/action", "AWS.S3/Bucket/write", "Microsoft.Compute/virtualMachines/read", "Radius.Core/newType/read"} {
		require.ErrorContains(t, catalog.ValidatePermissions([]string{permission}), "UnknownPermission")
	}
	require.NoError(t, catalog.ValidatePermissions([]string{"Applications.Core/extenders/listSecrets/action", "Radius.Core/applications/getGraph/action"}))
}

func TestRegistrations(t *testing.T) {
	for _, registration := range []Registration{
		{Type: "bad"}, {Type: "AWS.S3/Bucket"}, {Type: "System.Foo/types"},
		{Type: "Radius.Core/environments"}, {Type: "Team.Data/widgets", PermissionSets: []string{"all"}},
		{Type: "Team.Data/widgets", Actions: []string{"*"}},
		{Type: "Team.Data/widgets", Actions: []string{"connect"}}, {Type: "Team.Data/widgets", Actions: []string{"Connect"}},
		{Type: "Team.Data/widgets", Actions: []string{"CONNECT"}}, {Type: "Team.Data/widgets", Actions: []string{"use"}},
		{Type: "Team.Data/widgets", Actions: []string{"write"}}, {Type: "Team.Data/widgets", Actions: []string{"joinPermissionSet"}},
	} {
		_, err := NewCatalog([]Registration{registration})
		require.Error(t, err)
	}

	catalog, err := NewCatalog([]Registration{{Type: "Team.Data/widgets", Actions: []string{"restart"}}})
	require.NoError(t, err)
	require.NoError(t, catalog.ValidatePermissions([]string{"Team.Data/widgets/read", "Team.Data/widgets/restart/action"}))
	_, err = NewCatalog([]Registration{{Type: "Team.Data/widgets"}, {Type: "team.data/Widgets"}})
	require.ErrorContains(t, err, "duplicate registration")
}

func TestHyphenatedRegistrations(t *testing.T) {
	catalog, err := NewCatalog([]Registration{{
		Type: "Team-One.Data/my-widgets", PermissionSets: []string{ApplicationResources},
	}})
	require.NoError(t, err)
	target := teamA + "/providers/Team-One.Data/my-widgets/widget"
	mapping, err := catalog.MapRequest("PUT", target)
	require.NoError(t, err)
	require.Equal(t, []Check{{"Team-One.Data/my-widgets/write", lowerTarget(target)}}, mapping.Checks)
	checks, err := catalog.ReferenceChecks("PUT", appA, map[string]any{
		"properties": map[string]any{"widget": target},
	}, ReferenceOptions{References: []ReferenceDescriptor{{
		Path: []string{"properties", "widget"}, ResourceType: "Team-One.Data/my-widgets", Required: true,
	}}})
	require.NoError(t, err)
	require.Equal(t, []Check{{"Team-One.Data/my-widgets/read", lowerTarget(target)}}, checks)
}

func TestBuiltInRolesAreCopies(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	original := catalog.BuiltInRoles()
	modified := catalog.BuiltInRoles()
	for i := range modified {
		*modified[i].Properties.Permissions[0] = "System.Azure/credentials/use/action"
		*modified[i].Properties.AssignableScopes[0] = "/planes/azure/local"
	}
	require.Equal(t, original, catalog.BuiltInRoles())
}

func TestPermissionSetChecks(t *testing.T) {
	target := "/planes/radius/local/providers/System.Resources/resourceProviders/Team.Data/resourceTypes/widgets"
	for _, tt := range []struct {
		name           string
		before, after  []string
		create, needed bool
	}{
		{"createWithoutSet", nil, nil, true, false},
		{"createEmptySet", nil, []string{}, true, true},
		{"createWithSet", nil, []string{ApplicationResources}, true, true},
		{"add", nil, []string{ApplicationResources}, false, true},
		{"remove", []string{ApplicationResources}, nil, false, true},
		{"unchanged", []string{ApplicationResources}, []string{ApplicationResources}, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checks, err := PermissionSetChecks(target, tt.before, tt.after, tt.create)
			require.NoError(t, err)
			require.Equal(t, tt.needed, len(checks) == 1)
			if tt.needed {
				require.Equal(t, JoinPermissionSet, checks[0].Permission)
			}
		})
	}
	_, err := PermissionSetChecks(target, nil, []string{"unknown"}, true)
	require.Error(t, err)
}
