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
	"github.com/project-radius/radius/pkg/linkrp/api/v20220315privatepreview"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func TestDaprPubSubBroker_ConvertVersionedToDataModel(t *testing.T) {
	testCases := []struct {
		desc     string
		file     string
		expected *datamodel.DaprPubSubBroker
	}{
		{
			desc: "Manual provisioning of a DaprPubSubBroker",
			file: "pubsubbroker_manual_resource.json",
			expected: &datamodel.DaprPubSubBroker{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/pubSubBrokers/test-dpsb",
						Name:     "test-dpsb",
						Type:     linkrp.N_DaprPubSubBrokersResourceType,
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
				Properties: datamodel.DaprPubSubBrokerProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env",
					},
					ResourceProvisioning: linkrp.ResourceProvisioningManual,
					Metadata: map[string]any{
						"foo": "bar",
					},
					Resources: []*linkrp.ResourceReference{
						{
							ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ServiceBus/namespaces/radius-eastus-async",
						},
					},
					Type:    "pubsub.azure.servicebus",
					Version: "v1",
				},
			},
		},
		{
			desc: "Provisioning by a Recipe of a pubSubBroker",
			file: "pubsubbroker_recipe_resource.json",
			expected: &datamodel.DaprPubSubBroker{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/pubSubBrokers/test-dpsb",
						Name:     "test-dpsb",
						Type:     linkrp.N_DaprPubSubBrokersResourceType,
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
				Properties: datamodel.DaprPubSubBrokerProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app",
						Environment: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env",
					},
					ResourceProvisioning: linkrp.ResourceProvisioningRecipe,
					Recipe: linkrp.LinkRecipe{
						Name: "dpsb-recipe",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			// arrange
			rawPayload, err := v20220315privatepreview.LoadTestData("./testdata/" + tc.file)
			require.NoError(t, err)
			versionedResource := &DaprPubSubBrokerResource{}
			err = json.Unmarshal(rawPayload, versionedResource)
			require.NoError(t, err)

			// act
			dm, err := versionedResource.ConvertTo()

			// assert
			require.NoError(t, err)
			convertedResource := dm.(*datamodel.DaprPubSubBroker)

			require.Equal(t, tc.expected, convertedResource)
		})
	}
}

func TestDaprPubSubBroker_ConvertVersionedToDataModel_Invalid(t *testing.T) {
	testset := []struct {
		payload string
		errType error
		message string
	}{
		{
			"pubsubbroker_invalidmanual_resource.json",
			&v1.ErrClientRP{},
			"code BadRequest: err error(s) found:\n\trecipe details cannot be specified when resourceProvisioning is set to manual\n\tmetadata must be specified when resourceProvisioning is set to manual\n\ttype must be specified when resourceProvisioning is set to manual\n\tversion must be specified when resourceProvisioning is set to manual",
		},
		{
			"pubsubbroker_invalidrecipe_resource.json",
			&v1.ErrClientRP{},
			"code BadRequest: err error(s) found:\n\tmetadata cannot be specified when resourceProvisioning is set to recipe (default)\n\ttype cannot be specified when resourceProvisioning is set to recipe (default)\n\tversion cannot be specified when resourceProvisioning is set to recipe (default)",
		},
	}

	for _, test := range testset {
		t.Run(test.payload, func(t *testing.T) {
			rawPayload, err := v20220315privatepreview.LoadTestData("./testdata/" + test.payload)
			require.NoError(t, err)
			versionedResource := &DaprStateStoreResource{}
			err = json.Unmarshal(rawPayload, versionedResource)
			require.NoError(t, err)

			dm, err := versionedResource.ConvertTo()
			require.Error(t, err)
			require.Nil(t, dm)
			require.IsType(t, test.errType, err)
			require.Equal(t, test.message, err.Error())
		})
	}
}

func TestDaprPubSubBroker_ConvertDataModelToVersioned(t *testing.T) {
	testCases := []struct {
		desc     string
		file     string
		expected *DaprPubSubBrokerResource
	}{
		{
			desc: "Convert manually provisioned DaprPubSubBroker datamodel to versioned resource",
			file: "pubsubbroker_manual_datamodel.json",
			expected: &DaprPubSubBrokerResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &DaprPubSubBrokerProperties{
					Environment: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"),
					Application: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"),
					Metadata: map[string]any{
						"foo": "bar",
					},
					ResourceProvisioning: to.Ptr(ResourceProvisioningManual),
					Resources: []*ResourceReference{
						{
							ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ServiceBus/namespaces/radius-eastus-async"),
						},
					},
					Type:              to.Ptr("pubsub.azure.servicebus"),
					Version:           to.Ptr("v1"),
					ComponentName:     to.Ptr("test-dpsb"),
					ProvisioningState: to.Ptr(ProvisioningStateAccepted),
					Status: &ResourceStatus{
						OutputResources: []map[string]any{
							{
								"Identity": nil,
								"LocalID":  "Deployment",
								"Provider": "kubernetes",
							},
						},
					},
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/pubSubBrokers/test-dpsb"),
				Name: to.Ptr("test-dpsb"),
				Type: to.Ptr(linkrp.N_DaprPubSubBrokersResourceType),
			},
		},
		{
			desc: "Convert DaprPubSubBroker datamodel provisioned by a recipe to versioned resource",
			file: "pubsubbroker_recipe_datamodel.json",
			expected: &DaprPubSubBrokerResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &DaprPubSubBrokerProperties{
					Environment: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"),
					Application: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"),
					Recipe: &Recipe{
						Name: to.Ptr("dpsb-recipe"),
					},
					ResourceProvisioning: to.Ptr(ResourceProvisioningRecipe),
					ComponentName:        to.Ptr("test-dpsb"),
					ProvisioningState:    to.Ptr(ProvisioningStateAccepted),
					Status: &ResourceStatus{
						OutputResources: []map[string]any{
							{
								"Identity": nil,
								"LocalID":  "Deployment",
								"Provider": "kubernetes",
							},
						},
					},
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/pubSubBrokers/test-dpsb"),
				Name: to.Ptr("test-dpsb"),
				Type: to.Ptr(linkrp.N_DaprPubSubBrokersResourceType),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			rawPayload, err := v20220315privatepreview.LoadTestData("./testdata/" + tc.file)
			require.NoError(t, err)
			resource := &datamodel.DaprPubSubBroker{}
			err = json.Unmarshal(rawPayload, resource)
			require.NoError(t, err)

			versionedResource := &DaprPubSubBrokerResource{}
			err = versionedResource.ConvertFrom(resource)
			require.NoError(t, err)

			// Skip system data comparison
			versionedResource.SystemData = nil

			require.Equal(t, tc.expected, versionedResource)
		})
	}
}

func TestDaprPubSubBroker_ConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&v20220315privatepreview.FakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &DaprPubSubBrokerResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
