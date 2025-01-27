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

package dynamicrp

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/azure/armauth"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/components/queue/queueprovider"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/driver"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/sdk/clients"
	ucpconfig "github.com/radius-project/radius/pkg/ucp/config"
	sdk_cred "github.com/radius-project/radius/pkg/ucp/credentials"
)

// Options holds the configuration options and shared services for the DyanmicRP server.
//
// For testability, all fields on this struct MUST be constructed from the NewOptions function without any
// additional initialization required.
type Options struct {
	// Config is the configuration for the server.
	Config *Config

	// DatabaseProvider provides access to the database.
	DatabaseProvider *databaseprovider.DatabaseProvider

	// KubernetesProvider provides access to the Kubernetes clients.
	KubernetesProvider *kubernetesclientprovider.KubernetesClientProvider

	// QueueProvider provides access to the message queue client.
	QueueProvider *queueprovider.QueueProvider

	// Recipes is the configuration for the recipe engine subsystem.
	Recipes RecipeOptions

	// SecretProvider provides access to the secret storage system.
	SecretProvider *secretprovider.SecretProvider

	// StatusManager implements operations on async operation statuses.
	StatusManager statusmanager.StatusManager

	// UCP is the connection to UCP
	UCP sdk.Connection
}

// RecipeOptions holds the configuration options for the recipe engine subsystem.
type RecipeOptions struct {
	// ConfigurationLoader is the loader for recipe configurations.
	ConfigurationLoader configloader.ConfigurationLoader

	// Drivers is a map of recipe driver names to driver constructors. If nil, the default drivers are used (Bicep, Terraform) will
	// be used.
	Drivers map[string]func(options *Options) (driver.Driver, error)

	// SecretsLoader provides access to secrets for recipes.
	SecretsLoader configloader.SecretsLoader
}

// NewOptions creates a new Options instance from the given configuration.
func NewOptions(ctx context.Context, config *Config) (*Options, error) {
	var err error
	options := Options{
		Config: config,
	}

	options.QueueProvider = queueprovider.New(config.Queue)
	options.SecretProvider = secretprovider.NewSecretProvider(config.Secrets)
	options.DatabaseProvider = databaseprovider.FromOptions(config.Database)

	databaseClient, err := options.DatabaseProvider.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	queueClient, err := options.QueueProvider.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	options.StatusManager = statusmanager.New(databaseClient, queueClient, config.Environment.RoleLocation)

	options.KubernetesProvider, err = kubernetesclientprovider.FromOptions(config.Kubernetes)
	if err != nil {
		return nil, err
	}

	options.UCP, err = ucpconfig.NewConnectionFromUCPConfig(&config.UCP, options.KubernetesProvider.Config())
	if err != nil {
		return nil, err
	}

	options.Recipes.ConfigurationLoader = configloader.NewEnvironmentLoader(sdk.NewClientOptions(options.UCP))
	options.Recipes.SecretsLoader = configloader.NewSecretStoreLoader(sdk.NewClientOptions(options.UCP))

	// If this is set to nil, then the service will use the default recipe drivers.
	//
	// This pattern allows us to override the drivers for testing.
	options.Recipes.Drivers = nil

	return &options, nil
}

// RecipeEngine creates a new recipe engine from the options.
func (o *Options) RecipeEngine() (engine.Engine, error) {
	var errs error
	drivers := map[string]driver.Driver{}

	// Use the default drivers if not otherwise specified.
	if o.Recipes.Drivers == nil {
		o.Recipes.Drivers = map[string]func(options *Options) (driver.Driver, error){
			recipes.TemplateKindBicep:     bicepDriver,
			recipes.TemplateKindTerraform: terraformDriver,
		}
	}

	for name, driverConstructor := range o.Recipes.Drivers {
		driver, err := driverConstructor(o)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}

		drivers[name] = driver
	}

	if errs != nil {
		return nil, fmt.Errorf("failed to create recipe drivers: %w", errs)
	}

	return engine.NewEngine(engine.Options{
		ConfigurationLoader: o.Recipes.ConfigurationLoader,
		SecretsLoader:       o.Recipes.SecretsLoader,
		Drivers:             drivers}), nil
}

func bicepDriver(options *Options) (driver.Driver, error) {
	deploymentEngineClient, err := clients.NewResourceDeploymentsClient(&clients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          options.UCP.Endpoint(),
		ARMClientOptions: sdk.NewClientOptions(options.UCP),
	})
	if err != nil {
		return nil, err
	}

	provider, err := sdk_cred.NewAzureCredentialProvider(options.SecretProvider, options.UCP, &aztoken.AnonymousCredential{})
	if err != nil {
		return nil, err
	}

	armConfig, err := armauth.NewArmConfig(&armauth.Options{CredentialProvider: provider})
	if err != nil {
		return nil, err
	}

	resourceClient := processors.NewResourceClient(armConfig, options.UCP, options.KubernetesProvider)

	bicepDeleteRetryCount, err := strconv.Atoi(options.Config.Bicep.DeleteRetryCount)
	if err != nil {
		return nil, err
	}

	bicepDeleteRetryDeleteSeconds, err := strconv.Atoi(options.Config.Bicep.DeleteRetryDelaySeconds)
	if err != nil {
		return nil, err
	}

	return driver.NewBicepDriver(
		sdk.NewClientOptions(options.UCP),
		deploymentEngineClient,
		resourceClient,
		driver.BicepOptions{
			DeleteRetryCount:        bicepDeleteRetryCount,
			DeleteRetryDelaySeconds: bicepDeleteRetryDeleteSeconds,
		}), nil
}

func terraformDriver(options *Options) (driver.Driver, error) {
	return driver.NewTerraformDriver(
		options.UCP,
		options.SecretProvider,
		driver.TerraformOptions{
			Path: options.Config.Terraform.Path,
		}, *options.KubernetesProvider), nil
}
