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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const latest = "latest"

func Test_ScaffoldBicepConfig_CreatesFile(t *testing.T) {
	directory := t.TempDir()

	existed, err := ScaffoldBicepConfig(directory)
	require.NoError(t, err)
	require.False(t, existed)

	require.NoFileExists(t, filepath.Join(directory, "app.bicep"))
	require.FileExists(t, filepath.Join(directory, "bicepconfig.json"))

	b, err := os.ReadFile(filepath.Join(directory, "bicepconfig.json"))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf(bicepConfigTemplate, latest, latest), string(b))
}

func Test_ScaffoldBicepConfig_KeepsExistingFile(t *testing.T) {
	directory := t.TempDir()

	// Pre-create file
	err := os.WriteFile(filepath.Join(directory, "bicepconfig.json"), []byte("something else"), 0644)
	require.NoError(t, err)

	existed, err := ScaffoldBicepConfig(directory)
	require.NoError(t, err)
	require.True(t, existed)

	b, err := os.ReadFile(filepath.Join(directory, "bicepconfig.json"))
	require.NoError(t, err)
	require.Equal(t, "something else", string(b))
}

func Test_ScaffoldBicepConfig_DoesNotCreateAppBicep(t *testing.T) {
	directory := t.TempDir()

	_, err := ScaffoldBicepConfig(directory)
	require.NoError(t, err)

	require.NoFileExists(t, filepath.Join(directory, "app.bicep"))
}

func Test_ScaffoldBicepConfig_WriteFileError(t *testing.T) {
	// Pointing at a non-existent parent directory makes os.WriteFile fail
	// while os.Stat returns IsNotExist, which exercises the WriteFile
	// error branch in ScaffoldBicepConfig.
	directory := filepath.Join(t.TempDir(), "does-not-exist")

	existed, err := ScaffoldBicepConfig(directory)
	require.Error(t, err)
	require.False(t, existed)
}

func Test_ScaffoldBicepConfig_StatError(t *testing.T) {
	// Create a regular file and use it as the "directory" argument. The
	// resulting Stat call on "<file>/bicepconfig.json" returns ENOTDIR
	// (which is not IsNotExist), exercising the third branch of
	// ScaffoldBicepConfig where Stat returns a non-NotExist error.
	parent := t.TempDir()
	notADir := filepath.Join(parent, "not-a-dir")
	require.NoError(t, os.WriteFile(notADir, []byte("file"), 0644))

	existed, err := ScaffoldBicepConfig(notADir)
	require.Error(t, err)
	require.False(t, existed)
}
