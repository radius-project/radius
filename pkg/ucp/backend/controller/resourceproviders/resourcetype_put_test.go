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

	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func TestResourceTypePutController_updateSummary(t *testing.T) {
	id := resources.MustParse("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test/resourceTypes/testResources")
	resourceType := &datamodel.ResourceType{
		Properties: datamodel.ResourceTypeProperties{
			DefaultAPIVersion: to.Ptr("2025-01-01"),
		},
	}

	summary := &datamodel.ResourceProviderSummary{
		Properties: datamodel.ResourceProviderSummaryProperties{
			ResourceTypes: map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{},
		},
	}

	expected := &datamodel.ResourceProviderSummary{
		Properties: datamodel.ResourceProviderSummaryProperties{
			ResourceTypes: map[string]datamodel.ResourceProviderSummaryPropertiesResourceType{
				"testResources": {
					DefaultAPIVersion: to.Ptr("2025-01-01"),
				},
			},
		},
	}

	controller := &ResourceTypePutController{}
	err := controller.updateSummary(id, resourceType)(summary)
	require.NoError(t, err)
	require.Equal(t, expected, summary)
}
