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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/stretchr/testify/require"
)

func Test_ArmToK8sConversion(t *testing.T) {
	content, err := ioutil.ReadFile(path.Join("testdata", "frontend-backend.json"))
	require.NoError(t, err)

	applicationUnstructured, err := GetUnstructured(path.Join("testdata", "frontend-backend", "frontend-backend-application.json"))
	require.NoError(t, err)
	frontendUnstructured, err := GetUnstructured(path.Join("testdata", "frontend-backend", "frontend-backend-frontend.json"))
	require.NoError(t, err)
	backendUnstructured, err := GetUnstructured(path.Join("testdata", "frontend-backend", "frontend-backend-backend.json"))
	require.NoError(t, err)
	deploymentUnstructured, err := GetUnstructured(path.Join("testdata", "frontend-backend", "frontend-backend-deployment.json"))
	require.NoError(t, err)

	expected := map[string]K8sInfo{
		"radius-frontend-backend": {
			Name:         "radius-frontend-backend",
			Unstructured: applicationUnstructured,
			GVR:          schema.GroupVersionResource{Group: "applications.radius.dev", Version: "v1alpha1", Resource: "applications"},
		},
		"radius-frontend-backend-backend": {
			Name:         "radius-frontend-backend-backend",
			Unstructured: backendUnstructured,
			GVR:          schema.GroupVersionResource{Group: "applications.radius.dev", Version: "v1alpha1", Resource: "components"},
		},
		"radius-frontend-backend-frontend": {
			Name:         "radius-frontend-backend-frontend",
			Unstructured: frontendUnstructured,
			GVR:          schema.GroupVersionResource{Group: "applications.radius.dev", Version: "v1alpha1", Resource: "components"},
		},
		"radius-frontend-backend-default": {
			Name:         "radius-frontend-backend-default",
			Unstructured: deploymentUnstructured,
			GVR:          schema.GroupVersionResource{Group: "applications.radius.dev", Version: "v1alpha1", Resource: "deployments"},
		},
	}

	template, err := Parse(string(content))
	require.NoError(t, err)

	resources, err := Eval(template, TemplateOptions{})
	require.NoError(t, err)

	actual := map[string]K8sInfo{}
	for _, resource := range resources {
		k8sInfo, err := ConvertToK8s(resource, "default")
		require.NoError(t, err)
		actual[k8sInfo.Name] = k8sInfo
	}

	for k, actualInfo := range actual {
		expectedInfo := expected[k]
		require.Equal(t, expectedInfo.Name, actualInfo.Name)
		require.Equal(t, expectedInfo.GVR, actualInfo.GVR)

		// Unstructured comparison causes a comparison between interface{} and a string
		// so we need to convert to JSON
		expectedUns, err := json.Marshal(expectedInfo.Unstructured)
		require.NoError(t, err)

		actualUns, err := json.Marshal(actualInfo.Unstructured)
		require.NoError(t, err)

		require.Equal(t, expectedUns, actualUns)
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
