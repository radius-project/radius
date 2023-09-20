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

package v20220315privatepreview

import (
	"encoding/json"
	"fmt"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/testutil/resourcetypeutil"

	"github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

const (
	ApplicationID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication"
	EnvironmentID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0"
	RedisID       = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Datastores/redisCaches/redis0"
)

func TestRedisCache_ConvertVersionedToDataModel(t *testing.T) {
	testset := []struct {
		desc     string
		file     string
		expected *datamodel.RedisCache
	}{
		{
			desc: "redis cache with default recipe",
			file: "rediscacheresource_defaultrecipe.json",
			expected: &datamodel.RedisCache{
				BaseResource: createBaseResource(),
				Properties: datamodel.RedisCacheProperties{
					BasicResourceProperties: createBasicResourceProperties(),
					ResourceProvisioning:    portableresources.ResourceProvisioningRecipe,
					Host:                    "",
					Port:                    0,
					TLS:                     false,
					Username:                "",
					Recipe:                  portableresources.ResourceRecipe{Name: "default"},
				},
			},
		},
		{
			desc: "redis cache with named recipe",
			file: "rediscacheresource_recipe_named.json",
			expected: &datamodel.RedisCache{
				BaseResource: createBaseResource(),
				Properties: datamodel.RedisCacheProperties{
					BasicResourceProperties: createBasicResourceProperties(),
					ResourceProvisioning:    portableresources.ResourceProvisioningRecipe,
					Host:                    "",
					Port:                    0,
					TLS:                     false,
					Username:                "",
					Recipe:                  portableresources.ResourceRecipe{Name: "redis-test"},
				},
			},
		},
		{
			desc: "redis cache with recipe overridden values",
			file: "rediscacheresource_recipe_overridevalues.json",
			expected: &datamodel.RedisCache{
				BaseResource: createBaseResource(),
				Properties: datamodel.RedisCacheProperties{
					BasicResourceProperties: createBasicResourceProperties(),
					ResourceProvisioning:    portableresources.ResourceProvisioningRecipe,
					Host:                    "myrediscache.redis.cache.windows.net",
					Port:                    10255,
					TLS:                     false,
					Username:                "",
					Recipe:                  portableresources.ResourceRecipe{Name: "redis-test", Parameters: map[string]any{"port": float64(6081)}},
				},
			},
		},
		{
			desc: "redis cache manual with resources",
			file: "rediscacheresource_manual.json",
			expected: &datamodel.RedisCache{
				BaseResource: createBaseResource(),
				Properties: datamodel.RedisCacheProperties{
					BasicResourceProperties: createBasicResourceProperties(),
					ResourceProvisioning:    portableresources.ResourceProvisioningManual,
					Host:                    "myrediscache.redis.cache.windows.net",
					Port:                    10255,
					TLS:                     true,
					Username:                "admin",
					Resources:               []*portableresources.ResourceReference{{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache"}},
					Secrets: datamodel.RedisCacheSecrets{
						Password:         "testPassword",
						ConnectionString: "test-connection-string",
						URL:              "test-url",
					},
				},
			},
		},
		{
			desc: "redis cache manual without resources",
			file: "rediscacheresource_manual_noresources.json",
			expected: &datamodel.RedisCache{
				BaseResource: createBaseResource(),
				Properties: datamodel.RedisCacheProperties{
					BasicResourceProperties: createBasicResourceProperties(),
					ResourceProvisioning:    portableresources.ResourceProvisioningManual,
					Host:                    "myrediscache.redis.cache.windows.net",
					Port:                    10255,
					TLS:                     false,
					Username:                "",
				},
			},
		},
	}

	for _, tc := range testset {
		// arrange
		t.Run(tc.desc, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tc.file)
			versionedResource := &RedisCacheResource{}
			err := json.Unmarshal(rawPayload, versionedResource)
			require.NoError(t, err)

			// act
			dm, err := versionedResource.ConvertTo()

			// assert
			require.NoError(t, err)
			convertedResource := dm.(*datamodel.RedisCache)

			require.Equal(t, tc.expected, convertedResource)
		})
	}
}

func TestRedisCache_ConvertDataModelToVersioned(t *testing.T) {
	testset1 := []struct {
		desc     string
		file     string
		expected *RedisCacheResource
	}{
		{
			desc: "redis cache manual with resources",
			file: "rediscacheresourcedatamodel_manual.json",
			expected: &RedisCacheResource{
				Location: to.Ptr(""),
				Properties: &RedisCacheProperties{
					Environment:          to.Ptr(EnvironmentID),
					Application:          to.Ptr(ApplicationID),
					ResourceProvisioning: to.Ptr(ResourceProvisioningManual),
					Host:                 to.Ptr("myrediscache.redis.cache.windows.net"),
					Port:                 to.Ptr(int32(10255)),
					ProvisioningState:    to.Ptr(ProvisioningStateAccepted),
					Recipe:               &Recipe{Name: to.Ptr(""), Parameters: nil},
					Username:             to.Ptr(""),
					TLS:                  to.Ptr(false),
					Status:               resourcetypeutil.MustPopulateResourceStatus(&ResourceStatus{}),
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr(RedisID),
				Name: to.Ptr("redis0"),
				Type: to.Ptr(portableresources.RedisCachesResourceType),
			},
		},
		{
			desc: "redis cache default recipe",
			file: "rediscacheresourcedatamodel_recipe_default.json",
			expected: &RedisCacheResource{
				Location: to.Ptr(""),
				Properties: &RedisCacheProperties{
					Environment:          to.Ptr(EnvironmentID),
					Application:          to.Ptr(ApplicationID),
					ResourceProvisioning: to.Ptr(ResourceProvisioningRecipe),
					Host:                 to.Ptr(""),
					Port:                 to.Ptr(int32(0)),
					ProvisioningState:    to.Ptr(ProvisioningStateAccepted),
					Recipe:               &Recipe{Name: to.Ptr(""), Parameters: nil},
					Username:             to.Ptr(""),
					TLS:                  to.Ptr(false),
					Status:               resourcetypeutil.MustPopulateResourceStatus(&ResourceStatus{}),
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr(RedisID),
				Name: to.Ptr("redis0"),
				Type: to.Ptr(portableresources.RedisCachesResourceType),
			},
		},
		{
			desc: "redis cache named recipe",
			file: "rediscacheresourcedatamodel_recipe_params.json",
			expected: &RedisCacheResource{
				Location: to.Ptr(""),
				Properties: &RedisCacheProperties{
					Environment:          to.Ptr(EnvironmentID),
					Application:          to.Ptr(ApplicationID),
					ResourceProvisioning: to.Ptr(ResourceProvisioningRecipe),
					Host:                 to.Ptr(""),
					Port:                 to.Ptr(int32(0)),
					ProvisioningState:    to.Ptr(ProvisioningStateAccepted),
					Recipe:               &Recipe{Name: to.Ptr("redis-test"), Parameters: map[string]any{"port": float64(6081)}},
					Username:             to.Ptr(""),
					TLS:                  to.Ptr(false),
					Status:               resourcetypeutil.MustPopulateResourceStatus(&ResourceStatus{}),
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr(RedisID),
				Name: to.Ptr("redis0"),
				Type: to.Ptr(portableresources.RedisCachesResourceType),
			},
		},
		{
			desc: "redis cache manual with resources",
			file: "rediscacheresourcedatamodel_manual_resources.json",
			expected: &RedisCacheResource{
				Location: to.Ptr(""),
				Properties: &RedisCacheProperties{
					Environment:          to.Ptr(EnvironmentID),
					Application:          to.Ptr(ApplicationID),
					ResourceProvisioning: to.Ptr(ResourceProvisioningManual),
					Host:                 to.Ptr("myrediscache.redis.cache.windows.net"),
					Port:                 to.Ptr(int32(10255)),
					ProvisioningState:    to.Ptr(ProvisioningStateAccepted),
					Recipe:               &Recipe{Name: to.Ptr(""), Parameters: nil},
					Username:             to.Ptr(""),
					TLS:                  to.Ptr(true),
					Status: &ResourceStatus{
						OutputResources: nil,
					},
					Resources: []*ResourceReference{
						{ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache")},
						{ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1")},
					},
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr(RedisID),
				Name: to.Ptr("redis0"),
				Type: to.Ptr(portableresources.RedisCachesResourceType),
			},
		},
	}

	for _, tc := range testset1 {
		t.Run(tc.desc, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tc.file)
			resource := &datamodel.RedisCache{}
			err := json.Unmarshal(rawPayload, resource)
			require.NoError(t, err)

			versionedResource := &RedisCacheResource{}
			err = versionedResource.ConvertFrom(resource)
			require.NoError(t, err)

			// Skip system data comparison
			versionedResource.SystemData = nil

			require.Equal(t, tc.expected, versionedResource)
		})
	}
}

func TestRedisCache_ConvertVersionedToDataModel_InvalidRequest(t *testing.T) {
	testset := []string{"rediscacheresource-invalid.json", "rediscacheresource-invalid2.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := testutil.ReadFixture(payload)
		versionedResource := &RedisCacheResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)
		if payload == "rediscacheresource-invalid.json" {
			expectedErr := v1.ErrModelConversion{PropertyName: "$.properties.resourceProvisioning", ValidValue: fmt.Sprintf("one of %s", PossibleResourceProvisioningValues())}
			_, err = versionedResource.ConvertTo()
			require.Equal(t, &expectedErr, err)
		}
		if payload == "rediscacheresource-invalid2.json" {
			expectedErr := v1.ErrClientRP{Code: "BadRequest", Message: "multiple errors were found:\n\thost must be specified when resourceProvisioning is set to manual\n\tport must be specified when resourceProvisioning is set to manual"}
			_, err = versionedResource.ConvertTo()
			require.Equal(t, &expectedErr, err)
		}
	}
}

func TestRedisCache_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&resourcetypeutil.FakeResource{}, v1.ErrInvalidModelConversion},
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
	rawPayload := testutil.ReadFixture("/rediscachesecrets.json")
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
	require.Equal(t, "test-url", converted.URL)
}

func TestRedisCacheSecrets_ConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("rediscachesecretsdatamodel.json")
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
	require.Equal(t, "test-url", secrets.URL)
}

func TestRedisCacheSecrets_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&resourcetypeutil.FakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &RedisCacheSecrets{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}

func createBaseResource() v1.BaseResource {
	return v1.BaseResource{
		TrackedResource: v1.TrackedResource{
			ID:   RedisID,
			Name: "redis0",
			Type: portableresources.RedisCachesResourceType,
			Tags: map[string]string{},
		},
		InternalMetadata: v1.InternalMetadata{
			CreatedAPIVersion:      "",
			UpdatedAPIVersion:      "2022-03-15-privatepreview",
			AsyncProvisioningState: v1.ProvisioningStateAccepted,
		},
		SystemData: v1.SystemData{},
	}
}

func createBasicResourceProperties() rpv1.BasicResourceProperties {
	return rpv1.BasicResourceProperties{
		Application: ApplicationID,
		Environment: EnvironmentID,
	}
}
