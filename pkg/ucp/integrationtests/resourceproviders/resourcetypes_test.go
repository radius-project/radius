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
	resourceTypeEmptyListResponseFixture = "testdata/resourcetype_v20231001preview_emptylist_responsebody.json"
	resourceTypeListResponseFixture      = "testdata/resourcetype_v20231001preview_list_responsebody.json"
)

func Test_ResourceType_Lifecycle(t *testing.T) {
	server := testserver.StartWithETCD(t, api.DefaultModules)
	defer server.Close()

	createRadiusPlane(server)
	createResourceProvider(server)

	// We don't use t.Run() here because we want the test to fail if *any* of these steps fail.

	// List should start empty
	response := server.MakeRequest(http.MethodGet, resourceTypeCollectionURL, nil)
	response.EqualsFixture(200, resourceTypeEmptyListResponseFixture)

	// Getting a specific resource type should return 404 with the correct resource ID.
	response = server.MakeRequest(http.MethodGet, resourceTypeURL, nil)
	response.EqualsErrorCode(404, "NotFound")
	require.Equal(t, resourceTypeID, response.Error.Error.Target)

	// Create a resource provider
	createResourceType(server)

	// List should now contain the resource provider
	response = server.MakeRequest(http.MethodGet, resourceTypeCollectionURL, nil)
	response.EqualsFixture(200, resourceTypeListResponseFixture)

	response = server.MakeRequest(http.MethodGet, resourceTypeURL, nil)
	response.EqualsFixture(200, resourceTypeResponseFixture)

	deleteResourceType(server)

	deleteResourceProvider(server)
}

func Test_ResourceType_CascadingDelete(t *testing.T) {
	server := testserver.StartWithETCD(t, api.DefaultModules)
	defer server.Close()

	createRadiusPlane(server)
	createResourceProvider(server)

	// We don't use t.Run() here because we want the test to fail if *any* of these steps fail.

	// Create a resource provider
	createResourceType(server)

	// Create an API version inside the resource type
	createAPIVersion(server)

	// This will trigger deletion of the API version.
	deleteResourceType(server)

	// The API version should be gone now.
	response := server.MakeRequest(http.MethodGet, apiVersionURL, nil)
	response.EqualsErrorCode(404, "NotFound")
	require.Equal(t, apiVersionID, response.Error.Error.Target)

	deleteResourceProvider(server)
}
