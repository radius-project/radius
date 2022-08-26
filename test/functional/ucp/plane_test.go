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
	"testing"

	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_Plane_Operations(t *testing.T) {
	test := NewUCPTest(t, "Test_Plane_Operations", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		planeID := "/planes/testType/testPlane"
		planeURL := fmt.Sprintf("%s%s", url, planeID)

		// By default, we configure default planes (radius and deployments planes) in UCP. Verify that by calling List Planes
		planes := listPlanes(t, roundTripper, fmt.Sprintf("%s/planes", url))
		require.Equal(t, 2, len(planes))

		t.Cleanup(func() {
			deletePlane(t, roundTripper, planeURL)
		})

		// Create Plane
		testPlane := rest.Plane{
			ID:   planeID,
			Type: "System.Planes/testType",
			Name: "testPlane",
			Properties: rest.PlaneProperties{
				Kind: rest.PlaneKindUCPNative,
				ResourceProviders: map[string]string{
					"example.com": "http://localhost:8000",
				},
			},
		}

		createPlane(t, roundTripper, planeURL, testPlane)
		createPlane(t, roundTripper, planeURL, testPlane)

		// Get Plane
		plane, statusCode := getPlane(t, roundTripper, planeURL)
		require.Equal(t, http.StatusOK, statusCode)
		assert.DeepEqual(t, testPlane, plane)

		// Delete Plane
		deletePlane(t, roundTripper, planeURL)

		// Get Plane - Expected Not Found
		_, statusCode = getPlane(t, roundTripper, planeURL)
		require.Equal(t, http.StatusNotFound, statusCode)

	})
	test.Test(t)
}

func createPlane(t *testing.T, roundTripper http.RoundTripper, url string, plane rest.Plane) {
	body, err := json.Marshal(plane)
	require.NoError(t, err)
	createRequest, err := http.NewRequest(
		http.MethodPut,
		url,
		bytes.NewBuffer(body))
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(createRequest)
	require.NoError(t, err, "")

	require.Equal(t, http.StatusOK, res.StatusCode)
	t.Logf("Plane: %s created/updated successfully", url)
}

func getPlane(t *testing.T, roundTripper http.RoundTripper, url string) (rest.Plane, int) {
	getRequest, err := http.NewRequest(
		http.MethodGet,
		url,
		nil,
	)
	require.NoError(t, err, "")

	result, err := roundTripper.RoundTrip(getRequest)
	require.NoError(t, err, "")

	body := result.Body
	defer body.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	var plane rest.Plane
	err = json.Unmarshal(payload, &plane)
	require.NoError(t, err)

	return plane, result.StatusCode
}

func listPlanes(t *testing.T, roundTripper http.RoundTripper, url string) []interface{} {
	listRequest, err := http.NewRequest(
		http.MethodGet,
		url,
		nil,
	)
	require.NoError(t, err, "")

	result, err := roundTripper.RoundTrip(listRequest)
	require.NoError(t, err, "")
	require.Equal(t, http.StatusOK, result.StatusCode)

	body := result.Body
	defer body.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	var listOfPlanes []interface{}
	err = json.Unmarshal(payload, &listOfPlanes)
	require.NoError(t, err)

	return listOfPlanes
}

func deletePlane(t *testing.T, roundTripper http.RoundTripper, url string) {
	deleteRgRequest, err := http.NewRequest(
		http.MethodDelete,
		url,
		nil,
	)
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(deleteRgRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	t.Logf("Plane: %s deleted successfully", url)
}
