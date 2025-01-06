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

package controllerconfig

import (
	"strconv"

	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/driver"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/sdk/clients"
)

// RecipeControllerConfig is the configuration for the controllers which uses recipe.
type RecipeControllerConfig struct {
	// Kubernetes provides access to the Kubernetes clients.
	Kubernetes *kubernetesclientprovider.KubernetesClientProvider

	// ConfigLoader is the configuration loader.
	ConfigLoader configloader.ConfigurationLoader

	// DeploymentEngineClient is the client for interacting with the deployment engine.
	DeploymentEngineClient *clients.ResourceDeploymentsClient

	// Engine is the engine for executing recipes.
	Engine engine.Engine

	// UCPConnection is the connection to UCP
	UCPConnection *sdk.Connection
}

// New creates a new RecipeControllerConfig instance with the given host options.
func New(options hostoptions.HostOptions) (*RecipeControllerConfig, error) {
	cfg := &RecipeControllerConfig{}
	var err error

	cfg.Kubernetes = kubernetesclientprovider.FromConfig(options.K8sConfig)

	cfg.UCPConnection = &options.UCPConnection

	clientOptions := sdk.NewClientOptions(options.UCPConnection)

	cfg.DeploymentEngineClient, err = clients.NewResourceDeploymentsClient(&clients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          options.UCPConnection.Endpoint(),
		ARMClientOptions: sdk.NewClientOptions(options.UCPConnection),
	})
	if err != nil {
		return nil, err
	}

	if options.Config.Bicep.DeleteRetryCount == "" {
		options.Config.Bicep.DeleteRetryCount = "3"
	}

	if options.Config.Bicep.DeleteRetryDelaySeconds == "" {
		options.Config.Bicep.DeleteRetryDelaySeconds = "10"
	}

	bicepDeleteRetryCount, err := strconv.Atoi(options.Config.Bicep.DeleteRetryCount)
	if err != nil {
		return nil, err
	}

	bicepDeleteRetryDeleteSeconds, err := strconv.Atoi(options.Config.Bicep.DeleteRetryDelaySeconds)
	if err != nil {
		return nil, err
	}

	cfg.ConfigLoader = configloader.NewEnvironmentLoader(clientOptions)
	cfg.Engine = engine.NewEngine(engine.Options{
		ConfigurationLoader: cfg.ConfigLoader,
		SecretsLoader:       configloader.NewSecretStoreLoader(clientOptions),
		Drivers: map[string]driver.Driver{
			recipes.TemplateKindBicep: driver.NewBicepDriver(
				clientOptions,
				cfg.DeploymentEngineClient,
				processors.NewResourceClient(options.Arm, options.UCPConnection, cfg.Kubernetes),
				driver.BicepOptions{
					DeleteRetryCount:        bicepDeleteRetryCount,
					DeleteRetryDelaySeconds: bicepDeleteRetryDeleteSeconds,
				},
			),
			recipes.TemplateKindTerraform: driver.NewTerraformDriver(options.UCPConnection, secretprovider.NewSecretProvider(options.Config.SecretProvider),
				driver.TerraformOptions{
					Path: options.Config.Terraform.Path,
				}, *cfg.Kubernetes),
		},
	})

	return cfg, nil
}
