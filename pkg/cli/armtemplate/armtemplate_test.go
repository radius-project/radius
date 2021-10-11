// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"encoding/json"
	"io/ioutil"
	"path"
	"strings"
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
		Parameters: map[string]map[string]interface{}{
			// Setting one required parameter, and using the default value for 'backendRoute' parameter
			"frontendRoute": {
				"value": "frontend",
			},
		},
	})
	require.NoError(t, err)

	application, err := GetResource(path.Join("testdata", "arm", "Microsoft.CustomProviders-resourceProviders-Applicationradiusv3-azure-resources-container-httpbinding.json"))
	require.NoError(t, err)
	backend, err := GetResource(path.Join("testdata", "arm", "Microsoft.CustomProviders-resourceProviders-Application-ContainerComponentradiusv3-azure-resources-container-httpbinding-backend.json"))
	require.NoError(t, err)
	frontend, err := GetResource(path.Join("testdata", "arm", "Microsoft.CustomProviders-resourceProviders-Application-ContainerComponentradiusv3-azure-resources-container-httpbinding-frontend.json"))
	require.NoError(t, err)

	frontendRoute, err := GetResource(path.Join("testdata", "arm", "Microsoft.CustomProviders-resourceProviders-Application-HttpRouteradiusv3-azure-resources-container-httpbinding-frontend.json"))
	require.NoError(t, err)
	backendRoute, err := GetResource(path.Join("testdata", "arm", "Microsoft.CustomProviders-resourceProviders-Application-HttpRouteradiusv3-azure-resources-container-httpbinding-backend.json"))
	require.NoError(t, err)

	actual := map[string]Resource{}

	for _, resource := range resources {
		actual[strings.ReplaceAll(resource.Type+resource.Name, "/", "-")] = resource
	}

	expected := map[string]*Resource{
		"Microsoft.CustomProviders-resourceProviders-Application-HttpRouteradiusv3-azure-resources-container-httpbinding-frontend":          frontendRoute,
		"Microsoft.CustomProviders-resourceProviders-Applicationradiusv3-azure-resources-container-httpbinding":                             application,
		"Microsoft.CustomProviders-resourceProviders-Application-HttpRouteradiusv3-azure-resources-container-httpbinding-backend":           backendRoute,
		"Microsoft.CustomProviders-resourceProviders-Application-ContainerComponentradiusv3-azure-resources-container-httpbinding-backend":  backend,
		"Microsoft.CustomProviders-resourceProviders-Application-ContainerComponentradiusv3-azure-resources-container-httpbinding-frontend": frontend,
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

func GetResource(filePath string) (*Resource, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	uns := &Resource{}
	err = json.Unmarshal(content, uns)
	return uns, err
}
