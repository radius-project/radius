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

func TestDaprSecretStore_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{"secretstoreresource.json", "secretstoreresource2.json", "secretstoreresource_recipe.json"}
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
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/secretStores/daprSecretStore0", convertedResource.ID)
		require.Equal(t, "daprSecretStore0", convertedResource.Name)
		require.Equal(t, linkrp.N_DaprSecretStoresResourceType, convertedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", convertedResource.Properties.Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", convertedResource.Properties.Environment)
		switch versionedResource.Properties.(type) {
		case *RecipeDaprSecretStoreProperties:
			require.Equal(t, []rpv1.OutputResource(nil), convertedResource.Properties.Status.OutputResources)
			require.Equal(t, "daprSecretStore", convertedResource.Properties.Recipe.Name)
			require.Equal(t, "bar", convertedResource.Properties.Recipe.Parameters["foo"])
		case *ValuesDaprSecretStoreProperties:
			require.Equal(t, "secretstores.hashicorp.vault", convertedResource.Properties.Type)
			require.Equal(t, "v1", convertedResource.Properties.Version)
			require.Equal(t, "bar", convertedResource.Properties.Metadata["foo"])
			require.Equal(t, "2022-03-15-privatepreview", convertedResource.InternalMetadata.UpdatedAPIVersion)
		}
	}
}

func TestDaprSecretStore_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{"secretstoreresourcedatamodel.json", "secretstoreresourcedatamodel2.json", "secretstoreresourcedatamodel_recipe.json"}
	for _, payload := range testset {
		// arrange
		rawPayload := loadTestData(payload)
		resource := &datamodel.DaprSecretStore{}
		err := json.Unmarshal(rawPayload, resource)
		require.NoError(t, err)

		// act
		versionedResource := &DaprSecretStoreResource{}
		err = versionedResource.ConvertFrom(resource)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/secretStores/daprSecretStore0", *versionedResource.ID)
		require.Equal(t, "daprSecretStore0", *versionedResource.Name)
		require.Equal(t, linkrp.N_DaprSecretStoresResourceType, *versionedResource.Type)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication", *versionedResource.Properties.GetDaprSecretStoreProperties().Application)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", *versionedResource.Properties.GetDaprSecretStoreProperties().Environment)

		switch v := versionedResource.Properties.(type) {
		case *RecipeDaprSecretStoreProperties:
			require.Equal(t, "daprSecretStore", *v.Recipe.Name)
			require.Equal(t, "bar", v.Recipe.Parameters["foo"])
			require.Equal(t, "Deployment", versionedResource.Properties.GetDaprSecretStoreProperties().Status.OutputResources[0]["LocalID"])
			require.Equal(t, "kubernetes", versionedResource.Properties.GetDaprSecretStoreProperties().Status.OutputResources[0]["Provider"])
		case *ValuesDaprSecretStoreProperties:
			require.Equal(t, "secretstores.hashicorp.vault", *v.Type)
			require.Equal(t, "v1", *v.Version)
			require.Equal(t, "bar", v.Metadata["foo"])
		}
	}

}

func TestDaprSecretStore_ConvertVersionedToDataModel_InvalidRequest(t *testing.T) {
	testsFile := "secretstoreinvalid.json"
	rawPayload := loadTestData(testsFile)
	var testset []TestData
	err := json.Unmarshal(rawPayload, &testset)
	require.NoError(t, err)
	for _, testData := range testset {
		versionedResource := &DaprSecretStoreResource{}
		err := json.Unmarshal(testData.Payload, versionedResource)
		require.NoError(t, err)
		var expectedErr v1.ErrClientRP
		description := testData.Description
		if description == "unsupported_mode" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "Unsupported mode abc"
		}
		if description == "invalid_properties_with_mode_recipe" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "recipe is a required property for mode 'recipe'"
		}
		if description == "invalid_properties_with_mode_values" {
			expectedErr.Code = "BadRequest"
			expectedErr.Message = "type/version/metadata are required properties for mode 'values'"
		}
		_, err = versionedResource.ConvertTo()
		require.Equal(t, &expectedErr, err)
	}
}

func TestDaprSecretStore_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &DaprSecretStoreResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}

type TestData struct {
	Description string          `json:"description,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}
