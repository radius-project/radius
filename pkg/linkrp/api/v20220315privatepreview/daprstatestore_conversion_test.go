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

func TestDaprStateStore_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{
		"daprstatestoresqlserverresource.json",
		"daprstatestoreazuretablestorageresource.json",
		"daprstatestogenericreresource.json",
		"daprstatestoreresource_recipe.json",
		"daprstatestoreresource_recipe2.json"}

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
		switch versionedResource.Properties.(type) {
		case *ResourceDaprStateStoreProperties:
			if payload == "daprstatestoresqlserverresource.json" {
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", convertedResource.Properties.Resource)
				require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
			} else {
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Storage/storageAccounts/tableServices/tables/testTable", convertedResource.Properties.Resource)
			}
		case *ValuesDaprStateStoreProperties:
			require.Equal(t, "state.zookeeper", convertedResource.Properties.Type)
			require.Equal(t, "v1", convertedResource.Properties.Version)
			require.Equal(t, "bar", convertedResource.Properties.Metadata["foo"])
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		case *RecipeDaprStateStoreProperties:
			if payload == "daprstatestoreresource_recipe2.json" {
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
		"daprstatestoresqlserverresourcedatamodel.json",
		"daprstatestoreazuretablestorageresourcedatamodel.json",
		"daprstatestogenericreresourcedatamodel.json",
		"daprstatestoreresourcedatamodel_recipe.json",
		"daprstatestoreresourcedatamodel_recipe2.json"}

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
		switch v := versionedResource.Properties.(type) {
		case *ResourceDaprStateStoreProperties:
			if payload == "daprstatestoreazuretablestorageresourcedatamodel.json" {
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
			if payload == "daprstatestoreresourcedatamodel_recipe2.json" {
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
