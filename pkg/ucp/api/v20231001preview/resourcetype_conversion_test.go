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

func Test_ResourceType_VersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *datamodel.ResourceType
		err      error
	}{
		{
			filename: "resourcetype_resource.json",
			expected: &datamodel.ResourceType{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:   "/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources",
						Name: "testResources",
						Type: datamodel.ResourceTypeResourceType,
					},
					InternalMetadata: v1.InternalMetadata{
						UpdatedAPIVersion: Version,
					},
				},
				Properties: datamodel.ResourceTypeProperties{
					Capabilities:      []string{"SupportsRecipes"},
					DefaultAPIVersion: to.Ptr("2025-01-01"),
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			versioned := &ResourceTypeResource{}
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

func Test_ResourceType_DataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *ResourceTypeResource
		err      error
	}{
		{
			filename: "resourcetype_datamodel.json",
			expected: &ResourceTypeResource{
				ID:   to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources"),
				Type: to.Ptr(datamodel.ResourceTypeResourceType),
				Name: to.Ptr("testResources"),
				Properties: &ResourceTypeProperties{
					ProvisioningState: to.Ptr(ProvisioningStateSucceeded),
					Capabilities:      []*string{to.Ptr("SupportsRecipes")},
					DefaultAPIVersion: to.Ptr("2025-01-01"),
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			data := &datamodel.ResourceType{}
			err := json.Unmarshal(rawPayload, data)
			require.NoError(t, err)

			versioned := &ResourceTypeResource{}

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

func Test_validateCapability(t *testing.T) {
	tests := []struct {
		name        string
		input       *string
		expectedErr error
	}{
		{
			name:  "valid capability",
			input: to.Ptr(datamodel.CapabilitySupportsRecipes),
		},
		{
			name:        "invalid capability",
			input:       to.Ptr("InvalidCapability"),
			expectedErr: v1.NewClientErrInvalidRequest("capability \"InvalidCapability\" is not recognized. Supported capabilities: SupportsRecipes"),
		},
		{
			name:        "nil capability",
			input:       nil,
			expectedErr: v1.NewClientErrInvalidRequest("capability cannot be null"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCapability(tt.input)
			if tt.expectedErr != nil {
				require.Error(t, err)
				require.Equal(t, tt.expectedErr, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
