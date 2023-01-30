// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"testing"

	corerp_datamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/sdk/clients"
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
	expectedLinkContext := linkrp.RecipeContext{
		Resource: linkrp.Resource{
			ResourceInfo: linkrp.ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "applications.link/mongodatabases",
		},
		Application: linkrp.ResourceInfo{
			Name: "testApplication",
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
		},
		Environment: linkrp.ResourceInfo{
			Name: "env0",
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
		},
		Runtime: linkrp.Runtime{
			Kubernetes: linkrp.Kubernetes{
				Namespace:            "radius-test-app",
				EnvironmentNamespace: "radius-test-env",
			},
		},
	}

	linkContext, err := CreateRecipeContextParameter(linkID, "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0", "radius-test-env", "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication", "radius-test-app")
	require.NoError(t, err)
	require.Equal(t, expectedLinkContext, *linkContext)
}

func Test_DevParameterWithContextParameter(t *testing.T) {
	devParams := map[string]any{
		"throughput": 400,
		"port":       2030,
		"name":       "test-parameters",
	}
	recipeContext := linkrp.RecipeContext{
		Resource: linkrp.Resource{
			ResourceInfo: linkrp.ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "Applications.Link/mongoDatabases",
		},
		Application: linkrp.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			Name: "testApplication",
		},
		Environment: linkrp.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			Name: "env0",
		},
		Runtime: linkrp.Runtime{
			Kubernetes: linkrp.Kubernetes{
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

func Test_ContextParameterError(t *testing.T) {
	envID := "error-env"
	linkContext, err := CreateRecipeContextParameter("/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0", envID, "radius-test-env", "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication", "radius-test-app")
	require.Error(t, err)
	require.Nil(t, linkContext)
}

func Test_ACRPathParser(t *testing.T) {
	repository, tag, err := parseTemplatePath("radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0")
	require.NoError(t, err)
	require.Equal(t, "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure", repository)
	require.Equal(t, "1.0", tag)
}

func Test_ACRPathParserErr(t *testing.T) {
	repository, tag, err := parseTemplatePath("http://user:passwd@example.com/test/bar:v1")
	require.Error(t, err)
	require.Equal(t, "", repository)
	require.Equal(t, "", tag)
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
		"port": 6379,
	}
	response["result"] = map[string]any{
		"value": value,
	}
	expectedResponse := RecipeResponse{
		Resources: []string{"outputResourceId", "testId1", "testId2"},
		Secrets: map[string]any{
			"username":         "testUser",
			"password":         "testPassword",
			"connectionString": "test-connection-string",
		},
		Values: map[string]any{
			"host": "myrediscache.redis.cache.windows.net",
			"port": 6379,
		},
	}
	actualResp := RecipeResponse{
		Resources: []string{"outputResourceId"},
		Secrets:   map[string]any{},
		Values:    map[string]any{},
	}
	prepareRecipeResponse(response, &actualResp)
	require.Equal(t, expectedResponse, actualResp)
}

func Test_RecipeResponseWithoutSecret(t *testing.T) {
	response := map[string]any{}
	value := map[string]any{}
	value["resources"] = []any{"testId1", "testId2"}
	value["values"] = map[string]any{
		"host": "myrediscache.redis.cache.windows.net",
		"port": 6379,
	}
	response["result"] = map[string]any{
		"value": value,
	}
	expectedResponse := RecipeResponse{
		Resources: []string{"outputResourceId", "testId1", "testId2"},
		Secrets:   map[string]any{},
		Values: map[string]any{
			"host": "myrediscache.redis.cache.windows.net",
			"port": 6379,
		},
	}
	actualResp := RecipeResponse{
		Resources: []string{"outputResourceId"},
		Secrets:   map[string]any{},
		Values:    map[string]any{},
	}
	prepareRecipeResponse(response, &actualResp)
	require.Equal(t, expectedResponse, actualResp)
}

func Test_RecipeResponseWithoutResult(t *testing.T) {
	response := map[string]any{}
	expectedResponse := RecipeResponse{
		Resources: []string{"outputResourceId"},
		Secrets:   map[string]any{},
		Values:    map[string]any{},
	}
	actualResp := RecipeResponse{
		Resources: []string{"outputResourceId"},
		Secrets:   map[string]any{},
		Values:    map[string]any{},
	}
	prepareRecipeResponse(response, &actualResp)
	require.Equal(t, expectedResponse, actualResp)
}
