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

package v20250801preview

import (
	"encoding/json"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/testutil/resourcetypeutil"
	"github.com/stretchr/testify/require"
)

func TestApplicationConvertVersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		err      error
	}{
		{
			filename: "applicationresource.json",
			err:      nil,
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &ApplicationResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				ct := dm.(*datamodel.Application_v20250801preview)
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Radius.Core/applications/app0", ct.ID)
				require.Equal(t, "app0", ct.Name)
				require.Equal(t, "Radius.Core/applications", ct.Type)
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Radius.Core/environments/env0", ct.Properties.Environment)
				require.Equal(t, "2025-08-01-preview", ct.InternalMetadata.UpdatedAPIVersion)
			}
		})
	}
}

func TestApplicationConvertDataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		err      error
	}{
		{
			filename: "applicationresourcedatamodel.json",
			err:      nil,
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &datamodel.Application_v20250801preview{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			versioned := &ApplicationResource{}
			err = versioned.ConvertFrom(r)

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Radius.Core/applications/app0", r.ID)
				require.Equal(t, "app0", r.Name)
				require.Equal(t, "Radius.Core/applications", r.Type)
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Radius.Core/environments/env0", r.Properties.Environment)
			}
		})
	}
}

func TestApplicationConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&resourcetypeutil.FakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &ApplicationResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
