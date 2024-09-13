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

func Test_ResourceProvider_VersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *datamodel.ResourceProvider
		err      error
	}{
		{
			filename: "resourceprovider_resource.json",
			expected: &datamodel.ResourceProvider{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test",
						Name:     "Applications.Test",
						Type:     datamodel.ResourceProviderResourceType,
						Location: "global",
						Tags:     map[string]string{},
					},
					InternalMetadata: v1.InternalMetadata{
						UpdatedAPIVersion: Version,
					},
				},
				Properties: datamodel.ResourceProviderProperties{},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			versioned := &ResourceProviderResource{}
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

func Test_ResourceProvider_DataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *ResourceProviderResource
		err      error
	}{
		{
			filename: "resourceprovider_datamodel.json",
			expected: &ResourceProviderResource{
				ID:       to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test"),
				Type:     to.Ptr(datamodel.ResourceProviderResourceType),
				Name:     to.Ptr("Applications.Test"),
				Location: to.Ptr("global"),
				Tags:     map[string]*string{},
				Properties: &ResourceProviderProperties{
					ProvisioningState: to.Ptr(ProvisioningStateSucceeded),
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			data := &datamodel.ResourceProvider{}
			err := json.Unmarshal(rawPayload, data)
			require.NoError(t, err)

			versioned := &ResourceProviderResource{}

			err = versioned.ConvertFrom(data)

			// Ignore system data.
			versioned.SystemData = nil

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, versioned)
			}
		})
	}
}
