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

package validator

import (
	"context"
	"testing"

	"github.com/radius-project/radius/swagger"
	"github.com/stretchr/testify/require"
)

func Test_GetValidatorKey(t *testing.T) {
	keyTests := []struct {
		resourceType string
		version      string
		expected     string
	}{
		{"applications.core/environments", "2023-10-01-preview", "applications.core/environments-2023-10-01-preview"},
		{"applications.Core/environments", "2023-10-01-preview", "applications.core/environments-2023-10-01-preview"},
		{"Applications.Core/environments", "2023-10-01-preview", "applications.core/environments-2023-10-01-preview"},
	}

	for _, tt := range keyTests {
		require.Equal(t, tt.expected, getValidatorKey(tt.resourceType, tt.version))
	}
}

func Test_ParseSpecFilePath(t *testing.T) {
	pathTests := []struct {
		path   string
		parsed map[string]string
	}{
		{
			path: "specification/applications/resource-manager/Applications.Core/preview/2023-10-01-preview/environments.json",
			parsed: map[string]string{
				"productname":  "applications",
				"provider":     "applications.core",
				"state":        "preview",
				"version":      "2023-10-01-preview",
				"resourcetype": "environments",
			},
		},
		{
			path: "specification/applications/resource-manager/Applications.Core/stable/2023-10-01/gateways.json",
			parsed: map[string]string{
				"productname":  "applications",
				"provider":     "applications.core",
				"state":        "stable",
				"version":      "2023-10-01",
				"resourcetype": "gateways",
			},
		},
	}

	for _, tt := range pathTests {
		require.Equal(t, tt.parsed, parseSpecFilePath(tt.path))
	}
}

func Test_Loader(t *testing.T) {
	l, err := LoadSpec(context.Background(), "applications.core", swagger.SpecFiles, []string{"{rootScope:.*}"}, "rootScope")
	require.NoError(t, err)
	v, ok := l.GetValidator("applications.core/environments", "2023-10-01-preview")
	require.True(t, ok)
	require.NotNil(t, v)
}
