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

package rediscaches

import (
	"context"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"

	"github.com/stretchr/testify/require"
)

const (
	password         = "testpassword"
	connectionString = "test-connection-string"
)

func Test_Render_Success(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	redisResource := datamodel.RedisCache{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/redisCaches/redis0",
				Name: "redis0",
				Type: linkrp.RedisCachesResourceType,
			},
		},
		Properties: datamodel.RedisCacheProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			Mode: datamodel.LinkModeResource,
			RedisResourceProperties: datamodel.RedisResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/testGroup/providers/Microsoft.Cache/Redis/testCache",
			},
		},
	}
	expectedOutputResource := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureRedis,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureRedis,
			Provider: resourcemodel.ProviderAzure,
		},
		Identity: resourcemodel.ResourceIdentity{
			ResourceType: &resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRedis,
				Provider: resourcemodel.ProviderAzure,
			},
			Data: resourcemodel.ARMIdentity{
				ID:         "/subscriptions/test-sub/resourceGroups/testGroup/providers/Microsoft.Cache/Redis/testCache",
				APIVersion: clientv2.RedisManagementClientAPIVersion,
			},
		},
	}

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		renderers.Host: {
			LocalID:     rpv1.LocalIDAzureRedis,
			JSONPointer: "/properties/hostName",
		},
		renderers.Port: {
			LocalID:     rpv1.LocalIDAzureRedis,
			JSONPointer: "/properties/sslPort",
		},
	}
	expectedSecretValues := map[string]rpv1.SecretValueReference{
		renderers.PasswordStringHolder: {
			LocalID:       rpv1.LocalIDAzureRedis,
			Action:        "listKeys",
			ValueSelector: "/primaryKey",
		},
		renderers.ConnectionStringValue: {
			LocalID:       rpv1.LocalIDAzureRedis,
			Action:        "listKeys",
			ValueSelector: "/primaryKey",
			Transformer: resourcemodel.ResourceType{
				Provider: resourcemodel.ProviderAzure,
				Type:     resourcekinds.AzureRedis,
			},
		},
	}

	output, err := renderer.Render(ctx, &redisResource, renderers.RenderOptions{})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Equal(t, expectedOutputResource, output.Resources[0])
	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Equal(t, expectedSecretValues, output.SecretValues)
}

func Test_Render_UserSpecifiedValuesAndSecrets(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	redisResource := datamodel.RedisCache{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/redisCaches/redis0",
				Name: "redis0",
				Type: linkrp.RedisCachesResourceType,
			},
		},
		Properties: datamodel.RedisCacheProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			Mode: datamodel.LinkModeValues,
			RedisValuesProperties: datamodel.RedisValuesProperties{
				Host: "hello.com",
				Port: 1234,
			},
			Secrets: datamodel.RedisCacheSecrets{
				Password:         password,
				ConnectionString: connectionString,
			},
		},
	}

	output, err := renderer.Render(ctx, &redisResource, renderers.RenderOptions{})
	require.NoError(t, err)
	require.Len(t, output.Resources, 0)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		renderers.Host: {
			Value: "hello.com",
		},
		renderers.Port: {
			Value: int32(1234),
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)

	expectedSecretValues := map[string]rpv1.SecretValueReference{
		renderers.ConnectionStringValue: {Value: connectionString},
		renderers.PasswordStringHolder:  {Value: password},
	}
	require.Equal(t, expectedSecretValues, output.SecretValues)
}

func Test_Render_NoResourceSpecified(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	redisResource := datamodel.RedisCache{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/redisCaches/redis0",
				Name: "redis0",
				Type: linkrp.RedisCachesResourceType,
			},
		},
		Properties: datamodel.RedisCacheProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			Mode: datamodel.LinkModeResource,
		},
	}
	expectedErr := &v1.ErrClientRP{
		Code:    "BadRequest",
		Message: "the 'resource' field must be a valid resource id",
	}

	_, err := renderer.Render(ctx, &redisResource, renderers.RenderOptions{})
	require.Equal(t, expectedErr, err)
}

func Test_Render_InvalidResourceModel(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	redisResource := datamodel.SqlDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/mongoDatabases/mongo0",
				Name: "mongo0",
				Type: linkrp.MongoDatabasesResourceType,
			},
		},
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
		},
	}

	_, err := renderer.Render(ctx, &redisResource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, "invalid model conversion", err.Error())
}

func Test_Render_InvalidSourceResourceIdentifier(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	redisResource := datamodel.RedisCache{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/redisCaches/redis0",
				Name: "redis0",
				Type: linkrp.RedisCachesResourceType,
			},
		},
		Properties: datamodel.RedisCacheProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			Mode: datamodel.LinkModeResource,
			RedisResourceProperties: datamodel.RedisResourceProperties{
				Resource: "//subscriptions/test-sub/resourceGroups/testGroup/providers/Microsoft.Cache/Redis/testCache",
			},
		},
	}

	_, err := renderer.Render(ctx, &redisResource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "the 'resource' field must be a valid resource id", err.(*v1.ErrClientRP).Message)
}

