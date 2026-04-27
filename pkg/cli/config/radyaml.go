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

// DirectoryConfig is the model for repo/project scoped configuration stored in a directory
// next to code.
type DirectoryConfig struct {
	// Filepath is the filepath that was used to the read the configuration. This is
	// not stored in the file, and set by the configuration system for diagnostic purposes.
	Filepath string `yaml:"-"`

	// Workspace contains settings that affect the behavior of the current workspace.
	Workspace DirectoryWorkspaceConfig `yaml:"workspace,omitempty"`
}

// DirectoryWorkspaceConfig contains settings that override settings on the workspace.
type DirectoryWorkspaceConfig struct {
	// Application sets the current application name.
	Application string `yaml:"application,omitempty"`
}

// LoadDirectoryConfig previously loaded `.rad/rad.yaml` from the working directory hierarchy.
// This functionality has been removed. The function always returns nil.
func LoadDirectoryConfig(workingDirectory string) (*DirectoryConfig, error) {
	return nil, nil
}
