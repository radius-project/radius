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
	"time"

	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/testhost"
	"github.com/stretchr/testify/require"
)

const (
	resourceProviderEmptyListResponseFixture = "testdata/resourceprovider_v20231001preview_emptylist_responsebody.json"
	resourceProviderListResponseFixture      = "testdata/resourceprovider_v20231001preview_list_responsebody.json"

	manifestResourceProviderListResponseFixture = "testdata/resourceprovider_manifest_list_responsebody.json"
	manifestResourceProviderResponseFixture     = "testdata/resourceprovider_manifest_responsebody.json"

	manifestResourceTypeListResponseFixture = "testdata/resourcetype_manifest_list_responsebody.json"

	registerManifestWaitDuration = 30 * time.Second
	registerManifestWaitInterval = 3 * time.Second
)

func Test_ResourceProvider_Lifecycle(t *testing.T) {
	server := testhost.Start(t)
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
	server := testhost.Start(t)
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

func Test_ResourceProvider_RegisterManifests(t *testing.T) {
	server := testhost.Start(t, testhost.TestHostOptionFunc(func(options *ucp.Options) {
		options.Config.Initialization.ManifestDirectory = "testdata/manifests"
	}))
	defer server.Close()

	createRadiusPlane(server)

	require.Eventuallyf(t, func() bool {
		// Responses should contain the resource provider and resource type in the manifest
		response := server.MakeRequest(http.MethodGet, manifestResourceProviderCollectionURL, nil)
		response.EqualsFixture(200, manifestResourceProviderListResponseFixture)

		response = server.MakeRequest(http.MethodGet, manifestResourceProviderURL, nil)
		response.EqualsFixture(200, manifestResourceProviderResponseFixture)

		response = server.MakeRequest(http.MethodGet, manifestResourceTypeCollectionURL, nil)
		response.EqualsFixture(200, manifestResourceTypeListResponseFixture)

		response = server.MakeRequest(http.MethodGet, manifestResourceTypeURL, nil)
		response.EqualsFixture(200, manifestResourceTypeResponseFixture)

		deleteManifestResourceProvider(server)

		return true
	}, registerManifestWaitDuration, registerManifestWaitInterval, "manifest registration did not complete in time")
}
