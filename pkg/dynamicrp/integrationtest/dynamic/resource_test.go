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

package dynamic

import (
	"context"
	"net/http"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/dynamicrp"
	"github.com/radius-project/radius/pkg/dynamicrp/testhost"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/driver"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	ucptesthost "github.com/radius-project/radius/pkg/ucp/testhost"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	radiusPlaneName     = "testing"
	locationName        = v1.LocationGlobal
	resourceGroupName   = "test-group"
	testPlaneID         = "/planes/radius/" + radiusPlaneName
	testResourceGroupID = testPlaneID + "/resourceGroups/test-group"

	apiVersion                = "2024-01-01"
	resourceProviderNamespace = "Applications.Test"
	inertResourceTypeName     = "exampleInertResources"
	recipeResourceTypeName    = "exampleRecipeResources"

	testInertResourceName = "my-inert-example"
	testInertResourceID   = testResourceGroupID + "/providers/" + resourceProviderNamespace + "/" + inertResourceTypeName + "/" + testInertResourceName
	testInertResourceURL  = testInertResourceID + "?api-version=" + apiVersion

	testRecipeResourceName = "my-recipe-example"
	testRecipeResourceID   = testResourceGroupID + "/providers/" + resourceProviderNamespace + "/" + recipeResourceTypeName + "/" + testRecipeResourceName
	testRecipeResourceURL  = testRecipeResourceID + "?api-version=" + apiVersion
)

// This test covers the lifecycle of a dynamic resource with an "inert" lifecycle (not recipes).
func Test_Dynamic_Resource_Inert_Lifecycle(t *testing.T) {
	_, ucp := testhost.Start(t)

	// Setup a resource provider (Applications.Test/exampleInertResources)
	createRadiusPlane(ucp)
	createResourceProvider(ucp)
	createInertResourceType(ucp)
	createAPIVersion(ucp, inertResourceTypeName, nil)
	createLocation(ucp, inertResourceTypeName)

	// Setup a resource group where we can interact with the new resource type.
	createResourceGroup(ucp)

	// Now let's test the basic CRUD operations on the new resource type.
	//
	// This resource type DOES NOT support recipes, so it's "inert" and doesn't do anything in the backend.
	resource := map[string]any{
		"properties": map[string]any{
			"foo": "bar",
		},
		"tags": map[string]string{
			"costcenter": "12345",
		},
	}

	// Create the resource
	response := ucp.MakeTypedRequest(http.MethodPut, testInertResourceURL, resource)
	response.WaitForOperationComplete(nil)

	// Now lets verify the resource was created successfully.

	expectedResource := map[string]any{
		"id":       "/planes/radius/testing/resourcegroups/test-group/providers/Applications.Test/exampleInertResources/my-inert-example",
		"location": "global",
		"name":     "my-inert-example",
		"properties": map[string]any{
			"foo":               "bar",
			"provisioningState": "Succeeded",
		},
		"tags": map[string]any{
			"costcenter": "12345",
		},
		"type": "Applications.Test/exampleInertResources",
	}

	expectedList := map[string]any{
		"value": []any{expectedResource},
	}

	// GET (single)
	response = ucp.MakeRequest(http.MethodGet, testInertResourceURL, nil)
	response.EqualsValue(200, expectedResource)

	// GET (list at plane-scope)
	response = ucp.MakeRequest(http.MethodGet, "/planes/radius/testing/resourcegroups/test-group/providers/Applications.Test/exampleInertResources"+"?api-version="+apiVersion, nil)
	response.EqualsValue(200, expectedList)

	// GET (list at resourcegroup-scope)
	response = ucp.MakeRequest(http.MethodGet, "/planes/radius/testing/providers/Applications.Test/exampleInertResources"+"?api-version="+apiVersion, nil)
	response.EqualsValue(200, expectedList)

	// Now lets delete the resource
	response = ucp.MakeRequest(http.MethodDelete, testInertResourceURL, nil)
	response.WaitForOperationComplete(nil)

	// Now we should get a 404 when trying to get the resource
	response = ucp.MakeRequest(http.MethodGet, testInertResourceURL, nil)
	response.EqualsErrorCode(404, v1.CodeNotFound)
}

