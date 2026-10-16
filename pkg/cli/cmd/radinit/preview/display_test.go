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

package preview

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_toDisplayOptions_ConfigFiles(t *testing.T) {
	t.Run("includes bicepconfig.json in the resolved directory", func(t *testing.T) {
		directory := t.TempDir()

		display := toDisplayOptions(&initOptions{BicepConfigDirectory: directory})

		require.Equal(t, []string{filepath.Join(directory, "bicepconfig.json")}, display.ConfigFiles)
	})

	t.Run("omits bicepconfig.json when the directory is unresolved", func(t *testing.T) {
		display := toDisplayOptions(&initOptions{})

		require.Empty(t, display.ConfigFiles)
	})
}
