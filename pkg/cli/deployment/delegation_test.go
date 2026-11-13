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

package deployment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestPlanDelegatedResourcesPreservesLogicalOutputs(t *testing.T) {
	t.Parallel()
	var template map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{
		"languageVersion":"2.0","contentVersion":"1.0.0.0",
		"resources":{
			"server":{"import":"radius","type":"Radius.Compute/containers@2025-08-01-preview",
				"dependsOn":["application"],"properties":{"name":"[parameters('name')]","properties":{"image":"[parameters('image')]","application":"[reference('application').id]"}}},
			"route":{"import":"radius","type":"Radius.Compute/routes@2025-08-01-preview",
				"dependsOn":["application","server"],"properties":{"name":"route","properties":{"application":"[reference('application').id]","destination":{"resourceId":"[reference('server').id]"},"port":"[parameters('port')]","literal":"reference('server').id"}}},
			"consumer":{"type":"Example.Compute/consumers@2025-08-01-preview","dependsOn":["route"],"properties":{"name":"consumer","properties":{"endpoint":"[reference('route').properties.listener]"}}}
		},
		"outputs":{"endpoint":{"type":"string","value":"[reference('route').properties.listener]"}}
	}`), &template))
	definitions := map[string]*recipes.EnvironmentDefinition{
		"server": {Driver: "bicep", AcceptsDelegationFrom: []string{"Radius.Compute/routes"}},
		"route":  {Driver: "delegated", DelegatesTo: "Radius.Compute/containers"},
	}

	sourceSchema := map[string]any{"properties": map[string]any{"destination": map[string]any{"properties": map[string]any{
		"resourceId": map[string]any{"type": "string", schema.ResourceReferenceAnnotation: "Radius.Compute/containers"},
	}}}}
	plan, err := PlanDelegatedResources(template, "/planes/radius/local/resourceGroups/rg", definitions, map[string]map[string]any{"route": sourceSchema})
	require.NoError(t, err)
	require.Equal(t, template["outputs"], plan["outputs"])
	resources := plan["resources"].(map[string]any)
	require.Equal(t, template["resources"].(map[string]any)["consumer"], resources["consumer"])
	server := resources["server"].(map[string]any)
	require.Equal(t, []string{"application"}, server["dependsOn"])
	require.Contains(t, server["properties"].(map[string]any)["properties"], "__radiusDelegatingResources")
	route := resources["route"].(map[string]any)
	require.Equal(t, true, route["existing"])
	require.Equal(t, []string{"server"}, route["dependsOn"])
	originalRoute := template["resources"].(map[string]any)["route"].(map[string]any)
	require.NotContains(t, originalRoute, "existing", "planning must not mutate the caller's template")
	expression := server["properties"].(map[string]any)["properties"].(map[string]any)["__radiusDelegatingResources"].(string)
	require.Contains(t, expression, "'reference(''server'').id'", "literal text must not be rewritten as a reference")
	require.Contains(t, expression, "parameters('port')")
	require.Contains(t, expression, "format('/planes/radius/local/resourceGroups/rg/providers/Radius.Compute/containers/{0}',parameters('name'))")
	require.Contains(t, expression, "reference('application').id", "computed prerequisites must remain engine-evaluated resource properties")
	require.NotContains(t, plan, "variables", "runtime reference expressions cannot be moved into template variables")
}

func delegationModuleFixture(t *testing.T) (map[string]any, map[string]*recipes.EnvironmentDefinition, map[string]map[string]any) {
	t.Helper()
	var template map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{
			"$schema":"https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
			"languageVersion":"2.0","contentVersion":"1.0.0.0",
			"imports":{"radius":{"provider":"Radius","version":"1.0"}},
			"parameters":{"enabled":{"type":"bool","defaultValue":true},"count":{"type":"int","defaultValue":2}},
			"resources":{
				"server":{"import":"radius","type":"Example.Compute/targets@2025-08-01-preview",
					"properties":{"name":"target","properties":{"image":"native"}}},
				"peer":{"import":"radius","type":"Example.Compute/targets@2025-08-01-preview",
					"properties":{"name":"peer","properties":{"image":"peer"}}},
				"child":{"type":"Microsoft.Resources/deployments","apiVersion":"2020-10-01","name":"child",
					"dependsOn":["server"],
					"properties":{"mode":"Incremental","expressionEvaluationOptions":{"scope":"inner"},
						"parameters":{"targetId":{"value":"[reference('server').id]"},"enabled":{"value":"[parameters('enabled')]"},"count":{"value":"[parameters('count')]"}},
						"template":{
							"$schema":"https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
							"languageVersion":"2.0","contentVersion":"1.0.0.0",
							"imports":{"radius":{"provider":"Radius","version":"1.0"}},
							"parameters":{"targetId":{"type":"string"},"enabled":{"type":"bool"},"count":{"type":"int"}},
							"variables":{"ports":[6379,6380],"destination":"[parameters('targetId')]"},
							"resources":{
								"external":{"import":"radius","type":"Example.Compute/consumers@2025-08-01-preview","properties":{"name":"external","properties":{"image":"external"}}},
								"route":{"import":"radius","type":"Example.Compute/routes@2025-08-01-preview","dependsOn":["external"],
									"condition":"[parameters('enabled')]",
									"properties":{"name":"child-route","properties":{"destination":"[variables('destination')]","port":"[variables('ports')[0]]","computed":"[reference('external').properties.endpoint]",
										"settings":"[toObject(variables('ports'),lambda('p',string(lambdaVariables('p'))),lambda('p',lambdaVariables('p')))]"}}}
							},
							"outputs":{"endpoint":{"type":"string","value":"[if(parameters('enabled'),reference('route').properties.endpoint,'absent')]"}}
						}
					}
				},
				"consumer":{"import":"radius","type":"Example.Compute/consumers@2025-08-01-preview","dependsOn":["child"],
					"properties":{"name":"consumer","properties":{"endpoint":"[reference('child').outputs.endpoint.value]"}}}
			},
			"outputs":{"endpoint":{"type":"string","value":"[reference('child').outputs.endpoint.value]"}}
		}`), &template))
	definitions := map[string]*recipes.EnvironmentDefinition{
		"server":      {Driver: "bicep", AcceptsDelegationFrom: []string{"Example.Compute/routes"}},
		"peer":        {Driver: "bicep", AcceptsDelegationFrom: []string{"Example.Compute/routes"}},
		"child/route": {Driver: "delegated", DelegatesTo: "Example.Compute/targets"},
	}
	schemas := map[string]map[string]any{"child/route": {"properties": map[string]any{
		"destination": map[string]any{"type": "string", schema.ResourceReferenceAnnotation: "Example.Compute/targets"},
	}}}
	return template, definitions, schemas
}

