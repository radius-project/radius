// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"encoding/json"
	"io/ioutil"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_ArmToK8sConversion(t *testing.T) {
	content, err := ioutil.ReadFile(path.Join("testdata", "frontend-backend.json"))
	require.NoError(t, err)

	applicationUnstructured, err := GetUnstructured(path.Join("testdata", "frontend-backend", "Application-azure-resources-container-httpbinding.json"))
	require.NoError(t, err)
	backendUnstructured, err := GetUnstructured(path.Join("testdata", "frontend-backend", "ContainerComponent-backend.json"))
	require.NoError(t, err)
	frontendUnstructured, err := GetUnstructured(path.Join("testdata", "frontend-backend", "ContainerComponent-frontend.json"))
	require.NoError(t, err)

	frontendRouteUnstructured, err := GetUnstructured(path.Join("testdata", "frontend-backend", "HttpRoute-frontend.json"))
	require.NoError(t, err)
	backendRouteUnstructured, err := GetUnstructured(path.Join("testdata", "frontend-backend", "HttpRoute-backend.json"))
	require.NoError(t, err)

	expected := map[string]*unstructured.Unstructured{
		"HttpRoute-azure-resources-container-httpbinding-frontend":          frontendRouteUnstructured,
		"Application-azure-resources-container-httpbinding":                 applicationUnstructured,
		"HttpRoute-azure-resources-container-httpbinding-backend":           backendRouteUnstructured,
		"ContainerComponent-azure-resources-container-httpbinding-backend":  backendUnstructured,
		"ContainerComponent-azure-resources-container-httpbinding-frontend": frontendUnstructured,
	}

	template, err := Parse(string(content))
	require.NoError(t, err)

	resources, err := Eval(template, TemplateOptions{
		Parameters: map[string]map[string]interface{}{
			// Setting one required parameter, and using the default value for 'backendRoute' parameter
			"frontendRoute": {
				"value": "frontend",
			},
		},
	})
	require.NoError(t, err)

	actual := map[string]*unstructured.Unstructured{}
	for _, resource := range resources {
		k8sInfo, err := ConvertToK8s(resource, "default")
		require.NoError(t, err)
		actual[k8sInfo.GetObjectKind().GroupVersionKind().Kind+"-"+k8sInfo.GetName()] = k8sInfo
	}

	for k, actualInfo := range actual {
		expectedInfo := expected[k]
		// Unstructured comparison causes a comparison between interface{} and a string
		// so we need to convert to JSON
		expectedUns, err := json.Marshal(expectedInfo)

		require.NoError(t, err)

		actualUns, err := json.Marshal(actualInfo)
		require.NoError(t, err)

		require.JSONEq(t, string(expectedUns), string(actualUns))
	}
}

func TestUnwrapK8sUnstructured(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input       Resource
		expected    unstructured.Unstructured
		expectedErr string
	}{{
		name: "invalid resource.Type",
		input: Resource{
			APIVersion: "v1",
			Type:       "this/looks/like/an/arm/Type",
			Provider: &Provider{
				Name: "Kubernetes",
			},
		},
		expectedErr: "invalid resource type",
	}, {
		name: "has no properties",
		input: Resource{
			APIVersion: "v1",
			Type:       "kubernetes.core/Secret",
			Provider: &Provider{
				Name: "Kubernetes",
			},
		},
		expectedErr: "lacks required property 'properties'",
	}, {
		name: "empty secret",
		input: Resource{
			APIVersion: "v1",
			Type:       "kubernetes.core/Secret",
			Provider: &Provider{
				Name: "Kubernetes",
			},
			Body: map[string]interface{}{
				"properties": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "default",
					},
				},
			},
		},
		expected: unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "foo",
					"namespace": "default",
				},
			},
		},
	}, {
		name: "secret",
		input: Resource{
			APIVersion: "v1",
			Type:       "kubernetes.core/Secret",
			Provider: &Provider{
				Name: "Kubernetes",
			},
			Body: map[string]interface{}{
				"properties": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "moria",
						"namespace": "middle-earth",
					},
					"data": map[string]interface{}{
						"password": "Mellon",
						"username": "Gandalf",
					},
				},
			},
		},
		expected: unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "moria",
					"namespace": "middle-earth",
				},
				"data": map[string]interface{}{
					"password": "Mellon",
					"username": "Gandalf",
				},
			},
		},
	}, {
		name: "service",
		input: Resource{
			APIVersion: "v1",
			Type:       "kubernetes.core/Service",
			Provider: &Provider{
				Name: "Kubernetes",
			},
			Body: map[string]interface{}{
				"properties": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "redis-master",
						"namespace": "redis",
					},
					"spec": map[string]interface{}{
						"type": "ClusterIP",
						"selector": map[string]interface{}{
							"app.kubernetes.io/component": "master",
						},
					},
				},
			},
		},
		expected: unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata": map[string]interface{}{
					"name":      "redis-master",
					"namespace": "redis",
				},
				"spec": map[string]interface{}{
					"type": "ClusterIP",
					"selector": map[string]interface{}{
						"app.kubernetes.io/component": "master",
					},
				},
			},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			output, err := ConvertToK8s(tc.input, "default")
			if err != nil {
				require.True(t, tc.expectedErr != "", "unexpected err %v", err)
				require.Regexp(t, tc.expectedErr, err.Error())
				return
			}
			require.Equal(t, tc.expected, *output)
		})
	}
}

func GetUnstructured(filePath string) (*unstructured.Unstructured, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	uns := &unstructured.Unstructured{}
	err = json.Unmarshal(content, uns)
	return uns, err
}
