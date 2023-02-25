// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220901privatepreview

import (
	"encoding/json"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/test/testutil"

	"github.com/stretchr/testify/require"
)

func TestResourceGroupConvertVersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *datamodel.ResourceGroup
		err      error
	}{
		{
			filename: "resourcegroup.json",
			expected: &datamodel.ResourceGroup{
				TrackedResource: v1.TrackedResource{
					ID:       "/planes/radius/local/resourceGroups/test-rg",
					Name:     "test-rg",
					Type:     "System.Resources/resourceGroups",
					Location: v1.LocationGlobal,
					Tags: map[string]string{
						"env": "dev",
					},
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &ResourceGroupResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			// act
			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				ct := dm.(*datamodel.ResourceGroup)
				require.Equal(t, tt.expected, ct)
			}
		})
	}
}

func TestResourceGroupConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("resourcegroupresourcedatamodel.json")
	r := &datamodel.ResourceGroup{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	versioned := &ResourceGroupResource{}
	err = versioned.ConvertFrom(r)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/planes/radius/local/resourceGroups/test-rg", r.TrackedResource.ID)
	require.Equal(t, "test-rg", r.TrackedResource.Name)
}

func TestResourceGroupConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &ResourceGroupResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
