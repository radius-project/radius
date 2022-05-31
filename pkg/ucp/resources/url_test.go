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
		// ucp prefix does not appear in URL
		"ucp:/planes/radius/local/foo",

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
