// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/stretchr/testify/require"
)

func TestRedisCache_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"rediscacheresource.json", "rediscacheresource2.json"}
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
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/redisCaches/redis0", convertedResource.ID)
		require.Equal(t, "redis0", convertedResource.Name)
		require.Equal(t, "Applications.Connector/redisCaches", convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache", convertedResource.Properties.Resource)
		require.Equal(t, "myrediscache.redis.cache.windows.net", convertedResource.Properties.Host)
		require.Equal(t, int32(10255), convertedResource.Properties.Port)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		if payload == "rediscacheresource.json" {
			require.Equal(t, "test-connection-string", convertedResource.Properties.Secrets.ConnectionString)
			require.Equal(t, "testPassword", convertedResource.Properties.Secrets.Password)
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		}
	}
}

func TestRedisCacheRecipe_ConvertVersionedToDataModel(t *testing.T) {
	payload := "rediscacheresource3.json"

	rawPayload := loadTestData(payload)
	versionedResource := &RedisCacheResource{}
	err := json.Unmarshal(rawPayload, versionedResource)
	require.NoError(t, err)

	dm, err := versionedResource.ConvertTo()

	// assert
	require.NoError(t, err)
	convertedResource := dm.(*datamodel.RedisCache)
	require.Equal(t, "redis0", convertedResource.Name)
	require.Equal(t, "Applications.Connector/redisCaches", convertedResource.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
	require.Equal(t, "redis-test", convertedResource.Properties.Recipe.Name)
	require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
}

func TestRedisCacheRecipe_ConvertDataModelToVersioned(t *testing.T) {
	payload := "rediscacheresourcedatamodel3.json"

	rawPayload := loadTestData(payload)
	resource := &datamodel.RedisCache{}
	err := json.Unmarshal(rawPayload, resource)
	require.NoError(t, err)

	versionedResource := &RedisCacheResource{}
	err = versionedResource.ConvertFrom(resource)

	// assert
	require.NoError(t, err)
	require.Equal(t, "redis0", *versionedResource.Name)
	require.Equal(t, "Applications.Connector/redisCaches", *versionedResource.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.Application)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.Environment)
	require.Equal(t, "redis-test", *versionedResource.Properties.Recipe.Name)
}

func TestRedisCache_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"rediscacheresourcedatamodel.json", "rediscacheresourcedatamodel2.json"}
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
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/redisCaches/redis0", *versionedResource.ID)
		require.Equal(t, "redis0", *versionedResource.Name)
		require.Equal(t, "Applications.Connector/redisCaches", *versionedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.Environment)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache", *versionedResource.Properties.Resource)
		require.Equal(t, "myrediscache.redis.cache.windows.net", *versionedResource.Properties.Host)
		require.Equal(t, int32(10255), *versionedResource.Properties.Port)
		if payload == "rediscacheresourcedatamodel.json" {
			require.Equal(t, "test-connection-string", *versionedResource.Properties.Secrets.ConnectionString)
			require.Equal(t, "testPassword", *versionedResource.Properties.Secrets.Password)
			require.Equal(t, "Deployment", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "azure", versionedResource.Properties.Status.OutputResources[0]["Provider"])
		}
	}
}

func TestRedisCacheResponse_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"rediscacheresource.json", "rediscacheresource2.json"}
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
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/redisCaches/redis0", convertedResource.ID)
		require.Equal(t, "redis0", convertedResource.Name)
		require.Equal(t, "Applications.Connector/redisCaches", convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache", convertedResource.Properties.Resource)
		require.Equal(t, "myrediscache.redis.cache.windows.net", convertedResource.Properties.Host)
		require.Equal(t, int32(10255), convertedResource.Properties.Port)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		if payload == "rediscacheresource.json" {
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		}
	}
}

func TestRedisCacheResponse_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"rediscacheresourcedatamodel.json", "rediscacheresourcedatamodel2.json"}
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
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/redisCaches/redis0", *versionedResource.ID)
		require.Equal(t, "redis0", *versionedResource.Name)
		require.Equal(t, "Applications.Connector/redisCaches", *versionedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.Environment)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache", *versionedResource.Properties.Resource)
		require.Equal(t, "myrediscache.redis.cache.windows.net", *versionedResource.Properties.Host)
		require.Equal(t, int32(10255), *versionedResource.Properties.Port)
		if payload == "rediscacheresourcedatamodel.json" {
			require.Equal(t, "Deployment", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "azure", versionedResource.Properties.Status.OutputResources[0]["Provider"])
		}
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
