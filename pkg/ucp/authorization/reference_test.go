// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"strings"
	"testing"

	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/authorization/internal/cataloggen"
	"github.com/stretchr/testify/require"
)

func TestBuiltInReferences(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	targets := append(cataloggen.ApplicationTypes(), "Radius.Core/environments", "Applications.Core/environments")
	for _, typ := range targets {
		t.Run(typ, func(t *testing.T) {
			body := map[string]any{"properties": map[string]any{
				"environment": envA,
				"application": appA,
				"connections": map[string]any{"database": map[string]any{
					"source": teamB + "/providers/Applications.Datastores/mongoDatabases/shared",
				}},
			}}
			checks, err := catalog.ReferenceChecks("PUT", teamA+"/providers/"+typ+"/test", body, ReferenceOptions{})
			require.NoError(t, err)
			require.ElementsMatch(t, []Check{
				{"Radius.Core/environments/use/action", lowerTarget(envA)},
				{"Radius.Core/applications/write", lowerTarget(appA)},
				{"Applications.Datastores/mongoDatabases/connect/action", lowerTarget(teamB + "/providers/Applications.Datastores/mongoDatabases/shared")},
			}, checks)
			again, err := catalog.ReferenceChecks("PATCH", teamA+"/providers/"+typ+"/test", body, ReferenceOptions{})
			require.NoError(t, err)
			require.Equal(t, checks, again, "unchanged references must still be checked")
			for _, method := range []string{"GET", "DELETE"} {
				checks, err := catalog.ReferenceChecks(method, "not-used", body, ReferenceOptions{})
				require.NoError(t, err)
				require.Empty(t, checks)
			}
		})
	}
}

func lowerTarget(target string) string {
	return strings.ToLower(target)
}

func TestEnvironmentReferences(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	pack := plane + "/providers/Radius.Core/recipePacks/pack"
	body := map[string]any{"properties": map[string]any{
		"recipePacks": []any{pack},
		"providers":   map[string]any{"azure": map[string]any{"scope": "/subscriptions/sub/resourceGroups/rg"}, "aws": map[string]any{}},
	}}
	checks, err := catalog.ReferenceChecks("PUT", envA, body, ReferenceOptions{CloudPlanes: map[string]string{"azure": "/planes/azure/production", "aws": "/planes/aws/local"}})
	require.NoError(t, err)
	require.ElementsMatch(t, []Check{
		{"Radius.Core/recipePacks/use/action", lowerTarget(pack)},
		{"System.Azure/credentials/use/action", "/planes/azure/production"},
		{"System.AWS/credentials/use/action", "/planes/aws/local"},
	}, checks)
	_, err = catalog.ReferenceChecks("PUT", envA, body, ReferenceOptions{})
	require.ErrorContains(t, err, "resolve azure credential plane")
	_, err = catalog.ReferenceChecks("PUT", envA, body, ReferenceOptions{CloudPlanes: map[string]string{"azure": plane}})
	require.ErrorContains(t, err, "invalid azure credential plane")
}

func TestResolvedSchemaReferences(t *testing.T) {
	catalog, err := NewCatalog([]Registration{{Type: "Team.Data/widgets"}})
	require.NoError(t, err)
	descriptor := ReferenceDescriptor{Path: []string{"properties", "databases", "*", "source"}, ResourceType: "Applications.Datastores/mongoDatabases", Required: true}
	source := teamB + "/providers/Applications.Datastores/mongoDatabases/shared"
	for _, collection := range []any{
		[]any{map[string]any{"source": source}, map[string]any{"source": source}},
		map[string]any{"first": map[string]any{"source": source}},
	} {
		body := map[string]any{"properties": map[string]any{"databases": collection}}
		checks, err := catalog.ReferenceChecks("PUT", teamA+"/providers/Team.Data/widgets/test", body, ReferenceOptions{References: []ReferenceDescriptor{descriptor}})
		require.NoError(t, err)
		require.Equal(t, []Check{{"Applications.Datastores/mongoDatabases/read", lowerTarget(source)}}, checks)
	}
	for _, body := range []map[string]any{
		{}, {"properties": map[string]any{}},
		{"properties": map[string]any{"databases": []any{}}},
		{"properties": map[string]any{"databases": []any{map[string]any{"source": nil}}}},
		{"properties": map[string]any{"databases": []any{map[string]any{"source": appA}}}},
	} {
		_, err := catalog.ReferenceChecks("PUT", teamA+"/providers/Team.Data/widgets/test", body, ReferenceOptions{References: []ReferenceDescriptor{descriptor}})
		require.Error(t, err)
	}
	descriptor.Required = false
	checks, err := catalog.ReferenceChecks("PUT", teamA+"/providers/Team.Data/widgets/test", map[string]any{}, ReferenceOptions{References: []ReferenceDescriptor{descriptor}})
	require.NoError(t, err)
	require.Empty(t, checks)
}

func TestReferenceBoundaryDecisions(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	identity := testIdentity()
	evaluator, err := NewEvaluator(catalog, nil, []api.RoleAssignmentResource{testAssignment("developer", builtInID("application-developer"), teamA, identity.Principal)})
	require.NoError(t, err)
	body := map[string]any{"properties": map[string]any{"environment": envA, "application": teamB + "/providers/Radius.Core/applications/other"}}
	checks, err := catalog.ReferenceChecks("PUT", appA, body, ReferenceOptions{})
	require.NoError(t, err)
	results, err := evaluator.Evaluate(identity, append([]Check{{"Radius.Core/applications/write", appA}}, checks...))
	require.NoError(t, err)
	require.True(t, results[0].Allowed)
	for _, result := range results[1:] {
		require.False(t, result.Allowed, result.Permission+" "+result.Target)
	}
}
