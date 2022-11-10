// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/stretchr/testify/require"
)

func TestRedisCache_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"rediscacheresource.json", "rediscacheresource2.json", "rediscacheresource3.json", "rediscacheresource_recipe.json", "rediscacheresource_recipe2.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		versionedResource := &RedisCacheResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)

		// act
		dm, err := versionedResource.ConvertTo()

		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.RedisCache)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/redisCaches/redis0", convertedResource.ID)
		require.Equal(t, "redis0", convertedResource.Name)
		require.Equal(t, "Applications.Link/redisCaches", convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		switch v := versionedResource.Properties.(type) {
		case *ResourceRedisCacheProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache", convertedResource.Properties.Resource)
			require.Equal(t, "myrediscache.redis.cache.windows.net", convertedResource.Properties.Host)
			require.Equal(t, int32(10255), convertedResource.Properties.Port)
			require.Equal(t, datamodel.LinkModeResource, convertedResource.Properties.Mode)
			if payload == "rediscacheresource.json" {
				require.Equal(t, "test-connection-string", convertedResource.Properties.Secrets.ConnectionString)
				require.Equal(t, "testPassword", convertedResource.Properties.Secrets.Password)
				require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
			}
		case *RecipeRedisCacheProperties:
			require.Equal(t, "redis-test", convertedResource.Properties.Recipe.Name)
			if payload == "rediscacheresource_recipe2.json" {
				parameters := map[string]interface{}{"port": float64(6081)}
				require.Equal(t, parameters, convertedResource.Properties.Recipe.Parameters)
				require.Equal(t, "myrediscache.redis.cache.windows.net", convertedResource.Properties.Host)
				require.Equal(t, int32(10255), convertedResource.Properties.Port)
			}
			require.Equal(t, datamodel.LinkModeRecipe, convertedResource.Properties.Mode)
		case *ValuesRedisCacheProperties:
			require.Equal(t, "myrediscache.redis.cache.windows.net", *v.Host)
			require.Equal(t, int32(10255), *v.Port)
			require.Equal(t, "test-connection-string", convertedResource.Properties.Secrets.ConnectionString)
			require.Equal(t, "testPassword", convertedResource.Properties.Secrets.Password)
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
			require.Equal(t, datamodel.LinkModeValues, convertedResource.Properties.Mode)
		}
	}
}

func TestRedisCache_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"rediscacheresourcedatamodel.json", "rediscacheresourcedatamodel2.json", "rediscacheresourcedatamodel_recipe.json", "rediscacheresourcedatamodel_recipe2.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		resource := &datamodel.RedisCache{}
		err := json.Unmarshal(rawPayload, resource)
		require.NoError(t, err)

		// act
		versionedResource := &RedisCacheResource{}
		err = versionedResource.ConvertFrom(resource)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/redisCaches/redis0", *versionedResource.ID)
		require.Equal(t, "redis0", *versionedResource.Name)
		require.Equal(t, "Applications.Link/redisCaches", *versionedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.GetRedisCacheProperties().Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.GetRedisCacheProperties().Environment)
		switch v := versionedResource.Properties.(type) {
		case *ResourceRedisCacheProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache", *v.Resource)
			require.Equal(t, "myrediscache.redis.cache.windows.net", *v.Host)
			require.Equal(t, int32(10255), *v.Port)
			if payload == "rediscacheresourcedatamodel.json" {
				require.Equal(t, "Deployment", v.Status.OutputResources[0]["LocalID"])
				require.Equal(t, "azure", v.Status.OutputResources[0]["Provider"])
			}
		case *RecipeRedisCacheProperties:
			require.Equal(t, "redis-test", *v.Recipe.Name)
			if payload == "rediscacheresourcedatamodel_recipe2.json" {
				parameters := map[string]interface{}{"port": float64(6081)}
				require.Equal(t, parameters, v.Recipe.Parameters)
			}
		case *ValuesRedisCacheProperties:
			require.Equal(t, "testAccount1.mongo.cosmos.azure.com", *v.Host)
			require.Equal(t, int32(10255), *v.Port)
			require.Equal(t, "Deployment", v.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "azure", v.Status.OutputResources[0]["Provider"])
		}
	}
}

func TestRedisCache_ConvertVersionedToDataModel_InvalidRequest(t *testing.T) {
	testset := []string{"rediscacheresource-invalidmode.json", "rediscacheresource-invalidmode2.json", "rediscacheresource-invalidmode3.json", "rediscacheresource-invalidmode4.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		versionedResource := &RedisCacheResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)
		var expectedErr conv.ErrClientRP
		if payload == "rediscacheresource-invalidmode.json" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "Unsupported mode abc"
		}
		if payload == "rediscacheresource-invalidmode2.json" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "resource is a required property for mode \"resource\""
		}
		if payload == "rediscacheresource-invalidmode3.json" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "recipe is a required property for mode \"recipe\""
		}
		if payload == "rediscacheresource-invalidmode4.json" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "host and port are required properties for mode \"values\""
		}
		_, err = versionedResource.ConvertTo()
		require.Equal(t, &expectedErr, err)
	}
}

func TestRedisCache_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &RedisCacheResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}

func TestRedisCacheSecrets_ConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := loadTestData("rediscachesecrets.json")
	versioned := &RedisCacheSecrets{}
	err := json.Unmarshal(rawPayload, versioned)
	require.NoError(t, err)

	// act
	dm, err := versioned.ConvertTo()

	// assert
	require.NoError(t, err)
	converted := dm.(*datamodel.RedisCacheSecrets)
	require.Equal(t, "test-connection-string", converted.ConnectionString)
	require.Equal(t, "testPassword", converted.Password)
}

func TestRedisCacheSecrets_ConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := loadTestData("rediscachesecretsdatamodel.json")
	secrets := &datamodel.RedisCacheSecrets{}
	err := json.Unmarshal(rawPayload, secrets)
	require.NoError(t, err)

	// act
	versionedResource := &RedisCacheSecrets{}
	err = versionedResource.ConvertFrom(secrets)

	// assert
	require.NoError(t, err)
	require.Equal(t, "test-connection-string", secrets.ConnectionString)
	require.Equal(t, "testPassword", secrets.Password)
}

func TestRedisCacheSecrets_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &RedisCacheSecrets{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
