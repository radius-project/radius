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

	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/model/resourcesv1alpha3"
	"github.com/stretchr/testify/require"
)

func Test_ConvertComponentToInternal(t *testing.T) {
	original, err := ioutil.ReadFile(path.Join("testdata", "frontend-component.json"))
	require.NoError(t, err)

	resource := radiusv1alpha3.Resource{}
	err = json.Unmarshal(original, &resource)
	require.NoError(t, err)

	actual := resourcesv1alpha3.GenericResource{}
	expected := resourcesv1alpha3.GenericResource{
		Name: "frontend",
		Kind: "ContainerComponent",
		ID:   "/subscriptions//resourceGroups//providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/azure-resources-container-httpbinding/ContainerComponent/frontend",
		AdditionalProperties: map[string]interface{}{
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

	err = ConvertComponentToInternal(&resource, &actual, nil)
	require.NoError(t, err, "failed to convert component")

	require.Equal(t, expected, actual)
}