func Test_Dynamic_Resource_Recipe_Lifecycle(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockDriver := driver.NewMockDriver(ctrl)
	mockConfigLoader := configloader.NewMockConfigurationLoader(ctrl)

	_, ucp := testhost.Start(t, testhost.TestHostOptionFunc(func(options *dynamicrp.Options) {
		// Register a mock driver for the "test" recipe driver.
		options.Recipes.Drivers = map[string]func(options *dynamicrp.Options) (driver.Driver, error){
			"test": func(options *dynamicrp.Options) (driver.Driver, error) {
				return mockDriver, nil
			},
		}

		// Replace the configuration loader with a mock.
		//
		// We don't have an environment in this test because applications-rp isn't part of the
		// integration testing framework.
		options.Recipes.ConfigurationLoader = mockConfigLoader
	}))

	schema := map[string]any{
		"properties": map[string]any{
			"hostname": map[string]any{},
			"port":     map[string]any{},
			"password": map[string]any{},
		},
	}
	// Setup a resource provider (Applications.Test/exampleRecipeResources)
	createRadiusPlane(ucp)
	createResourceProvider(ucp)
	createRecipeResourceType(ucp)
	createAPIVersion(ucp, recipeResourceTypeName, schema)
	createLocation(ucp, recipeResourceTypeName)

	// Setup a resource group where we can interact with the new resource type.
	createResourceGroup(ucp)

	// Setup a recipe for the new resource type.
	mockConfigLoader.EXPECT().
		LoadRecipe(gomock.Any(), gomock.Any()).
		Return(&recipes.EnvironmentDefinition{
			Name:            "default",
			Driver:          "test",
			ResourceType:    "Applications.Test/exampleRecipeResources",
			TemplatePath:    "test-path",
			TemplateVersion: "test-version",
		}, nil).
		AnyTimes()
	mockConfigLoader.EXPECT().
		LoadConfiguration(gomock.Any(), gomock.Any()).
		Return(&recipes.Configuration{}, nil).
		AnyTimes()

	mockDriver.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		Return(&recipes.RecipeOutput{
			Resources: []string{"/planes/example/testing/providers/Test.Namespace/testResource/example"},
			Values: map[string]any{
				"port":     8080,
				"hostname": "example.com",
			},
			Secrets: map[string]any{
				"password": "v3ryS3cr3t",
			},
			Status: &rpv1.RecipeStatus{
				TemplateKind:    "test",
				TemplatePath:    "test-path",
				TemplateVersion: "test-version",
			},
		}, nil).
		Times(1)

	mockDriver.EXPECT().
		Delete(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	// Now let's test the basic CRUD operations on the new resource type.
	//
	// This resource type supports recipes, so we expect to see some additional output.
	resource := map[string]any{
		"properties": map[string]any{
			"foo": "bar",
		},
		"tags": map[string]string{
			"costcenter": "12345",
		},
	}

	// Create the resource
	response := ucp.MakeTypedRequest(http.MethodPut, testRecipeResourceURL, resource)
	response.WaitForOperationComplete(nil)

	// Now lets verify the resource was created successfully.
	expectedResource := map[string]any{
		"id":       "/planes/radius/testing/resourcegroups/test-group/providers/Applications.Test/exampleRecipeResources/my-recipe-example",
		"location": "global",
		"name":     "my-recipe-example",
		"properties": map[string]any{
			"port":              float64(8080), // This is an artifact of the JSON unmarshal process. It's wierd but intended.
			"hostname":          "example.com",
			"password":          "v3ryS3cr3t", // TODO: See comments in dynamicresource.go
			"foo":               "bar",
			"provisioningState": "Succeeded",
			"recipe": map[string]any{
				"name":         "default",
				"recipeStatus": "success",
			},
			"status": map[string]any{
				"computedValues": map[string]any{
					"port":     float64(8080), // This is an artifact of the JSON unmarshal process. It's wierd but intended.
					"hostname": "example.com",
				},
				"secrets": map[string]any{
					"password": map[string]any{
						"Value": "v3ryS3cr3t",
					},
				},
				"outputResources": []any{
					map[string]any{
						"id":            "/planes/example/testing/providers/Test.Namespace/testResource/example",
						"localID":       "",
						"radiusManaged": true,
					},
				},
				"recipe": map[string]any{
					"templateKind":    "test",
					"templatePath":    "test-path",
					"templateVersion": "test-version",
				},
			},
		},
		"tags": map[string]any{
			"costcenter": "12345",
		},
		"type": "Applications.Test/exampleRecipeResources",
	}

	expectedList := map[string]any{
		"value": []any{expectedResource},
	}

	// GET (single)
	response = ucp.MakeRequest(http.MethodGet, testRecipeResourceURL, nil)
	response.EqualsValue(200, expectedResource)

	// GET (list at plane-scope)
	response = ucp.MakeRequest(http.MethodGet, "/planes/radius/testing/resourcegroups/test-group/providers/Applications.Test/exampleRecipeResources"+"?api-version="+apiVersion, nil)
	response.EqualsValue(200, expectedList)

	// GET (list at resourcegroup-scope)
	response = ucp.MakeRequest(http.MethodGet, "/planes/radius/testing/providers/Applications.Test/exampleRecipeResources"+"?api-version="+apiVersion, nil)
	response.EqualsValue(200, expectedList)

	// Now lets delete the resource
	response = ucp.MakeRequest(http.MethodDelete, testRecipeResourceURL, nil)
	response.WaitForOperationComplete(nil)

	// Now we should get a 404 when trying to get the resource
	response = ucp.MakeRequest(http.MethodGet, testRecipeResourceURL, nil)
	response.EqualsErrorCode(404, v1.CodeNotFound)
}

func createRadiusPlane(server *ucptesthost.TestHost) v20231001preview.RadiusPlanesClientCreateOrUpdateResponse {
	ctx := context.Background()

	plane := v20231001preview.RadiusPlaneResource{
		Location: to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.RadiusPlaneResourceProperties{
			// Note: this is a workaround. Properties is marked as a required field in
			// the API. Without passing *something* here the body will be rejected.
			ProvisioningState: to.Ptr(v20231001preview.ProvisioningStateSucceeded),
			ResourceProviders: map[string]*string{},
		},
	}

	client := server.UCP().NewRadiusPlanesClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, plane, nil)
	require.NoError(server.T(), err)

	response, err := poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)

	return response
}

