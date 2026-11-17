/*
Copyright 2026 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package recipes

import (
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestDelegationRegistration(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		definition datamodel.RecipeDefinition
		valid      bool
	}{
		{"flat target", datamodel.RecipeDefinition{Kind: "delegated", DelegatesTo: "Radius.Compute/containers"}, true},
		{"missing target", datamodel.RecipeDefinition{Kind: "delegated"}, false},
		{"property path is not a type", datamodel.RecipeDefinition{Kind: "delegated", DelegatesTo: "rules[].destinationContainer.resourceId"}, false},
		{"source forbidden", datamodel.RecipeDefinition{Kind: "delegated", DelegatesTo: "Radius.Compute/containers", Source: "module"}, false},
		{"empty parameters forbidden", datamodel.RecipeDefinition{Kind: "delegated", DelegatesTo: "Radius.Compute/containers", Parameters: map[string]any{}}, false},
		{"self delegation", datamodel.RecipeDefinition{Kind: "delegated", DelegatesTo: "Radius.Compute/routes"}, false},
		{"provisioning cannot delegate", datamodel.RecipeDefinition{Kind: "bicep", Source: "module", DelegatesTo: "Radius.Compute/containers"}, false},
		{"ordinary registration unchanged", datamodel.RecipeDefinition{Kind: "bicep", Source: "module"}, true},
		{"provisioning source still required", datamodel.RecipeDefinition{Kind: "bicep"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRecipeDefinition("Radius.Compute/routes", &tt.definition)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	for _, kind := range []string{TemplateKindBicep, TemplateKindTerraform} {
		accepting := &datamodel.RecipeDefinition{Kind: kind, Source: "module", AcceptsDelegationFrom: []string{"Radius.Compute/routes"}}
		require.NoError(t, ValidateRecipeDefinition("Radius.Compute/containers", accepting))
		accepting.AcceptsDelegationFrom = append(accepting.AcceptsDelegationFrom, "radius.compute/ROUTES")
		require.ErrorContains(t, ValidateRecipeDefinition("Radius.Compute/containers", accepting), "duplicate accepted")
		accepting.AcceptsDelegationFrom = []string{""}
		require.ErrorContains(t, ValidateRecipeDefinition("Radius.Compute/containers", accepting), "invalid acceptsDelegationFrom")
	}
}

func TestSelectDelegationTarget(t *testing.T) {
	t.Parallel()
	const target = "/planes/radius/local/resourceGroups/rg/providers/Radius.Compute/containers/cache"
	reference := map[string]any{"type": "string", schema.ResourceReferenceAnnotation: "Radius.Compute/containers"}
	source := map[string]any{"properties": map[string]any{"targets": map[string]any{"items": reference}}}
	for _, tt := range []struct {
		name  string
		ids   []any
		valid bool
	}{
		{"one target", []any{target}, true},
		{"same target repeated", []any{target, target}, true},
		{"distinct targets", []any{target, target + "-other"}, false},
		{"missing reference", nil, false},
		{"invalid value", []any{"private-input"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, err := SelectDelegationTarget(map[string]any{"targets": tt.ids}, source, "Radius.Compute/containers")
			if tt.valid {
				require.NoError(t, err)
				require.Equal(t, target, id.String())
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "private-input")
				require.True(t, id.IsEmpty())
			}
		})
	}
}

func TestDelegationBindingBoundaries(t *testing.T) {
	t.Parallel()
	const scope = "/planes/radius/local/resourceGroups/rg/providers/"
	source := ResourceMetadata{ResourceID: scope + "Radius.Compute/routes/route", EnvironmentID: "environment", ApplicationID: "application"}
	target := ResourceMetadata{ResourceID: scope + "Radius.Compute/containers/cache", EnvironmentID: source.EnvironmentID, ApplicationID: source.ApplicationID}
	recipe := &EnvironmentDefinition{Driver: TemplateKindBicep, AcceptsDelegationFrom: []string{"Radius.Compute/routes"}}
	require.NoError(t, ValidateDelegationBinding(source, target, recipe, ""))
	require.NoError(t, ValidateDelegationBinding(source, target, recipe, target.ResourceID))
	for _, changed := range []ResourceMetadata{
		{ResourceID: "/planes/radius/other/resourceGroups/rg/providers/Radius.Compute/containers/cache", EnvironmentID: target.EnvironmentID, ApplicationID: target.ApplicationID},
		{ResourceID: "/planes/radius/local/resourceGroups/other/providers/Radius.Compute/containers/cache", EnvironmentID: target.EnvironmentID, ApplicationID: target.ApplicationID},
		{ResourceID: target.ResourceID, EnvironmentID: "other", ApplicationID: target.ApplicationID},
		{ResourceID: target.ResourceID, EnvironmentID: target.EnvironmentID, ApplicationID: "other"},
	} {
		require.ErrorContains(t, ValidateDelegationBinding(source, changed, recipe, ""), "same tenant")
	}
	require.ErrorContains(t, ValidateDelegationBinding(source, source, recipe, ""), "itself")
	require.ErrorContains(t, ValidateDelegationBinding(source, target, recipe, target.ResourceID+"-old"), "delete the delegating resource")
	require.ErrorContains(t, ValidateDelegationBinding(source, target, &EnvironmentDefinition{Driver: "delegated"}, ""), "provisioning recipe")
	require.ErrorContains(t, ValidateDelegationBinding(source, target, &EnvironmentDefinition{Driver: TemplateKindBicep}, ""), "does not accept")
}
