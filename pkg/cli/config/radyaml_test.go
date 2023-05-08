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

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	validContent = `workspace:
  application: 'foo'`
)

func Test_load_valid(t *testing.T) {
	content := validContent

	directory := t.TempDir()
	file := filepath.Join(directory, "rad.yaml")
	err := os.WriteFile(file, []byte(content), 0644)
	require.NoError(t, err)

	config, err := load(file)
	require.NoError(t, err)

	expected := &DirectoryConfig{
		Filepath: file, // Populated by the loader
		Workspace: DirectoryWorkspaceConfig{
			Application: "foo",
		},
	}
	require.Equal(t, expected, config)
}

func Test_load_invalid(t *testing.T) {
	content := `
workspace:
  application: 'foo'
  anotherField: 17`

	directory := t.TempDir()
	file := filepath.Join(directory, "rad.yaml")
	err := os.WriteFile(file, []byte(content), 0644)
	require.NoError(t, err)

	config, err := load(file)
	require.Error(t, err)
	require.Nil(t, config)
}

func Test_LoadDirectoryConfig_NotFound_FileSystemRoot(t *testing.T) {
	// Don't set up anything, this will walk all the way up to the filesystem root.
	directory := t.TempDir()

	config, err := LoadDirectoryConfig(directory)
	require.NoError(t, err)
	require.Nil(t, config)
}

func Test_LoadDirectoryConfig_NotFound_GitRepoRoot(t *testing.T) {
	// Set this up like we're in the root of a git repo.
	directory := t.TempDir()
	start := directory
	err := os.MkdirAll(filepath.Join(directory, ".git"), 0755)
	require.NoError(t, err)

	config, err := LoadDirectoryConfig(start)
	require.NoError(t, err)
	require.Nil(t, config)
}

func Test_LoadDirectoryConfig_NotFound_GitRepoChildDirectory(t *testing.T) {
	// Set this up like we're in a subdirectory of a git repo.
	directory := t.TempDir()
	start := filepath.Join(directory, "a", "b", "c")

	err := os.MkdirAll(filepath.Join(directory, ".git"), 0755)
	require.NoError(t, err)
	err = os.MkdirAll(start, 0755)
	require.NoError(t, err)

	config, err := LoadDirectoryConfig(start)
	require.NoError(t, err)
	require.Nil(t, config)
}

func Test_LoadDirectoryConfig_Found_GitRepoRoot(t *testing.T) {
	// Set this up like we're in the root of a git repo.
	directory := t.TempDir()
	start := directory

	err := os.MkdirAll(filepath.Join(directory, ".git"), 0755)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(start, ".rad"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(start, ".rad", "rad.yaml"), []byte(validContent), 0644)
	require.NoError(t, err)

	config, err := LoadDirectoryConfig(start)
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Equal(t, filepath.Join(directory, ".rad", "rad.yaml"), config.Filepath)
}

func Test_LoadDirectoryConfig_Found_GitRepoChildDirectory(t *testing.T) {
	// Set this up like we're in a subdirectory of a git repo.
	directory := t.TempDir()
	start := filepath.Join(directory, "a", "b", "c")

	err := os.MkdirAll(filepath.Join(directory, ".git"), 0755)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(directory, "a", ".rad"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(directory, "a", ".rad", "rad.yaml"), []byte(validContent), 0644)
	require.NoError(t, err)

	config, err := LoadDirectoryConfig(start)
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Equal(t, filepath.Join(directory, "a", ".rad", "rad.yaml"), config.Filepath)
}

func Test_LoadDirectoryConfig_Found_OverrideInRepo(t *testing.T) {
	// Set this up like we're in a subdirectory of a git repo.
	directory := t.TempDir()
	start := filepath.Join(directory, "a", "b", "c")

	// We're going to create TWO rad.yamls, we'll find the one in 'b' not in 'a'.

	err := os.MkdirAll(filepath.Join(directory, ".git"), 0755)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(directory, "a", "b", ".rad"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(directory, "a", "b", ".rad", "rad.yaml"), []byte(validContent), 0644)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(directory, "a", ".rad"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(directory, "a", ".rad", "rad.yaml"), []byte("something invalid, it won't be used"), 0644)
	require.NoError(t, err)

	config, err := LoadDirectoryConfig(start)
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Equal(t, filepath.Join(directory, "a", "b", ".rad", "rad.yaml"), config.Filepath)
}