func createResourceProvider(server *ucptesthost.TestHost) {
	ctx := context.Background()

	resourceProvider := v20231001preview.ResourceProviderResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.ResourceProviderProperties{},
	}

	client := server.UCP().NewResourceProvidersClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, resourceProviderNamespace, resourceProvider, nil)
	require.NoError(server.T(), err)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)
}

func createInertResourceType(server *ucptesthost.TestHost) {
	ctx := context.Background()

	resourceType := v20231001preview.ResourceTypeResource{
		Properties: &v20231001preview.ResourceTypeProperties{},
	}

	client := server.UCP().NewResourceTypesClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, resourceProviderNamespace, inertResourceTypeName, resourceType, nil)
	require.NoError(server.T(), err)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)
}

func createRecipeResourceType(server *ucptesthost.TestHost) {
	ctx := context.Background()

	resourceType := v20231001preview.ResourceTypeResource{
		Properties: &v20231001preview.ResourceTypeProperties{
			Capabilities: []*string{
				to.Ptr(datamodel.CapabilitySupportsRecipes),
			},
		},
	}

	client := server.UCP().NewResourceTypesClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, resourceProviderNamespace, recipeResourceTypeName, resourceType, nil)
	require.NoError(server.T(), err)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)
}

func createAPIVersion(server *ucptesthost.TestHost, resourceType string, schema map[string]any) {
	ctx := context.Background()

	apiVersionResource := v20231001preview.APIVersionResource{
		Properties: &v20231001preview.APIVersionProperties{
			Schema: schema,
		},
	}

	client := server.UCP().NewAPIVersionsClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, resourceProviderNamespace, resourceType, apiVersion, apiVersionResource, nil)
	require.NoError(server.T(), err)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)
}

func createLocation(server *ucptesthost.TestHost, resourceType string) {
	ctx := context.Background()

	location := v20231001preview.LocationResource{
		Properties: &v20231001preview.LocationProperties{
			ResourceTypes: map[string]*v20231001preview.LocationResourceType{
				resourceType: {
					APIVersions: map[string]map[string]any{
						apiVersion: {},
					},
				},
			},
		},
	}

	client := server.UCP().NewLocationsClient()
	poller, err := client.BeginCreateOrUpdate(ctx, radiusPlaneName, resourceProviderNamespace, locationName, location, nil)
	require.NoError(server.T(), err)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(server.T(), err)
}

func createResourceGroup(server *ucptesthost.TestHost) {
	ctx := context.Background()

	resourceGroup := v20231001preview.ResourceGroupResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.ResourceGroupProperties{},
	}

	client := server.UCP().NewResourceGroupsClient()
	_, err := client.CreateOrUpdate(ctx, radiusPlaneName, resourceGroupName, resourceGroup, nil)
	require.NoError(server.T(), err)
}
