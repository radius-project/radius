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
	"github.com/radius-project/radius/pkg/daprrp/datamodel"
	"github.com/radius-project/radius/pkg/daprrp/frontend/controller"
	"github.com/radius-project/radius/pkg/portableresources"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/testutil/resourcetypeutil"
	"github.com/stretchr/testify/require"
)

func TestDaprBinding_ConvertVersionedToDataModel(t *testing.T) {
	testCases := []struct {
		desc     string
		file     string
		expected *datamodel.DaprBinding
	}{
		{
			desc: "Manual provisioning of a DaprBinding",
			file: "binding_manual_resource.json",
			expected: &datamodel.DaprBinding{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/bindings/test-dbd",
						Name:     "test-dbd",
						Type:     controller.DaprBindingsResourceType,
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
				Properties: datamodel.DaprBindingProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env",
					},
					ResourceProvisioning: portableresources.ResourceProvisioningManual,
					Metadata: map[string]*rpv1.DaprComponentMetadataValue{
						"foo": {
							Value: "bar",
						},
					},
					Resources: []*portableresources.ResourceReference{
						{
							ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Storage/storageAccounts/testAcc/blobServices/containers/testCtn",
						},
					},
					Type:    "bindings.azure.blobstorage",
					Version: "v1",
					Scopes:  []string{"test-scope-1", "test-scope-2"},
				},
			},
		},
		{
			desc: "Provisioning by a Recipe of a binding",
			file: "binding_recipe_resource.json",
			expected: &datamodel.DaprBinding{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/bindings/test-dbd",
						Name:     "test-dbd",
						Type:     controller.DaprBindingsResourceType,
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
				Properties: datamodel.DaprBindingProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env",
					},
					ResourceProvisioning: portableresources.ResourceProvisioningRecipe,
					Recipe: portableresources.ResourceRecipe{
						Name: "dbd-recipe",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// arrange
			rawPayload := testutil.ReadFixture(tc.file)
			versionedResource := &DaprBindingResource{}
			err := json.Unmarshal(rawPayload, versionedResource)
			require.NoError(t, err)

			// act
			dm, err := versionedResource.ConvertTo()

			// assert
			require.NoError(t, err)
			convertedResource := dm.(*datamodel.DaprBinding)

			require.Equal(t, tc.expected, convertedResource)
		})
	}
}

func TestDaprBinding_ConvertVersionedToDataModel_Invalid(t *testing.T) {
	testset := []struct {
		payload string
		errType error
		message string
	}{
		{
			"binding_invalidmanual_resource.json",
			&v1.ErrClientRP{},
			"code BadRequest: err error(s) found:\n\trecipe details cannot be specified when resourceProvisioning is set to manual\n\tmetadata must be specified when resourceProvisioning is set to manual\n\ttype must be specified when resourceProvisioning is set to manual\n\tversion must be specified when resourceProvisioning is set to manual",
		},
		{
			"binding_invalidrecipe_resource.json",
			&v1.ErrClientRP{},
			"code BadRequest: err error(s) found:\n\tmetadata cannot be specified when resourceProvisioning is set to recipe (default)\n\ttype cannot be specified when resourceProvisioning is set to recipe (default)\n\tversion cannot be specified when resourceProvisioning is set to recipe (default)",
		},
	}

	for _, test := range testset {
		t.Run(test.payload, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(test.payload)
			versionedResource := &DaprBindingResource{}
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

func TestDaprBinding_ConvertDataModelToVersioned(t *testing.T) {
	testCases := []struct {
		desc     string
		file     string
		expected *DaprBindingResource
	}{
		{
			desc: "Convert manually provisioned DaprBinding datamodel to versioned resource",
			file: "binding_manual_datamodel.json",
			expected: &DaprBindingResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &DaprBindingProperties{
					Environment: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"),
					Application: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"),
					Metadata: map[string]*MetadataValue{
						"foo": {
							Value: to.Ptr("bar"),
						},
					},
					ResourceProvisioning: to.Ptr(ResourceProvisioningManual),
					Resources: []*ResourceReference{
						{
							ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.Storage/storageAccounts/testAcc/blobServices/containers/testCtn"),
						},
					},
					Type:              to.Ptr("bindings.azure.blobstorage"),
					Version:           to.Ptr("v1"),
					ComponentName:     to.Ptr("test-dbd"),
					ProvisioningState: to.Ptr(ProvisioningStateAccepted),
					Status:            resourcetypeutil.MustPopulateResourceStatus(&ResourceStatus{}),
					Auth:              &DaprResourceAuth{SecretStore: to.Ptr("test-secret-store")},
					Scopes:            []*string{to.Ptr("test-scope-1"), to.Ptr("test-scope-2")},
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/bindings/test-dbd"),
				Name: to.Ptr("test-dbd"),
				Type: to.Ptr(controller.DaprBindingsResourceType),
			},
		},
		{
			desc: "Convert DaprBinding datamodel provisioned by a recipe to versioned resource",
			file: "binding_recipe_datamodel.json",
			expected: &DaprBindingResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &DaprBindingProperties{
					Environment: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"),
					Application: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"),
					Recipe: &Recipe{
						Name: to.Ptr("dbd-recipe"),
					},
					ResourceProvisioning: to.Ptr(ResourceProvisioningRecipe),
					ComponentName:        to.Ptr("test-dbd"),
					ProvisioningState:    to.Ptr(ProvisioningStateAccepted),
					Status:               resourcetypeutil.MustPopulateResourceStatusWithRecipe(&ResourceStatus{}),
					Auth:                 nil,
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/bindings/test-dbd"),
				Name: to.Ptr("test-dbd"),
				Type: to.Ptr(controller.DaprBindingsResourceType),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tc.file)
			resource := &datamodel.DaprBinding{}
			err := json.Unmarshal(rawPayload, resource)
			require.NoError(t, err)

			versionedResource := &DaprBindingResource{}
			err = versionedResource.ConvertFrom(resource)
			require.NoError(t, err)

			// Skip system data comparison
			versionedResource.SystemData = nil

			require.Equal(t, tc.expected, versionedResource)
		})
	}
}

func TestDaprBinding_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&resourcetypeutil.FakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &DaprBindingResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
