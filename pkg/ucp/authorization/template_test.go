// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func policyBicepTemplate(t *testing.T) map[string]any {
	t.Helper()
	var template map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{
		"languageVersion":"2.0",
		"imports":{"radius":{"provider":"Radius","version":"latest"}},
		"parameters":{"subject":{"type":"string","defaultValue":"bob"}},
		"resources":{
			"reader":{"import":"radius","type":"Radius.Core/roleDefinitions@2025-08-01-preview","properties":{
				"name":"deployed-reader","properties":{"permissions":["Radius.Core/applications/read"]}
			}},
			"grant":{"import":"radius","type":"Radius.Core/roleAssignments@2025-08-01-preview","properties":{
				"name":"[guid(reference('reader').id,parameters('subject'))]",
				"scope":"[format('{0}/providers/Radius.Core/applications/demo',resourceGroup().id)]",
				"properties":{"roleDefinitionId":"[reference('reader').id]","principal":{"type":"user","issuer":"kubernetes:test","subject":"[parameters('subject')]"}}
			},"dependsOn":["reader"]}
		},
		"outputs":{"roleId":{"type":"string","value":"[reference('reader').id]"}}
	}`), &template))
	return template
}

func TestResolvePolicyBicepTemplate(t *testing.T) {
	t.Parallel()
	template := policyBicepTemplate(t)
	resolved, err := ResolveDeploymentTemplate(template, map[string]any{"SUBJECT": map[string]any{"value": "case-sensitive-user"}}, teamA)
	require.NoError(t, err)
	items := resolved["resources"].([]any)
	require.Len(t, items, 2)
	assignment := items[0].(map[string]any)
	id, err := DeploymentResourceID(teamA, assignment)
	require.NoError(t, err)
	require.Contains(t, id, "/applications/demo/providers/radius.core/roleassignments/")
	properties := assignment["properties"].(map[string]any)
	require.Equal(t, strings.ToLower(teamA)+"/providers/radius.core/roledefinitions/deployed-reader", properties["roleDefinitionId"])
	require.Equal(t, "case-sensitive-user", properties["principal"].(map[string]any)["subject"])
	require.Equal(t, []any{properties["roleDefinitionId"]}, assignment["dependsOn"])
	require.Equal(t, properties["roleDefinitionId"], resolved["outputs"].(map[string]any)["roleId"].(map[string]any)["value"])
	original := template["resources"].(map[string]any)["grant"].(map[string]any)["properties"].(map[string]any)
	require.Contains(t, original["name"], "[guid")
}

func TestTemplatePreflightFailsClosed(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name string
		edit func(map[string]any)
		want string
	}{
		{"unknown function", func(m map[string]any) {
			m["variables"] = map[string]any{"x": "[listKeys('external','version')]"}
			m["outputs"] = map[string]any{"x": map[string]any{"value": "[variables('x')]"}}
		}, "not supported"},
		{"external reference", func(m map[string]any) {
			m["outputs"] = map[string]any{"x": map[string]any{"value": "[reference('external').id]"}}
		}, "declared"},
		{"cyclic variables", func(m map[string]any) {
			m["variables"] = map[string]any{"x": "[variables('y')]", "y": "[variables('x')]"}
			m["outputs"] = map[string]any{"x": map[string]any{"value": "[variables('x')]"}}
		}, "cyclic"},
		{"expression suffix", func(m map[string]any) {
			m["outputs"] = map[string]any{"x": map[string]any{"value": "[guid('a') trailing]"}}
		}, "syntax"},
		{"condition", func(m map[string]any) { m["resources"].(map[string]any)["grant"].(map[string]any)["condition"] = true }, "condition"},
		{"cross-group scope", func(m map[string]any) {
			m["resources"].(map[string]any)["grant"].(map[string]any)["properties"].(map[string]any)["scope"] = teamB
		}, "ScopeNotAssignable"},
		{"dependency cycle", func(m map[string]any) {
			m["resources"].(map[string]any)["reader"].(map[string]any)["dependsOn"] = []any{"grant"}
		}, "cyclic"},
		{"unknown template field", func(m map[string]any) { m["expressionEvaluationOptions"] = map[string]any{} }, "unsupported template field"},
		{"parameter wrong type", func(m map[string]any) {
			m["parameters"].(map[string]any)["subject"] = map[string]any{"type": "string", "defaultValue": true}
		}, "does not match"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			template := policyBicepTemplate(t)
			scenario.edit(template)
			_, err := ResolveDeploymentTemplate(template, nil, teamA)
			require.ErrorContains(t, err, scenario.want)
		})
	}
}

func TestTemplatePreflightExpansionLimits(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"string", "array", "depth"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			template := policyBicepTemplate(t)
			variables := map[string]any{"v0": "a"}
			for index := 1; index <= 80; index++ {
				expression := fmt.Sprintf("[variables('v%d')]", index-1)
				switch kind {
				case "string":
					variables[fmt.Sprintf("v%d", index)] = fmt.Sprintf("[concat(variables('v%d'),variables('v%d'))]", index-1, index-1)
				case "array":
					variables[fmt.Sprintf("v%d", index)] = []any{expression, expression}
				case "depth":
					variables[fmt.Sprintf("v%d", index)] = expression
				}
			}
			template["variables"] = variables
			template["outputs"] = map[string]any{"x": map[string]any{"value": "[variables('v80')]"}}
			_, err := ResolveDeploymentTemplate(template, nil, teamA)
			require.ErrorContains(t, err, "limit")
		})
	}
}

func TestTemplateParameterConstraints(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name       string
		typ        string
		value      any
		constraint map[string]any
		want       string
	}{
		{"string at length bounds", "string", "bob", map[string]any{"minLength": 3, "maxLength": 3}, ""},
		{"string below minimum", "string", "ab", map[string]any{"minLength": 3}, "violates minLength"},
		{"string above maximum", "string", "abcd", map[string]any{"maxLength": 3}, "violates maxLength"},
		{"array at length bounds", "array", []any{"a"}, map[string]any{"minLength": 1, "maxLength": 1}, ""},
		{"array below minimum", "array", []any{}, map[string]any{"minLength": 1}, "violates minLength"},
		{"integer at value bounds", "int", float64(-3), map[string]any{"minValue": -3, "maxValue": -3}, ""},
		{"integer below minimum", "int", float64(2), map[string]any{"minValue": 3}, "violates minValue"},
		{"integer above maximum", "int", float64(4), map[string]any{"maxValue": 3}, "violates maxValue"},
		{"malformed bound", "string", "bob", map[string]any{"minLength": "3"}, "must be an integer"},
		{"fractional bound", "string", "bob", map[string]any{"maxLength": 3.5}, "must be an integer"},
		{"negative length", "string", "bob", map[string]any{"minLength": -1}, "must not be negative"},
		{"length on integer", "int", float64(3), map[string]any{"minLength": 1}, "requires a string or array"},
		{"value on string", "string", "bob", map[string]any{"minValue": 1}, "requires an integer"},
		{"allowed value", "string", "bob", map[string]any{"allowedValues": []any{"bob"}}, ""},
		{"disallowed value", "string", "alice", map[string]any{"allowedValues": []any{"bob"}}, "not an allowed value"},
		{"malformed allowed values", "string", "bob", map[string]any{"allowedValues": "bob"}, "must be an array"},
		{"unsupported constraint", "string", "bob", map[string]any{"nullable": true}, "unsupported parameter field"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			for _, supplied := range []bool{false, true} {
				definition := map[string]any{"type": scenario.typ, "defaultValue": scenario.value}
				for key, value := range scenario.constraint {
					definition[key] = value
				}
				template := map[string]any{"resources": []any{}, "parameters": map[string]any{"input": definition}}
				parameters := map[string]any{}
				if supplied {
					delete(definition, "defaultValue")
					parameters["input"] = map[string]any{"value": scenario.value}
				}
				_, err := ResolveDeploymentTemplate(template, parameters, teamA)
				if scenario.want == "" {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, scenario.want)
				}
			}
		})
	}
}

func TestClassicTemplateDependencies(t *testing.T) {
	t.Parallel()
	for _, literal := range []bool{false, true} {
		for _, cyclic := range []bool{false, true} {
			t.Run(fmt.Sprintf("literal=%t/cyclic=%t", literal, cyclic), func(t *testing.T) {
				t.Parallel()
				dependency := func(name string) string {
					if literal {
						return teamA + "/providers/Radius.Core/applications/" + name
					}
					return fmt.Sprintf("[resourceId('Radius.Core/applications','%s')]", name)
				}
				resource := func(name string) map[string]any {
					return map[string]any{"type": "Radius.Core/applications", "apiVersion": "2025-08-01-preview", "name": name, "properties": map[string]any{}}
				}
				a, b, c := resource("a"), resource("b"), resource("c")
				a["dependsOn"] = []any{dependency("b")}
				c["dependsOn"] = []any{dependency("a")}
				if cyclic {
					b["dependsOn"] = []any{dependency("c")}
				}
				resolved, err := ResolveDeploymentTemplate(map[string]any{"resources": []any{a, c, b}}, nil, teamA)
				if cyclic {
					require.ErrorContains(t, err, "cyclic")
					return
				}
				require.NoError(t, err)
				items := resolved["resources"].([]any)
				require.Equal(t, []any{strings.ToLower(teamA) + "/providers/radius.core/applications/b"}, items[0].(map[string]any)["dependsOn"])
				for _, item := range items {
					require.Equal(t, "global", item.(map[string]any)["location"])
				}
			})
		}
	}
}
