// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220901privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/stretchr/testify/require"

	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
)

func TestPlaneConvertVersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *datamodel.Plane
		err      error
	}{
		{
			filename: "planeresource.json",
			expected: &datamodel.Plane{
				TrackedResource: v1.TrackedResource{
					ID:   "/planes/radius/local",
					Name: "local",
					Type: "System.Planes/radius",
				},
				Properties: datamodel.PlaneProperties{
					Kind: datamodel.PlaneKind(PlaneKindUCPNative),
					ResourceProviders: map[string]*string{
						"Applications.Core": to.Ptr("https://applications.core.radius.azure.com"),
					},
				},
			},
		},
		{
			filename: "planeresource-invalid-missing-kind.json",
			err:      &conv.ErrModelConversion{PropertyName: "$.properties.kind", ValidValue: "63 characters or less"},
		},
		{
			filename: "planeresource-empty-resourceproviders.json",
			err:      &conv.ErrModelConversion{PropertyName: "$.properties.resourceProviders", ValidValue: "at least one provided"},
		},
		{
			filename: "planeresource-invalid-missing-url.json",
			err:      &conv.ErrModelConversion{PropertyName: "$.properties.URL", ValidValue: "non-empty string"},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := radiustesting.ReadFixture(tt.filename)
			r := &PlaneResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			// act
			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				ct := dm.(*datamodel.Plane)
				require.Equal(t, tt.expected, ct)
			}
		})
	}
}

func TestPlaneConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := radiustesting.ReadFixture("planeresourcedatamodel.json")
	r := &datamodel.Plane{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	versioned := &PlaneResource{}
	err = versioned.ConvertFrom(r)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/planes/radius/local", r.TrackedResource.ID)
	require.Equal(t, "local", r.TrackedResource.Name)
	require.Equal(t, "System.Planes/radius", r.TrackedResource.Type)
	require.Equal(t, datamodel.PlaneKind("UCPNative"), r.Properties.Kind)
	require.Equal(t, "https://applications.core.radius.azure.com", *r.Properties.ResourceProviders["Applications.Core"])
}

type fakeResource struct{}

func (f *fakeResource) ResourceTypeName() string {
	return "FakeResource"
}

func TestPlaneConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &PlaneResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
