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

func Test_AWSPlane_ConvertVersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *datamodel.AWSPlane
		err      error
	}{
		{
			filename: "awsplane-resource-empty.json",
			expected: &datamodel.AWSPlane{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/aws/aws",
						Name:     "aws",
						Type:     datamodel.AWSPlaneResourceType,
						Location: "global",
						Tags: map[string]string{
							"env": "dev",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						UpdatedAPIVersion: Version,
					},
				},
				Properties: datamodel.AWSPlaneProperties{},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &AwsPlaneResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				ct := dm.(*datamodel.AWSPlane)
				require.Equal(t, tt.expected, ct)
			}
		})
	}
}

func Test_AWSPlane_ConvertDataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *AwsPlaneResource
		err      error
	}{
		{
			filename: "awsplane-datamodel-empty.json",
			expected: &AwsPlaneResource{
				ID:       to.Ptr("/planes/aws/aws"),
				Name:     to.Ptr("aws"),
				Type:     to.Ptr(datamodel.AWSPlaneResourceType),
				Location: to.Ptr("global"),
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				Properties: &AwsPlaneResourceProperties{
					ProvisioningState: fromProvisioningStateDataModel(v1.ProvisioningStateSucceeded),
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			dm := &datamodel.AWSPlane{}
			err := json.Unmarshal(rawPayload, dm)
			require.NoError(t, err)

			resource := &AwsPlaneResource{}
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
