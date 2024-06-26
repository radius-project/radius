/*
Copyright 2023 The Radius Authors.

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

package ucp

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	core_v20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	ucp_v20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/radius-project/radius/test/testutil"
	test "github.com/radius-project/radius/test/ucp"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: most functionality in UCP is tested with integration tests. Testing
// done here is intentionally minimal.

func Test_UserDefinedType_Operations(t *testing.T) {
	apiVersion := ucp_v20231001preview.Version

	// Randomize names to avoid interference with other tests.
	planeName := fmt.Sprintf("test-%s", uuid.New().String())
	resourceGroupName := fmt.Sprintf("test-%s", uuid.New().String())
	environmentName := fmt.Sprintf("test-%s", uuid.New().String())
	resourceName := fmt.Sprintf("test-%s", uuid.New().String())

	test := test.NewUCPTest(t, "Test_UserDefinedType_Operations", func(t *testing.T, test *test.UCPTest) {

		localPlaneUrl := fmt.Sprintf("%s/planes/radius/local?api-version=%s", test.URL, apiVersion)
		planeUrl := fmt.Sprintf("%s/planes/radius/%s?api-version=%s", test.URL, planeName, apiVersion)
		resourceProviderUrl := fmt.Sprintf("%s/planes/radius/%s/providers/Contoso.Example?api-version=%s", test.URL, planeName, apiVersion)
		resourceGroupUrl := fmt.Sprintf("%s/planes/radius/%s/resourceGroups/%s?api-version=%s", test.URL, planeName, resourceGroupName, apiVersion)

		environmentID := fmt.Sprintf("/planes/radius/%s/resourceGroups/%s/providers/Applications.Core/environments/%s", planeName, resourceGroupName, environmentName)
		resourceID := fmt.Sprintf("/planes/radius/%s/resourceGroups/%s/providers/Contoso.Example/postgreSQLDatabases/%s", planeName, resourceGroupName, resourceName)

		// Create a plane that we can use for testing. Not using t.Run here because we want to
		// stop the test if this fails.

		localPlane, _ := getPlane[ucp_v20231001preview.RadiusPlaneResource](t, test.Transport, localPlaneUrl)

		createPlane(t, test.Transport, planeUrl, &ucp_v20231001preview.RadiusPlaneResource{
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &ucp_v20231001preview.RadiusPlaneResourceProperties{
				ResourceProviders: localPlane.Properties.ResourceProviders, // Copy configured resource providers
			},
		})

		resourceProvider := &ucp_v20231001preview.ResourceProviderResource{}
		testutil.MustUnmarshalFromFile("resourceprovider-requestbody.json", resourceProvider)
		createResourceProvider(t, test.Transport, resourceProviderUrl, resourceProvider)

		createResourceGroup(t, test.Transport, resourceGroupUrl)

		test.CreateResource(t, environmentID, core_v20231001preview.EnvironmentResource{
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &core_v20231001preview.EnvironmentProperties{
				Compute: &core_v20231001preview.KubernetesCompute{
					Kind:      to.Ptr("Kubernetes"),
					Namespace: to.Ptr(environmentName),
				},
				Recipes: map[string]map[string]core_v20231001preview.RecipePropertiesClassification{
					"Contoso.Example/postgreSQLDatabases": {
						"default": &core_v20231001preview.BicepRecipeProperties{
							TemplateKind: to.Ptr("Bicep"),
							TemplatePath: to.Ptr("rynowak.azurecr.io/recipes/postgres:latest"),
						},
					},
				},
			},
		})

		_, err := test.Options.K8sClient.CoreV1().Namespaces().Create(testcontext.New(t), &core_v1.Namespace{ObjectMeta: meta_v1.ObjectMeta{Name: environmentName}}, meta_v1.CreateOptions{})
		require.NoError(t, err)

		input := &generated.GenericResource{}
		testutil.MustUnmarshalFromFile("postgresqldatabases-requestbody.json", input)
		input.Properties["environment"] = environmentID

		test.CreateResource(t, resourceID, input)

		expected := &generated.GenericResource{}
		testutil.MustUnmarshalFromFile("postgresqldatabases-responsebody.json", expected)
		expected.ID = to.Ptr(resourceID)
		expected.Properties["environment"] = environmentID

		actual := &generated.GenericResource{}
		test.GetResource(t, resourceID, actual)

		actual.SystemData = nil
		require.Equal(t, expected, actual)

		test.DeleteResource(t, resourceID)

		deleteResourceGroup(t, test.Transport, resourceGroupUrl)
		deletePlane(t, test.Transport, planeUrl)
	})
	test.Test(t)
}
