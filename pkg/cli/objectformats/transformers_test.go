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

package objectformats

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ResourceIDToResourceNameTransformer(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "invalid input",
			input:    "////",
			expected: "<error>",
		},
		{
			name:     "valid input",
			input:    "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test",
			expected: "test",
		},
	}

	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			transformer := &ResourceIDToResourceNameTransformer{}
			actual := transformer.Transform(testcase.input)
			require.Equal(t, testcase.expected, actual)
		})
	}
}

func Test_ResourceIDToResourceGroupNameTransformer(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "invalid input",
			input:    "////",
			expected: "<error>",
		},
		{
			name:     "valid input",
			input:    "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test",
			expected: "test-group",
		},
	}

	for _, testcase := range cases {
		t.Run(testcase.name, func(t *testing.T) {
			transformer := &ResourceIDToResourceGroupNameTransformer{}
			actual := transformer.Transform(testcase.input)
			require.Equal(t, testcase.expected, actual)
		})
	}
}
