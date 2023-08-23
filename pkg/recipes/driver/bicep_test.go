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

package driver

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	gomock "github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	corerp_datamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/recipecontext"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	clients "github.com/project-radius/radius/pkg/sdk/clients"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
	resources_kubernetes "github.com/project-radius/radius/pkg/ucp/resources/kubernetes"
	"github.com/project-radius/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_CreateRecipeParameters_NoContextParameter(t *testing.T) {
	devParams := map[string]any{}
	operatorParams := map[string]any{}
	expectedParams := map[string]any{}

	actualParams := createRecipeParameters(devParams, operatorParams, false, nil)
	require.Equal(t, expectedParams, actualParams)
}

func Test_CreateRecipeParameters_ParameterConflict(t *testing.T) {
	devParams := map[string]any{
		"throughput": 400,
		"port":       2030,
		"name":       "test-parameters",
	}
	operatorParams := map[string]any{
		"port":     2040,
		"name":     "test-parameters-conflict",
		"location": "us-east1",
	}
	expectedParams := map[string]any{
		"throughput": map[string]any{
			"value": 400,
		},
		"port": map[string]any{
			"value": 2030,
		},
		"name": map[string]any{
			"value": "test-parameters",
		},
		"location": map[string]any{
			"value": "us-east1",
		},
	}

	actualParams := createRecipeParameters(devParams, operatorParams, false, nil)
	require.Equal(t, expectedParams, actualParams)
}

func Test_CreateRecipeParameters_WithContextParameter(t *testing.T) {
	devParams := map[string]any{
		"throughput": 400,
		"port":       2030,
		"name":       "test-parameters",
	}
	recipeContext := recipecontext.Context{
		Resource: recipecontext.Resource{
			ResourceInfo: recipecontext.ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "Applications.Link/mongoDatabases",
		},
		Application: recipecontext.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			Name: "testApplication",
		},
		Environment: recipecontext.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			Name: "env0",
		},
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				EnvironmentNamespace: "radius-test-env",
				Namespace:            "radius-test-app",
			},
		},
	}

	expectedParams := map[string]any{
		"throughput": map[string]any{
			"value": 400,
		},
		"port": map[string]any{
			"value": 2030,
		},
		"name": map[string]any{
			"value": "test-parameters",
		},
		"context": map[string]any{
			"value": recipeContext,
		},
	}
	actualParams := createRecipeParameters(devParams, nil, true, &recipeContext)
	require.Equal(t, expectedParams, actualParams)
}

func Test_CreateRecipeParameters_EmptyResourceParameters(t *testing.T) {
	operatorParams := map[string]any{
		"throughput": 400,
		"port":       2030,
		"name":       "test-parameters",
	}
	recipeContext := recipecontext.Context{
		Resource: recipecontext.Resource{
			ResourceInfo: recipecontext.ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "Applications.Link/mongoDatabases",
		},
		Application: recipecontext.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			Name: "testApplication",
		},
		Environment: recipecontext.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			Name: "env0",
		},
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				EnvironmentNamespace: "radius-test-env",
				Namespace:            "radius-test-app",
			},
		},
	}

	expectedParams := map[string]any{
		"throughput": map[string]any{
			"value": 400,
		},
		"port": map[string]any{
			"value": 2030,
		},
		"name": map[string]any{
			"value": "test-parameters",
		},
		"context": map[string]any{
			"value": recipeContext,
		},
	}
	actualParams := createRecipeParameters(nil, operatorParams, true, &recipeContext)
	require.Equal(t, expectedParams, actualParams)
}

func Test_CreateRecipeParameters_ResourceAndEnvParameters(t *testing.T) {
	operatorParams := map[string]any{
		"throughput": 400,
		"port":       2030,
		"name":       "test-parameters",
	}
	devParams := map[string]any{
		"throughput": 800,
		"port":       2060,
	}
	recipeContext := recipecontext.Context{
		Resource: recipecontext.Resource{
			ResourceInfo: recipecontext.ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "Applications.Link/mongoDatabases",
		},
		Application: recipecontext.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			Name: "testApplication",
		},
		Environment: recipecontext.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			Name: "env0",
		},
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				EnvironmentNamespace: "radius-test-env",
				Namespace:            "radius-test-app",
			},
		},
	}

	expectedParams := map[string]any{
		"throughput": map[string]any{
			"value": 800,
		},
		"port": map[string]any{
			"value": 2060,
		},
		"name": map[string]any{
			"value": "test-parameters",
		},
		"context": map[string]any{
			"value": recipeContext,
		},
	}
	actualParams := createRecipeParameters(devParams, operatorParams, true, &recipeContext)
	require.Equal(t, expectedParams, actualParams)
}

