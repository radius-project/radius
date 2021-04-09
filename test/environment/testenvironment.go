// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environment

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/test/config"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

var (
	accountName      = "deploytests"
	accountGroupName = "deploytests"
)

type TestEnvironment struct {
	UsingReservedTestCluster bool
	ResourceGroup            string
	SubscriptionID           string
	ConfigPath               string
}

func GetTestEnvironment(ctx context.Context, config *config.AzureConfig) (*TestEnvironment, error) {
	if config.SubscriptionID() == "" {
		// using local environment for testing
		return useLocalEnvironment(ctx)
	}

	return findTestEnvironment(ctx, config)
}

func ReleaseTestEnvironment(ctx context.Context, config *config.AzureConfig, env TestEnvironment) error {
	return BreakStorageContainerLease(ctx, config.Authorizer, config.SubscriptionID(), accountName, accountGroupName, env.ResourceGroup)
}

func useLocalEnvironment(ctx context.Context) (*TestEnvironment, error) {
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
		UsingReservedTestCluster: false,
		SubscriptionID:           azure.SubscriptionID,
		ResourceGroup:            azure.ResourceGroup,
		ConfigPath:               v.ConfigFileUsed(),
	}, nil
}

func findTestEnvironment(ctx context.Context, config *config.AzureConfig) (*TestEnvironment, error) {
	file, err := os.Open("deploy-tests-clusters.txt")
	if err != nil {
		return nil, fmt.Errorf("cannot read test cluster manifest: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		resourceGroup := scanner.Text()

		// Check if cluster is in use
		err = AcquireStorageContainerLease(ctx, config.Authorizer, config.SubscriptionID(), accountName, accountGroupName, resourceGroup)
		if err != nil {
			fmt.Printf("Test cluster: %s not available. err: %v\n", resourceGroup, err)
			continue
		}

		// Found test cluster and acquired lease
		return &TestEnvironment{
			UsingReservedTestCluster: true,
			SubscriptionID:           config.SubscriptionID(),
			ResourceGroup:            resourceGroup,
			ConfigPath:               filepath.Join("./", fmt.Sprintf("%s.yaml", resourceGroup)),
		}, nil
	}

	return nil, errors.New("Could not find a test cluster. Retry later")
}
