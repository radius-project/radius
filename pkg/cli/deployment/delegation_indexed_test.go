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
	"testing"

	"github.com/radius-project/radius/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestPlanDelegatedResourcesIndexedTargetsFailClosed(t *testing.T) {
	for _, scenario := range []string{"computed-prerequisite", "computed-target-selection", "multiple-instances", "property-lambda"} {
		t.Run(scenario, func(t *testing.T) {
			template, definitions, schemas := delegationModuleFixture(t)
			resources := template["resources"].(map[string]any)
			route := delegationModuleTemplate(resources["child"].(map[string]any))["resources"].(map[string]any)["route"].(map[string]any)
			delete(resources, "child")
			delete(resources, "consumer")
			server := resources["server"].(map[string]any)
			server["copy"] = map[string]any{"name": "server", "count": 2}
			server["properties"].(map[string]any)["name"] = "[format('target{0}',copyIndex())]"
			route["copy"] = map[string]any{"name": "route", "count": 2}
			route["properties"].(map[string]any)["name"] = "[format('route{0}',copyIndex())]"
			properties := route["properties"].(map[string]any)["properties"].(map[string]any)
			properties["destination"] = "[reference(format('server[{0}]',copyIndex())).id]"
			resources["route"] = route
			definitions["route"] = definitions["child/route"]
			schemas["route"] = schemas["child/route"]
			expected := ""
			switch scenario {
			case "computed-prerequisite":
				properties["computed"] = "[reference(format('external[{0}]',copyIndex())).properties.endpoint]"
				expected = "computed indexed resource reference"
			case "computed-target-selection":
				properties["destination"] = "[reference(format('server[{0}]',reference('external').properties.index)).id]"
				expected = "cannot depend on computed resource outputs"
			case "multiple-instances":
				properties["other"] = "[reference('server[1]').id]"
				schemas["route"]["properties"].(map[string]any)["other"] = map[string]any{
					"type": "string", schema.ResourceReferenceAnnotation: "Example.Compute/targets",
				}
				expected = "one distinct target instance expression"
			case "property-lambda":
				properties["destination"] = "[reference(format('server[{0}]',lambdaVariables('p'))).id]"
				expected = "not a property lambda"
			}
			_, err := PlanDelegatedResources(template, "/planes/radius/local/resourceGroups/fixture", definitions, schemas)
			require.ErrorContains(t, err, expected)
			require.NotContains(t, server["properties"].(map[string]any)["properties"], "__radiusDelegatingResources")
		})
	}
}
