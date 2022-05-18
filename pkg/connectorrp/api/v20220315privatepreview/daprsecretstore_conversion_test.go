// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/api"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestDaprSecretStore_ConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := loadTestData("daprsecretstoreresource.json")
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
}

func TestDaprSecretStore_ConvertDataModelToVersioned(t *testing.T) {
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
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Connector/daprSecretStores/daprSecretStore0", resource.ID)
	require.Equal(t, "daprSecretStore0", resource.Name)
	require.Equal(t, "Applications.Connector/daprSecretStores", resource.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", resource.Properties.Application)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", resource.Properties.Environment)
	require.Equal(t, "generic", string(resource.Properties.Kind))
	require.Equal(t, "secretstores.hashicorp.vault", resource.Properties.Type)
	require.Equal(t, "v1", resource.Properties.Version)
	require.Equal(t, "bar", resource.Properties.Metadata["foo"])
}

func TestDaprSecretStore_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src api.DataModelInterface
		err error
	}{
		{&fakeResource{}, api.ErrInvalidModelConversion},
		{nil, api.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &DaprSecretStoreResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
