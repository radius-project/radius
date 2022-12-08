// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources

import (
	"errors"
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

func Test_ExtractCredentialFromURLPath(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedName string
		err          error
	}{
		{
			name:         "valid_azure_credential_url",
			input:        "/planes/azure/azurecloud/providers/System.Azure/credentials/default",
			expectedName: "azure_azurecloud_default",
			err:          nil,
		},
		{
			name:         "invalid_azure_credential_url",
			input:        "/planes/azure/azurecloud/providers/System.Azure/credentials",
			expectedName: "",
			err:          errors.New("URL path is not a valid UCP path"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, err := ExtractSecretNameFromPath(tt.input)
			if err != nil {
				require.Equal(t, tt.err, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedName, name)
			}
		})
	}
}