func TestPlanDelegatedResourcesModules(t *testing.T) {
	for _, scenario := range []string{"parent-target", "loop-members", "whole-child", "existing-child-target", "conditional-module", "property-copy", "mapped-relationships", "object-map-relationships", "output-only-parameter"} {
		t.Run(scenario, func(t *testing.T) {
			template, definitions, schemas := delegationModuleFixture(t)
			resources := template["resources"].(map[string]any)
			module := resources["child"].(map[string]any)
			child := delegationModuleTemplate(module)
			childResources := child["resources"].(map[string]any)
			route := childResources["route"].(map[string]any)
			if scenario == "output-only-parameter" {
				module["properties"].(map[string]any)["parameters"].(map[string]any)["ownerEndpoint"] =
					map[string]any{"value": "[reference('server').properties.endpoint]"}
				child["parameters"].(map[string]any)["ownerEndpoint"] = map[string]any{"type": "string"}
				child["variables"].(map[string]any)["publishedOwnerEndpoint"] = "[parameters('ownerEndpoint')]"
				child["outputs"].(map[string]any)["ownerEndpoint"] = map[string]any{"type": "string", "value": "[variables('publishedOwnerEndpoint')]"}
				template["outputs"].(map[string]any)["ownerEndpoint"] = map[string]any{"type": "string", "value": "[reference('child').outputs.ownerEndpoint.value]"}
			}
			if scenario == "property-copy" || scenario == "mapped-relationships" || scenario == "object-map-relationships" {
				properties := route["properties"].(map[string]any)["properties"].(map[string]any)
				delete(properties, "destination")
				schemas["child/route"] = map[string]any{"properties": map[string]any{"destinations": map[string]any{
					"type": "array", "items": map[string]any{"properties": map[string]any{"id": map[string]any{
						"type": "string", schema.ResourceReferenceAnnotation: "Example.Compute/targets",
					}}},
				}}}
				if scenario == "property-copy" {
					properties["copy"] = []any{map[string]any{
						"name": "destinations", "count": "[length(variables('ports'))]",
						"input": map[string]any{"id": "[variables('destination')]", "port": "[variables('ports')[copyIndex('destinations')]]"},
					}}
				} else if scenario == "mapped-relationships" {
					properties["destinations"] = "[map(variables('ports'),lambda('p',createObject('id',variables('destination'),'port',lambdaVariables('p'))))]"
				} else {
					schemas["child/route"] = map[string]any{"properties": map[string]any{"destinations": map[string]any{
						"type": "object", "additionalProperties": map[string]any{"properties": map[string]any{"id": map[string]any{
							"type": "string", schema.ResourceReferenceAnnotation: "Example.Compute/targets",
						}}},
					}}}
					properties["destinations"] = "[toObject(variables('ports'),lambda('p',string(lambdaVariables('p'))),lambda('p',createObject('id',variables('destination'),'port',lambdaVariables('p'))))]"
				}
			}
			if scenario == "loop-members" {
				route["copy"] = map[string]any{"name": "route", "count": "[parameters('count')]"}
				route["condition"] = "[and(parameters('enabled'),less(copyIndex(),2))]"
				body := route["properties"].(map[string]any)
				body["name"] = "[format('child-route{0}',copyIndex())]"
				body["properties"].(map[string]any)["port"] = "[variables('ports')[copyIndex()]]"
				child["outputs"].(map[string]any)["endpoint"].(map[string]any)["value"] =
					"[if(parameters('enabled'),reference('route[1]').properties.endpoint,'absent')]"
			}
			if scenario == "whole-child" {
				childResources["server"] = resources["server"]
				delete(resources, "server")
				definitions["child/server"] = definitions["server"]
				delete(definitions, "server")
				route["properties"].(map[string]any)["properties"].(map[string]any)["destination"] = "[reference('server').id]"
				module["dependsOn"] = []any{}
				delete(module["properties"].(map[string]any)["parameters"].(map[string]any), "targetId")
				delete(child["parameters"].(map[string]any), "targetId")
				delete(child["variables"].(map[string]any), "destination")
			}
			if scenario == "existing-child-target" {
				childResources["localTarget"] = map[string]any{
					"import": "radius", "type": "Example.Compute/targets@2025-08-01-preview",
					"existing": true, "properties": map[string]any{"name": "target"},
				}
				route["dependsOn"] = []any{"external", "localTarget"}
				route["properties"].(map[string]any)["properties"].(map[string]any)["destination"] = "[reference('localTarget').id]"
			}
			if scenario == "conditional-module" {
				module["condition"] = "[parameters('enabled')]"
				template["outputs"].(map[string]any)["endpoint"].(map[string]any)["value"] =
					"[if(parameters('enabled'),reference('child','2020-10-01').outputs.endpoint.value,'absent')]"
				resources["consumer"].(map[string]any)["properties"].(map[string]any)["properties"].(map[string]any)["endpoint"] =
					"[if(parameters('enabled'),reference('child','2020-10-01').outputs.endpoint.value,'absent')]"
				resources["consumer"].(map[string]any)["dependsOn"] = []any{}
			}
			planned, err := PlanDelegatedResources(template, "/planes/radius/local/resourceGroups/fixture", definitions, schemas)
			require.NoError(t, err)
			require.Equal(t, template["outputs"], planned["outputs"])
			plannedResources := planned["resources"].(map[string]any)
			require.Equal(t, resources["consumer"], plannedResources["consumer"])
			plannedChild := delegationModuleTemplate(plannedResources["child"].(map[string]any))
			require.Equal(t, child["outputs"], plannedChild["outputs"])
			if scenario != "whole-child" {
				input := delegationModuleTemplate(plannedResources["__radiusInputs_child"].(map[string]any))
				for key, value := range input["variables"].(map[string]any) {
					require.Equal(t, child["variables"].(map[string]any)[key], value)
				}
				require.Equal(t, child["variables"].(map[string]any)["ports"], input["variables"].(map[string]any)["ports"])
				if scenario == "output-only-parameter" {
					require.NotContains(t, input["parameters"], "ownerEndpoint")
					require.NotContains(t, input["variables"], "publishedOwnerEndpoint")
					require.Contains(t, plannedChild["parameters"], "ownerEndpoint")
				}
				require.NotContains(t, input["resources"], "route")
				require.Contains(t, input["resources"], "external")
				require.NotContains(t, input["outputs"], "endpoint")
				if scenario == "conditional-module" {
					require.Empty(t, plannedResources["server"].(map[string]any)["dependsOn"])
				} else {
					require.Equal(t, []string{"__radiusInputs_child"}, plannedResources["server"].(map[string]any)["dependsOn"])
				}
				require.Equal(t, true, plannedChild["resources"].(map[string]any)["external"].(map[string]any)["existing"])
				require.NotContains(t, plannedResources["peer"].(map[string]any)["dependsOn"], "__radiusInputs_child")
			} else {
				require.NotContains(t, plannedResources, "__radiusInputs_child")
			}
			writeDelegationEvaluatorFixture(t, scenario, planned)
		})
	}
}

