// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func TestApplicationConvertVersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		err      error
		emptyExt bool
	}{
		{
			filename: "applicationresource.json",
			err:      nil,
			emptyExt: false,
		},
		{
			filename: "applicationresourceemptyext.json",
			err:      nil,
			emptyExt: true,
		},
		{
			filename: "applicationresourceemptyext2.json",
			err:      nil,
			emptyExt: true,
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &ApplicationResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			// act
			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				// assert
				require.NoError(t, err)
				ct := dm.(*datamodel.Application)
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/app0", ct.ID)
				require.Equal(t, "app0", ct.Name)
				require.Equal(t, "Applications.Core/applications", ct.Type)
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/environments/env0", ct.Properties.Environment)
				require.Equal(t, "2022-03-15-privatepreview", ct.InternalMetadata.UpdatedAPIVersion)
				if tt.emptyExt {
					require.Equal(t, getTestKubernetesEmptyMetadataExtensions(t), ct.Properties.Extensions)
				} else {
					exts := getTestKubernetesMetadataExtensions(t)
					exts = append(exts, datamodel.Extension{Kind: datamodel.KubernetesNamespaceExtension, KubernetesNamespace: &datamodel.KubeNamespaceExtension{Namespace: "app0-ns"}})
					require.Equal(t, exts, ct.Properties.Extensions)
				}
			}
		})
	}

}

func TestApplicationConvertDataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		err      error
		emptyExt bool
	}{
		{
			filename: "applicationresourcedatamodel.json",
			err:      nil,
			emptyExt: false,
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &datamodel.Application{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			// act
			versioned := &ApplicationResource{}
			err = versioned.ConvertFrom(r)

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				// assert
				require.NoError(t, err)
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/app0", r.ID)
				require.Equal(t, "app0", r.Name)
				require.Equal(t, "Applications.Core/applications", r.Type)
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/environments/env0", r.Properties.Environment)
				require.Equal(t, "kubernetesMetadata", *versioned.Properties.Extensions[0].GetExtension().Kind)
				require.Equal(t, "kubernetesNamespace", *versioned.Properties.Extensions[1].GetExtension().Kind)
				require.Equal(t, "app0-ns", *(versioned.Properties.Status.Compute.(*KubernetesCompute).Namespace))
			}
		})
	}
}

func TestApplicationConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &ApplicationResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
