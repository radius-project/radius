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
	"bytes"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/output"
	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_GetResourceTableFormat(t *testing.T) {
	obj := corerpv20231001preview.EnvironmentResource{
		Name: to.Ptr("test"),
		Type: to.Ptr("test-type"),
		ID:   to.Ptr("/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test"),
		Properties: &corerpv20231001preview.EnvironmentProperties{
			ProvisioningState: to.Ptr(corerpv20231001preview.ProvisioningStateUpdating),
		},
	}

	buffer := &bytes.Buffer{}
	err := output.Write(output.FormatTable, obj, buffer, GetResourceTableFormat())
	require.NoError(t, err)
	expected := "RESOURCE  TYPE       GROUP       ENVIRONMENT  STATE\ntest      test-type  test-group  default      Updating\n"
	require.Equal(t, expected, buffer.String())
}

func Test_GetGenericResourceTableFormat(t *testing.T) {
	obj := generated.GenericResource{
		Name: to.Ptr("test"),
		Type: to.Ptr("test-type"),
		ID:   to.Ptr("/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test"),
		Properties: map[string]any{
			"provisioningState": corerpv20231001preview.ProvisioningStateUpdating,
			"environment":       "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env",
		},
	}

	buffer := &bytes.Buffer{}
	err := output.Write(output.FormatTable, obj, buffer, GetGenericResourceTableFormat())
	require.NoError(t, err)

	expected := "RESOURCE  TYPE       GROUP       ENVIRONMENT  STATE\ntest      test-type  test-group  test-env     Updating\n"
	require.Equal(t, expected, buffer.String())
}

func Test_ResourceEnvironmentNameTransformer(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty_input",
			input:    "",
			expected: "default",
		},
		{
			name:     "valid_input",
			input:    "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env",
			expected: "test-env",
		},
		{
			name:     "just_name",
			input:    "test-env",
			expected: "test-env",
		},
	}

	transformer := ResourceEnvironmentNameTransformer{}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := transformer.Transform(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}
