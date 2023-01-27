// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"testing"

	"github.com/project-radius/radius/pkg/linkrp/datamodel"
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
	expectedLinkContext := datamodel.RecipeContext{
		Resource: datamodel.Resource{
			ResourceInfo: datamodel.ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "applications.link/mongodatabases",
		},
		Application: datamodel.ResourceInfo{
			Name: "testApplication",
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
		},
		Environment: datamodel.ResourceInfo{
			Name: "env0",
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
		},
		Runtime: datamodel.Runtime{
			Kubernetes: datamodel.Kubernetes{
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
	recipeContext := datamodel.RecipeContext{
		Resource: datamodel.Resource{
			ResourceInfo: datamodel.ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "Applications.Link/mongoDatabases",
		},
		Application: datamodel.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			Name: "testApplication",
		},
		Environment: datamodel.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			Name: "env0",
		},
		Runtime: datamodel.Runtime{
			Kubernetes: datamodel.Kubernetes{
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
