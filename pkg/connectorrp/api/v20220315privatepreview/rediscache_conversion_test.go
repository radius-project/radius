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
	"github.com/stretchr/testify/require"
)

func TestRedisCache_ConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := loadTestData("rediscacheresource.json")
	versionedResource := &RedisCacheResource{}
	err := json.Unmarshal(rawPayload, versionedResource)
	require.NoError(t, err)

	// act
	dm, err := versionedResource.ConvertTo()

	resourceType := map[string]interface{}{"Provider": "azure", "Type": "azure.redis"}
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
	require.Equal(t, "test-connection-string", convertedResource.Properties.Secrets.ConnectionString)
	require.Equal(t, "testPassword", convertedResource.Properties.Secrets.Password)
	require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
	require.Equal(t, "Deployment", convertedResource.Properties.Status.OutputResources[0]["LocalID"])
	require.Equal(t, resourceType, convertedResource.Properties.Status.OutputResources[0]["ResourceType"])
}

func TestRedisCache_ConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := loadTestData("rediscacheresourcedatamodel.json")
	resource := &datamodel.RedisCache{}
	err := json.Unmarshal(rawPayload, resource)
	require.NoError(t, err)

	// act
	versionedResource := &RedisCacheResource{}
	err = versionedResource.ConvertFrom(resource)

	resourceType := map[string]interface{}{"Provider": "azure", "Type": "azure.redis"}
	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/redisCaches/redis0", resource.ID)
	require.Equal(t, "redis0", resource.Name)
	require.Equal(t, "Applications.Connector/redisCaches", resource.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache", resource.Properties.Resource)
	require.Equal(t, "myrediscache.redis.cache.windows.net", resource.Properties.Host)
	require.Equal(t, int32(10255), resource.Properties.Port)
	require.Equal(t, "test-connection-string", resource.Properties.Secrets.ConnectionString)
	require.Equal(t, "testPassword", resource.Properties.Secrets.Password)
	require.Equal(t, "Deployment", resource.Properties.Status.OutputResources[0]["LocalID"])
	require.Equal(t, resourceType, resource.Properties.Status.OutputResources[0]["ResourceType"])
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
