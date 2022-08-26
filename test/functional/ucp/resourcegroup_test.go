// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_ResourceGroup_Operations(t *testing.T) {
	test := NewUCPTest(t, "Test_ResourceGroup_Operations", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		// Create resource groups
		rgID := "/planes/radius/local/resourceGroups/test-rg"
		rgURL := fmt.Sprintf("%s%s", url, rgID)

		t.Cleanup(func() {
			deleteResourceGroup(t, roundTripper, rgURL)
		})

		createResourceGroup(t, roundTripper, rgURL)
		createResourceGroup(t, roundTripper, rgURL)

		// List Resource Groups
		rgs := listResourceGroups(t, roundTripper, fmt.Sprintf("%s/planes/radius/local/resourceGroups", url))
		require.GreaterOrEqual(t, len(rgs), 1)

		// Get Resource Group
		rg, statusCode := getResourceGroup(t, roundTripper, rgURL)
		expectedResourceGroup := rest.ResourceGroup{
			ID:   rgID,
			Name: "test-rg",
		}
		require.Equal(t, http.StatusOK, statusCode)
		assert.DeepEqual(t, expectedResourceGroup, rg)

		// Delete Resource Group
		deleteResourceGroup(t, roundTripper, rgURL)

		// Get Resource Group - Expected Not Found
		_, statusCode = getResourceGroup(t, roundTripper, rgURL)
		require.Equal(t, http.StatusNotFound, statusCode)
	})
	test.Test(t)
}

func createResourceGroup(t *testing.T, roundTripper http.RoundTripper, url string) {
	createRequest, err := http.NewRequest(
		http.MethodPut,
		url,
		strings.NewReader(`{}`),
	)
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(createRequest)
	require.NoError(t, err, "")

	require.Equal(t, http.StatusOK, res.StatusCode)
	t.Logf("Resource group: %s created/updated successfully", url)
}

func listResourceGroups(t *testing.T, roundTripper http.RoundTripper, url string) []interface{} {
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
	var listOfResourceGroups []interface{}
	err = json.Unmarshal(payload, &listOfResourceGroups)
	require.NoError(t, err)

	return listOfResourceGroups
}

func getResourceGroup(t *testing.T, roundTripper http.RoundTripper, url string) (rest.ResourceGroup, int) {
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
	var resourceGroup rest.ResourceGroup
	err = json.Unmarshal(payload, &resourceGroup)
	require.NoError(t, err)

	return resourceGroup, result.StatusCode
}

func deleteResourceGroup(t *testing.T, roundTripper http.RoundTripper, url string) {
	deleteRgRequest, err := http.NewRequest(
		http.MethodDelete,
		url,
		nil,
	)
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(deleteRgRequest)
	require.NoError(t, err, "")
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	t.Logf("Resource group: %s deleted successfully", url)
}
