// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environment

import (
	"context"
	"fmt"
	"path"

	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/test/config"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

type TestEnvironment struct {
	ResourceGroup             string
	ControlPlaneResourceGroup string
	SubscriptionID            string
	ClusterName               string
	ConfigPath                string
}

func GetTestEnvironment(ctx context.Context, config *config.AzureConfig) (*TestEnvironment, error) {

	if config.ConfigPath == "" {
		v, err := loadDefaultConfig()
		if err != nil {
			return nil, err
		}

		return readDefaultEnvironment(v)
	}

	v, err := loadExternalConfig(config.ConfigPath)
	if err != nil {
		return nil, err
	}

	return readDefaultEnvironment(v)
}

func loadDefaultConfig() (*viper.Viper, error) {
	// We need to read the environment so we can get the subscription ID
	home, err := homedir.Dir()
	if err != nil {
		return nil, fmt.Errorf("cannot locate home directory: %w", err)
	}

	file := path.Join(home, ".rad")
	v := viper.New()
	v.AddConfigPath(file)
	err = v.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration: %w", err)
	}

	return v, nil
}

func loadExternalConfig(configpath string) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigFile(configpath)
	err := v.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration: %w", err)
	}

	return v, nil
}

func readDefaultEnvironment(v *viper.Viper) (*TestEnvironment, error) {
	env, err := rad.ReadEnvironmentSection(v)
	if err != nil {
		return nil, fmt.Errorf("failed to read environment configuration: %w", err)
	}

	current, err := env.GetEnvironment("")
	if err != nil {
		return nil, err
	}

	azure, err := environments.RequireAzureCloud(current)
	if err != nil {
		return nil, err
	}

	return &TestEnvironment{
		SubscriptionID:            azure.SubscriptionID,
		ResourceGroup:             azure.ResourceGroup,
		ControlPlaneResourceGroup: azure.ControlPlaneResourceGroup,
		ConfigPath:                v.ConfigFileUsed(),
	}, nil
}
