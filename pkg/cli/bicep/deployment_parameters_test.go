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

package bicep

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/stretchr/testify/require"
)

func Test_Parameters_Invalid(t *testing.T) {
	inputs := []string{
		"foo",
		"foo.json",
		"foo bar.json",
		"foo bar",
	}

	parser := ParameterParser{
		FileSystem: fstest.MapFS{},
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			parameters, err := parser.Parse(input)
			require.Error(t, err)
			require.Nil(t, parameters)
		})
	}
}

func Test_ParseParameters_Overwrite(t *testing.T) {
	inputs := []string{
		"@many.json",
		"key2=@single.json",
		"key3=value2",
		"key3=value3",
	}

	parser := ParameterParser{
		FileSystem: fstest.MapFS{
			"many.json": {
				Data: []byte(`{ "parameters": { "key1": { "value": { "someValue": true } }, "key2": { "value": "overridden-value" } } }`),
			},
			"single.json": {
				Data: []byte(`{ "someValue": "another-value" }`),
			},
		},
	}

	parameters, err := parser.Parse(inputs...)
	require.NoError(t, err)

	expected := clients.DeploymentParameters{
		"key1": map[string]any{
			"value": map[string]any{
				"someValue": true,
			},
		},
		"key2": map[string]any{
			"value": map[string]any{
				"someValue": "another-value",
			},
		},
		"key3": map[string]any{
			"value": "value3",
		},
	}

	require.Equal(t, expected, parameters)
}

func Test_ParseParameters_File(t *testing.T) {
	parser := ParameterParser{
		FileSystem: fstest.MapFS{},
	}

	input, err := os.ReadFile(filepath.Join("testdata", "test-parameters.json"))
	require.NoError(t, err)

	template := map[string]any{}
	err = json.Unmarshal(input, &template)
	require.NoError(t, err)

	parameters, err := parser.ParseFileContents(template)
	require.NoError(t, err)

	expected := clients.DeploymentParameters{
		"param1": map[string]any{
			"value": "value1",
		},
		"param2": map[string]any{
			"value": "value2",
		},
	}

	require.Equal(t, expected, parameters)
}
