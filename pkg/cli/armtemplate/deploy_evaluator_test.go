// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"io/ioutil"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

// Main purpose of deploy evaluator is to verify reference works between deployed resources
func Test_DeploymentEvaluator_ReferenceWorks(t *testing.T) {
	content, err := ioutil.ReadFile(path.Join("testdata", "frontend-backend.json"))
	require.NoError(t, err)

	template, err := Parse(string(content))
	require.NoError(t, err)
	options := TemplateOptions{
		SubscriptionID: "test-sub",
		ResourceGroup:  "test-group",
	}

	resources, err := Eval(template, options)
	require.NoError(t, err)

	deployed := map[string]map[string]interface{}{}
	evaluator := &DeploymentEvaluator{
		Template:  template,
		Options:   options,
		Deployed:  deployed,
		Variables: map[string]interface{}{},
	}

	for name, variable := range template.Variables {
		value, err := evaluator.VisitValue(variable)
		require.NoError(t, err)

		evaluator.Variables[name] = value
	}
	var evaluated []Resource

	for _, resource := range resources {
		body, err := evaluator.VisitMap(resource.Body)
		require.NoError(t, err)

		resource.Body = body

		deployed[resource.ID] = map[string]interface{}{}
		properties := body["properties"]
		if properties != nil {
			for k, v := range properties.(map[string]interface{}) {
				deployed[resource.ID][k] = v
			}
		}
		evaluated = append(evaluated, resource)
	}

	expected := []Resource{
		{
			ID:         "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/frontend-backend",
			Type:       "Microsoft.CustomProviders/resourceProviders/Applications",
			APIVersion: "2018-09-01-preview",
			Name:       "radius/frontend-backend",
			DependsOn:  []string{},
			Body:       map[string]interface{}{},
		},
		{
			ID:         "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/frontend-backend/Components/backend",
			Type:       "Microsoft.CustomProviders/resourceProviders/Applications/Components",
			APIVersion: "2018-09-01-preview",
			Name:       "radius/frontend-backend/backend",
			DependsOn: []string{
				"/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/frontend-backend",
			},
			Body: map[string]interface{}{
				"kind": "radius.dev/Container@v1alpha1",
				"properties": map[string]interface{}{
					"bindings": map[string]interface{}{
						"web": map[string]interface{}{
							"kind":       "http",
							"targetPort": 81.0,
						},
					},
					"run": map[string]interface{}{
						"container": map[string]interface{}{
							"image": "rynowak/backend:0.5.0-dev",
						},
					},
				},
			},
		},
		{
			ID:         "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/frontend-backend/Components/frontend",
			Type:       "Microsoft.CustomProviders/resourceProviders/Applications/Components",
			APIVersion: "2018-09-01-preview",
			Name:       "radius/frontend-backend/frontend",
			DependsOn: []string{
				"/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/frontend-backend",
			},
			Body: map[string]interface{}{
				"kind": "radius.dev/Container@v1alpha1",
				"properties": map[string]interface{}{
					"bindings": map[string]interface{}{
						"web": map[string]interface{}{
							"kind":       "http",
							"targetPort": 80.0,
						},
					},
					"run": map[string]interface{}{
						"container": map[string]interface{}{
							"image": "rynowak/frontend:0.5.0-dev",
						},
					},
					"uses": []interface{}{
						map[string]interface{}{
							"binding": map[string]interface{}{
								"kind":       "http",
								"targetPort": 81.0,
							},
							"env": map[string]interface{}{
								"SERVICE__BACKEND__TARGETPORT": 81.0,
							},
						},
					},
				},
			},
		},
		{
			ID:         "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/frontend-backend/Deployments/default",
			Type:       "Microsoft.CustomProviders/resourceProviders/Applications/Deployments",
			APIVersion: "2018-09-01-preview",
			Name:       "radius/frontend-backend/default",
			DependsOn: []string{
				"/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/frontend-backend",
				"/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/frontend-backend/Components/backend",
				"/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications/frontend-backend/Components/frontend",
			},
			Body: map[string]interface{}{
				"properties": map[string]interface{}{
					"components": []interface{}{
						map[string]interface{}{
							"componentName": "backend",
						},
						map[string]interface{}{
							"componentName": "frontend",
						},
					},
				},
			},
		},
	}
	require.Equal(t, expected, evaluated)
}
