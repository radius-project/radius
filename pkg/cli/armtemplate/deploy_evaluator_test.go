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

		deployed[resource.ID] = body
		evaluated = append(evaluated, resource)
	}

	application, err := GetResource(path.Join("testdata", "armevaluated", "Microsoft.CustomProviders-resourceProviders-Applicationradiusv3-azure-resources-container-httpbinding.json"))
	require.NoError(t, err)
	backend, err := GetResource(path.Join("testdata", "armevaluated", "Microsoft.CustomProviders-resourceProviders-Application-ContainerComponentradiusv3-azure-resources-container-httpbinding-backend.json"))
	require.NoError(t, err)
	frontend, err := GetResource(path.Join("testdata", "armevaluated", "Microsoft.CustomProviders-resourceProviders-Application-ContainerComponentradiusv3-azure-resources-container-httpbinding-frontend.json"))
	require.NoError(t, err)

	frontendRoute, err := GetResource(path.Join("testdata", "armevaluated", "Microsoft.CustomProviders-resourceProviders-Application-HttpRouteradiusv3-azure-resources-container-httpbinding-frontend.json"))
	require.NoError(t, err)
	backendRoute, err := GetResource(path.Join("testdata", "armevaluated", "Microsoft.CustomProviders-resourceProviders-Application-HttpRouteradiusv3-azure-resources-container-httpbinding-backend.json"))
	require.NoError(t, err)

	actual := map[string]Resource{}

	for _, resource := range evaluated {
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
