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

		resourceType := map[string]interface{}{"Provider": "kubernetes", "Type": "DaprStateStoreProvider"}
		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.DaprStateStore)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/daprStateStores/daprStateStore0", convertedResource.ID)
		require.Equal(t, "daprStateStore0", convertedResource.Name)
		require.Equal(t, "Applications.Connector/daprStateStores", convertedResource.Type)
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)

		switch v := convertedResource.Properties.(type) {
		case *datamodel.DaprStateStoreAzureTableStorageResourceProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", v.Application)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", v.Environment)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Storage/storageAccounts/tableServices/tables/testTable", v.Resource)
			require.Equal(t, "state.azure.tablestorage", v.Kind)

		case *datamodel.DaprStateStoreSQLServerResourceProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", v.Application)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", v.Environment)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", v.Resource)
			require.Equal(t, "state.sqlserver", v.Kind)
			require.Equal(t, "Deployment", v.Status.OutputResources[0]["LocalID"])
			require.Equal(t, resourceType, v.Status.OutputResources[0]["ResourceType"])

		case *datamodel.DaprStateStoreGenericResourceProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", v.Application)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", v.Environment)
			require.Equal(t, "generic", v.Kind)
			require.Equal(t, "state.zookeeper", v.Type)
			require.Equal(t, "v1", v.Version)
			require.Equal(t, "bar", v.Metadata["foo"])
			require.Equal(t, "Deployment", v.Status.OutputResources[0]["LocalID"])
			require.Equal(t, resourceType, v.Status.OutputResources[0]["ResourceType"])
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

		resourceType := map[string]interface{}{"Provider": "kubernetes", "Type": "DaprStateStoreProvider"}
		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/daprStateStores/daprStateStore0", resource.ID)
		require.Equal(t, "daprStateStore0", resource.Name)
		require.Equal(t, "Applications.Connector/daprStateStores", resource.Type)
		switch v := resource.Properties.(type) {
		case *datamodel.DaprStateStoreAzureTableStorageResourceProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", v.Application)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", v.Environment)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Storage/storageAccounts/tableServices/tables/testTable", v.Resource)
			require.Equal(t, "state.azure.tablestorage", v.Kind)
			require.Equal(t, "Deployment", v.Status.OutputResources[0]["LocalID"])
			require.Equal(t, resourceType, v.Status.OutputResources[0]["ResourceType"])
		case *datamodel.DaprStateStoreSQLServerResourceProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", v.Application)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", v.Environment)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Sql/servers/testServer/databases/testDatabase", v.Resource)
			require.Equal(t, "state.sqlserver", v.Kind)
		case *datamodel.DaprStateStoreGenericResourceProperties:
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", v.Application)
			require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", v.Environment)
			require.Equal(t, "generic", v.Kind)
			require.Equal(t, "state.zookeeper", v.Type)
			require.Equal(t, "v1", v.Version)
			require.Equal(t, "bar", v.Metadata["foo"])
			require.Equal(t, "Deployment", v.Status.OutputResources[0]["LocalID"])
			require.Equal(t, resourceType, v.Status.OutputResources[0]["ResourceType"])
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
