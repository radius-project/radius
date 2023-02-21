// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/to"
	v20220901privatepreview "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
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
			Type:     to.Ptr("System.Resources/resourceGroups"),
			Location: to.Ptr(v1.LocationGlobal),
		}
		require.Equal(t, http.StatusOK, statusCode)
		assert.DeepEqual(t, expectedResourceGroup, rg)

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

	createRequest, err := http.NewRequest(
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
	listRgsRequest, err := http.NewRequest(
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
	getRgRequest, err := http.NewRequest(
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
	deleteRgRequest, err := http.NewRequest(
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
