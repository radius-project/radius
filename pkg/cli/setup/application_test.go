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

package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func Test_ScaffoldApplication_CreatesBothFiles(t *testing.T) {
	directory := t.TempDir()
	outputSink := &output.MockOutput{}

	err := ScaffoldApplication(outputSink, directory, "cool-application")
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(directory, ".rad", "rad.yaml"))
	require.FileExists(t, filepath.Join(directory, "app.bicep"))

	b, err := os.ReadFile(filepath.Join(directory, ".rad", "rad.yaml"))
	require.NoError(t, err)

	actualYaml := map[string]any{}
	err = yaml.Unmarshal(b, &actualYaml)
	require.NoError(t, err)

	expectedYaml := map[string]any{
		"workspace": map[string]any{
			"application": "cool-application",
		},
	}
	require.Equal(t, expectedYaml, actualYaml)

	b, err = os.ReadFile(filepath.Join(directory, "app.bicep"))
	require.NoError(t, err)
	require.Equal(t, appBicepTemplate, string(b))

	expectedOutput := []any{
		output.LogOutput{
			Format: "Created %q",
			Params: []any{"app.bicep"},
		},
		output.LogOutput{
			Format: "Created %q",
			Params: []any{filepath.Join(".rad", "rad.yaml")},
		},
	}
	require.Equal(t, expectedOutput, outputSink.Writes)
}

func Test_ScaffoldApplication_KeepsAppBicepButWritesRadYaml(t *testing.T) {
	directory := t.TempDir()
	outputSink := &output.MockOutput{}

	// Pre-create files
	err := os.Mkdir(filepath.Join(directory, ".rad"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(directory, ".rad", "rad.yaml"), []byte("something else"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(directory, "app.bicep"), []byte("something else"), 0644)
	require.NoError(t, err)

	err = ScaffoldApplication(outputSink, directory, "cool-application")
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(directory, ".rad", "rad.yaml"))
	require.FileExists(t, filepath.Join(directory, "app.bicep"))

	b, err := os.ReadFile(filepath.Join(directory, ".rad", "rad.yaml"))
	require.NoError(t, err)

	actualYaml := map[string]any{}
	err = yaml.Unmarshal(b, &actualYaml)
	require.NoError(t, err)

	expectedYaml := map[string]any{
		"workspace": map[string]any{
			"application": "cool-application",
		},
	}
	require.Equal(t, expectedYaml, actualYaml)

	b, err = os.ReadFile(filepath.Join(directory, "app.bicep"))
	require.NoError(t, err)
	require.Equal(t, "something else", string(b))

	expectedOutput := []any{
		output.LogOutput{
			Format: "Created %q",
			Params: []any{filepath.Join(".rad", "rad.yaml")},
		},
	}
	require.Equal(t, expectedOutput, outputSink.Writes)
}
