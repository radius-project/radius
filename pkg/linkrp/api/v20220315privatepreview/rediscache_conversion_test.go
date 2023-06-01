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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/to"

	"github.com/stretchr/testify/require"
)

func TestRedisCache_ConvertVersionedToDataModel(t *testing.T) {
	testset := []struct {
		filename       string
		recipe         linkrp.LinkRecipe
		overrideRecipe bool
		resources      []*linkrp.ResourceReference
	}{
		{
			// Default recipe
			filename: "rediscacheresource_defaultrecipe.json",
			recipe:   linkrp.LinkRecipe{},
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
		},
		{
			// Opt-out with resources
			filename:  "rediscacheresource.json",
			resources: []*linkrp.ResourceReference{{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache"}},
		},
		{
			// Opt-out without resources
			filename: "rediscacheresource2.json",
		},
	}

	for _, payload := range testset {
		// arrange
		rawPayload, err := loadTestData("./testdata/" + payload.filename)
		require.NoError(t, err)
		versionedResource := &RedisCacheResource{}
		err = json.Unmarshal(rawPayload, versionedResource)
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
		if versionedResource.Properties.ResourceProvisioning == nil {
			require.Equal(t, payload.recipe, convertedResource.Properties.Recipe)
			require.Equal(t, linkrp.ResourceProvisioningRecipe, convertedResource.Properties.ResourceProvisioning)
			if payload.overrideRecipe {
				require.Equal(t, *versionedResource.Properties.Host, convertedResource.Properties.Host)
				require.Equal(t, int32(*versionedResource.Properties.Port), convertedResource.Properties.Port)
			}
		} else {
			require.Equal(t, linkrp.LinkRecipe{}, convertedResource.Properties.Recipe)
			require.Equal(t, linkrp.ResourceProvisioningManual, convertedResource.Properties.ResourceProvisioning)
			require.Equal(t, *versionedResource.Properties.Host, convertedResource.Properties.Host)
			require.Equal(t, int32(*versionedResource.Properties.Port), convertedResource.Properties.Port)
			require.Equal(t, payload.resources, convertedResource.Properties.Resources)
			if convertedResource.Properties.Secrets.ConnectionString != "" {
				require.Equal(t, *versionedResource.Properties.Secrets.ConnectionString, convertedResource.Properties.Secrets.ConnectionString)
			}
			if convertedResource.Properties.Secrets.Password != "" {
				require.Equal(t, *versionedResource.Properties.Secrets.Password, convertedResource.Properties.Secrets.Password)
			}
		}
	}
}

func TestRedisCache_ConvertDataModelToVersioned(t *testing.T) {
	testset := []struct {
		filename       string
		recipe         Recipe
		overrideRecipe bool
		resources      []*ResourceReference
	}{
		{
			// Opt-out without resources
			filename: "rediscacheresourcedatamodel.json",
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
			filename:  "rediscacheresourcedatamodel2.json",
			resources: []*ResourceReference{{ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache")}, {ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1")}},
		},
	}

	for _, payload := range testset {
		// arrange
		rawPayload, err := loadTestData("./testdata/" + payload.filename)
		require.NoError(t, err)
		resource := &datamodel.RedisCache{}
		err = json.Unmarshal(rawPayload, resource)
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
		if resource.Properties.ResourceProvisioning == "" {
			require.Equal(t, payload.recipe, *versionedResource.Properties.Recipe)
		} else {
			require.Equal(t, ResourceProvisioningManual, *versionedResource.Properties.ResourceProvisioning)
			require.Equal(t, Recipe{Name: to.Ptr(""), Parameters: nil}, *versionedResource.Properties.Recipe)
			require.Equal(t, resource.Properties.Host, *versionedResource.Properties.Host)
			require.Equal(t, resource.Properties.Port, *versionedResource.Properties.Port)
			require.ElementsMatch(t, payload.resources, versionedResource.Properties.Resources)
			if resource.Properties.Status.OutputResources != nil {
				require.Equal(t, "Deployment", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
				require.Equal(t, "azure", versionedResource.Properties.Status.OutputResources[0]["Provider"])
			}
		}
	}
}

func TestRedisCache_ConvertVersionedToDataModel_InvalidRequest(t *testing.T) {
	testset := []string{"rediscacheresource-invalid.json", "rediscacheresource-invalid2.json"}
	for _, payload := range testset {
		// arrange
		rawPayload, err := loadTestData("./testdata/" + payload)
		require.NoError(t, err)
		versionedResource := &RedisCacheResource{}
		err = json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)
		if payload == "rediscacheresource-invalid.json" {
			expectedErr := v1.ErrModelConversion{PropertyName: "$.properties.resourceProvisioning", ValidValue: fmt.Sprintf("one of %s", PossibleResourceProvisioningValues())}
			_, err = versionedResource.ConvertTo()
			require.Equal(t, &expectedErr, err)
		}
		if payload == "rediscacheresource-invalid2.json" {
			expectedErr := v1.ErrClientRP{Code: "Bad Request", Message: fmt.Sprintf("host and port are required when resourceProvisioning is %s", ResourceProvisioningManual)}
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
	rawPayload, err := loadTestData("./testdata/rediscachesecrets.json")
	require.NoError(t, err)
	versioned := &RedisCacheSecrets{}
	err = json.Unmarshal(rawPayload, versioned)
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
	rawPayload, err := loadTestData("./testdata/rediscachesecretsdatamodel.json")
	require.NoError(t, err)
	secrets := &datamodel.RedisCacheSecrets{}
	err = json.Unmarshal(rawPayload, secrets)
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
