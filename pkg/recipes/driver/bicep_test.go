/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package driver

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	corerp_datamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	clients "github.com/project-radius/radius/pkg/sdk/clients"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func Test_ParameterConflict(t *testing.T) {
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

func Test_ContextParameter(t *testing.T) {
	linkID := "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0"
	expectedLinkContext := RecipeContext{
		Resource: Resource{
			ResourceInfo: ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "applications.link/mongodatabases",
		},
		Application: ResourceInfo{
			Name: "testApplication",
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
		},
		Environment: ResourceInfo{
			Name: "env0",
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
		},
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace:            "radius-test-app",
				EnvironmentNamespace: "radius-test-env",
			},
		},
	}

	linkContext, err := createRecipeContextParameter(linkID, "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0", "radius-test-env", "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication", "radius-test-app")
	require.NoError(t, err)
	require.Equal(t, expectedLinkContext, *linkContext)
}

func Test_DevParameterWithContextParameter(t *testing.T) {
	devParams := map[string]any{
		"throughput": 400,
		"port":       2030,
		"name":       "test-parameters",
	}
	recipeContext := RecipeContext{
		Resource: Resource{
			ResourceInfo: ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "Applications.Link/mongoDatabases",
		},
		Application: ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			Name: "testApplication",
		},
		Environment: ResourceInfo{
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

func Test_EmptyDevParameterWithOperatorParameter(t *testing.T) {
	operatorParams := map[string]any{
		"throughput": 400,
		"port":       2030,
		"name":       "test-parameters",
	}
	recipeContext := RecipeContext{
		Resource: Resource{
			ResourceInfo: ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "Applications.Link/mongoDatabases",
		},
		Application: ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			Name: "testApplication",
		},
		Environment: ResourceInfo{
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

func Test_DevParameterWithOperatorParameter(t *testing.T) {
	operatorParams := map[string]any{
		"throughput": 400,
		"port":       2030,
		"name":       "test-parameters",
	}
	devParams := map[string]any{
		"throughput": 800,
		"port":       2060,
	}
	recipeContext := RecipeContext{
		Resource: Resource{
			ResourceInfo: ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "Applications.Link/mongoDatabases",
		},
		Application: ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			Name: "testApplication",
		},
		Environment: ResourceInfo{
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
func Test_ContextParameterError(t *testing.T) {
	envID := "error-env"
	linkContext, err := createRecipeContextParameter("/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0", envID, "radius-test-env", "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication", "radius-test-app")
	require.Error(t, err)
	require.Nil(t, linkContext)
}

func Test_createDeploymentID(t *testing.T) {
	expected, err := resources.ParseResource("/planes/deployments/local/resourceGroups/cool-group/providers/Microsoft.Resources/deployments/test-deployment")
	require.NoError(t, err)

	actual, err := createDeploymentID("/planes/radius/local/resourceGroups/cool-group/providers/Applications.Link/mongoDatabases/test-db", "test-deployment")
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func Test_createProviderConfig_defaults(t *testing.T) {
	expected := clients.NewDefaultProviderConfig("test-rg")
	actual := createProviderConfig("test-rg", corerp_datamodel.Providers{})
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
	actual := createProviderConfig("test-rg", providers)
	require.Equal(t, expected, actual)
}
func Test_RecipeResponseSuccess(t *testing.T) {
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
	expectedResponse := recipes.RecipeOutput{
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

	actualResponse, err := prepareRecipeResponse(response, resources)
	require.NoError(t, err)
	require.Equal(t, expectedResponse, actualResponse)
}

func Test_RecipeResponseWithoutSecret(t *testing.T) {
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
	expectedResponse := recipes.RecipeOutput{
		Resources: []string{"testId1", "testId2", "outputResourceId"},
		Secrets:   map[string]any{},
		Values: map[string]any{
			"host": "myrediscache.redis.cache.windows.net",
			"port": float64(6379),
		},
	}

	actualResponse, err := prepareRecipeResponse(response, resources)
	require.NoError(t, err)
	require.Equal(t, expectedResponse, actualResponse)
}

func Test_RecipeResponseWithoutResult(t *testing.T) {
	resources := []*armresources.ResourceReference{
		{
			ID: to.Ptr("outputResourceId"),
		},
	}
	response := map[string]any{}
	expectedResponse := recipes.RecipeOutput{
		Resources: []string{"outputResourceId"},
		Secrets:   map[string]any{},
		Values:    map[string]any{},
	}

	actualResponse, err := prepareRecipeResponse(response, resources)
	require.NoError(t, err)
	require.Equal(t, expectedResponse, actualResponse)
}
