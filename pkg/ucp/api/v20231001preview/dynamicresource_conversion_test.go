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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/test/testutil"

	"github.com/stretchr/testify/require"
)

func Test_DynamicResource_ConvertVersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *datamodel.DynamicResource
		err      error
	}{
		{
			filename: "dynamicresource-resource.json",
			expected: &datamodel.DynamicResource{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/radius/local/resourceGroups/test/providers/Applications.Test/testResources/testResource",
						Name:     "testResource",
						Type:     "Applications.Test/testResources",
						Location: "global",
						Tags: map[string]string{
							"env": "dev",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						UpdatedAPIVersion: Version,
					},
				},
				Properties: map[string]any{
					"message": "Hello, world!",
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &DynamicResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				ct := dm.(*datamodel.DynamicResource)
				require.Equal(t, tt.expected, ct)
			}
		})
	}
}

func Test_DynamicResource_ConvertDataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *DynamicResource
		err      error
	}{
		{
			filename: "dynamicresource-datamodel.json",
			expected: &DynamicResource{
				ID:       to.Ptr("/planes/radius/local/resourceGroups/test/providers/Applications.Test/testResources/testResource"),
				Name:     to.Ptr("testResource"),
				Type:     to.Ptr("Applications.Test/testResources"),
				Location: to.Ptr("global"),
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				Properties: map[string]any{
					"provisioningState": fromProvisioningStateDataModel(v1.ProvisioningStateSucceeded),
					"message":           "Hello, world!",
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			dm := &datamodel.DynamicResource{}
			err := json.Unmarshal(rawPayload, dm)
			require.NoError(t, err)

			resource := &DynamicResource{}
			err = resource.ConvertFrom(dm)

			// Avoid hardcoding the SystemData field in tests.
			tt.expected.SystemData = fromSystemDataModel(dm.SystemData)

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, resource)
			}
		})
	}
}
