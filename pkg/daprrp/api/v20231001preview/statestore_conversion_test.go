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

package v20231001preview

import (
	"encoding/json"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	daprrp_types "github.com/radius-project/radius/pkg/daprrp"
	"github.com/radius-project/radius/pkg/daprrp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/testutil/resourcetypeutil"
	"github.com/stretchr/testify/require"
)

func TestDaprStateStore_ConvertVersionedToDataModel(t *testing.T) {
	testset := []string{
		"statestore_values_resource.json",
		"statestore_recipe_resource.json",
	}

	for _, payload := range testset {
		t.Run(payload, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(payload)
			versionedResource := &DaprStateStoreResource{}
			err := json.Unmarshal(rawPayload, versionedResource)
			require.NoError(t, err)

			dm, err := versionedResource.ConvertTo()

			require.NoError(t, err)
			convertedResource := dm.(*datamodel.DaprStateStore)

			expected := &datamodel.DaprStateStore{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/stateStores/stateStore0",
						Name:     "stateStore0",
						Type:     daprrp_types.DaprStateStoresResourceType,
						Location: v1.LocationGlobal,
						Tags: map[string]string{
							"env": "dev",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "",
						UpdatedAPIVersion:      "2023-10-01-preview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
					SystemData: v1.SystemData{},
				},
				Properties: datamodel.DaprStateStoreProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
					},
				},
			}
			if payload == "statestore_values_resource.json" {
				expected.Properties.ResourceProvisioning = portableresources.ResourceProvisioningManual
				expected.Properties.Type = "state.zookeeper"
				expected.Properties.Version = "v1"
				expected.Properties.Metadata = map[string]any{
					"foo": "bar",
				}
				expected.Properties.Resources = []*portableresources.ResourceReference{
					{
						ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Sql/servers/testServer/databases/testDatabase",
					},
				}
			} else if payload == "statestore_recipe_resource.json" {
				expected.Properties.ResourceProvisioning = portableresources.ResourceProvisioningRecipe
				expected.Properties.Recipe.Name = "recipe-test"
			}

			require.Equal(t, expected, convertedResource)
		})
	}
}

func TestDaprStateStore_ConvertVersionedToDataModel_Invalid(t *testing.T) {
	testset := []struct {
		payload string
		errType error
		message string
	}{
		{"statestore_invalidvalues_resource.json", &v1.ErrClientRP{}, "code BadRequest: err error(s) found:\n\trecipe details cannot be specified when resourceProvisioning is set to manual\n\tmetadata must be specified when resourceProvisioning is set to manual\n\ttype must be specified when resourceProvisioning is set to manual\n\tversion must be specified when resourceProvisioning is set to manual"},
		{"statestore_invalidrecipe_resource.json", &v1.ErrClientRP{}, "code BadRequest: err error(s) found:\n\tmetadata cannot be specified when resourceProvisioning is set to recipe (default)\n\ttype cannot be specified when resourceProvisioning is set to recipe (default)\n\tversion cannot be specified when resourceProvisioning is set to recipe (default)"},
	}

	for _, test := range testset {
		t.Run(test.payload, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(test.payload)
			versionedResource := &DaprStateStoreResource{}
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

func TestDaprStateStore_ConvertDataModelToVersioned(t *testing.T) {
	testset := []string{
		"statestore_values_resourcedatamodel.json",
		"statestore_recipe_resourcedatamodel.json",
	}

	for _, payload := range testset {
		t.Run(payload, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(payload)
			resource := &datamodel.DaprStateStore{}
			err := json.Unmarshal(rawPayload, resource)
			require.NoError(t, err)

			versionedResource := &DaprStateStoreResource{}
			err = versionedResource.ConvertFrom(resource)
			require.NoError(t, err)

			// Skip system data comparison
			versionedResource.SystemData = nil

			expected := &DaprStateStoreResource{
				ID:       to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/stateStores/stateStore0"),
				Name:     to.Ptr("stateStore0"),
				Type:     to.Ptr(daprrp_types.DaprStateStoresResourceType),
				Location: to.Ptr(v1.LocationGlobal),
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				Properties: &DaprStateStoreProperties{
					Application:       to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/testApplication"),
					Environment:       to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0"),
					ComponentName:     to.Ptr("stateStore0"),
					ProvisioningState: to.Ptr(ProvisioningStateAccepted),
					Status:            resourcetypeutil.MustPopulateResourceStatus(&ResourceStatus{}),
				},
			}

			if payload == "statestore_values_resourcedatamodel.json" {
				expected.Properties.ResourceProvisioning = to.Ptr(ResourceProvisioningManual)
				expected.Properties.Type = to.Ptr("state.zookeeper")
				expected.Properties.Version = to.Ptr("v1")
				expected.Properties.Metadata = map[string]any{
					"foo": "bar",
				}
				expected.Properties.Resources = []*ResourceReference{
					{
						ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Sql/servers/testServer/databases/testDatabase"),
					},
				}
			} else if payload == "statestore_recipe_resourcedatamodel.json" {
				expected.Properties.ResourceProvisioning = to.Ptr(ResourceProvisioningRecipe)
				expected.Properties.Recipe = &Recipe{
					Name: to.Ptr("recipe-test"),
				}
			}

			require.Equal(t, expected, versionedResource)
		})
	}
}

func TestDaprStateStore_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&resourcetypeutil.FakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &DaprStateStoreResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
