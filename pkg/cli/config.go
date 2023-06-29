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

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	validator "github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
	"github.com/gofrs/flock"
	"github.com/spf13/viper"
	"golang.org/x/text/cases"

	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
)

// EnvironmentKey is the key used for the environment section
const (
	ApplicationKey string = "application"
	WorkspacesKey  string = "workspaces"
)

type WorkspaceSection struct {
	Default string                          `json:"default" mapstructure:"default" yaml:"default"`
	Items   map[string]workspaces.Workspace `json:"items" mapstructure:"items" yaml:"items" validate:"dive"`
}

// HasWorkspace returns true if the specified workspace already exists. This function ignores the default workspace.
func (ws WorkspaceSection) HasWorkspace(name string) bool {
	_, ok := ws.Items[cases.Fold().String(name)]
	return ok
}

// GetWorkspace returns the specified workspace or the default workspace if 'name' is empty.
//
// # Function Explanation
//
// GetWorkspace checks if the given workspace name is empty and if so, checks if a default workspace is set. If a
// workspace name is provided, it looks up the workspace in the Items map and returns it. If the workspace does not exist,
// it returns an error.
func (ws WorkspaceSection) GetWorkspace(name string) (*workspaces.Workspace, error) {
	if name == "" && ws.Default == "" {
		return nil, nil
	} else if name == "" {
		name = ws.Default
	}

	result, ok := ws.Items[cases.Fold().String(name)]
	if !ok {
		return nil, fmt.Errorf("the workspace '%v' does not exist. use `rad init` or `rad workspace create` and try again", name)
	}

	return &result, nil
}

// ReadWorkspaceSection reads the WorkspaceSection from radius config.
//
// # Function Explanation
//
// ReadWorkspaceSection reads the WorkspaceSection from the given viper instance, validates it and returns it. If the
// WorkspaceSection is not present, an empty one is returned. If any errors occur during validation, an error is returned.
func ReadWorkspaceSection(v *viper.Viper) (WorkspaceSection, error) {
	section := WorkspaceSection{}
	s := v.Sub(WorkspacesKey)
	if s == nil {
		// This may happen if the key was set directly to one of our structs, so let's try reading
		// that.
		obj := v.Get(WorkspacesKey)
		if obj == nil {
			// OK really nil, return a blank config.
			return WorkspaceSection{Items: map[string]workspaces.Workspace{}}, nil
		}

		s, ok := obj.(WorkspaceSection)
		if !ok {
			return WorkspaceSection{}, fmt.Errorf("failed to read the config file: %s", v.ConfigFileUsed())
		}

		section = s
	} else {
		err := s.UnmarshalExact(&section)
		if err != nil {
			return WorkspaceSection{}, err
		}
	}

	// if items is not present it will be nil
	if section.Items == nil {
		section.Items = map[string]workspaces.Workspace{}
	}

	for name, ws := range section.Items {
		copy := ws

		// The names of the workspace aren't serialized to the configuration in the same
		// way, so set the field here.
		copy.Name = name

		// We also want to make it clear these workspaces came from the per-user (config.yaml)
		// file.
		copy.Source = workspaces.SourceUserConfig

		section.Items[name] = copy
	}

	err := validate(section)
	if err != nil {
		return WorkspaceSection{}, err
	}

	return section, nil
}

// # Function Explanation
//
// UpdateWorkspaceSection updates the WorkspacesKey in the given viper instance with the given WorkspaceSection.
func UpdateWorkspaceSection(v *viper.Viper, section WorkspaceSection) {
	v.Set(WorkspacesKey, section)
}

// HasWorkspace returns true if the specified workspace already exists. This function ignores the default workspace.
//
// # Function Explanation
//
// HasWorkspace reads the workspace section from the given Viper instance and checks if it contains a workspace with the
// given name. If an error occurs while reading the workspace section, it is returned to the caller.
func HasWorkspace(v *viper.Viper, name string) (bool, error) {
	section, err := ReadWorkspaceSection(v)
	if err != nil {
		return false, err
	}

	return section.HasWorkspace(name), nil
}

