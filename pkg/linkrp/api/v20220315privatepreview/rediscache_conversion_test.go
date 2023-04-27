// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/to"

	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"

	"github.com/stretchr/testify/require"
)

func TestRedisCache_ConvertVersionedToDataModel(t *testing.T) {
	testset := []struct {
		filename         string
		recipe           linkrp.LinkRecipe
		disableRecipe    bool
		overrideRecipe   bool
		host             string
		port             int
		connectionString string
		password         string
		resources        []linkrp.SupportingResources
	}{
		{
			// Default recipe
			filename: "rediscacheresource_defaultrecipe.json",
			recipe:   linkrp.LinkRecipe{Name: "", Parameters: nil},
		},
		{
			// Named recipe
			filename: "rediscacheresource_recipe.json",
			recipe:   linkrp.LinkRecipe{Name: "redis-test", Parameters: nil},
		},
		{
			// Named recipe with overridden values
			filename:       "rediscacheresource_recipe2.json",
			recipe:         linkrp.LinkRecipe{Name: "redis-test", Parameters: map[string]any{"port": float64(6081)}},
			overrideRecipe: true,
			port:           10255,
			host:           "myrediscache.redis.cache.windows.net",
		},
		{
			// Opt-out with resources
			filename:         "rediscacheresource.json",
			resources:        []linkrp.SupportingResources{{ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache"}},
			host:             "myrediscache.redis.cache.windows.net",
			port:             10255,
			connectionString: "test-connection-string",
			password:         "testPassword",
			disableRecipe:    true,
		},
		{
			// Opt-out without resources
			filename:      "rediscacheresource2.json",
			resources:     []linkrp.SupportingResources{{ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache"}, {ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1"}},
			host:          "myrediscache.redis.cache.windows.net",
			port:          10255,
			disableRecipe: true,
		},
	}

	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload.filename)
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
		require.Equal(t, linkrp.RedisCachesResourceType, convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		if !payload.disableRecipe {
			require.Equal(t, payload.recipe, convertedResource.Properties.Recipe)
			if payload.overrideRecipe {
				require.Equal(t, payload.host, convertedResource.Properties.Host)
				require.Equal(t, int32(payload.port), convertedResource.Properties.Port)
			}
		} else {
			require.Equal(t, linkrp.LinkRecipe{}, convertedResource.Properties.Recipe)
			require.Equal(t, payload.disableRecipe, convertedResource.Properties.DisableRecipe)
			require.Equal(t, payload.host, convertedResource.Properties.Host)
			require.Equal(t, int32(payload.port), convertedResource.Properties.Port)
			if convertedResource.Properties.Secrets.ConnectionString != "" {
				require.Equal(t, payload.connectionString, convertedResource.Properties.Secrets.ConnectionString)
			}
		}
	}
}

func TestRedisCache_ConvertDataModelToVersioned(t *testing.T) {
	testset := []struct {
		filename         string
		recipe           Recipe
		disableRecipe    bool
		overrideRecipe   bool
		host             string
		port             int32
		connectionString string
		password         string
		resources        []linkrp.SupportingResources
	}{
		{
			// Opt-out without resources
			filename:         "rediscacheresourcedatamodel.json",
			disableRecipe:    true,
			host:             "myrediscache.redis.cache.windows.net",
			port:             10255,
			connectionString: "test-connection-string",
			password:         "testPassword",
		},
		{
			// Default recipe
			filename: "rediscacheresourcedatamodel_recipe.json",
			recipe:   Recipe{Name: to.Ptr(""), Parameters: nil},
		},
		{
			// Named recipe
			filename: "rediscacheresourcedatamodel_recipe2.json",
			recipe:   Recipe{Name: to.Ptr("redis-test"), Parameters: map[string]any{"port": float64(6081)}},
		},
		{
			// Opt-out with resources
			filename:      "rediscacheresourcedatamodel.json",
			disableRecipe: true,
			host:          "myrediscache.redis.cache.windows.net",
			port:          10255,
			resources:     []linkrp.SupportingResources{{ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache"}, {ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1"}},
		},
	}

	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload.filename)
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
		require.Equal(t, linkrp.RedisCachesResourceType, *versionedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.Environment)
		if !payload.disableRecipe {
			require.Equal(t, payload.recipe, *versionedResource.Properties.Recipe)
		} else {
			require.Equal(t, payload.disableRecipe, *versionedResource.Properties.DisableRecipe)
			require.Equal(t, Recipe{Name: to.Ptr(""), Parameters: nil}, *versionedResource.Properties.Recipe)
			require.Equal(t, "myrediscache.redis.cache.windows.net", *versionedResource.Properties.Host)
			require.Equal(t, payload.port, *versionedResource.Properties.Port)
			require.Equal(t, "Deployment", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "azure", versionedResource.Properties.Status.OutputResources[0]["Provider"])
		}
	}
}

/*func TestRedisCache_ConvertVersionedToDataModel_InvalidRequest(t *testing.T) {
	testset := []string{"rediscacheresource-invalidmode.json", "rediscacheresource-invalidmode2.json", "rediscacheresource-invalidmode3.json", "rediscacheresource-invalidmode4.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		versionedResource := &RedisCacheResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)
		var expectedErr v1.ErrClientRP
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
}*/

func TestRedisCache_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
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
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &RedisCacheSecrets{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
