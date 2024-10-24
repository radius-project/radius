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

package resourceproviders

import (
	"testing"

	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func TestAPIVersionDeleteController_updateSummary(t *testing.T) {
	tests := []struct {
		name     string
		id       resources.ID
		summary  *datamodel.ResourceProviderSummary
		expected *datamodel.ResourceProviderSummary
	}{
		{
			name: "Delete existing API version",
			id:   resources.MustParse("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources/apiVersions/2025-01-01"),
			summary: &datamodel.ResourceProviderSummary{
				Properties: datamodel.ResourceProviderSummaryProperties{
					ResourceTypes: map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{
						"testResources": {
							APIVersions: map[string]datamodel.ResourceProviderSummaryPropertiesAPIVersion{
								"2025-01-01": {},
							},
						},
					},
				},
			},
			expected: &datamodel.ResourceProviderSummary{
				Properties: datamodel.ResourceProviderSummaryProperties{
					ResourceTypes: map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{
						"testResources": {
							APIVersions: map[string]datamodel.ResourceProviderSummaryPropertiesAPIVersion{},
						},
					},
				},
			},
		},
		{
			name: "Delete non-existing API version",
			id:   resources.MustParse("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources/apiVersions/2025-01-01"),
			summary: &datamodel.ResourceProviderSummary{
				Properties: datamodel.ResourceProviderSummaryProperties{
					ResourceTypes: map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{
						"testResources": {
							APIVersions: map[string]datamodel.ResourceProviderSummaryPropertiesAPIVersion{
								"2024-01-01": {},
							},
						},
					},
				},
			},
			expected: &datamodel.ResourceProviderSummary{
				Properties: datamodel.ResourceProviderSummaryProperties{
					ResourceTypes: map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{
						"testResources": {
							APIVersions: map[string]datamodel.ResourceProviderSummaryPropertiesAPIVersion{
								"2024-01-01": {},
							},
						},
					},
				},
			},
		},
		{
			name: "Delete API version from non-existing resource type",
			id:   resources.MustParse("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/nonExistingResources/apiVersions/2025-01-01"),
			summary: &datamodel.ResourceProviderSummary{
				Properties: datamodel.ResourceProviderSummaryProperties{
					ResourceTypes: map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{
						"testResources": {
							APIVersions: map[string]datamodel.ResourceProviderSummaryPropertiesAPIVersion{
								"2025-01-01": {},
							},
						},
					},
				},
			},
			expected: &datamodel.ResourceProviderSummary{
				Properties: datamodel.ResourceProviderSummaryProperties{
					ResourceTypes: map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{
						"testResources": {
							APIVersions: map[string]datamodel.ResourceProviderSummaryPropertiesAPIVersion{
								"2025-01-01": {},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &APIVersionDeleteController{}
			updateFunc := controller.updateSummary(tt.id)
			err := updateFunc(tt.summary)
			require.NoError(t, err)
			require.Equal(t, tt.expected, tt.summary)
		})
	}
}
