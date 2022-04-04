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
	"github.com/mitchellh/mapstructure"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/spf13/viper"
	"golang.org/x/text/cases"
)

// EnvironmentKey is the key used for the environment section
const (
	EnvironmentKey string = "environment"
	ApplicationKey string = "application"
)

// EnvironmentSection is the representation of the environment section of radius config.
type EnvironmentSection struct {
	Default string                            `json:"default" mapstructure:"default" yaml:"default"`
	Items   map[string]map[string]interface{} `json:"items" mapstructure:"items" yaml:"items"`
}

type ApplicationSection struct {
	Default string `mapstructure:"default" yaml:"default"`
}

func GetEnvironment(v *viper.Viper, name string) (environments.Environment, error) {
	section, err := ReadEnvironmentSection(v)
	if err != nil {
		return nil, err
	}

	return section.GetEnvironment(name)
}

// ReadEnvironmentSection reads the EnvironmentSection from radius config.
func ReadEnvironmentSection(v *viper.Viper) (EnvironmentSection, error) {
	s := v.Sub(EnvironmentKey)
	if s == nil {
		return EnvironmentSection{
			Items: map[string]map[string]interface{}{},
		}, nil
	}

	section := EnvironmentSection{}
	err := s.UnmarshalExact(&section)
	if err != nil {
		return EnvironmentSection{}, nil
	}

	// if items is not present it will be nil
	if section.Items == nil {
		section.Items = map[string]map[string]interface{}{}
	}

	return section, nil
}

// Required to be called while holding the exclusive lock on config.yaml.lock file.
func UpdateEnvironmentWithLatestConfig(env EnvironmentSection, mergeConfigs func(EnvironmentSection, EnvironmentSection) EnvironmentSection) func(*viper.Viper) error {
	return func(config *viper.Viper) error {

		latestConfig, err := LoadConfigNoLock(GetConfigFilePath(config))
		if err != nil {
			return err
		}
		updatedEnv, err := ReadEnvironmentSection(latestConfig)
		if err != nil {
			return err
		}
		updatedEnv = mergeConfigs(env, updatedEnv)
		UpdateEnvironmentSection(config, updatedEnv)
		return nil
	}
}

// UpdateEnvironmentSection updates the EnvironmentSection in radius config.
func UpdateEnvironmentSection(v *viper.Viper, env EnvironmentSection) {
	v.Set(EnvironmentKey, env)
}

// GetEnvironment returns the specified environment or the default environment if 'name' is empty.
func (env EnvironmentSection) GetEnvironment(name string) (environments.Environment, error) {
	if name == "" && env.Default == "" {
		return nil, errors.New("the default environment is not configured. use `rad env switch` to change the selected environment.")
	}

	if name == "" {
		name = env.Default
	}

	return env.decodeEnvironmentSection(name)
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

func MergeInitEnvConfig(envName string) func(EnvironmentSection, EnvironmentSection) EnvironmentSection {
	return func(currentEnvironment EnvironmentSection, latestEnvironment EnvironmentSection) EnvironmentSection {
		latestEnvironment.Default = envName
		latestEnvironment.Items[envName] = currentEnvironment.Items[envName]
		return latestEnvironment
	}
}

func MergeDeleteEnvConfig(envName string) func(EnvironmentSection, EnvironmentSection) EnvironmentSection {
	return func(currentEnvironment EnvironmentSection, latestEnvironment EnvironmentSection) EnvironmentSection {
		delete(latestEnvironment.Items, envName)
		latestEnvironment.Default = currentEnvironment.Default
		return latestEnvironment
	}
}

func MergeSwitchEnvConfig(envName string) func(EnvironmentSection, EnvironmentSection) EnvironmentSection {
	return func(currentEnvironment EnvironmentSection, latestEnvironment EnvironmentSection) EnvironmentSection {
		latestEnvironment.Default = currentEnvironment.Default
		return latestEnvironment
	}
}

func MergeWithLatestConfig(envName string) func(EnvironmentSection, EnvironmentSection) EnvironmentSection {
	return func(currentEnvironment EnvironmentSection, latestEnvironment EnvironmentSection) EnvironmentSection {
		for k, v := range currentEnvironment.Items {
			if _, ok := latestEnvironment.Items[k]; ok {
				for k1, v1 := range v {
					latestEnvironment.Items[k][k1] = v1
				}
			}
		}
		return latestEnvironment
	}
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

func (env EnvironmentSection) decodeEnvironmentSection(name string) (environments.Environment, error) {
	raw, ok := env.Items[cases.Fold().String(name)]
	if !ok {
		return nil, fmt.Errorf("the environment '%v' could not be found in the list of environments. use `rad env list` to list environments", name)
	}

	obj, ok := raw["kind"]
	if !ok {
		return nil, fmt.Errorf("the environment entry '%v' must contain required field 'kind'", name)
	}

	kind, ok := obj.(string)
	if !ok {
		return nil, fmt.Errorf("the 'kind' field for environment entry '%v' must be a string", name)
	}

	if kind == environments.KindAzureCloud {
		decoded := &environments.AzureCloudEnvironment{}
		err := mapstructure.Decode(raw, decoded)
		if err != nil {
			return nil, fmt.Errorf("failed to decode environment entry '%v': %w", name, err)
		}

		decoded.Name = name

		err = validate(decoded)
		if err != nil {
			return nil, fmt.Errorf("the environment entry '%v' is invalid: %w", name, err)
		}

		return decoded, nil
	} else if kind == environments.KindDev {
		decoded := &environments.LocalEnvironment{}
		err := mapstructure.Decode(raw, decoded)
		if err != nil {
			return nil, fmt.Errorf("failed to decode environment entry '%v': %w", name, err)
		}

		decoded.Name = name

		err = validate(decoded)
		if err != nil {
			return nil, fmt.Errorf("the environment entry '%v' is invalid: %w", name, err)
		}

		return decoded, nil
	} else if kind == environments.KindKubernetes {
		decoded := &environments.KubernetesEnvironment{}
		err := mapstructure.Decode(raw, decoded)
		if err != nil {
			return nil, fmt.Errorf("failed to decode environment entry '%v': %w", name, err)
		}

		decoded.Name = name
		err = validate(decoded)
		if err != nil {
			return nil, fmt.Errorf("the environment entry '%v' is invalid: %w", name, err)
		}

		return decoded, nil
	} else if kind == environments.KindLocalRP {
		decoded := &environments.LocalRPEnvironment{}
		err := mapstructure.Decode(raw, decoded)
		if err != nil {
			return nil, fmt.Errorf("failed to decode environment entry '%v': %w", name, err)
		}

		decoded.Name = name

		err = validate(decoded)
		if err != nil {
			return nil, fmt.Errorf("the environment entry '%v' is invalid: %w", name, err)
		}

		return decoded, nil
	} else {
		decoded := &environments.GenericEnvironment{}
		err := mapstructure.Decode(raw, decoded)
		if err != nil {
			return nil, fmt.Errorf("failed to decode environment entry '%v': %w", name, err)
		}

		decoded.Name = name

		err = validate(decoded)
		if err != nil {
			return nil, fmt.Errorf("the environment entry '%v' is invalid: %w", name, err)
		}

		return decoded, nil
	}
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
