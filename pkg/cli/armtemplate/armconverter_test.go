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
		"Application-azure-resources-container-httpbinding": applicationUnstructured,
		"HttpRoute-backend":           backendRouteUnstructured,
		"ContainerComponent-backend":  backendUnstructured,
		"HttpRoute-frontend":          frontendRouteUnstructured,
		"ContainerComponent-frontend": frontendUnstructured,
	}

	template, err := Parse(string(content))
	require.NoError(t, err)

	resources, err := Eval(template, TemplateOptions{})
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

func GetUnstructured(filePath string) (*unstructured.Unstructured, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	uns := &unstructured.Unstructured{}
	err = json.Unmarshal(content, uns)
	return uns, err
}
