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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaprStateStore_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{
		"daprstatestoresqlserverresource.json",
		"daprstatestoreazuretablestorageresource.json",
		"daprstatestogenericreresource.json",
		"daprstatestoresqlserverresource_recipe.json",
		"daprstatestoresqlserverresource_recipe2.json",
		"daprstatestoreazuretablestorageresource_recipe.json",
		"daprstatestoreazuretablestorageresource_recipe2.json",
		"daprstatestogenericreresource_recipe.json",
		"daprstatestogenericreresource_recipe2.json"}

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
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/daprStateStores/daprStateStore0", convertedResource.ID)
		require.Equal(t, "daprStateStore0", convertedResource.Name)
		require.Equal(t, "Applications.Link/daprStateStores", convertedResource.Type)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		if convertedResource.Properties.Mode != datamodel.DaprStateStoreModeRecipe {
			switch convertedResource.Properties.Kind {
			case datamodel.DaprStateStoreKindAzureTableStorage:
				if convertedResource.Properties.Mode == datamodel.DaprStateStoreModeResource {
					require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Storage/storageAccounts/tableServices/tables/testTable", convertedResource.Properties.Resource)
					require.Equal(t, "state.azure.tablestorage", string(convertedResource.Properties.Kind))
				}

			case datamodel.DaprStateStoreKindStateSqlServer:
				if convertedResource.Properties.Mode == datamodel.DaprStateStoreModeResource {
					require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", convertedResource.Properties.Resource)
					require.Equal(t, "state.sqlserver", string(convertedResource.Properties.Kind))
					require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
				}

			case datamodel.DaprStateStoreKindGeneric:
				if convertedResource.Properties.Mode == datamodel.DaprStateStoreModeValues {
					require.Equal(t, "generic", string(convertedResource.Properties.Kind))
					require.Equal(t, "state.zookeeper", convertedResource.Properties.Type)
					require.Equal(t, "v1", convertedResource.Properties.Version)
					require.Equal(t, "bar", convertedResource.Properties.Metadata["foo"])
					require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
				}
			default:
				assert.Fail(t, "Kind of DaprStateStore is specified.")
			}
		}

		if payload == "daprstatestoresqlserverresource_recipe.json" ||
			payload == "daprstatestoresqlserverresource_recipe2.json" ||
			payload == "daprstatestoreazuretablestorageresource_recipe.json" ||
			payload == "daprstatestoreazuretablestorageresource_recipe2.json" ||
			payload == "daprstatestogenericreresource_recipe.json" ||
			payload == "daprstatestogenericreresource_recipe2.json" {
			require.Equal(t, "recipe-test", convertedResource.Properties.Recipe.Name)
			if payload == "daprstatestoresqlserverresource_recipe2.json" ||
				payload == "daprstatestoreazuretablestorageresource_recipe2.json" ||
				payload == "daprstatestogenericreresource_recipe2.json" {
				parameters := map[string]interface{}{"port": float64(6081)}
				require.Equal(t, parameters, convertedResource.Properties.Recipe.Parameters)
			}
		}
	}

}

func TestDaprStateStore_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{
		"daprstatestoresqlserverresourcedatamodel.json",
		"daprstatestoreazuretablestorageresourcedatamodel.json",
		"daprstatestogenericreresourcedatamodel.json",
		"daprstatestoresqlserverresourcedatamodel_recipe.json",
		"daprstatestoresqlserverresourcedatamodel_recipe2.json",
		"daprstatestoreazuretablestorageresourcedatamodel_recipe.json",
		"daprstatestoreazuretablestorageresourcedatamodel_recipe2.json",
		"daprstatestogenericreresourcedatamodel_recipe.json",
		"daprstatestogenericreresourcedatamodel_recipe2.json"}

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
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Link/daprStateStores/daprStateStore0", resource.ID)
		require.Equal(t, "daprStateStore0", resource.Name)
		require.Equal(t, "Applications.Link/daprStateStores", resource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)
		if resource.Properties.Mode != datamodel.DaprStateStoreModeRecipe {
			switch resource.Properties.Kind {
			case datamodel.DaprStateStoreKindAzureTableStorage:
				if resource.Properties.Mode == datamodel.DaprStateStoreModeResource {
					require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Storage/storageAccounts/tableServices/tables/testTable", resource.Properties.Resource)
					require.Equal(t, "state.azure.tablestorage", string(resource.Properties.Kind))
					require.Equal(t, "Deployment", versionedResource.Properties.GetDaprStateStoreProperties().Status.OutputResources[0]["LocalID"])
					require.Equal(t, "kubernetes", versionedResource.Properties.GetDaprStateStoreProperties().Status.OutputResources[0]["Provider"])
				}
			case datamodel.DaprStateStoreKindStateSqlServer:
				if resource.Properties.Mode == datamodel.DaprStateStoreModeResource {
					require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", resource.Properties.Resource)
					require.Equal(t, "state.sqlserver", string(resource.Properties.Kind))
				}
			case datamodel.DaprStateStoreKindGeneric:
				if resource.Properties.Mode == datamodel.DaprStateStoreModeValues {
					require.Equal(t, "generic", string(resource.Properties.Kind))
					require.Equal(t, "state.zookeeper", resource.Properties.Type)
					require.Equal(t, "v1", resource.Properties.Version)
					require.Equal(t, "bar", resource.Properties.Metadata["foo"])
					require.Equal(t, "Deployment", versionedResource.Properties.GetDaprStateStoreProperties().Status.OutputResources[0]["LocalID"])
					require.Equal(t, "kubernetes", versionedResource.Properties.GetDaprStateStoreProperties().Status.OutputResources[0]["Provider"])
				}
			default:
				assert.Fail(t, "Kind of DaprStateStore is specified.")
			}
		}

		if payload == "daprstatestoresqlserverresourcedatamodel_recipe.json" ||
			payload == "daprstatestoresqlserverresourcedatamodel_recipe2.json" ||
			payload == "daprstatestoreazuretablestorageresourcedatamodel_recipe.json" ||
			payload == "daprstatestoreazuretablestorageresourcedatamodel_recipe2.json" ||
			payload == "daprstatestogenericreresourcedatamodel_recipe.json" ||
			payload == "daprstatestogenericreresourcedatamodel_recipe2.json" {
			require.Equal(t, "recipe-test", resource.Properties.Recipe.Name)
			if payload == "daprstatestoresqlserverresourcedatamodel_recipe2.json" ||
				payload == "daprstatestoreazuretablestorageresourcedatamodel_recipe2.json" ||
				payload == "daprstatestogenericreresourcedatamodel_recipe2.json" {
				parameters := map[string]interface{}{"port": float64(6081)}
				require.Equal(t, parameters, resource.Properties.Recipe.Parameters)
			}
		}
	}

}

func TestDaprStateStore_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &DaprStateStoreResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