// GetWorkspace returns the specified workspace or the default workspace in configuration if 'name' is empty.
//
// # Function Explanation
//
// GetWorkspace reads the workspace section from the viper configuration and returns the workspace with the given name, or
// an error if the workspace does not exist or there was an issue reading the configuration.
func GetWorkspace(v *viper.Viper, name string) (*workspaces.Workspace, error) {
	section, err := ReadWorkspaceSection(v)
	if err != nil {
		return nil, err
	}

	return section.GetWorkspace(name)
}

func getConfig(configFilePath string) (*viper.Viper, error) {
	config := viper.New()

	if configFilePath == "" {
		// Set config file using the HOME directory.

		// This is extremely unlikely to fail on us. This would only happen
		// if the user has no HOME (or USERPROFILE on Windows) directory.
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to find the user's home directory: %w", err)
		}

		rad := path.Join(home, ".rad")
		config.AddConfigPath(rad)
		config.SetConfigName("config")
	} else {
		config.SetConfigFile(configFilePath)
	}
	return config, nil
}

// Create a config if its not present
func createConfigFile(configFilePath string) error {
	dir := path.Dir(configFilePath)
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		err := os.MkdirAll(dir, os.ModeDir|0755)
		if err != nil {
			return fmt.Errorf("failed to create directory '%s': %w", dir, err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to find directory '%s': %w", dir, err)
	}
	return nil
}

// # Function Explanation
//
// LoadConfigNoLock reads in a configuration file from the given path and creates it if it doesn't exist. It handles errors
//
//	if the file is not found or if there is an issue reading it.
func LoadConfigNoLock(configFilePath string) (*viper.Viper, error) {
	config, err := getConfig(configFilePath)
	if err != nil {
		return nil, err
	}

	configFile, err := GetConfigFilePath(config)
	if err != nil {
		return nil, err
	}

	// On Ubuntu OS,  getConfig() function doesnt create a config file if its not present.
	err = createConfigFile(configFile)
	if err != nil {
		return nil, err
	}
	err = config.ReadInConfig()
	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		// It's ok the config file is not found, this could be the first time the CLI
		// is running. Commands that require configuration will check for the data they need.
	} else if os.IsNotExist(err) {
		// It's ok the config file is not found, this could be the first time the CLI
		// is running. Commands that require configuration will check for the data they need.
	} else if err != nil {
		return nil, err
	}

	return config, nil
}

// # Function Explanation
//
// LoadConfig() attempts to read a configuration file from the given path and acquire a shared lock on it. If the file does
//
//	not exist, it will be created. If the lock cannot be acquired, an error will be returned.
func LoadConfig(configFilePath string) (*viper.Viper, error) {
	config, err := getConfig(configFilePath)
	if err != nil {
		return nil, err
	}

	configFile, err := GetConfigFilePath(config)
	if err != nil {
		return nil, err
	}

	// On Ubuntu OS,  getConfig() function doesnt create a config file if its not present.
	err = createConfigFile(configFile)
	if err != nil {
		return nil, err
	}
	// Acquire shared lock on the config.yaml.lock file.
	// Retry it every second for 5 times if other goroutine is holding the lock i.e other cmd is writing to the config file.
	// created a new file config.yaml.lock as windows os doesnt let us acuire lock on a file i.e config.yaml and write to it.
	fileLock := flock.New(configFile + ".lock")
	lockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = fileLock.TryRLockContext(lockCtx, 1*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock on '%s': %w", configFile, err)
	}

	defer func() {
		err = fileLock.Unlock()
		if err != nil {
			output.LogInfo("failed to release lock on the config file : %s", configFile)
		}
	}()

	err = config.ReadInConfig()
	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		// It's ok the config file is not found, this could be the first time the CLI
		// is running. Commands that require configuration will check for the data they need.
	} else if os.IsNotExist(err) {
		// It's ok the config file is not found, this could be the first time the CLI
		// is running. Commands that require configuration will check for the data they need.
	} else if err != nil {
		return nil, err
	}

	return config, nil
}

