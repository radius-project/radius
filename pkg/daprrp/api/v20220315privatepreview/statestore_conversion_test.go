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
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/daprrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
)

func TestDaprStateStore_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{
		"statestoresqlserverresource.json",
		"statestoreazuretablestorageresource.json",
		"statestogenericreresource.json",
		"statestoreresource_recipe.json",
		"statestoreresource_recipe2.json"}

	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		versionedResource := &DaprStateStoreResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)

		// act
		dm, err := versionedResource.ConvertTo()

		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.DaprStateStore)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/stateStores/daprStateStore0", convertedResource.ID)
		require.Equal(t, "daprStateStore0", convertedResource.Name)
		require.Equal(t, linkrp.N_DaprStateStoresResourceType, convertedResource.Type)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		switch versionedResource.Properties.(type) {
		case *ResourceDaprStateStoreProperties:
			if payload == "statestoresqlserverresource.json" {
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", convertedResource.Properties.Resource)
				require.Equal(t, []rpv1.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
			} else {
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Storage/storageAccounts/tableServices/tables/testTable", convertedResource.Properties.Resource)
			}
		case *ValuesDaprStateStoreProperties:
			require.Equal(t, "state.zookeeper", convertedResource.Properties.Type)
			require.Equal(t, "v1", convertedResource.Properties.Version)
			require.Equal(t, "bar", convertedResource.Properties.Metadata["foo"])
			require.Equal(t, []rpv1.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		case *RecipeDaprStateStoreProperties:
			if payload == "statestoreresource_recipe2.json" {
				parameters := map[string]any{"port": float64(6081)}
				require.Equal(t, parameters, convertedResource.Properties.Recipe.Parameters)
			} else {
				require.Equal(t, "recipe-test", convertedResource.Properties.Recipe.Name)
			}
		}
	}

}

func TestDaprStateStore_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{
		"statestoresqlserverresourcedatamodel.json",
		"statestoreazuretablestorageresourcedatamodel.json",
		"statestogenericreresourcedatamodel.json",
		"statestoreresourcedatamodel_recipe.json",
		"statestoreresourcedatamodel_recipe2.json"}

	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		resource := &datamodel.DaprStateStore{}
		err := json.Unmarshal(rawPayload, resource)
		require.NoError(t, err)

		// act
		versionedResource := &DaprStateStoreResource{}
		err = versionedResource.ConvertFrom(resource)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/stateStores/daprStateStore0", resource.ID)
		require.Equal(t, "daprStateStore0", resource.Name)
		require.Equal(t, linkrp.N_DaprStateStoresResourceType, resource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)
		switch v := versionedResource.Properties.(type) {
		case *ResourceDaprStateStoreProperties:
			if payload == "statestoreazuretablestorageresourcedatamodel.json" {
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Storage/storageAccounts/tableServices/tables/testTable", resource.Properties.Resource)
				require.Equal(t, "Deployment", versionedResource.Properties.GetDaprStateStoreProperties().Status.OutputResources[0]["LocalID"])
				require.Equal(t, "kubernetes", versionedResource.Properties.GetDaprStateStoreProperties().Status.OutputResources[0]["Provider"])
			} else {
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", resource.Properties.Resource)
			}
		case *ValuesDaprStateStoreProperties:
			require.Equal(t, "state.zookeeper", *v.Type)
			require.Equal(t, "v1", *v.Version)
			require.Equal(t, "bar", v.Metadata["foo"])
			require.Equal(t, "Deployment", v.GetDaprStateStoreProperties().Status.OutputResources[0]["LocalID"])
			require.Equal(t, "kubernetes", v.GetDaprStateStoreProperties().Status.OutputResources[0]["Provider"])
		case *RecipeDaprStateStoreProperties:
			if payload == "statestoreresourcedatamodel_recipe2.json" {
				parameters := map[string]any{"port": float64(6081)}
				require.Equal(t, parameters, resource.Properties.Recipe.Parameters)
			} else {
				require.Equal(t, "recipe-test", resource.Properties.Recipe.Name)
			}
		}
	}

}

func TestDaprStateStore_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &DaprStateStoreResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
