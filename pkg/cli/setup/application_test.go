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

	"github.com/stretchr/testify/require"
)

func Test_ScaffoldApplication_CreatesBothFiles(t *testing.T) {
	templates := map[string]string{
		"classic": AppBicepTemplate,
	}

	for name, tmpl := range templates {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()

			err := ScaffoldApplication(directory, tmpl)
			require.NoError(t, err)

			require.FileExists(t, filepath.Join(directory, "app.bicep"))
			require.FileExists(t, filepath.Join(directory, "bicepconfig.json"))

			b, err := os.ReadFile(filepath.Join(directory, "app.bicep"))
			require.NoError(t, err)
			require.Equal(t, tmpl, string(b))

			b, err = os.ReadFile(filepath.Join(directory, "bicepconfig.json"))
			require.NoError(t, err)
			require.Equal(t, GetVersionedBicepConfig(), string(b))
		})
	}
}

func Test_ScaffoldApplication_KeepsExistingFiles(t *testing.T) {
	templates := map[string]string{
		"classic": AppBicepTemplate,
	}

	for name, tmpl := range templates {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()

			// Pre-create files
			err := os.WriteFile(filepath.Join(directory, "app.bicep"), []byte("something else"), 0644)
			require.NoError(t, err)
			err = os.WriteFile(filepath.Join(directory, "bicepconfig.json"), []byte("something else"), 0644)
			require.NoError(t, err)

			err = ScaffoldApplication(directory, tmpl)
			require.NoError(t, err)

			require.FileExists(t, filepath.Join(directory, "app.bicep"))

			b, err := os.ReadFile(filepath.Join(directory, "app.bicep"))
			require.NoError(t, err)
			require.Equal(t, "something else", string(b))

			b, err = os.ReadFile(filepath.Join(directory, "bicepconfig.json"))
			require.NoError(t, err)
			require.Equal(t, "something else", string(b))
		})
	}
}

func Test_WriteBicepConfig_CreatesFile(t *testing.T) {
	directory := t.TempDir()

	err := WriteBicepConfig(directory)
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(directory, "bicepconfig.json"))
	require.NoError(t, err)
	require.Equal(t, GetVersionedBicepConfig(), string(b))
	require.NoFileExists(t, filepath.Join(directory, "app.bicep"))
}

func Test_WriteBicepConfig_KeepsExistingFile(t *testing.T) {
	directory := t.TempDir()

	err := os.WriteFile(filepath.Join(directory, "bicepconfig.json"), []byte("something else"), 0644)
	require.NoError(t, err)

	err = WriteBicepConfig(directory)
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(directory, "bicepconfig.json"))
	require.NoError(t, err)
	require.Equal(t, "something else", string(b))
}

func Test_WriteBicepConfig_ExistingDirectoryIsPreserved(t *testing.T) {
	directory := t.TempDir()
	bicepConfigPath := filepath.Join(directory, "bicepconfig.json")

	err := os.Mkdir(bicepConfigPath, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(bicepConfigPath, "keep.txt"), []byte("keep"), 0644)
	require.NoError(t, err)

	err = WriteBicepConfig(directory)
	require.NoError(t, err)

	require.DirExists(t, bicepConfigPath)
	b, err := os.ReadFile(filepath.Join(bicepConfigPath, "keep.txt"))
	require.NoError(t, err)
	require.Equal(t, "keep", string(b))
}

func Test_WriteBicepConfig_ReturnsErrorWhenDirectoryMissing(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "does-not-exist")

	err := WriteBicepConfig(directory)
	require.Error(t, err)
	require.NoFileExists(t, filepath.Join(directory, "bicepconfig.json"))
}
