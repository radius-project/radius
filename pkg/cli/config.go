// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cli

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/Azure/radius/pkg/cli/environments"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	validator "github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
	"github.com/mitchellh/go-homedir"
	"github.com/mitchellh/mapstructure"
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
	Default string                            `mapstructure:"default" yaml:"default"`
	Items   map[string]map[string]interface{} `mapstructure:"items" yaml:"items"`
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

func LoadConfig(configFilePath string) (*viper.Viper, error) {
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

	err := config.ReadInConfig()
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

func SaveConfig(v *viper.Viper) error {
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

	err = v.WriteConfigAs(configFilePath)
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
	} else if kind == environments.KindLocal {
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