func writeDelegationEvaluatorFixture(t *testing.T, name string, template map[string]any) {
	t.Helper()
	directory := os.Getenv("RADIUS_DELEGATION_FIXTURES")
	if directory == "" {
		return
	}
	require.False(t, filepath.IsAbs(directory))
	require.NotEqual(t, "..", filepath.Clean(directory))
	require.False(t, strings.HasPrefix(filepath.Clean(directory), ".."+string(filepath.Separator)))
	require.NoError(t, os.MkdirAll(directory, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "evaluate.py"), []byte(delegationEvaluatorFixture), 0600))
	encoded, err := json.MarshalIndent(template, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, name+".json"), encoded, 0600))
}

func TestPlanDelegatedResourcesModuleRejectsComputedTargetPrerequisite(t *testing.T) {
	template, definitions, schemas := delegationModuleFixture(t)
	module := template["resources"].(map[string]any)["child"].(map[string]any)
	module["properties"].(map[string]any)["parameters"].(map[string]any)["computed"] =
		map[string]any{"value": "[reference('server').properties.endpoint]"}
	delegationModuleTemplate(module)["resources"].(map[string]any)["route"].(map[string]any)["properties"].(map[string]any)["properties"].(map[string]any)["computed"] =
		"[parameters('computed')]"
	_, err := PlanDelegatedResources(template, "/planes/radius/local/resourceGroups/fixture", definitions, schemas)
	require.ErrorContains(t, err, "computed target output")
}

