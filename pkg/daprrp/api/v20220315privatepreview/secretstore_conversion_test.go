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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/daprrp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/testutil/resourcetypeutil"
	"github.com/stretchr/testify/require"
)

func TestDaprSecretStore_ConvertVersionedToDataModel(t *testing.T) {
	testCases := []struct {
		desc     string
		file     string
		expected *datamodel.DaprSecretStore
	}{
		{
			desc: "secretstore manual resource",
			file: "secretstore_manual_resource.json",
			expected: &datamodel.DaprSecretStore{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/secretStores/test-dss",
						Name:     "test-dss",
						Type:     portableresources.DaprSecretStoresResourceType,
						Location: v1.LocationGlobal,
						Tags: map[string]string{
							"env": "dev",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "",
						UpdatedAPIVersion:      "2022-03-15-privatepreview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
					SystemData: v1.SystemData{},
				},
				Properties: datamodel.DaprSecretStoreProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env",
					},
					ResourceProvisioning: portableresources.ResourceProvisioningManual,
					Metadata: map[string]any{
						"foo": "bar",
					},
					Type:    "secretstores.hashicorp.vault",
					Version: "v1",
				},
			},
		},
		{
			desc: "secretstore recipe resource",
			file: "secretstore_recipe_resource.json",
			expected: &datamodel.DaprSecretStore{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/secretStores/test-dss",
						Name:     "test-dss",
						Type:     portableresources.DaprSecretStoresResourceType,
						Location: v1.LocationGlobal,
						Tags: map[string]string{
							"env": "dev",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "",
						UpdatedAPIVersion:      "2022-03-15-privatepreview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
					SystemData: v1.SystemData{},
				},
				Properties: datamodel.DaprSecretStoreProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env",
					},
					ResourceProvisioning: portableresources.ResourceProvisioningRecipe,
					Recipe: portableresources.LinkRecipe{
						Name: "daprSecretStore",
						Parameters: map[string]any{
							"foo": "bar",
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// arrange
			rawPayload := testutil.ReadFixture(tc.file)
			versionedResource := &DaprSecretStoreResource{}
			err := json.Unmarshal(rawPayload, versionedResource)
			require.NoError(t, err)

			// act
			dm, err := versionedResource.ConvertTo()

			// assert
			require.NoError(t, err)
			convertedResource := dm.(*datamodel.DaprSecretStore)

			require.Equal(t, tc.expected, convertedResource)
		})
	}
}

func TestDaprSecretStore_ConvertDataModelToVersioned(t *testing.T) {
	testCases := []struct {
		desc     string
		file     string
		expected *DaprSecretStoreResource
	}{
		{
			desc: "secretstore manual resource data model",
			file: "secretstore_manual_resourcedatamodel.json",
			expected: &DaprSecretStoreResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &DaprSecretStoreProperties{
					Environment: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"),
					Application: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"),
					Metadata: map[string]any{
						"foo": "bar",
					},
					ResourceProvisioning: to.Ptr(ResourceProvisioningManual),
					Type:                 to.Ptr("secretstores.hashicorp.vault"),
					Version:              to.Ptr("v1"),
					ComponentName:        to.Ptr("test-dss"),
					ProvisioningState:    to.Ptr(ProvisioningStateAccepted),
					Status:               resourcetypeutil.MustPopulateResourceStatus(&ResourceStatus{}),
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/secretStores/test-dss"),
				Name: to.Ptr("test-dss"),
				Type: to.Ptr(portableresources.DaprSecretStoresResourceType),
			},
		},
		{
			desc: "secretstore recipe resource data model",
			file: "secretstore_recipe_resourcedatamodel.json",
			expected: &DaprSecretStoreResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &DaprSecretStoreProperties{
					Environment:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"),
					Application:          to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"),
					ResourceProvisioning: to.Ptr(ResourceProvisioningRecipe),
					Recipe: &Recipe{
						Name: to.Ptr("daprSecretStore"),
						Parameters: map[string]any{
							"foo": "bar",
						},
					},
					Type:              to.Ptr("secretstores.hashicorp.vault"),
					Version:           to.Ptr("v1"),
					Metadata:          map[string]any{"foo": "bar"},
					ComponentName:     to.Ptr("test-dss"),
					ProvisioningState: to.Ptr(ProvisioningStateAccepted),
					Status:            resourcetypeutil.MustPopulateResourceStatus(&ResourceStatus{}),
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/secretStores/test-dss"),
				Name: to.Ptr("test-dss"),
				Type: to.Ptr(portableresources.DaprSecretStoresResourceType),
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tc.file)
			resource := &datamodel.DaprSecretStore{}
			err := json.Unmarshal(rawPayload, resource)
			require.NoError(t, err)

			versionedResource := &DaprSecretStoreResource{}
			err = versionedResource.ConvertFrom(resource)
			require.NoError(t, err)

			// Skip system data comparison
			versionedResource.SystemData = nil

			require.Equal(t, tc.expected, versionedResource)
		})
	}

}

func TestDaprSecretStore_ConvertVersionedToDataModel_InvalidRequest(t *testing.T) {
	testset := []struct {
		payload string
		errType error
		message string
	}{
		{
			"secretstore_invalidvalues_resource.json",
			&v1.ErrClientRP{},
			"code BadRequest: err error(s) found:\n\trecipe details cannot be specified when resourceProvisioning is set to manual\n\tmetadata must be specified when resourceProvisioning is set to manual\n\ttype must be specified when resourceProvisioning is set to manual\n\tversion must be specified when resourceProvisioning is set to manual",
		},
		{
			"secretstore_invalidrecipe_resource.json",
			&v1.ErrClientRP{},
			"code BadRequest: err error(s) found:\n\tmetadata cannot be specified when resourceProvisioning is set to recipe (default)\n\ttype cannot be specified when resourceProvisioning is set to recipe (default)\n\tversion cannot be specified when resourceProvisioning is set to recipe (default)",
		},
	}
	for _, test := range testset {
		t.Run(test.payload, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(test.payload)
			versionedResource := &DaprSecretStoreResource{}
			err := json.Unmarshal(rawPayload, versionedResource)
			require.NoError(t, err)

			dm, err := versionedResource.ConvertTo()
			require.Error(t, err)
			require.Nil(t, dm)
			require.IsType(t, test.errType, err)
			require.Equal(t, test.message, err.Error())
		})
	}
}

func TestDaprSecretStore_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&resourcetypeutil.FakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &DaprSecretStoreResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
