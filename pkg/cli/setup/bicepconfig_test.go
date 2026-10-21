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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/version"
	"github.com/stretchr/testify/require"
)

func Test_GetVersionedBicepConfig(t *testing.T) {
	t.Parallel()
	var config struct {
		ExperimentalFeaturesEnabled map[string]bool   `json:"experimentalFeaturesEnabled"`
		Extensions                  map[string]string `json:"extensions"`
	}
	require.NoError(t, json.Unmarshal([]byte(GetVersionedBicepConfig()), &config))
	require.True(t, config.ExperimentalFeaturesEnabled["ociEnabled"])
	require.Equal(t, map[string]string{
		"radius": "br:ghcr.io/radius-project/bicep-types-radius:" + version.Channel(),
		"aws":    "br:ghcr.io/radius-project/bicep-types-aws:" + version.Channel(),
	}, config.Extensions)
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
	const existingConfig = `{
		"experimentalFeaturesEnabled": {"ociEnabled": false},
		"extensions": {"radius": "br:example.azurecr.io/custom@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"cloud": {"credentialPrecedence": ["AzureCLI"]}
	}`
	directory := t.TempDir()

	err := os.WriteFile(filepath.Join(directory, "bicepconfig.json"), []byte(existingConfig), 0644)
	require.NoError(t, err)

	err = WriteBicepConfig(directory)
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(directory, "bicepconfig.json"))
	require.NoError(t, err)
	require.Equal(t, existingConfig, string(b))
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