func TestPlanDelegatedResourcesPreservesOrdinaryModules(t *testing.T) {
	template, _, _ := delegationModuleFixture(t)
	planned, err := PlanDelegatedResources(template, "/planes/radius/local/resourceGroups/fixture", nil, nil)
	require.NoError(t, err)
	require.Equal(t, template, planned)
}

func TestPlanDelegatedResourcesModuleFailsClosed(t *testing.T) {
	for _, scenario := range []string{"computed-local-target", "publication-prerequisite", "foreign-scope", "delegate-prerequisite", "cyclic-parent-prerequisite"} {
		t.Run(scenario, func(t *testing.T) {
			template, definitions, schemas := delegationModuleFixture(t)
			resources := template["resources"].(map[string]any)
			module := resources["child"].(map[string]any)
			child := delegationModuleTemplate(module)
			childResources := child["resources"].(map[string]any)
			expected := ""
			switch scenario {
			case "computed-local-target":
				childResources["localTarget"] = map[string]any{
					"import": "radius", "type": "Example.Compute/targets@2025-08-01-preview",
					"existing": true, "properties": map[string]any{"name": "target"},
				}
				childResources["route"].(map[string]any)["properties"].(map[string]any)["properties"].(map[string]any)["computed"] =
					"[reference('localTarget').properties.endpoint]"
				expected = "computed target output"
			case "publication-prerequisite":
				resources["server"].(map[string]any)["dependsOn"] = []any{"child"}
				expected = "publication module"
			case "foreign-scope":
				module["properties"].(map[string]any)["parameters"].(map[string]any)["targetId"] =
					map[string]any{"value": "/planes/radius/local/resourceGroups/other/providers/Example.Compute/targets/target"}
				expected = "cannot identify"
			case "delegate-prerequisite":
				definitions["child/external"] = &recipes.EnvironmentDefinition{Driver: "bicep", AcceptsDelegationFrom: []string{"Example.Compute/routes"}}
				expected = "requires its own delegation group"
			case "cyclic-parent-prerequisite":
				resources["prerequisite"] = map[string]any{
					"import": "radius", "type": "Example.Compute/consumers@2025-08-01-preview",
					"dependsOn": []any{"server"}, "properties": map[string]any{"name": "prerequisite"},
				}
				module["dependsOn"] = []any{"server", "prerequisite"}
				expected = "cyclic desired-input prerequisite"
			}
			_, err := PlanDelegatedResources(template, "/planes/radius/local/resourceGroups/fixture", definitions, schemas)
			require.ErrorContains(t, err, expected)
			require.NotContains(t, resources, "__radiusInputs_child", "failure must not mutate the submitted template")
		})
	}
}

