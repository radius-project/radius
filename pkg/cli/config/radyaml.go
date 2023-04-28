// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DirectoryConfig is the model for repo/project scopes configuration (rad.yaml) stored in a directory
// next to code.
//
// Use LoadDirectoryConfig to load the configuration.
type DirectoryConfig struct {
	// Filepath is the filepath that was used to the read the configuration. This is
	// not stored in the file, and set by the configuration system for diagnotic purposes.
	Filepath string `yaml:"-"`

	// Workspace contains settings that affect the behavior of the current workspace.
	Workspace DirectoryWorkspaceConfig `yaml:"workspace,omitempty"`
}

// DirectoryWorkspaceConfig contains settings that override settings on the workspace.
type DirectoryWorkspaceConfig struct {
	// Application sets the current application name.
	Application string `yaml:"application,omitempty"`
}

// LoadDirectoryConfig uses the provided directory to find and load `.rad/rad.yaml`. The lookup
// will walk ^up^ the directory hierarchy looking for `.rad/rad.yaml` relative to the current
// directory, halting when it reaches git repo root (`.git`) or the filesystem root.
//
// This scheme allows multiple applications to exist in the same git repo, but also prevents
// configuration from outside of a repo affecting the stuff inside.
func LoadDirectoryConfig(workingDirectory string) (*DirectoryConfig, error) {
	// Root path and clean traversals
	current, err := filepath.Abs(workingDirectory)
	if err != nil {
		return nil, err
	}

	for {
		candidate := filepath.Join(current, ".rad", "rad.yaml")
		file, err := os.Stat(candidate)
		if os.IsNotExist(err) {
			// Not found, continue
		} else if err != nil {
			return nil, err
		} else if file.IsDir() {
			return nil, fmt.Errorf("found rad.yaml at %q, but it is a directory", candidate)
		} else {
			return load(candidate)
		}

		// Not found.... should we keep looking?

		// Is this the root of a git repo?
		_, err = os.Stat(filepath.Join(current, ".git"))
		if os.IsNotExist(err) {
			// Ignore a NotExist errors, that means it's not a git repo.
		} else if err != nil {
			return nil, err
		} else if err == nil {
			// Git repo detected! Stop looking then!
			break
		}

		// Not root of a git repo. Is this the root of the filesystem?

		// We can detect the filesystem root, or drive root (windows) because `filepath.Dir(X)` will return `X`.
		// this way we can detect and prevent infinite loops.
		next := filepath.Dir(current)
		if next == current {
			break
		}

		current = next
	}

	// Unreachable.
	return nil, nil
}

// load loads the actual configuration. Private because we want the rest of our code to use LoadDirectoryConfig.
func load(file string) (*DirectoryConfig, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	config := DirectoryConfig{}
	decoder := yaml.NewDecoder(bytes.NewBuffer(b))
	decoder.KnownFields(true) // Error on unknown fields.

	err = decoder.Decode(&config)
	if err != nil {
		return nil, err
	}

	config.Filepath = file

	return &config, nil
}
