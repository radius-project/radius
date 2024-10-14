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

func Test_APIVersion_VersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *datamodel.APIVersion
		err      error
	}{
		{
			filename: "apiversion_resource.json",
			expected: &datamodel.APIVersion{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:   "/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources/apiVersions/2025-01-01",
						Name: "2025-01-01",
						Type: datamodel.APIVersionResourceType,
					},
					InternalMetadata: v1.InternalMetadata{
						UpdatedAPIVersion: Version,
					},
				},
				Properties: datamodel.APIVersionProperties{},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			versioned := &APIVersionResource{}
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

func Test_APIVersion_DataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *APIVersionResource
		err      error
	}{
		{
			filename: "apiversion_datamodel.json",
			expected: &APIVersionResource{
				ID:   to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources/apiVersions/2025-01-01"),
				Type: to.Ptr(datamodel.APIVersionResourceType),
				Name: to.Ptr("2025-01-01"),
				Properties: &APIVersionProperties{
					ProvisioningState: to.Ptr(ProvisioningStateSucceeded),
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			data := &datamodel.APIVersion{}
			err := json.Unmarshal(rawPayload, data)
			require.NoError(t, err)

			versioned := &APIVersionResource{}

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
