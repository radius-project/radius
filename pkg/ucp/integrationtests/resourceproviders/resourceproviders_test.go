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
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/testhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

// Test_ResourceProvider_RegisterManifests_NoLocation verifies that manifests
// without a "location" field are registered successfully at startup. This is
// the code path used by resource type manifests copied from
// resource-types-contrib, which omit location so that UCP routes requests via
// DefaultDownstreamEndpoint (dynamic-rp).
//
// The test directory contains two manifest files (containers.yaml and
// persistentVolumes.yaml) that share the same namespace (Radius.Compute).
// This verifies that the initializer correctly merges types from multiple
// files into the same resource provider and location.
func Test_ResourceProvider_RegisterManifests_NoLocation(t *testing.T) {
	server := testhost.Start(t, testhost.TestHostOptionFunc(func(options *ucp.Options) {
		options.Config.Initialization.ManifestDirectory = "testdata/manifests-no-location"
	}))
	defer server.Close()

	createRadiusPlane(server)

	noLocationNamespace := "Radius.Compute"
	noLocationResourceProviderURL := "/planes/radius/local/providers/System.Resources/resourceproviders/" + noLocationNamespace + radiusAPIVersion
	noLocationContainersURL := "/planes/radius/local/providers/System.Resources/resourceproviders/" + noLocationNamespace + "/resourcetypes/containers" + radiusAPIVersion
	noLocationPersistentVolumesURL := "/planes/radius/local/providers/System.Resources/resourceproviders/" + noLocationNamespace + "/resourcetypes/persistentVolumes" + radiusAPIVersion
	noLocationLocationURL := "/planes/radius/local/providers/System.Resources/resourceproviders/" + noLocationNamespace + "/locations/global" + radiusAPIVersion

	require.EventuallyWithTf(t, func(collect *assert.CollectT) {
		// Verify the resource provider was registered.
		response := server.MakeRequest(http.MethodGet, noLocationResourceProviderURL, nil)
		assert.Equal(collect, 200, response.Raw.StatusCode, "resource provider Radius.Compute should be registered")

		// Verify both resource types from separate manifest files are registered
		// under the same namespace.
		response = server.MakeRequest(http.MethodGet, noLocationContainersURL, nil)
		assert.Equal(collect, 200, response.Raw.StatusCode, "resource type Radius.Compute/containers should be registered")

		response = server.MakeRequest(http.MethodGet, noLocationPersistentVolumesURL, nil)
		assert.Equal(collect, 200, response.Raw.StatusCode, "resource type Radius.Compute/persistentVolumes should be registered")

		// Verify the location was created with no address, so UCP uses
		// DefaultDownstreamEndpoint for routing.
		response = server.MakeRequest(http.MethodGet, noLocationLocationURL, nil)
		if !assert.Equal(collect, 200, response.Raw.StatusCode, "location global should be registered") {
			return
		}

		var locationBody map[string]any
		if !assert.NoError(collect, json.Unmarshal(response.Body.Bytes(), &locationBody)) {
			return
		}

		props, _ := locationBody["properties"].(map[string]any)
		assert.Nil(collect, props["address"], "location address should be absent for no-location manifests")

		// Verify the location contains both resource types from the two manifest
		// files. Without the namespace merge fix, the location would only contain
		// the types from the last file processed (alphabetically).
		resourceTypes, _ := props["resourceTypes"].(map[string]any)
		if !assert.Contains(collect, resourceTypes, "containers", "location should contain containers type") {
			return
		}
		if !assert.Contains(collect, resourceTypes, "persistentVolumes", "location should contain persistentVolumes type") {
			return
		}
	}, registerManifestWaitDuration, registerManifestWaitInterval, "no-location manifest registration did not complete in time")
}

// Test_ResourceProvider_DefaultsRegistered verifies that all resource types
// listed in deploy/manifest/defaults.yaml are registered after startup when
// the initializer loads the real manifest files from built-in-providers/dev/.
//
// This catches regressions where:
//   - A manifest file fails to load at startup
//   - Multiple files sharing a namespace overwrite each other's types
//   - A type is added to defaults.yaml but its manifest is missing or invalid
func Test_ResourceProvider_DefaultsRegistered(t *testing.T) {
	// Read defaults.yaml to get the list of expected resource types.
	data, err := os.ReadFile("../../../../deploy/manifest/defaults.yaml")
	require.NoError(t, err, "failed to read defaults.yaml")

	var defaults struct {
		DefaultRegistration []string `yaml:"defaultRegistration"`
	}
	require.NoError(t, yaml.Unmarshal(data, &defaults), "failed to parse defaults.yaml")
	require.NotEmpty(t, defaults.DefaultRegistration, "defaults.yaml should list at least one resource type")

	// Start the test host with the real manifest directory.
	server := testhost.Start(t, testhost.TestHostOptionFunc(func(options *ucp.Options) {
		options.Config.Initialization.ManifestDirectory = "../../../../deploy/manifest/built-in-providers/dev"
	}))
	defer server.Close()

	createRadiusPlane(server)

	// Build expected types grouped by namespace for location verification.
	namespaceTypes := map[string][]string{}
	for _, entry := range defaults.DefaultRegistration {
		parts := strings.SplitN(entry, "/", 2)
		require.Len(t, parts, 2, "invalid entry in defaults.yaml: %s", entry)
		namespaceTypes[parts[0]] = append(namespaceTypes[parts[0]], parts[1])
	}

	require.EventuallyWithTf(t, func(collect *assert.CollectT) {
		// Verify each resource type is queryable via the API.
		for _, entry := range defaults.DefaultRegistration {
			parts := strings.SplitN(entry, "/", 2)
			namespace := parts[0]
			typeName := parts[1]

			typeURL := "/planes/radius/local/providers/System.Resources/resourceproviders/" + namespace + "/resourcetypes/" + typeName + radiusAPIVersion
			response := server.MakeRequest(http.MethodGet, typeURL, nil)
			if !assert.Equal(collect, 200, response.Raw.StatusCode, "resource type %s should be registered", entry) {
				return
			}
		}

		// Verify each namespace's location contains all of its types.
		for namespace, types := range namespaceTypes {
			locationURL := "/planes/radius/local/providers/System.Resources/resourceproviders/" + namespace + "/locations/global" + radiusAPIVersion
			response := server.MakeRequest(http.MethodGet, locationURL, nil)
			if !assert.Equal(collect, 200, response.Raw.StatusCode, "location for %s should exist", namespace) {
				return
			}

			var body map[string]any
			if !assert.NoError(collect, json.Unmarshal(response.Body.Bytes(), &body)) {
				return
			}

			props, _ := body["properties"].(map[string]any)
			resourceTypes, _ := props["resourceTypes"].(map[string]any)
			for _, typeName := range types {
				if !assert.Contains(collect, resourceTypes, typeName, "location for %s should contain type %s", namespace, typeName) {
					return
				}
			}
		}
	}, registerManifestWaitDuration, registerManifestWaitInterval, "default resource type registration did not complete in time")
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
