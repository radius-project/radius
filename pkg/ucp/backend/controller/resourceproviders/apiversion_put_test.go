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

func TestAPIVersionPutController_updateSummary(t *testing.T) {
	id := resources.MustParse("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources/apiVersions/2025-01-01")
	tests := []struct {
		name            string
		id              resources.ID
		initialSummary  *datamodel.ResourceProviderSummary
		expectedSummary *datamodel.ResourceProviderSummary
		expectError     bool
	}{
		{
			name: "Resource type entry not found",
			id:   id,
			initialSummary: &datamodel.ResourceProviderSummary{
				Properties: datamodel.ResourceProviderSummaryProperties{
					ResourceTypes: map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{},
				},
			},
			expectedSummary: &datamodel.ResourceProviderSummary{
				Properties: datamodel.ResourceProviderSummaryProperties{
					ResourceTypes: map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{},
				},
			},
			expectError: true,
		},
		{
			name: "APIVersion entry added",
			id:   id,
			initialSummary: &datamodel.ResourceProviderSummary{
				Properties: datamodel.ResourceProviderSummaryProperties{
					ResourceTypes: map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{
						"testResources": {
							APIVersions: map[string]datamodel.ResourceProviderSummaryPropertiesAPIVersion{},
						},
					},
				},
			},
			expectedSummary: &datamodel.ResourceProviderSummary{
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
			expectError: false,
		},
		{
			name: "APIVersion entry already exists",
			id:   id,
			initialSummary: &datamodel.ResourceProviderSummary{
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
			expectedSummary: &datamodel.ResourceProviderSummary{
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
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &APIVersionPutController{}
			updateFunc := controller.updateSummary(tt.id, nil)
			err := updateFunc(tt.initialSummary)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedSummary, tt.initialSummary)
			}
		})
	}
}
