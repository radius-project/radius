// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
	"github.com/mitchellh/go-homedir"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/viper"
	"golang.org/x/text/cases"
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
func (ws WorkspaceSection) GetWorkspace(name string) (*workspaces.Workspace, error) {
	if name == "" && ws.Default == "" {
		return nil, errors.New("the default workspace is not configured. use `rad workspace switch` to change the selected workspace.")
	}

	if name == "" {
		name = ws.Default
	}

	result, ok := ws.Items[cases.Fold().String(name)]
	if !ok {
		return nil, fmt.Errorf("the workspace '%v' could not be found in the list of workspace. use `rad workspace list` to list workspaces", name)
	}

	return &result, nil
}

// EnvironmentSection is the representation of the environment section of radius config.
type EnvironmentSection struct {
	Default string                            `json:"default" mapstructure:"default" yaml:"default"`
	Items   map[string]map[string]interface{} `json:"items" mapstructure:"items" yaml:"items"`
}

type ApplicationSection struct {
	Default string `mapstructure:"default" yaml:"default"`
}

// ReadWorkspaceSection reads the WorkspaceSection from radius config.
func ReadWorkspaceSection(v *viper.Viper) (WorkspaceSection, error) {
	s := v.Sub(WorkspacesKey)
	if s == nil {
		return WorkspaceSection{Items: map[string]workspaces.Workspace{}}, nil
	}

	section := WorkspaceSection{}
	err := s.UnmarshalExact(&section)
	if err != nil {
		return WorkspaceSection{}, err
	}

	// if items is not present it will be nil
	if section.Items == nil {
		section.Items = map[string]workspaces.Workspace{}
	}

	// Fixup names for easier access.
	for name, ws := range section.Items {
		copy := ws
		copy.Name = name
		section.Items[name] = copy
	}

	err = validate(section)
	if err != nil {
		return WorkspaceSection{}, err
	}

	return section, nil
}

func UpdateWorkspaceSection(v *viper.Viper, section WorkspaceSection) {
	v.Set(WorkspacesKey, section)
}

// HasWorkspace returns true if the specified workspace already exists. This function ignores the default workspace.
func HasWorkspace(v *viper.Viper, name string) (bool, error) {
	section, err := ReadWorkspaceSection(v)
	if err != nil {
		return false, err
	}

	return section.HasWorkspace(name), nil
}

// GetWorkspace returns the specified workspace or the default workspace if 'name' is empty.
func GetWorkspace(v *viper.Viper, name string) (*workspaces.Workspace, error) {
	section, err := ReadWorkspaceSection(v)
	if err != nil {
		return nil, err
	}

	return section.GetWorkspace(name)
}

func getConfig(configFilePath string) *viper.Viper {
	config := viper.New()

	if configFilePath == "" {
		// Set config file using the HOME directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		rad := path.Join(home, ".rad")
		config.AddConfigPath(rad)
		config.SetConfigName("config")
	} else {
		config.SetConfigFile(configFilePath)
	}
	return config
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

func LoadConfigNoLock(configFilePath string) (*viper.Viper, error) {
	config := getConfig(configFilePath)
	configFile := GetConfigFilePath(config)
	// On Ubuntu OS,  getConfig() function doesnt create a config file if its not present.
	err := createConfigFile(configFile)
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

func LoadConfig(configFilePath string) (*viper.Viper, error) {
	config := getConfig(configFilePath)
	configFile := GetConfigFilePath(config)

	// On Ubuntu OS,  getConfig() function doesnt create a config file if its not present.
	err := createConfigFile(configFile)
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

func GetConfigFilePath(v *viper.Viper) string {
	configFilePath := v.ConfigFileUsed()
	if configFilePath == "" {
		// Set config file using the HOME directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		configFilePath = path.Join(home, ".rad", "config.yaml")
	}
	return configFilePath
}

// Required to be called while holding the exclusive lock on config.yaml.lock file.
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
func SaveConfigOnLock(ctx context.Context, config *viper.Viper, updateConfig func(*viper.Viper) error) error {
	// Acquire exclusive lock on the config.yaml.lock file.
	// Retry it every second for 5 times if other goroutine is holding the lock i.e other cmd is writing to the config file.
	configFilePath := GetConfigFilePath(config)
	fileLock := flock.New(configFilePath + ".lock")
	lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := fileLock.TryLockContext(lockCtx, 1*time.Second)
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

func SaveConfig(v *viper.Viper) error {
	configFilePath := GetConfigFilePath(v)

	err := v.WriteConfigAs(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to write config to '%s': %w", configFilePath, err)
	}

	fmt.Printf("Successfully wrote configuration to %v\n", configFilePath)

	return nil
}

func validate(value interface{}) error {
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
