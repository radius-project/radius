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

func Test_RadiusPlane_ConvertVersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *datamodel.RadiusPlane
		err      error
	}{
		{
			filename: "radiusplane-resource-empty.json",
			expected: &datamodel.RadiusPlane{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/radius/local",
						Name:     "local",
						Type:     "System.Radius/planes",
						Location: "global",
						Tags: map[string]string{
							"env": "dev",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						UpdatedAPIVersion: Version,
					},
				},
				Properties: datamodel.RadiusPlaneProperties{
					ResourceProviders: map[string]string{
						"Applications.Core": "http://applications-rp:9000",
					},
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &RadiusPlaneResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				ct := dm.(*datamodel.RadiusPlane)
				require.Equal(t, tt.expected, ct)
			}
		})
	}
}

func Test_RadiusPlane_ConvertDataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *RadiusPlaneResource
		err      error
	}{
		{
			filename: "radiusplane-datamodel-empty.json",
			expected: &RadiusPlaneResource{
				ID:       to.Ptr("/planes/radius/local"),
				Name:     to.Ptr("local"),
				Type:     to.Ptr("System.Radius/planes"),
				Location: to.Ptr("global"),
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				Properties: &RadiusPlaneResourceProperties{
					ProvisioningState: fromProvisioningStateDataModel(v1.ProvisioningStateSucceeded),
					ResourceProviders: map[string]*string{
						"Applications.Core": to.Ptr("http://applications-rp:9000"),
					},
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			dm := &datamodel.RadiusPlane{}
			err := json.Unmarshal(rawPayload, dm)
			require.NoError(t, err)

			resource := &RadiusPlaneResource{}
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
