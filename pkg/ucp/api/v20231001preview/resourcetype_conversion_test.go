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
	"crypto/sha256"
	"encoding/hex"
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
					Capabilities:      []string{},
					DefaultAPIVersion: new("2025-01-01"),
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
				ID:   new("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources"),
				Type: to.Ptr(datamodel.ResourceTypeResourceType),
				Name: new("testResources"),
				Properties: &ResourceTypeProperties{
					ProvisioningState: new(ProvisioningStateSucceeded),
					Capabilities:      []*string{},
					DefaultAPIVersion: new("2025-01-01"),
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

func Test_ResourceType_Icon_VersionedToDataModel(t *testing.T) {
	const svg = `<svg xmlns="http://www.w3.org/2000/svg"></svg>`
	sum := sha256.Sum256([]byte(svg))
	expectedHash := hex.EncodeToString(sum[:])

	t.Run("icon present is stored verbatim and hashed server-side", func(t *testing.T) {
		versioned := &ResourceTypeResource{
			ID:   to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources"),
			Name: to.Ptr("testResources"),
			Properties: &ResourceTypeProperties{
				Icon: to.Ptr(svg),
			},
		}

		dm, err := versioned.ConvertTo()
		require.NoError(t, err)

		rt := dm.(*datamodel.ResourceType)
		require.NotNil(t, rt.Properties.Icon)
		require.Equal(t, svg, *rt.Properties.Icon)
		require.NotNil(t, rt.Properties.IconHash)
		require.Equal(t, expectedHash, *rt.Properties.IconHash)
	})

	t.Run("no icon leaves icon and hash unset", func(t *testing.T) {
		versioned := &ResourceTypeResource{
			ID:         to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources"),
			Name:       to.Ptr("testResources"),
			Properties: &ResourceTypeProperties{},
		}

		dm, err := versioned.ConvertTo()
		require.NoError(t, err)

		rt := dm.(*datamodel.ResourceType)
		require.Nil(t, rt.Properties.Icon)
		require.Nil(t, rt.Properties.IconHash)
	})
}

func Test_ResourceType_ConvertTo_RejectsInvalidIcon(t *testing.T) {
	tests := []struct {
		name    string
		icon    string
		wantErr string
	}{
		{
			name:    "not svg",
			icon:    "<html><body/></html>",
			wantErr: "invalid icon",
		},
		{
			name:    "script element",
			icon:    `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
			wantErr: "invalid icon",
		},
		{
			name:    "event handler",
			icon:    `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`,
			wantErr: "invalid icon",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			versioned := &ResourceTypeResource{
				ID:   to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources"),
				Name: to.Ptr("testResources"),
				Properties: &ResourceTypeProperties{
					Icon: to.Ptr(tc.icon),
				},
			}
			_, err := versioned.ConvertTo()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func Test_ResourceType_Icon_DataModelToVersioned(t *testing.T) {
	dm := &datamodel.ResourceType{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources",
				Name: "testResources",
				Type: datamodel.ResourceTypeResourceType,
			},
		},
		Properties: datamodel.ResourceTypeProperties{
			Capabilities: []string{},
			Icon:         to.Ptr(`<svg/>`),
			IconHash:     to.Ptr("abc123"),
		},
	}

	versioned := &ResourceTypeResource{}
	err := versioned.ConvertFrom(dm)
	require.NoError(t, err)

	require.NotNil(t, versioned.Properties.Icon)
	require.Equal(t, `<svg/>`, *versioned.Properties.Icon)
	require.NotNil(t, versioned.Properties.IconHash)
	require.Equal(t, "abc123", *versioned.Properties.IconHash)
}

func Test_validateCapability(t *testing.T) {
	tests := []struct {
		name        string
		input       *string
		expectedErr error
	}{
		{
			name:  "valid capability",
			input: to.Ptr(string(datamodel.CapabilityManualResourceProvisioning)),
		},
		{
			name:        "invalid capability",
			input:       new("InvalidCapability"),
			expectedErr: v1.NewClientErrInvalidRequest("capability \"InvalidCapability\" is not recognized. Supported capabilities: ManualResourceProvisioning"),
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
