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
	"github.com/stretchr/testify/require"
)

func TestDaprSecretStore_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"daprsecretstoreresource.json", "daprsecretstoreresource2.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		versionedResource := &DaprSecretStoreResource{}
		err := json.Unmarshal(rawPayload, versionedResource)
		require.NoError(t, err)

		// act
		dm, err := versionedResource.ConvertTo()

		// assert
		require.NoError(t, err)
		convertedResource := dm.(*datamodel.DaprSecretStore)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/daprSecretStores/daprSecretStore0", convertedResource.ID)
		require.Equal(t, "daprSecretStore0", convertedResource.Name)
		require.Equal(t, "Applications.Connector/daprSecretStores", convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		require.Equal(t, "generic", string(convertedResource.Properties.Kind))
		require.Equal(t, "secretstores.hashicorp.vault", convertedResource.Properties.Type)
		require.Equal(t, "v1", convertedResource.Properties.Version)
		require.Equal(t, "bar", convertedResource.Properties.Metadata["foo"])
		require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		if payload == "daprsecretstoreresource.json" {
			require.Equal(t, []outputresource.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
		}
	}
}

func TestDaprSecretStore_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"daprsecretstoreresourcedatamodel.json", "daprsecretstoreresourcedatamodel2.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData("daprsecretstoreresourcedatamodel.json")
		resource := &datamodel.DaprSecretStore{}
		err := json.Unmarshal(rawPayload, resource)
		require.NoError(t, err)

		// act
		versionedResource := &DaprSecretStoreResource{}
		err = versionedResource.ConvertFrom(resource)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/daprSecretStores/daprSecretStore0", *versionedResource.ID)
		require.Equal(t, "daprSecretStore0", *versionedResource.Name)
		require.Equal(t, "Applications.Connector/daprSecretStores", *versionedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.Environment)
		require.Equal(t, "generic", string(*versionedResource.Properties.Kind))
		require.Equal(t, "secretstores.hashicorp.vault", *versionedResource.Properties.Type)
		require.Equal(t, "v1", *versionedResource.Properties.Version)
		require.Equal(t, "bar", versionedResource.Properties.Metadata["foo"])
		if payload == "daprsecretstoreresource.json" {
			require.Equal(t, "Deployment", versionedResource.Properties.Status.OutputResources[0]["LocalID"])
			require.Equal(t, "ExtenderProvider", versionedResource.Properties.Status.OutputResources[0]["Provider"])
		}
	}

}

func TestDaprSecretStore_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &DaprSecretStoreResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
