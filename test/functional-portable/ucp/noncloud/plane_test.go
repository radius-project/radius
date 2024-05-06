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
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	v20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	test "github.com/radius-project/radius/test/ucp"
	"github.com/stretchr/testify/require"
)

func Test_Plane_Operations(t *testing.T) {
	apiVersion := v20231001preview.Version

	t.Run("Default planes", func(t *testing.T) {
		test := test.NewUCPTest(t, "Test_Plane_Operations", func(t *testing.T, url string, roundTripper http.RoundTripper) {
			// By default, we configure default planes in UCP. Verify that by calling List Planes
			planes := listPlanes(t, roundTripper, fmt.Sprintf("%s/planes?api-version=%s", url, apiVersion))
			require.Equal(t, 2, len(planes.Value))
		})
		test.Test(t)
	})

	t.Run("AWS", func(t *testing.T) {
		test := test.NewUCPTest(t, "AWS_Plane_Operations", func(t *testing.T, url string, roundTripper http.RoundTripper) {
			apiVersion := v20231001preview.Version

			planeID := "/planes/aws/testplane"
			planeURL := fmt.Sprintf("%s%s?api-version=%s", url, planeID, apiVersion)

			t.Cleanup(func() {
				_ = deletePlane(t, roundTripper, planeURL)
			})

			body := v20231001preview.AwsPlaneResource{
				Location:   to.Ptr(v1.LocationGlobal),
				Properties: &v20231001preview.AwsPlaneResourceProperties{},
			}

			createPlane(t, roundTripper, planeURL, body)

			expected := v20231001preview.AwsPlaneResource{
				ID:       to.Ptr(planeID),
				Type:     to.Ptr("System.AWS/planes"),
				Name:     to.Ptr("testplane"),
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &v20231001preview.AwsPlaneResourceProperties{
					ProvisioningState: to.Ptr(v20231001preview.ProvisioningStateSucceeded),
				},
				Tags: map[string]*string{},
			}

			// Get Plane
			actual, statusCode := getPlane[v20231001preview.AwsPlaneResource](t, roundTripper, planeURL)
			require.Equal(t, http.StatusOK, statusCode)

			// SystemData includes timestamps, so we can't compare it directly
			expected.SystemData = actual.SystemData
			require.Equal(t, expected, actual)

			// Delete Plane
			statusCode = deletePlane(t, roundTripper, planeURL)
			require.Equal(t, http.StatusOK, statusCode)

			// Get Plane - Expected Not Found
			_, statusCode = getPlane[any](t, roundTripper, planeURL)
			require.Equal(t, http.StatusNotFound, statusCode)
		})
		test.Test(t)
	})

	t.Run("Azure", func(t *testing.T) {
		test := test.NewUCPTest(t, "Azure_Plane_Operations", func(t *testing.T, url string, roundTripper http.RoundTripper) {
			apiVersion := v20231001preview.Version

			planeID := "/planes/azure/testplane"
			planeURL := fmt.Sprintf("%s%s?api-version=%s", url, planeID, apiVersion)

			t.Cleanup(func() {
				_ = deletePlane(t, roundTripper, planeURL)
			})

			body := v20231001preview.AzurePlaneResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &v20231001preview.AzurePlaneResourceProperties{
					URL: to.Ptr("https://www.example.com"),
				},
			}

			createPlane(t, roundTripper, planeURL, body)

			expected := v20231001preview.AzurePlaneResource{
				ID:       to.Ptr(planeID),
				Type:     to.Ptr("System.Azure/planes"),
				Name:     to.Ptr("testplane"),
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &v20231001preview.AzurePlaneResourceProperties{
					ProvisioningState: to.Ptr(v20231001preview.ProvisioningStateSucceeded),
					URL:               to.Ptr("https://www.example.com"),
				},
				Tags: map[string]*string{},
			}

			// Get Plane
			actual, statusCode := getPlane[v20231001preview.AzurePlaneResource](t, roundTripper, planeURL)
			require.Equal(t, http.StatusOK, statusCode)

			// SystemData includes timestamps, so we can't compare it directly
			expected.SystemData = actual.SystemData
			require.Equal(t, expected, actual)

			// Delete Plane
			statusCode = deletePlane(t, roundTripper, planeURL)
			require.Equal(t, http.StatusOK, statusCode)

			// Get Plane - Expected Not Found
			_, statusCode = getPlane[any](t, roundTripper, planeURL)
			require.Equal(t, http.StatusNotFound, statusCode)
		})
		test.Test(t)
	})

	t.Run("Radius", func(t *testing.T) {
		test := test.NewUCPTest(t, "Radius_Plane_Operations", func(t *testing.T, url string, roundTripper http.RoundTripper) {
			apiVersion := v20231001preview.Version

			planeID := "/planes/radius/testplane"
			planeURL := fmt.Sprintf("%s%s?api-version=%s", url, planeID, apiVersion)

			t.Cleanup(func() {
				_ = deletePlane(t, roundTripper, planeURL)
			})

			body := v20231001preview.RadiusPlaneResource{
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &v20231001preview.RadiusPlaneResourceProperties{
					ResourceProviders: map[string]*string{
						"Applications.Core": to.Ptr("https://applications.core.example.com"),
					},
				},
			}

			createPlane(t, roundTripper, planeURL, body)

			expected := v20231001preview.RadiusPlaneResource{
				ID:       to.Ptr(planeID),
				Type:     to.Ptr("System.Radius/planes"),
				Name:     to.Ptr("testplane"),
				Location: to.Ptr(v1.LocationGlobal),
				Properties: &v20231001preview.RadiusPlaneResourceProperties{
					ProvisioningState: to.Ptr(v20231001preview.ProvisioningStateSucceeded),
					ResourceProviders: map[string]*string{
						"Applications.Core": to.Ptr("https://applications.core.example.com"),
					},
				},
				Tags: map[string]*string{},
			}

			// Get Plane
			actual, statusCode := getPlane[v20231001preview.RadiusPlaneResource](t, roundTripper, planeURL)
			require.Equal(t, http.StatusOK, statusCode)

			// SystemData includes timestamps, so we can't compare it directly
			expected.SystemData = actual.SystemData
			require.Equal(t, expected, actual)

			// Delete Plane
			statusCode = deletePlane(t, roundTripper, planeURL)
			require.Equal(t, http.StatusOK, statusCode)

			// Get Plane - Expected Not Found
			_, statusCode = getPlane[any](t, roundTripper, planeURL)
			require.Equal(t, http.StatusNotFound, statusCode)
		})
		test.Test(t)
	})
}

