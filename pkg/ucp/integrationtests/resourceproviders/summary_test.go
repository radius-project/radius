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
	"fmt"
	"net/http"
	"testing"

	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/testhost"
	"github.com/stretchr/testify/require"
)

func Test_ResourceProviderSummary_Lifecycle(t *testing.T) {
	server := testhost.Start(t)
	defer server.Close()

	createRadiusPlane(server)

	// List should begin empty
	response := server.MakeRequest(http.MethodGet, resourceProviderSummaryCollectionURL, nil)
	response.EqualsEmptyList()

	// Getting a specific resource provider should return 404 with the correct resource ID.
	response = server.MakeRequest(http.MethodGet, resourceProviderSummaryURL, nil)
	response.EqualsErrorCode(404, "NotFound")
	require.Equal(t, fmt.Sprintf("the resource provider with name '%s' was not found", resourceProviderNamespace), response.Error.Error.Message)
	require.Equal(t, "", response.Error.Error.Target)

	createResourceProvider(server)

	// List should now contain a resource provider.
	expected := v20231001preview.ResourceProviderSummary{
		Name:          to.Ptr(resourceProviderNamespace),
		Locations:     map[string]map[string]any{},
		ResourceTypes: map[string]*v20231001preview.ResourceProviderSummaryResourceType{},
	}
	response = server.MakeRequest(http.MethodGet, resourceProviderSummaryCollectionURL, nil)
	response.EqualsValue(200, map[string]any{
		"value": []any{expected},
	})

	// Getting a specific resource provider should return the resource provide summary.
	response = server.MakeRequest(http.MethodGet, resourceProviderSummaryURL, nil)
	response.EqualsValue(200, expected)

	// Now we'll iteratively add/remove elements and verify the summary is updated.
	createResourceType(server)
	expected.ResourceTypes[resourceTypeName] = &v20231001preview.ResourceProviderSummaryResourceType{
		APIVersions:       map[string]map[string]any{},
		DefaultAPIVersion: to.Ptr("2025-01-01"),
	}

	response = server.MakeRequest(http.MethodGet, resourceProviderSummaryURL, nil)
	response.EqualsValue(200, expected)

	createAPIVersion(server)
	expected.ResourceTypes[resourceTypeName].APIVersions["2025-01-01"] = map[string]any{}

	response = server.MakeRequest(http.MethodGet, resourceProviderSummaryURL, nil)
	response.EqualsValue(200, expected)

	createLocation(server)
	expected.Locations["east"] = map[string]any{}

	response = server.MakeRequest(http.MethodGet, resourceProviderSummaryURL, nil)
	response.EqualsValue(200, expected)

	deleteAPIVersion(server)
	delete(expected.ResourceTypes[resourceTypeName].APIVersions, "2025-01-01")

	response = server.MakeRequest(http.MethodGet, resourceProviderSummaryURL, nil)
	response.EqualsValue(200, expected)

	deleteResourceType(server)
	delete(expected.ResourceTypes, resourceTypeName)

	response = server.MakeRequest(http.MethodGet, resourceProviderSummaryURL, nil)
	response.EqualsValue(200, expected)

	deleteLocation(server)
	delete(expected.Locations, "east")

	response = server.MakeRequest(http.MethodGet, resourceProviderSummaryURL, nil)
	response.EqualsValue(200, expected)

	deleteResourceProvider(server)

	response = server.MakeRequest(http.MethodGet, resourceProviderSummaryURL, nil)
	response.EqualsErrorCode(404, "NotFound")
	require.Equal(t, fmt.Sprintf("the resource provider with name '%s' was not found", resourceProviderNamespace), response.Error.Error.Message)
	require.Equal(t, "", response.Error.Error.Target)
}
