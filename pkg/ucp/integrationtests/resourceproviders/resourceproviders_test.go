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
	"net/http"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/frontend/api"
	"github.com/radius-project/radius/pkg/ucp/integrationtests/testserver"
	"github.com/stretchr/testify/require"
)

const (
	resourceProviderEmptyListResponseFixture = "testdata/resourceprovider_v20231001preview_emptylist_responsebody.json"
	resourceProviderListResponseFixture      = "testdata/resourceprovider_v20231001preview_list_responsebody.json"
)

func Test_ResourceProvider_Lifecycle(t *testing.T) {
	server := testserver.StartWithETCD(t, api.DefaultModules)
	defer server.Close()

	createRadiusPlane(server)

	// We don't use t.Run() here because we want the test to fail if *any* of these steps fail.

	// List should start empty
	response := server.MakeRequest(http.MethodGet, resourceProviderCollectionURL, nil)
	response.EqualsFixture(200, resourceProviderEmptyListResponseFixture)

	// Getting a specific resource provider should return 404 with the correct resource ID.
	response = server.MakeRequest(http.MethodGet, resourceProviderURL, nil)
	response.EqualsErrorCode(404, "NotFound")
	require.Equal(t, resourceProviderID, response.Error.Error.Target)

	// Create a resource provider
	createResourceProvider(server)

	// List should now contain the resource provider
	response = server.MakeRequest(http.MethodGet, resourceProviderCollectionURL, nil)
	response.EqualsFixture(200, resourceProviderListResponseFixture)

	response = server.MakeRequest(http.MethodGet, resourceProviderURL, nil)
	response.EqualsFixture(200, resourceProviderResponseFixture)

	deleteResourceProvider(server)
}

func Test_ResourceProvider_CascadingDelete(t *testing.T) {
	server := testserver.StartWithETCD(t, api.DefaultModules)
	defer server.Close()

	createRadiusPlane(server)

	// We don't use t.Run() here because we want the test to fail if *any* of these steps fail.

	// Create a resource provider
	createResourceProvider(server)

	// Create a resource type and location inside the resource provider
	createResourceType(server)
	createLocation(server)

	// This will trigger deletion of the resource type and location
	deleteResourceProvider(server)

	// The resource type should be gone now.
	response := server.MakeRequest(http.MethodGet, resourceTypeURL, nil)
	response.EqualsErrorCode(404, "NotFound")
	require.Equal(t, resourceTypeID, response.Error.Error.Target)

	// The location should be gone now.
	response = server.MakeRequest(http.MethodGet, locationURL, nil)
	response.EqualsErrorCode(404, "NotFound")
	require.Equal(t, locationID, response.Error.Error.Target)
}
