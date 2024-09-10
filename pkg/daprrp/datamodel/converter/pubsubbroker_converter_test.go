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

package converter

import (
	"encoding/json"
	"testing"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/daprrp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/daprrp/datamodel"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/testutil/resourcetypeutil"
	"github.com/stretchr/testify/require"
)

// Validates type conversion between versioned client side data model and RP data model.
func TestPubSubBrokerDataModelToVersioned(t *testing.T) {
	createdAt, err := time.Parse(time.RFC3339Nano, "2021-09-24T19:09:54.2403864Z")
	require.NoError(t, err)

	lastModifiedAt, err := time.Parse(time.RFC3339Nano, "2021-09-24T20:09:54.2403864Z")
	require.NoError(t, err)

	testset := []struct {
		dataModelFile string
		apiVersion    string
		apiModelType  any
		expected      *v20231001preview.DaprPubSubBrokerResource
		err           error
	}{
		{
			"../../api/v20231001preview/testdata/pubsubbroker_manual_datamodel.json",
			"2023-10-01-preview",
			&v20231001preview.DaprPubSubBrokerResource{},
			&v20231001preview.DaprPubSubBrokerResource{
				Location: to.Ptr("global"),
				Properties: &v20231001preview.DaprPubSubBrokerProperties{
					Environment: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"),
					Application: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"),
					Metadata: map[string]*v20231001preview.MetadataValue{
						"foo": {
							Value: to.Ptr("bar"),
						},
					},
					Recipe:               nil,
					ResourceProvisioning: to.Ptr(v20231001preview.ResourceProvisioningManual),
					Resources: []*v20231001preview.ResourceReference{
						{
							ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ServiceBus/namespaces/radius-eastus-async"),
						},
					},
					Type:              to.Ptr("pubsub.azure.servicebus"),
					Version:           to.Ptr("v1"),
					ComponentName:     to.Ptr("test-dpsb"),
					ProvisioningState: to.Ptr(v20231001preview.ProvisioningStateAccepted),
					Status:            resourcetypeutil.MustPopulateResourceStatus(&v20231001preview.ResourceStatus{}),
					Auth: &v20231001preview.DaprResourceAuth{
						SecretStore: to.Ptr("test-secret-store"),
					},
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/pubSubBrokers/test-dpsb"),
				Name: to.Ptr("test-dpsb"),
				SystemData: &v20231001preview.SystemData{
					CreatedAt:          &createdAt,
					CreatedBy:          to.Ptr("fakeid@live.com"),
					CreatedByType:      to.Ptr(v20231001preview.CreatedByTypeUser),
					LastModifiedAt:     &lastModifiedAt,
					LastModifiedBy:     to.Ptr("fakeid@live.com"),
					LastModifiedByType: to.Ptr(v20231001preview.CreatedByTypeUser),
				},
				Type: to.Ptr("Applications.Dapr/pubSubBrokers"),
			},
			nil,
		},
		{
			"../../api/v20231001preview/testdata/pubsubbroker_manual_generic_datamodel.json",
			"2023-10-01-preview",
			&v20231001preview.DaprPubSubBrokerResource{},
			&v20231001preview.DaprPubSubBrokerResource{
				Location: to.Ptr("global"),
				Properties: &v20231001preview.DaprPubSubBrokerProperties{
					Environment: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"),
					Application: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"),
					Metadata: map[string]*v20231001preview.MetadataValue{
						"foo": {
							Value: to.Ptr("bar"),
						},
					},
					Recipe:               nil,
					ResourceProvisioning: to.Ptr(v20231001preview.ResourceProvisioningManual),
					Resources:            nil,
					Type:                 to.Ptr("pubsub.kafka"),
					Version:              to.Ptr("v1"),
					ComponentName:        to.Ptr("test-dpsb"),
					ProvisioningState:    to.Ptr(v20231001preview.ProvisioningStateAccepted),
					Status:               resourcetypeutil.MustPopulateResourceStatus(&v20231001preview.ResourceStatus{}),
					Auth: &v20231001preview.DaprResourceAuth{
						SecretStore: to.Ptr("test-secret-store"),
					},
				},
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				ID:   to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Dapr/pubSubBrokers/test-dpsb"),
				Name: to.Ptr("test-dpsb"),
				SystemData: &v20231001preview.SystemData{
					CreatedAt:          &createdAt,
					CreatedBy:          to.Ptr("fakeid@live.com"),
					CreatedByType:      to.Ptr(v20231001preview.CreatedByTypeUser),
					LastModifiedAt:     &lastModifiedAt,
					LastModifiedBy:     to.Ptr("fakeid@live.com"),
					LastModifiedByType: to.Ptr(v20231001preview.CreatedByTypeUser),
				},
				Type: to.Ptr("Applications.Dapr/pubSubBrokers"),
			},
			nil,
		},
		{
			"../../api/v20231001preview/testdata/pubsubbroker_manual_generic_datamodel.json",
			"unsupported",
			nil,
			nil,
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := testutil.ReadFixture("../" + tc.dataModelFile)
			dm := &datamodel.DaprPubSubBroker{}
			err = json.Unmarshal(c, dm)
			require.NoError(t, err)

			am, err := PubSubBrokerDataModelToVersioned(dm, tc.apiVersion)
			if tc.err != nil {
				require.ErrorAs(t, tc.err, &err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiModelType, am)
				require.Equal(t, tc.expected, am)
			}
		})
	}
}

func TestDaprPubSubBrokerDataModelFromVersioned(t *testing.T) {
	testset := []struct {
		versionedModelFile string
		apiVersion         string
		err                error
	}{
		{
			"../../api/v20231001preview/testdata/pubsubbroker_invalidrecipe_resource.json",
			"2023-10-01-preview",
			&v1.ErrClientRP{
				Code:    v1.CodeInvalid,
				Message: "error(s) found:\n\tmetadata cannot be specified when resourceProvisioning is set to recipe (default)\n\ttype cannot be specified when resourceProvisioning is set to recipe (default)\n\tversion cannot be specified when resourceProvisioning is set to recipe (default)",
			},
		},
		{
			"../../api/v20231001preview/testdata/pubsubbroker_invalidmanual_resource.json",
			"2023-10-01-preview",
			&v1.ErrClientRP{
				Code:    "BadRequest",
				Message: "error(s) found:\n\trecipe details cannot be specified when resourceProvisioning is set to manual\n\tmetadata must be specified when resourceProvisioning is set to manual\n\ttype must be specified when resourceProvisioning is set to manual\n\tversion must be specified when resourceProvisioning is set to manual",
			},
		},
		{
			"../../api/v20231001preview/testdata/pubsubbroker_recipe_resource.json",
			"2023-10-01-preview",
			nil,
		},
		{
			"../../api/v20231001preview/testdata/pubsubbroker_manual_resource.json",
			"2023-10-01-preview",
			nil,
		},
		{
			"../../api/v20231001preview/testdata/pubsubbroker_manual_resource.json",
			"unsupported",
			v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.apiVersion, func(t *testing.T) {
			c := testutil.ReadFixture("../" + tc.versionedModelFile)
			dm, err := PubSubBrokerDataModelFromVersioned(c, tc.apiVersion)
			if tc.err != nil {
				require.Equal(t, tc.err, err)
			} else {
				require.NoError(t, err)
				require.IsType(t, tc.apiVersion, dm.InternalMetadata.UpdatedAPIVersion)
			}
		})
	}
}