func TestPlanDelegatedResourcesPreservesExistingParentTargetUpdate(t *testing.T) {
	template, definitions, schemas := delegationModuleFixture(t)
	resources := template["resources"].(map[string]any)
	resources["server"].(map[string]any)["existing"] = true
	delete(definitions, "peer")
	planned, err := PlanDelegatedResources(template, "/planes/radius/local/resourceGroups/fixture", definitions, schemas)
	require.NoError(t, err)
	require.Equal(t, template, planned)
}

func TestPlanDelegatedResourcesIndexedTargetLoops(t *testing.T) {
	for _, scenario := range []string{"indexed-targets", "indexed-targets-offset", "indexed-targets-child", "indexed-targets-mapping", "indexed-targets-conditional"} {
		t.Run(scenario, func(t *testing.T) {
			template, definitions, schemas := delegationModuleFixture(t)
			resources := template["resources"].(map[string]any)
			child := delegationModuleTemplate(resources["child"].(map[string]any))
			childResources := child["resources"].(map[string]any)
			route := childResources["route"].(map[string]any)
			external := childResources["external"].(map[string]any)
			server := resources["server"].(map[string]any)
			server["copy"] = map[string]any{"name": "server", "count": "[parameters('targetCount')]"}
			server["properties"].(map[string]any)["name"] = "[format('target{0}',copyIndex())]"
			server["properties"].(map[string]any)["properties"].(map[string]any)["image"] = "[format('image{0}',copyIndex())]"
			route["copy"] = map[string]any{"name": "route", "count": "[parameters('sourceCount')]"}
			route["condition"] = "[and(parameters('enabled'),not(equals(copyIndex(),3)))]"
			route["dependsOn"] = []any{"[format('server[{0}]',mod(copyIndex(),parameters('targetCount')))]", "external"}
			routeBody := route["properties"].(map[string]any)
			routeBody["name"] = "[format('route{0}',copyIndex())]"
			properties := routeBody["properties"].(map[string]any)
			properties["destination"] = "[reference(format('server[{0}]',mod(copyIndex(),parameters('targetCount')))).id]"
			properties["port"] = "[add(6379,copyIndex())]"
			properties["computed"] = "[reference('external').properties.endpoint]"
			delete(properties, "settings")
			properties["literal"] = "reference(format('server[{0}]',copyIndex())).id"
			properties["expressionLiteral"] = "[concat('reference(format(''server[{0}]'',copyIndex())).id','-literal')]"
			if scenario == "indexed-targets-offset" {
				server["properties"].(map[string]any)["name"] = "[format('target{0}',copyIndex('server',5))]"
				properties["destination"] = "[reference(format('server[{0}]',mod(copyIndex(1),parameters('targetCount')))).id]"
			}

			definitions["route"] = definitions["child/route"]
			delete(definitions, "child/route")
			schemas["route"] = schemas["child/route"]
			delete(schemas, "child/route")
			resources["route"] = route
			resources["external"] = external
			delete(resources, "child")
			delete(resources, "consumer")
			template["variables"] = child["variables"]
			delete(template["variables"].(map[string]any), "destination")
			parameters := template["parameters"].(map[string]any)
			parameters["targetCount"] = map[string]any{"type": "int", "defaultValue": 2}
			parameters["sourceCount"] = map[string]any{"type": "int", "defaultValue": 4}
			if scenario == "indexed-targets-mapping" {
				parameters["targetMap"] = map[string]any{"type": "array", "defaultValue": []any{1, 0, 1, 0}}
				properties["destination"] = "[reference(format('server[{0}]',parameters('targetMap')[copyIndex()])).id]"
			}
			template["outputs"] = map[string]any{"endpoint": map[string]any{
				"type": "string", "value": "[if(and(parameters('enabled'),greater(parameters('sourceCount'),1)),reference('route[1]').properties.endpoint,'absent')]",
			}}
			fixed, err := cloneDelegationObject(route)
			require.NoError(t, err)
			delete(fixed, "copy")
			fixed["condition"] = "[parameters('enabled')]"
			fixed["dependsOn"] = []any{"server[1]", "external"}
			fixed["properties"] = map[string]any{"name": "fixed-route", "properties": map[string]any{
				"destination": "[reference('server[1]').id]", "computed": "[reference('external').properties.endpoint]", "port": 7000,
			}}
			resources["fixed"] = fixed
			definitions["fixed"] = definitions["route"]
			schemas["fixed"] = schemas["route"]
			if scenario == "indexed-targets-conditional" {
				server["condition"] = "[less(copyIndex(),1)]"
				route["condition"] = "[and(parameters('enabled'),not(equals(copyIndex(),3)),less(mod(copyIndex(),parameters('targetCount')),1))]"
				fixed["condition"] = false
				template["outputs"].(map[string]any)["endpoint"].(map[string]any)["value"] =
					"[if(parameters('enabled'),reference('route[0]').properties.endpoint,'absent')]"
			}
			if scenario == "indexed-targets-child" {
				template, err = cloneDelegationObject(template)
				require.NoError(t, err)
				inner := template
				template = map[string]any{
					"$schema": inner["$schema"], "languageVersion": "2.0", "contentVersion": "1.0.0.0", "parameters": parameters,
					"resources": map[string]any{"child": map[string]any{
						"type": "Microsoft.Resources/deployments", "apiVersion": "2020-10-01", "name": "loop-child",
						"properties": map[string]any{"mode": "Incremental", "expressionEvaluationOptions": map[string]any{"scope": "inner"},
							"parameters": map[string]any{
								"enabled": map[string]any{"value": "[parameters('enabled')]"}, "count": map[string]any{"value": "[parameters('count')]"},
								"targetCount": map[string]any{"value": "[parameters('targetCount')]"}, "sourceCount": map[string]any{"value": "[parameters('sourceCount')]"},
							}, "template": inner},
					}},
					"outputs": map[string]any{"endpoint": map[string]any{"type": "string", "value": "[reference('child').outputs.endpoint.value]"}},
				}
				childDefinitions := map[string]*recipes.EnvironmentDefinition{}
				childSchemas := map[string]map[string]any{}
				for symbol, definition := range definitions {
					childDefinitions["child/"+symbol] = definition
				}
				for symbol, schema := range schemas {
					childSchemas["child/"+symbol] = schema
				}
				definitions, schemas = childDefinitions, childSchemas
			}
			planned, err := PlanDelegatedResources(template, "/planes/radius/local/resourceGroups/fixture", definitions, schemas)
			require.NoError(t, err)
			require.Equal(t, template["outputs"], planned["outputs"])
			plannedResources := planned["resources"].(map[string]any)
			if scenario == "indexed-targets-child" {
				plannedResources = delegationModuleTemplate(plannedResources["child"].(map[string]any))["resources"].(map[string]any)
			}
			plannedServer := plannedResources["server"].(map[string]any)
			require.Equal(t, server["copy"], plannedServer["copy"])
			require.Equal(t, []string{"external"}, plannedServer["dependsOn"])
			require.Equal(t, []string{"server"}, plannedResources["route"].(map[string]any)["dependsOn"])
			expression := plannedServer["properties"].(map[string]any)["properties"].(map[string]any)["__radiusDelegatingResources"].(string)
			require.Contains(t, expression, "copyIndex('server')")
			require.Contains(t, expression, "reference('external').properties.endpoint")
			require.Contains(t, expression, "'reference(format(''server[{0}]'',copyIndex())).id'")
			writeDelegationEvaluatorFixture(t, scenario+"-original", template)
			writeDelegationEvaluatorFixture(t, scenario+"-metadata", map[string]any{
				"scope": "/planes/radius/local/resourceGroups/fixture", "definitions": definitions, "schemas": schemas,
			})
			writeDelegationEvaluatorFixture(t, scenario, planned)
		})
	}
}

func TestReplaceIdentityReferenceDoesNotRewriteExpressionLiterals(t *testing.T) {
	expression := "[createObject('literal','reference(''server'').id','identity',reference('server').id,'computed',reference('server').properties.endpoint)]"
	rewritten := replaceIdentityReference(expression, "server", "/target").(string)
	require.Equal(t, "[createObject('literal','reference(''server'').id','identity','/target','computed',reference('server').properties.endpoint)]", rewritten)
}
func TestPlanDelegatedResourcesDoesNotChangeOrdinaryTemplate(t *testing.T) {
	t.Parallel()
	template := map[string]any{"resources": map[string]any{"ordinary": map[string]any{
		"type": "Radius.Compute/containers@2025-08-01-preview", "properties": map[string]any{"name": "ordinary"},
	}}}
	planned, err := PlanDelegatedResources(template, "/planes/radius/local/resourceGroups/rg",
		map[string]*recipes.EnvironmentDefinition{"ordinary": {Driver: "bicep"}}, nil)
	require.NoError(t, err)
	require.Equal(t, template, planned)
	require.NotContains(t, planned, "variables")
}
