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

package ucp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	v20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/test/testutil"
	test "github.com/radius-project/radius/test/ucp"
	"github.com/stretchr/testify/require"
)

// NOTE: most functionality in UCP is tested with integration tests. Testing
// done here is intentionally minimal.

func Test_ResourceProvider_Operations(t *testing.T) {
	apiVersion := v20231001preview.Version

	// Randomize plane name to avoid interference with other tests.
	planeName := fmt.Sprintf("test-%s", uuid.New().String())

	test := test.NewUCPTest(t, "Test_ResourceProvider_Operations", func(t *testing.T, test *test.UCPTest) {
		planeUrl := fmt.Sprintf("%s/planes/radius/%s?api-version=%s", test.URL, planeName, apiVersion)
		resourceProviderUrl := fmt.Sprintf("%s/planes/radius/%s/providers/Contoso.Example?api-version=%s", test.URL, planeName, apiVersion)

		createPlane(t, test.Transport, planeUrl, &v20231001preview.RadiusPlaneResource{
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &v20231001preview.RadiusPlaneResourceProperties{
				ResourceProviders: map[string]*string{},
			},
		})

		resourceProvider := &v20231001preview.ResourceProviderResource{}
		testutil.MustUnmarshalFromFile("resourceprovider-requestbody.json", resourceProvider)
		createResourceProvider(t, test.Transport, resourceProviderUrl, resourceProvider)

		expected := &v20231001preview.ResourceProviderResource{}
		testutil.MustUnmarshalFromFile("resourceprovider-responsebody.json", expected)
		expected.ID = to.Ptr(fmt.Sprintf("/planes/radius/%s/providers/System.Resources/resourceProviders/Contoso.Example", planeName))

		actual := getResourceProvider(t, test.Transport, resourceProviderUrl)
		actual.SystemData = nil
		require.Equal(t, expected, actual)

		deleteResourceProvider(t, test.Transport, resourceProviderUrl)

		deletePlane(t, test.Transport, planeUrl)
	})
	test.Test(t)
}

func createResourceProvider(t *testing.T, roundTripper http.RoundTripper, url string, resourceProvider *v20231001preview.ResourceProviderResource) {
	body, err := json.Marshal(resourceProvider)
	require.NoError(t, err)
	createRequest, err := test.NewUCPRequest(
		http.MethodPut,
		url,
		bytes.NewBuffer(body))
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(createRequest)
	require.NoError(t, err, "")

	// Right now resource providers are synchronous
	require.Equal(t, http.StatusOK, res.StatusCode)
	t.Logf("Resource provider: %s created/updated successfully", url)
}

func deleteResourceProvider(t *testing.T, roundTripper http.RoundTripper, url string) {
	createRequest, err := test.NewUCPRequest(
		http.MethodDelete,
		url,
		nil)
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(createRequest)
	require.NoError(t, err, "")

	// Right now resource providers are synchronous
	require.Truef(t, res.StatusCode == http.StatusOK || res.StatusCode == http.StatusNoContent, "Status Code: %d", res.StatusCode)
	t.Logf("Resource provider: %s deleted successfully", url)
}

func getResourceProvider(t *testing.T, roundTripper http.RoundTripper, url string) *v20231001preview.ResourceProviderResource {
	getRequest, err := test.NewUCPRequest(http.MethodGet, url, nil)
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(getRequest)
	require.NoError(t, err, "")
	require.Equal(t, http.StatusOK, res.StatusCode)

	decoder := json.NewDecoder(res.Body)
	decoder.DisallowUnknownFields()

	resourceProvider := &v20231001preview.ResourceProviderResource{}
	err = decoder.Decode(resourceProvider)
	require.NoError(t, err)
	t.Logf("Resource provider: %s fetched successfully", url)

	return resourceProvider
}
