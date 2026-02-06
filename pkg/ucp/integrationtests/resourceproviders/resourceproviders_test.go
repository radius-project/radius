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
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/testhost"
	"github.com/stretchr/testify/assert"
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

	// Use EventuallyWithTf so that assertion failures inside the callback are collected
	// on a CollectT instead of the real *testing.T. This allows retries to work correctly;
	// EqualsFixture uses require (t.FailNow) which would permanently fail the test on the
	// first unsuccessful attempt if called with the real t.
	require.EventuallyWithTf(t, func(collect *assert.CollectT) {
		fixtures := []struct {
			method  string
			url     string
			fixture string
		}{
			{http.MethodGet, manifestResourceProviderCollectionURL, manifestResourceProviderListResponseFixture},
			{http.MethodGet, manifestResourceProviderURL, manifestResourceProviderResponseFixture},
			{http.MethodGet, manifestResourceTypeCollectionURL, manifestResourceTypeListResponseFixture},
			{http.MethodGet, manifestResourceTypeURL, manifestResourceTypeResponseFixture},
		}

		for _, f := range fixtures {
			response := server.MakeRequest(f.method, f.url, nil)

			expected, err := os.ReadFile(f.fixture)
			if !assert.NoError(collect, err, "reading fixture %s failed", f.fixture) {
				return
			}

			var expectedBody map[string]any
			if !assert.NoError(collect, json.Unmarshal(expected, &expectedBody), "unmarshalling expected response failed") {
				return
			}

			var actualBody map[string]any
			if !assert.NoError(collect, json.Unmarshal(response.Body.Bytes(), &actualBody), "unmarshalling actual response failed") {
				return
			}

			// Remove systemData for comparison consistency, matching the behavior
			// of TestResponse.removeSystemData.
			removeSystemData(actualBody)

			if !assert.EqualValues(collect, expectedBody, actualBody, "response body did not match expected for %s", f.url) {
				return
			}

			if !assert.Equal(collect, 200, response.Raw.StatusCode, "status code did not match expected for %s", f.url) {
				return
			}
		}
	}, registerManifestWaitDuration, registerManifestWaitInterval, "manifest registration did not complete in time")

	// Clean up after successful registration.
	deleteManifestResourceProvider(server)
}

// removeSystemData removes the systemData property from the response body recursively.
// This matches the behavior of TestResponse.removeSystemData in the testhost package.
func removeSystemData(body map[string]any) {
	if _, ok := body["systemData"]; ok {
		delete(body, "systemData")
		return
	}

	value, ok := body["value"]
	if !ok {
		return
	}

	valueSlice, ok := value.([]any)
	if !ok {
		return
	}

	for _, v := range valueSlice {
		if vMap, ok := v.(map[string]any); ok {
			removeSystemData(vMap)
		}
	}
}
