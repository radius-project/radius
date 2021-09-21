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

func Test_DeploymentTemplate(t *testing.T) {
	content, err := ioutil.ReadFile(path.Join("testdata", "frontend-backend.json"))
	require.NoError(t, err)

	template, err := Parse(string(content))
	require.NoError(t, err)

	resources, err := Eval(template, TemplateOptions{
		SubscriptionID: "test-sub",
		ResourceGroup:  "test-group",
	})
	require.NoError(t, err)

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
							"binding": "[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'backend')).bindings.web]",
							"env": map[string]interface{}{
								"SERVICE__BACKEND__TARGETPORT": "[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'frontend-backend', 'backend')).bindings.web.targetPort]",
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
	require.Equal(t, expected, resources)
}
