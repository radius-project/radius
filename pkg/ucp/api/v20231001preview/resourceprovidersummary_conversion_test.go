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
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/test/testutil"

	"github.com/stretchr/testify/require"
)

// Note: ResourceProviderSummary is READONLY. There is no conversion from versioned to datamodel.

func Test_ResourceProviderSummary_DataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *ResourceProviderSummary
		err      error
	}{
		{
			filename: "resourceprovidersummary_datamodel.json",
			expected: &ResourceProviderSummary{
				Name: to.Ptr("Applications.Test"),
				Locations: map[string]map[string]any{
					"east": {},
				},
				ResourceTypes: map[string]*ResourceProviderSummaryResourceType{
					"testResources": {
						Capabilities:      []*string{to.Ptr("SupportsRecipes")},
						DefaultAPIVersion: to.Ptr("2025-01-01"),
						APIVersions: map[string]*ResourceTypeSummaryResultAPIVersion{
							"2024-01-01": {
								Schema: map[string]any{
									"2024-01-01": map[string]any{},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			data := &datamodel.ResourceProviderSummary{}
			err := json.Unmarshal(rawPayload, data)
			require.NoError(t, err)

			versioned := &ResourceProviderSummary{}

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
