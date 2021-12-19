// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converters

import (
	"encoding/json"
	"io/ioutil"
	"path"
	"testing"

	"github.com/Azure/radius/pkg/cli/armtemplate"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func Test_ConvertToRenderResource(t *testing.T) {
	original, err := ioutil.ReadFile(path.Join("testdata", "frontend-resource.json"))
	require.NoError(t, err)

	resource := radiusv1alpha3.Resource{}
	err = json.Unmarshal(original, &resource)
	require.NoError(t, err)

	actual := renderers.RendererResource{}
	expected := renderers.RendererResource{
		ResourceName:    "frontend",
		ApplicationName: "azure-resources-container-httpbinding",
		ResourceType:    "Container",
		Definition: map[string]interface{}{
			"connections": map[string]interface{}{
				"backend": map[string]interface{}{
					"kind":   "Http",
					"source": "[resourceId('Microsoft.CustomProviders/resourceProviders/Application/HttpRoute', 'radiusv3', 'azure-resources-container-httpbinding', 'backend')]",
				},
			},
			"container": map[string]interface{}{
				"env": map[string]interface{}{
					"SERVICE__BACKEND__HOST": "[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Application/HttpRoute', 'radiusv3', 'azure-resources-container-httpbinding', 'backend')).host]",
					"SERVICE__BACKEND__PORT": "[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Application/HttpRoute', 'radiusv3', 'azure-resources-container-httpbinding', 'backend')).port]",
				},
				"image": "rynowak/frontend:0.5.0-dev",
				"ports": map[string]interface{}{
					"web": map[string]interface{}{
						"containerPort": 80.0,
						"provides":      "[resourceId('Microsoft.CustomProviders/resourceProviders/Application/HttpRoute', 'radiusv3', 'azure-resources-container-httpbinding', 'frontend')]",
					},
				},
			},
		},
	}

	err = ConvertToRenderResource(&resource, &actual)
	require.NoError(t, err, "failed to convert resource")

	require.Equal(t, expected, actual)
}

func Test_ConvertToARMResource(t *testing.T) {
	original, err := ioutil.ReadFile(path.Join("testdata", "frontend-resource.json"))
	require.NoError(t, err)

	resource := radiusv1alpha3.Resource{}
	err = json.Unmarshal(original, &resource)
	require.NoError(t, err)

	// Our test data has no status defined so add some computed values for testing.
	computedValues := map[string]renderers.ComputedValueReference{
		"A": {
			Value: "A-Value",
		},
		"B": {
			Value: "B-Value",
		},
	}

	b, err := json.Marshal(&computedValues)
	require.NoError(t, err)
	resource.Status.ComputedValues = &runtime.RawExtension{Raw: b}

	arm := armtemplate.Resource{}
	err = json.Unmarshal(resource.Spec.Template.Raw, &arm)
	require.NoError(t, err)

	// Modifies arm.Body in place
	err = ConvertToARMResource(&resource, arm.Body)
	require.NoError(t, err)

	expected := map[string]interface{}{
		"properties": map[string]interface{}{
			"A": "A-Value",
			"B": "B-Value",
			"connections": map[string]interface{}{
				"backend": map[string]interface{}{
					"kind":   "Http",
					"source": "[resourceId('Microsoft.CustomProviders/resourceProviders/Application/HttpRoute', 'radiusv3', 'azure-resources-container-httpbinding', 'backend')]",
				},
			},
			"container": map[string]interface{}{
				"env": map[string]interface{}{
					"SERVICE__BACKEND__HOST": "[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Application/HttpRoute', 'radiusv3', 'azure-resources-container-httpbinding', 'backend')).host]",
					"SERVICE__BACKEND__PORT": "[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Application/HttpRoute', 'radiusv3', 'azure-resources-container-httpbinding', 'backend')).port]",
				},
				"image": "rynowak/frontend:0.5.0-dev",
				"ports": map[string]interface{}{
					"web": map[string]interface{}{
						"containerPort": 80.0,
						"provides":      "[resourceId('Microsoft.CustomProviders/resourceProviders/Application/HttpRoute', 'radiusv3', 'azure-resources-container-httpbinding', 'frontend')]",
					},
				},
			},
		},
	}
	require.Equal(t, expected, arm.Body)
}
