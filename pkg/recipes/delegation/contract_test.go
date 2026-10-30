// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package delegation

import (
	"encoding/json"
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

const targetID = "/planes/radius/local/resourceGroups/demo/providers/Test.App/containers/cache"

func TestSelectTarget(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, selector, properties string
		valid                      bool
	}{
		{"one", "$.rules[*].destination.resourceId", `{"rules":[{"destination":{"resourceId":"` + targetID + `"}}]}`, true},
		{"deduplicate", "$.ids[*]", `{"ids":["` + targetID + `","/planes/radius/local/resourceGroups/demo/providers/test.app/containers/cache"]}`, true},
		{"zero", "$.ids[*]", `{"ids":[]}`, false},
		{"multiple", "$.ids[*]", `{"ids":["` + targetID + `","` + targetID + `-other"]}`, false},
		{"wrong type", "$.ids[*]", `{"ids":["/planes/radius/local/resourceGroups/demo/providers/Test.App/routes/route"]}`, false},
		{"missing", "$.missing", `{}`, false},
		{"null", "$.id", `{"id":null}`, false},
		{"array required", "$.id[*]", `{"id":"` + targetID + `"}`, false},
		{"filter rejected", "$.rules[?(@.kind)]", `{}`, false},
		{"recursive rejected", "$..id", `{}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var properties map[string]any
			require.NoError(t, json.Unmarshal([]byte(test.properties), &properties))
			id, err := SelectTarget(properties, &datamodel.DelegationTarget{ResourceType: "Test.App/containers", IDsFrom: test.selector})
			if test.valid {
				require.NoError(t, err)
				require.Equal(t, "/planes/radius/local/resourcegroups/demo/providers/test.app/containers/cache", id)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestValidatePack(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		change func(map[string]*datamodel.RecipeDefinition)
	}{
		{"source on delegate", func(d map[string]*datamodel.RecipeDefinition) { d["Test.App/routes"].Source = "not-allowed" }},
		{"parameters on delegate", func(d map[string]*datamodel.RecipeDefinition) {
			d["Test.App/routes"].Parameters = map[string]any{"x": true}
		}},
		{"missing consent", func(d map[string]*datamodel.RecipeDefinition) { d["Test.App/containers"].AcceptsDelegationFrom = nil }},
		{"chain", func(d map[string]*datamodel.RecipeDefinition) { d["Test.App/containers"].Kind = Kind }},
		{"missing target", func(d map[string]*datamodel.RecipeDefinition) { d["Test.App/routes"].Target = nil }},
		{"self", func(d map[string]*datamodel.RecipeDefinition) {
			d["Test.App/routes"].Target.ResourceType = "Test.App/routes"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			definitions := validPack()
			require.NoError(t, ValidatePack(definitions, true))
			test.change(definitions)
			require.Error(t, ValidatePack(definitions, true))
		})
	}
	require.Error(t, ValidatePack(validPack(), false))
	require.NoError(t, ValidatePack(map[string]*datamodel.RecipeDefinition{"Test.App/items": {Kind: "bicep", Source: "existing"}}, false))
}

func validPack() map[string]*datamodel.RecipeDefinition {
	return map[string]*datamodel.RecipeDefinition{
		"Test.App/containers": {Kind: "terraform", Source: "example", AcceptsDelegationFrom: []string{"test.app/routes"}},
		"Test.App/routes": {Kind: Kind, Target: &datamodel.DelegationTarget{
			ResourceType: "Test.App/containers", IDsFrom: "$.rules[*].destination.resourceId",
		}},
	}
}

func TestDesiredProperties(t *testing.T) {
	t.Parallel()
	resourceSchema := map[string]any{"properties": map[string]any{
		"desired":  map[string]any{"type": "string"},
		"computed": map[string]any{"readOnly": true},
		"nested": map[string]any{"properties": map[string]any{
			"writable": map[string]any{"type": "integer"},
			"computed": map[string]any{"readOnly": true},
		}},
	}}
	properties := map[string]any{
		"desired": "keep", "computed": "drop", "status": map[string]any{"value": "drop"},
		"nested": map[string]any{"writable": 4, "computed": "drop"},
	}
	desired, err := DesiredProperties(properties, resourceSchema)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"desired": "keep", "nested": map[string]any{"writable": 4}}, desired)
	require.Contains(t, properties, "computed")
	first, err := Revision(desired)
	require.NoError(t, err)
	second, err := Revision(map[string]any{"nested": map[string]any{"writable": 4}, "desired": "keep"})
	require.NoError(t, err)
	require.Equal(t, first, second)
	_, err = DesiredProperties(map[string]any{"password": "do-not-export"}, map[string]any{"properties": map[string]any{
		"password": map[string]any{"x-radius-sensitive": true},
	}})
	require.Error(t, err)
}

func TestValidateOutputs(t *testing.T) {
	t.Parallel()
	references := []recipes.IncomingReference{{ID: "a", Revision: "r1"}, {ID: "b", Revision: "r2"}}
	for _, test := range []struct {
		name    string
		outputs []recipes.DelegatedOutput
		valid   bool
	}{
		{"exact unordered", []recipes.DelegatedOutput{{ID: "B", Revision: "r2"}, {ID: "a", Revision: "r1"}}, true},
		{"missing", []recipes.DelegatedOutput{{ID: "a", Revision: "r1"}}, false},
		{"foreign", []recipes.DelegatedOutput{{ID: "a", Revision: "r1"}, {ID: "c", Revision: "r2"}}, false},
		{"duplicate", []recipes.DelegatedOutput{{ID: "a", Revision: "r1"}, {ID: "a", Revision: "r1"}}, false},
		{"stale", []recipes.DelegatedOutput{{ID: "a", Revision: "old"}, {ID: "b", Revision: "r2"}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateOutputs(references, test.outputs)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	require.NoError(t, ValidateOutputs(nil, nil))
}
