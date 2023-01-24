// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/redis/mgmt/redis"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
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
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			Mode: datamodel.LinkModeResource,
			RedisResourceProperties: datamodel.RedisResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/testGroup/providers/Microsoft.Cache/Redis/testCache",
			},
		},
	}
	expectedOutputResource := outputresource.OutputResource{
		LocalID: outputresource.LocalIDAzureRedis,
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
				APIVersion: clients.GetAPIVersionFromUserAgent(redis.UserAgent()),
			},
		},
	}

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		renderers.Host: {
			LocalID:     outputresource.LocalIDAzureRedis,
			JSONPointer: "/properties/hostName",
		},
		renderers.Port: {
			LocalID:     outputresource.LocalIDAzureRedis,
			JSONPointer: "/properties/sslPort",
		},
	}
	expectedSecretValues := map[string]rp.SecretValueReference{
		renderers.PasswordStringHolder: {
			LocalID:       outputresource.LocalIDAzureRedis,
			Action:        "listKeys",
			ValueSelector: "/primaryKey",
		},
		renderers.ConnectionStringValue: {
			LocalID:       outputresource.LocalIDAzureRedis,
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
			BasicResourceProperties: rp.BasicResourceProperties{
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

	expectedSecretValues := map[string]rp.SecretValueReference{
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
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			},
			Mode: datamodel.LinkModeResource,
		},
	}

	_, err := renderer.Render(ctx, &redisResource, renderers.RenderOptions{})
	require.NoError(t, err)
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
			BasicResourceProperties: rp.BasicResourceProperties{
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
			BasicResourceProperties: rp.BasicResourceProperties{
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
			BasicResourceProperties: rp.BasicResourceProperties{
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
			BasicResourceProperties: rp.BasicResourceProperties{
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
