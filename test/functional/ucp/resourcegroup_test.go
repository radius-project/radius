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
	"io"
	"net/http"
	"strings"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	v20220901privatepreview "github.com/radius-project/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/radius-project/radius/pkg/ucp/frontend/controller/resourcegroups"
	"github.com/stretchr/testify/require"
)

func Test_ResourceGroup_Operations(t *testing.T) {
	test := NewUCPTest(t, "Test_ResourceGroup_Operations", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		// Create resource groups
		rgID := "/planes/radius/local/resourcegroups/test-RG"
		apiVersion := v20220901privatepreview.Version
		rgURL := fmt.Sprintf("%s%s?api-version=%s", url, rgID, apiVersion)

		t.Cleanup(func() {
			_ = deleteResourceGroup(t, roundTripper, rgURL)
		})

		createResourceGroup(t, roundTripper, rgURL)
		createResourceGroup(t, roundTripper, rgURL)

		// List Resource Groups
		listRGsURL := fmt.Sprintf("%s%s?api-version=%s", url, "/planes/radius/local/resourceGroups", apiVersion)
		rgs := listResourceGroups(t, roundTripper, listRGsURL)
		require.GreaterOrEqual(t, len(rgs.Value), 1)

		// Get Resource Group by calling lower case URL.
		rg, statusCode := getResourceGroup(t, roundTripper, strings.ToLower(rgURL))
		expectedResourceGroup := v20220901privatepreview.ResourceGroupResource{
			ID:       to.Ptr(rgID),
			Name:     to.Ptr("test-RG"),
			Tags:     map[string]*string{},
			Type:     to.Ptr(resourcegroups.ResourceGroupType),
			Location: to.Ptr(v1.LocationGlobal),
		}
		require.Equal(t, http.StatusOK, statusCode)
		require.Equal(t, expectedResourceGroup, rg)

		// Delete Resource Group
		statusCode = deleteResourceGroup(t, roundTripper, rgURL)
		require.Equal(t, http.StatusOK, statusCode)

		// Get Resource Group - Expected Not Found
		_, statusCode = getResourceGroup(t, roundTripper, rgURL)
		require.Equal(t, http.StatusNotFound, statusCode)
	})
	test.Test(t)
}

func createResourceGroup(t *testing.T, roundTripper http.RoundTripper, url string) {
	model := v20220901privatepreview.ResourceGroupResource{
		Location: to.Ptr(v1.LocationGlobal),
	}

	b, err := json.Marshal(&model)
	if err != nil {
		require.NoError(t, err, "failed to marshal resource group")
	}

	createRequest, err := NewUCPRequest(
		http.MethodPut,
		url,
		bytes.NewBuffer(b))
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(createRequest)
	require.NoError(t, err, "")

	require.Equal(t, http.StatusOK, res.StatusCode)
	t.Logf("Resource group: %s created/updated successfully", url)
}

func listResourceGroups(t *testing.T, roundTripper http.RoundTripper, url string) v20220901privatepreview.ResourceGroupResourceListResult {
	listRgsRequest, err := NewUCPRequest(
		http.MethodGet,
		url,
		nil,
	)
	require.NoError(t, err, "")

	result, err := roundTripper.RoundTrip(listRgsRequest)
	require.NoError(t, err, "")
	require.Equal(t, http.StatusOK, result.StatusCode)

	body := result.Body
	defer body.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)

	items := v20220901privatepreview.ResourceGroupResourceListResult{}
	err = json.Unmarshal(payload, &items)
	require.NoError(t, err)

	return items
}

func getResourceGroup(t *testing.T, roundTripper http.RoundTripper, url string) (v20220901privatepreview.ResourceGroupResource, int) {
	getRgRequest, err := NewUCPRequest(
		http.MethodGet,
		url,
		nil,
	)
	require.NoError(t, err, "")

	result, err := roundTripper.RoundTrip(getRgRequest)
	require.NoError(t, err, "")

	body := result.Body
	defer body.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)

	resourceGroup := v20220901privatepreview.ResourceGroupResource{}
	err = json.Unmarshal(payload, &resourceGroup)
	require.NoError(t, err)

	return resourceGroup, result.StatusCode
}

func deleteResourceGroup(t *testing.T, roundTripper http.RoundTripper, url string) int {
	deleteRgRequest, err := NewUCPRequest(
		http.MethodDelete,
		url,
		nil,
	)
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(deleteRgRequest)
	require.NoError(t, err, "")
	t.Logf("Resource group: %s deleted successfully", url)
	return res.StatusCode
}
