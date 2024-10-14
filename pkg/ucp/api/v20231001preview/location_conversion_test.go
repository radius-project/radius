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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/test/testutil"

	"github.com/stretchr/testify/require"
)

func Test_Location_VersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *datamodel.Location
		err      error
	}{
		{
			filename: "location_resource.json",
			expected: &datamodel.Location{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:   "/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/locations/east",
						Name: "east",
						Type: datamodel.LocationResourceType,
					},
					InternalMetadata: v1.InternalMetadata{
						UpdatedAPIVersion: Version,
					},
				},
				Properties: datamodel.LocationProperties{
					Address: to.Ptr("https://east.myrp.com"),
					ResourceTypes: map[string]datamodel.LocationResourceTypeConfiguration{
						"testResources": {
							APIVersions: map[string]datamodel.LocationAPIVersionConfiguration{
								"2025-01-01": {},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			versioned := &LocationResource{}
			err := json.Unmarshal(rawPayload, versioned)
			require.NoError(t, err)

			dm, err := versioned.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, dm)
			}
		})
	}
}

func Test_Location_DataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *LocationResource
		err      error
	}{
		{
			filename: "location_datamodel.json",
			expected: &LocationResource{
				ID:   to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/locations/east"),
				Type: to.Ptr(datamodel.LocationResourceType),
				Name: to.Ptr("east"),
				Properties: &LocationProperties{
					ProvisioningState: to.Ptr(ProvisioningStateSucceeded),
					Address:           to.Ptr("https://east.myrp.com"),
					ResourceTypes: map[string]*LocationResourceType{
						"testResources": {
							APIVersions: map[string]map[string]any{
								"2025-01-01": {},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			data := &datamodel.Location{}
			err := json.Unmarshal(rawPayload, data)
			require.NoError(t, err)

			versioned := &LocationResource{}

			err = versioned.ConvertFrom(data)

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, versioned)
			}
		})
	}
}
