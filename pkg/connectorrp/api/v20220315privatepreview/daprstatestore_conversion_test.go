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
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaprStateStore_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"daprstatestoresqlserverresource.json", "daprstatestoreazuretablestorageresource.json", "daprstatestogenericreresource.json"}

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
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/daprStateStores/daprStateStore0", convertedResource.ID)
		require.Equal(t, "daprStateStore0", convertedResource.Name)
		require.Equal(t, "Applications.Connector/daprStateStores", convertedResource.Type)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		switch convertedResource.Properties.Kind {
		case datamodel.DaprStateStoreKindAzureTableStorage:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Storage/storageAccounts/tableServices/tables/testTable", convertedResource.Properties.DaprStateStoreAzureTableStorage.Resource)
			require.Equal(t, "state.azure.tablestorage", string(convertedResource.Properties.Kind))

		case datamodel.DaprStateStoreKindStateSqlServer:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", convertedResource.Properties.DaprStateStoreSQLServer.Resource)
			require.Equal(t, "state.sqlserver", string(convertedResource.Properties.Kind))
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)

		case datamodel.DaprStateStoreKindGeneric:
			require.Equal(t, "generic", string(convertedResource.Properties.Kind))
			require.Equal(t, "state.zookeeper", convertedResource.Properties.DaprStateStoreGeneric.Type)
			require.Equal(t, "v1", convertedResource.Properties.DaprStateStoreGeneric.Version)
			require.Equal(t, "bar", convertedResource.Properties.DaprStateStoreGeneric.Metadata["foo"])
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		default:
			assert.Fail(t, "Kind of DaprStateStore is specified.")
		}
	}

}

func TestDaprStateStore_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"daprstatestoresqlserverresourcedatamodel.json", "daprstatestoreazuretablestorageresourcedatamodel.json", "daprstatestogenericreresourcedatamodel.json"}

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
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/daprStateStores/daprStateStore0", resource.ID)
		require.Equal(t, "daprStateStore0", resource.Name)
		require.Equal(t, "Applications.Connector/daprStateStores", resource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)
		switch resource.Properties.Kind {
		case datamodel.DaprStateStoreKindAzureTableStorage:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Storage/storageAccounts/tableServices/tables/testTable", resource.Properties.DaprStateStoreAzureTableStorage.Resource)
			require.Equal(t, "state.azure.tablestorage", string(resource.Properties.Kind))
			require.Equal(t, "Deployment", versionedResource.Properties.GetDaprStateStoreProperties().BasicResourceProperties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "kubernetes", versionedResource.Properties.GetDaprStateStoreProperties().BasicResourceProperties.Status.OutputResources[0]["Provider"])
		case datamodel.DaprStateStoreKindStateSqlServer:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", resource.Properties.DaprStateStoreSQLServer.Resource)
			require.Equal(t, "state.sqlserver", string(resource.Properties.Kind))
		case datamodel.DaprStateStoreKindGeneric:
			require.Equal(t, "generic", string(resource.Properties.Kind))
			require.Equal(t, "state.zookeeper", resource.Properties.DaprStateStoreGeneric.Type)
			require.Equal(t, "v1", resource.Properties.DaprStateStoreGeneric.Version)
			require.Equal(t, "bar", resource.Properties.DaprStateStoreGeneric.Metadata["foo"])
			require.Equal(t, "Deployment", versionedResource.Properties.GetDaprStateStoreProperties().BasicResourceProperties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "kubernetes", versionedResource.Properties.GetDaprStateStoreProperties().BasicResourceProperties.Status.OutputResources[0]["Provider"])
		default:
			assert.Fail(t, "Kind of DaprStateStore is specified.")
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