// # Function Explanation
//
// GetConfigFilePath attempts to find the user's config file path, first by checking if one has already been set, and if
// not, by using the user's home directory. If the home directory cannot be found, an error is returned.
func GetConfigFilePath(v *viper.Viper) (string, error) {
	configFilePath := v.ConfigFileUsed()

	// Set config file using the HOME directory.
	if configFilePath == "" {
		// This is extremely unlikely to fail on us. This would only happen
		// if the user has no HOME (or USERPROFILE on Windows) directory.
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to find the user's home directory: %w", err)
		}

		configFilePath = path.Join(home, ".rad", "config.yaml")
	}
	return configFilePath, nil
}

// Required to be called while holding the exclusive lock on config.yaml.lock file.
//
// # Function Explanation
//
// EditWorkspaces is a function that allows users to edit the workspaces in a config file. It takes in a context, config
// and an editor function as parameters. It reads the workspace section from the config, passes it to the editor function
// and then updates the workspace section in the config. It also checks for case-invariance to prevent duplicates. If an
// error occurs, it is returned to the caller.
func EditWorkspaces(ctx context.Context, config *viper.Viper, editor func(section *WorkspaceSection) error) error {
	return SaveConfigOnLock(ctx, config, func(v *viper.Viper) error {
		section, err := ReadWorkspaceSection(v)
		if err != nil {
			return err
		}

		err = editor(&section)
		if err != nil {
			return err
		}

		// We need to check the workspaces for case-invariance. Viper stores everything as lowercase but it's
		// possible for us to introduce bugs by creating duplicates. This section is only here so that we can easily identify a bug
		// in the code that's calling EditWorkspaces.
		names := map[string]bool{}
		for name := range section.Items {
			name = strings.ToLower(name)
			_, ok := names[name]
			if ok {
				return fmt.Errorf("usage of name %q with different casings found. This is a bug in rad, the caller needs to lowercase the name before storage", name)
			}

			names[name] = true
		}

		UpdateWorkspaceSection(v, section)
		return nil
	})
}

// Save Config with exclusive lock on the config file
//
// # Function Explanation
//
// SaveConfigOnLock acquires an exclusive lock on the config file and updates it with the given config, retrying every
// second for 5 times if another goroutine is holding the lock. It returns an error if it fails to acquire the lock or if
// the updateConfig function fails.
func SaveConfigOnLock(ctx context.Context, config *viper.Viper, updateConfig func(*viper.Viper) error) error {
	// Acquire exclusive lock on the config.yaml.lock file.
	// Retry it every second for 5 times if other goroutine is holding the lock i.e other cmd is writing to the config file.
	configFilePath, err := GetConfigFilePath(config)
	if err != nil {
		return err
	}

	fileLock := flock.New(configFilePath + ".lock")
	lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = fileLock.TryLockContext(lockCtx, 1*time.Second)
	if err != nil {
		return fmt.Errorf("failed to acquire lock on '%s': %w", configFilePath, err)
	}

	defer func() {
		err = fileLock.Unlock()
		if err != nil {
			output.LogInfo("failed to release lock on the config file : %s", configFilePath)
		}
	}()
	err = updateConfig(config)
	if err != nil {
		return err
	}
	err = SaveConfig(config)
	if err != nil {
		return err
	}
	return nil
}

// # Function Explanation
//
// SaveConfig saves the configuration to the file path specified in the viper instance. It returns an error if the file
// path is not found or if the configuration could not be written.
func SaveConfig(v *viper.Viper) error {
	configFilePath, err := GetConfigFilePath(v)
	if err != nil {
		return err
	}

	err = v.WriteConfigAs(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to write config to '%s': %w", configFilePath, err)
	}

	return nil
}

func validate(value any) error {
	english := en.New()
	uni := ut.New(english, english)
	trans, _ := uni.GetTranslator("en")

	val := validator.New()
	err := en_translations.RegisterDefaultTranslations(val, trans)
	if err != nil {
		return err
	}

	err = val.Struct(value)
	if err != nil {
		return translateError(err, trans)
	}

	return nil
}

func translateError(err error, trans ut.Translator) error {
	if err == nil {
		return nil
	}

	messages := []string{}
	for _, e := range err.(validator.ValidationErrors) {
		translated := e.Translate(trans)
		messages = append(messages, translated)
	}

	return errors.New(strings.Join(messages, ", "))
}
