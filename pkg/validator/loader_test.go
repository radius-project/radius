// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validator

import (
	"context"
	"testing"

	"github.com/project-radius/radius/swagger"
	"github.com/stretchr/testify/require"
)

func TestGetValidatorKey(t *testing.T) {
	keyTests := []struct {
		resourceType string
		version      string
		expected     string
	}{
		{"applications.core/environments", "2022-03-15-privatepreview", "applications.core/environments-2022-03-15-privatepreview"},
		{"applications.Core/environments", "2022-03-15-privatepreview", "applications.core/environments-2022-03-15-privatepreview"},
		{"Applications.Core/environments", "2022-03-15-privatepreview", "applications.core/environments-2022-03-15-privatepreview"},
	}

	for _, tt := range keyTests {
		require.Equal(t, tt.expected, getValidatorKey(tt.resourceType, tt.version))
	}
}

func TestParseSpecFilePath(t *testing.T) {
	pathTests := []struct {
		path   string
		parsed map[string]string
	}{
		{
			path: "specification/applications/resource-manager/Applications.Core/preview/2022-03-15-privatepreview/environments.json",
			parsed: map[string]string{
				"productname":  "applications",
				"provider":     "applications.core",
				"state":        "preview",
				"version":      "2022-03-15-privatepreview",
				"resourcetype": "environments",
			},
		},
		{
			path: "specification/applications/resource-manager/Applications.Core/stable/2022-03-15/gateways.json",
			parsed: map[string]string{
				"productname":  "applications",
				"provider":     "applications.core",
				"state":        "stable",
				"version":      "2022-03-15",
				"resourcetype": "gateways",
			},
		},
	}

	for _, tt := range pathTests {
		require.Equal(t, tt.parsed, parseSpecFilePath(tt.path))
	}
}

func TestLoader(t *testing.T) {
	l, err := LoadSpec(context.Background(), "applications.core", swagger.SpecFiles, "{rootScope:.*}", "rootScope")
	require.NoError(t, err)
	v, ok := l.GetValidator("applications.core/environments", "2022-03-15-privatepreview")
	require.True(t, ok)
	require.NotNil(t, v)
}