func Test_createDeploymentID(t *testing.T) {
	expected, err := resources.ParseResource("/planes/radius/local/resourceGroups/cool-group/providers/Microsoft.Resources/deployments/test-deployment")
	require.NoError(t, err)

	actual, err := createDeploymentID("/planes/radius/local/resourceGroups/cool-group/providers/Applications.Link/mongoDatabases/test-db", "test-deployment")
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func Test_createProviderConfig_defaults(t *testing.T) {
	expected := clients.NewDefaultProviderConfig("test-rg")
	actual := newProviderConfig("test-rg", corerp_datamodel.Providers{})
	require.Equal(t, expected, actual)
}

func Test_createProviderConfig_hasProviders(t *testing.T) {
	aws := "/planes/aws/aws/accounts/000/regions/cool-region"
	azure := "/subscriptions/000/resourceGroups/cool-azure-group"
	providers := corerp_datamodel.Providers{
		Azure: corerp_datamodel.ProvidersAzure{Scope: azure},
		AWS:   corerp_datamodel.ProvidersAWS{Scope: aws},
	}

	expected := clients.NewDefaultProviderConfig("test-rg")
	expected.Az = &clients.Az{
		Type:  clients.ProviderTypeAzure,
		Value: clients.Value{Scope: azure},
	}
	expected.AWS = &clients.AWS{
		Type:  clients.ProviderTypeAWS,
		Value: clients.Value{Scope: aws},
	}
	actual := newProviderConfig("test-rg", providers)
	require.Equal(t, expected, actual)
}

func Test_Bicep_PrepareRecipeResponse_Success(t *testing.T) {
	d := &bicepDriver{}

	resources := []*armresources.ResourceReference{
		{
			ID: to.Ptr("outputResourceId"),
		},
	}

	response := map[string]any{}
	value := map[string]any{}
	value["resources"] = []any{"testId1", "testId2"}
	value["secrets"] = map[string]any{
		"username":         "testUser",
		"password":         "testPassword",
		"connectionString": "test-connection-string",
	}
	value["values"] = map[string]any{
		"host": "myrediscache.redis.cache.windows.net",
		"port": float64(6379), // This will be a float64 not an int in real scenarios, it's read from JSON.
	}
	response["result"] = map[string]any{
		"value": value,
	}
	expectedResponse := &recipes.RecipeOutput{
		Resources: []string{"testId1", "testId2", "outputResourceId"},
		Secrets: map[string]any{
			"username":         "testUser",
			"password":         "testPassword",
			"connectionString": "test-connection-string",
		},
		Values: map[string]any{
			"host": "myrediscache.redis.cache.windows.net",
			"port": float64(6379),
		},
	}

	actualResponse, err := d.prepareRecipeResponse(response, resources)
	require.NoError(t, err)
	require.Equal(t, expectedResponse, actualResponse)
}

func Test_Bicep_PrepareRecipeResponse_EmptySecret(t *testing.T) {
	d := &bicepDriver{}

	resources := []*armresources.ResourceReference{
		{
			ID: to.Ptr("outputResourceId"),
		},
	}

	response := map[string]any{}
	value := map[string]any{}
	value["resources"] = []any{"testId1", "testId2"}
	value["values"] = map[string]any{
		"host": "myrediscache.redis.cache.windows.net",
		"port": float64(6379), // This will be a float64 not an int in real scenarios, it's read from JSON.
	}
	response["result"] = map[string]any{
		"value": value,
	}
	expectedResponse := &recipes.RecipeOutput{
		Resources: []string{"testId1", "testId2", "outputResourceId"},
		Secrets:   map[string]any{},
		Values: map[string]any{
			"host": "myrediscache.redis.cache.windows.net",
			"port": float64(6379),
		},
	}

	actualResponse, err := d.prepareRecipeResponse(response, resources)
	require.NoError(t, err)
	require.Equal(t, expectedResponse, actualResponse)
}

func Test_Bicep_PrepareRecipeResponse_EmptyResult(t *testing.T) {
	d := &bicepDriver{}

	resources := []*armresources.ResourceReference{
		{
			ID: to.Ptr("outputResourceId"),
		},
	}
	response := map[string]any{}
	expectedResponse := &recipes.RecipeOutput{
		Resources: []string{"outputResourceId"},
	}

	actualResponse, err := d.prepareRecipeResponse(response, resources)
	require.NoError(t, err)
	require.Equal(t, expectedResponse, actualResponse)
}

func setupDeleteInputs(t *testing.T) (bicepDriver, *processors.MockResourceClient) {
	ctrl := gomock.NewController(t)
	client := processors.NewMockResourceClient(ctrl)

	driver := bicepDriver{
		ResourceClient: client,
	}

	return driver, client
}

func Test_Bicep_Delete_Success(t *testing.T) {
	ctx := testcontext.New(t)
	driver, client := setupDeleteInputs(t)
	outputResources := []rpv1.OutputResource{
		{
			LocalID: "RecipeResource0",
			ID: resources_kubernetes.IDFromParts(
				resources_kubernetes.PlaneNameTODO,
				"apps",
				"Deployment",
				"recipe-app",
				"redis"),
			RadiusManaged: to.Ptr(true),
		},
		{
			LocalID: "RecipeResource1",
			ID: resources_kubernetes.IDFromParts(
				resources_kubernetes.PlaneNameTODO,
				"",
				"Service",
				"recipe-app",
				"redis"),
			// We don't expect a call to delete to be made when RadiusManaged is false.
			RadiusManaged: to.Ptr(false),
		},
	}
	client.EXPECT().Delete(ctx, "/planes/kubernetes/local/namespaces/recipe-app/providers/apps/Deployment/redis").Times(1).Return(nil)

	err := driver.Delete(ctx, outputResources)
	require.NoError(t, err)
}

func Test_Bicep_Delete_Error(t *testing.T) {
	ctx := testcontext.New(t)
	driver, client := setupDeleteInputs(t)
	outputResources := []rpv1.OutputResource{
		{
			ID: resources_kubernetes.IDFromParts(
				resources_kubernetes.PlaneNameTODO,
				"core",
				"Deployment",
				"recipe-app",
				"redis"),
			RadiusManaged: to.Ptr(true),
		},
	}
	recipeError := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeDeletionFailed,
			Message: fmt.Sprintf("could not find API version for type %q, no supported API versions", outputResources[0].GetResourceType().Type),
		},
	}
	client.EXPECT().
		Delete(ctx, "/planes/kubernetes/local/namespaces/recipe-app/providers/core/Deployment/redis").
		Return(fmt.Errorf("could not find API version for type %q, no supported API versions", outputResources[0].GetResourceType().Type)).
		Times(1)

	err := driver.Delete(ctx, outputResources)
	require.Error(t, err)
	require.Equal(t, err, &recipeError)
}
