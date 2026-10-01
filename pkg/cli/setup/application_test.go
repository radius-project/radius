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
	"fmt"
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

func Test_ScaffoldApplication_CreatesBothFiles(t *testing.T) {
	templates := map[string]string{
		"classic": AppBicepTemplate,
		"preview": PreviewAppBicepTemplate,
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
			require.Equal(t, fmt.Sprintf(bicepConfigTemplate, version.Channel(), version.Channel()), string(b))
		})
	}
}

func Test_ScaffoldApplication_KeepsExistingFiles(t *testing.T) {
	const existingConfig = `{
		"experimentalFeaturesEnabled": {"ociEnabled": false},
		"extensions": {"radius": "br:example.azurecr.io/custom@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"cloud": {"credentialPrecedence": ["AzureCLI"]}
	}`
	templates := map[string]string{
		"classic": AppBicepTemplate,
		"preview": PreviewAppBicepTemplate,
	}

	for name, tmpl := range templates {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()

			// Pre-create files
			err := os.WriteFile(filepath.Join(directory, "app.bicep"), []byte("something else"), 0644)
			require.NoError(t, err)
			err = os.WriteFile(filepath.Join(directory, "bicepconfig.json"), []byte(existingConfig), 0644)
			require.NoError(t, err)

			err = ScaffoldApplication(directory, tmpl)
			require.NoError(t, err)

			require.FileExists(t, filepath.Join(directory, "app.bicep"))

			b, err := os.ReadFile(filepath.Join(directory, "app.bicep"))
			require.NoError(t, err)
			require.Equal(t, "something else", string(b))

			b, err = os.ReadFile(filepath.Join(directory, "bicepconfig.json"))
			require.NoError(t, err)
			require.Equal(t, existingConfig, string(b))
		})
	}
}
