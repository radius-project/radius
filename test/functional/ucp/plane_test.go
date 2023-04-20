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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/to"
	v20230415preview "github.com/project-radius/radius/pkg/ucp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/stretchr/testify/require"
)

func Test_Plane_Operations(t *testing.T) {
	test := NewUCPTest(t, "Test_Plane_Operations", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		planeID := "/planes/testtype/testplane"
		apiVersion := v20230415preview.Version
		planeURL := fmt.Sprintf("%s%s?api-version=%s", url, planeID, apiVersion)

		// By default, we configure default planes (radius and deployments planes) in UCP. Verify that by calling List Planes
		planes := listPlanes(t, roundTripper, fmt.Sprintf("%s/planes?api-version=%s", url, apiVersion))
		require.Equal(t, 3, len(planes.Value))

		t.Cleanup(func() {
			_ = deletePlane(t, roundTripper, planeURL)
		})

		// Create Plane
		testPlane := v20230415preview.PlaneResource{
			ID:       to.Ptr(planeID),
			Type:     to.Ptr("System.Planes/testtype"),
			Name:     to.Ptr("testplane"),
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &v20230415preview.PlaneResourceProperties{
				Kind: to.Ptr(v20230415preview.PlaneKindUCPNative),
				ResourceProviders: map[string]*string{
					"example.com": to.Ptr("http://localhost:8000"),
				},
			},
		}

		createPlane(t, roundTripper, planeURL, testPlane)
		createPlane(t, roundTripper, planeURL, testPlane)

		testPlaneRest := rest.Plane{
			ID:   planeID,
			Type: "System.Planes/testtype",
			Name: "testplane",
			Properties: rest.PlaneProperties{
				Kind: rest.PlaneKindUCPNative,
				ResourceProviders: map[string]string{
					"example.com": "http://localhost:8000",
				},
			},
		}
		// Get Plane
		plane, statusCode := getPlane(t, roundTripper, planeURL)
		require.Equal(t, http.StatusOK, statusCode)
		require.Equal(t, testPlaneRest, plane)

		// Delete Plane
		statusCode = deletePlane(t, roundTripper, planeURL)
		require.Equal(t, http.StatusOK, statusCode)

		// Get Plane - Expected Not Found
		_, statusCode = getPlane(t, roundTripper, planeURL)
		require.Equal(t, http.StatusNotFound, statusCode)

	})
	test.Test(t)
}

func createPlane(t *testing.T, roundTripper http.RoundTripper, url string, plane v20230415preview.PlaneResource) {
	body, err := json.Marshal(plane)
	require.NoError(t, err)
	createRequest, err := NewUCPRequest(
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
	getRequest, err := NewUCPRequest(
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

func listPlanes(t *testing.T, roundTripper http.RoundTripper, url string) v20230415preview.PlaneResourceListResult {
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
	listOfPlanes := v20230415preview.PlaneResourceListResult{}
	require.NoError(t, json.Unmarshal(payload, &listOfPlanes))
	return listOfPlanes
}

func deletePlane(t *testing.T, roundTripper http.RoundTripper, url string) int {
	deleteRgRequest, err := NewUCPRequest(
		http.MethodDelete,
		url,
		nil,
	)
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(deleteRgRequest)
	require.NoError(t, err)
	t.Logf("Plane: %s deleted successfully", url)
	return res.StatusCode
}