func Test_Render_InvalidResourceType(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	redisResource := datamodel.RedisCache{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/redisCaches/redis0",
				Name: "redis0",
				Type: linkrp.RedisCachesResourceType,
			},
		},
		Properties: datamodel.RedisCacheProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			Mode: datamodel.LinkModeResource,
			RedisResourceProperties: datamodel.RedisResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/testGroup/providers/Microsoft.SomethingElse/Redis/testCache",
			},
		},
	}

	_, err := renderer.Render(ctx, &redisResource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "the 'resource' field must refer to an Azure Redis Cache", err.(*v1.ErrClientRP).Message)
}

func Test_Render_InvalidApplicationID(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	redisResource := datamodel.RedisCache{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/redisCaches/redis0",
				Name: "redis0",
				Type: linkrp.RedisCachesResourceType,
			},
		},
		Properties: datamodel.RedisCacheProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "invalid-app-id",
			},
			Mode: datamodel.LinkModeResource,
			RedisResourceProperties: datamodel.RedisResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/testGroup/providers/Microsoft.Cache/Redis/testCache",
			},
		},
	}
	_, err := renderer.Render(ctx, &redisResource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "failed to parse application from the property: 'invalid-app-id' is not a valid resource id", err.(*v1.ErrClientRP).Message)
}

func Test_Render_Recipe_Success(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	redisResource := datamodel.RedisCache{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/redisCaches/redis0",
				Name: "redis0",
				Type: linkrp.RedisCachesResourceType,
			},
		},
		Properties: datamodel.RedisCacheProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			Mode: datamodel.LinkModeRecipe,
			RedisRecipeProperties: datamodel.RedisRecipeProperties{
				Recipe: linkrp.LinkRecipe{
					Name: "redis",
					Parameters: map[string]any{
						"throughput": 400,
					},
				},
			},
		},
	}
	expectedOutputResource := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureRedis,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureRedis,
			Provider: resourcemodel.ProviderAzure,
		},
		ProviderResourceType: azresources.CacheRedis,
		RadiusManaged:        to.Ptr(true),
	}

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		renderers.Host: {
			LocalID:     rpv1.LocalIDAzureRedis,
			JSONPointer: "/properties/hostName",
		},
		renderers.Port: {
			LocalID:     rpv1.LocalIDAzureRedis,
			JSONPointer: "/properties/sslPort",
		},
	}
	expectedSecretValues := map[string]rpv1.SecretValueReference{
		renderers.PasswordStringHolder: {
			LocalID:       rpv1.LocalIDAzureRedis,
			Action:        "listKeys",
			ValueSelector: "/primaryKey",
		},
		renderers.ConnectionStringValue: {
			LocalID:       rpv1.LocalIDAzureRedis,
			Action:        "listKeys",
			ValueSelector: "/primaryKey",
			Transformer: resourcemodel.ResourceType{
				Provider: resourcemodel.ProviderAzure,
				Type:     resourcekinds.AzureRedis,
			},
		},
	}

	output, err := renderer.Render(ctx, &redisResource, renderers.RenderOptions{
		RecipeProperties: linkrp.RecipeProperties{
			LinkRecipe: linkrp.LinkRecipe{
				Name: "redis",
				Parameters: map[string]any{
					"throughput": 400,
				},
			},
			TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/redis:v1",
			LinkType:     linkrp.RedisCachesResourceType,
		}})

	require.NoError(t, err)
	// Recipe properties
	require.Equal(t, redisResource.Properties.Recipe.Name, output.RecipeData.Name)
	require.Equal(t, redisResource.Properties.Recipe.Parameters, output.RecipeData.Parameters)
	require.Equal(t, "testpublicrecipe.azurecr.io/bicep/modules/redis:v1", output.RecipeData.TemplatePath)
	require.Equal(t, clientv2.RedisManagementClientAPIVersion, output.RecipeData.APIVersion)

	// secrets and computed values
	require.Equal(t, expectedSecretValues, output.SecretValues)
	require.Equal(t, expectedComputedValues, output.ComputedValues)

	// output resources
	require.Len(t, output.Resources, 1)
	require.Equal(t, expectedOutputResource, output.Resources[0])
}

func Test_Render_Recipe_InvalidLinkType(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	redisResource := datamodel.RedisCache{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/redisCaches/redis0",
				Name: "redis0",
				Type: linkrp.RedisCachesResourceType,
			},
		},
		Properties: datamodel.RedisCacheProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			Mode: datamodel.LinkModeRecipe,
			RedisRecipeProperties: datamodel.RedisRecipeProperties{
				Recipe: linkrp.LinkRecipe{
					Name: "redis",
					Parameters: map[string]any{
						"throughput": 400,
					},
				},
			},
		},
	}

	_, err := renderer.Render(ctx, &redisResource, renderers.RenderOptions{
		RecipeProperties: linkrp.RecipeProperties{
			LinkRecipe: linkrp.LinkRecipe{
				Name: "redis",
			},
			TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/rediscaches:v1",
			LinkType:     linkrp.MongoDatabasesResourceType,
			EnvParameters: map[string]any{
				"throughput": 400,
			},
		}})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "link type \"Applications.Link/mongoDatabases\" of provided recipe \"redis\" is incompatible with \"Applications.Link/redisCaches\" resource type. Recipe link type must match link resource type.", err.(*v1.ErrClientRP).Message)
}
