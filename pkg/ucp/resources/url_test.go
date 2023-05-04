// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ExtractPlanesPrefixFromURLPath_Invalid(t *testing.T) {
	data := []string{
		"planes/radius", // Not long enough

		"planes//foo", // Empty segment

		"/subscriptions/test/anotherone/bar", // missing planes
	}
	for _, datum := range data {
		planeType, planeName, remainder, err := ExtractPlanesPrefixFromURLPath(datum)
		require.Errorf(t, err, "%q should have failed", datum)
		require.Empty(t, planeType)
		require.Empty(t, planeName)
		require.Empty(t, remainder)
	}
}

func Test_ExtractPlanesPrefixFromURLPath_Valid(t *testing.T) {
	data := []struct {
		input     string
		planeType string
		planeName string
		remainder string
	}{
		{
			input:     "/planes/radius/local",
			planeType: "radius",
			planeName: "local",
			remainder: "/",
		},
		{
			input:     "/planes/radius/local/",
			planeType: "radius",
			planeName: "local",
			remainder: "/",
		},
		{
			input:     "/plAnes/rAdius/lOcal",
			planeType: "rAdius",
			planeName: "lOcal",
			remainder: "/",
		},
		{
			input:     "/planes/radius/local/subscriptions/sid/resourceGroups/rg",
			planeType: "radius",
			planeName: "local",
			remainder: "/subscriptions/sid/resourceGroups/rg",
		},
	}
	for _, datum := range data {
		planeType, planeName, remainder, err := ExtractPlanesPrefixFromURLPath(datum.input)
		require.NoError(t, err, "%q should have not have failed", datum)
		require.Equal(t, datum.planeType, planeType)
		require.Equal(t, datum.planeName, planeName)
		require.Equal(t, datum.remainder, remainder)
	}
}

func Test_ExtractRegionFromURLPath_Invalid(t *testing.T) {
	URLPath := "/planes/deployments/local/resourcegroups/localrp/providers/Microsoft.Resources/deployments/rad-deploy-06221c5e-104d-4472-bb74-876b441c7663"
	region, err := ExtractRegionFromURLPath(URLPath)
	require.Error(t, err, "%q should have failed", URLPath)
	require.Empty(t, region)

}

func Test_ExtractRegionFromURLPath_Valid(t *testing.T) {
	URLPath := "/apis/api.ucp.dev/v1alpha3/planes/aws/aws/accounts/817312594854/regions/us-west-2/providers/AWS.S3/Bucket/:put?api-version=default"
	region, err := ExtractRegionFromURLPath(URLPath)
	require.NoError(t, err, "%q should have not have failed", URLPath)
	require.Equal(t, "us-west-2", region)
}