func createPlane(t *testing.T, roundTripper http.RoundTripper, url string, plane any) {
	body, err := json.Marshal(plane)
	require.NoError(t, err)
	createRequest, err := test.NewUCPRequest(
		http.MethodPut,
		url,
		bytes.NewBuffer(body))
	require.NoError(t, err, "")

	res, err := roundTripper.RoundTrip(createRequest)
	require.NoError(t, err, "")

	require.Equal(t, http.StatusOK, res.StatusCode)
	t.Logf("Plane: %s created/updated successfully", url)
}

func getPlane[T any](t *testing.T, roundTripper http.RoundTripper, url string) (T, int) {
	getRequest, err := test.NewUCPRequest(
		http.MethodGet,
		url,
		nil)
	require.NoError(t, err, "")

	result, err := roundTripper.RoundTrip(getRequest)
	require.NoError(t, err, "")

	body := result.Body
	defer body.Close()
	payload, err := io.ReadAll(body)
	require.NoError(t, err)
	var plane T
	err = json.Unmarshal(payload, &plane)
	require.NoError(t, err)

	return plane, result.StatusCode
}

func listPlanes(t *testing.T, roundTripper http.RoundTripper, url string) v1.PaginatedList {
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
	listOfPlanes := v1.PaginatedList{}
	require.NoError(t, json.Unmarshal(payload, &listOfPlanes))
	return listOfPlanes
}

func deletePlane(t *testing.T, roundTripper http.RoundTripper, url string) int {
	deleteRgRequest, err := test.NewUCPRequest(
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
