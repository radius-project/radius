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
