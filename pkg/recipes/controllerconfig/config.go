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
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/driver"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/secret/provider"
)

// RecipeControllerConfig is the configuration for the controllers which uses recipe.
type RecipeControllerConfig struct {
	// K8sClients is the collections of Kubernetes clients.
	K8sClients *kubeutil.Clients

	// ResourceClient is a client used by resource processors for interacting with UCP resources.
	ResourceClient processors.ResourceClient

	// ConfigLoader is the configuration loader.
	ConfigLoader configloader.ConfigurationLoader

	// DeploymentEngineClient is the client for interacting with the deployment engine.
	DeploymentEngineClient *clients.ResourceDeploymentsClient

	// Engine is the engine for executing recipes.
	Engine engine.Engine
}

// New creates a new RecipeControllerConfig instance with the given host options.
func New(options hostoptions.HostOptions) (*RecipeControllerConfig, error) {
	cfg := &RecipeControllerConfig{}
	var err error
	cfg.K8sClients, err = kubeutil.NewClients(options.K8sConfig)
	if err != nil {
		return nil, err
	}

	cfg.ResourceClient = processors.NewResourceClient(options.Arm, options.UCPConnection, cfg.K8sClients.RuntimeClient, cfg.K8sClients.DiscoveryClient)
	clientOptions := sdk.NewClientOptions(options.UCPConnection, nil)

	cfg.DeploymentEngineClient, err = clients.NewResourceDeploymentsClient(&clients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          options.UCPConnection.Endpoint(),
		ARMClientOptions: sdk.NewClientOptions(options.UCPConnection, nil),
	})
	if err != nil {
		return nil, err
	}

	cfg.ConfigLoader = configloader.NewEnvironmentLoader(clientOptions)
	cfg.Engine = engine.NewEngine(engine.Options{
		ConfigurationLoader: cfg.ConfigLoader,
		Drivers: map[string]driver.Driver{
			recipes.TemplateKindBicep: driver.NewBicepDriver(clientOptions, cfg.DeploymentEngineClient, cfg.ResourceClient),
			recipes.TemplateKindTerraform: driver.NewTerraformDriver(options.UCPConnection, provider.NewSecretProvider(options.Config.SecretProvider),
				driver.TerraformOptions{
					Path: options.Config.Terraform.Path,
				}, cfg.K8sClients.ClientSet),
		},
	})

	return cfg, nil
}
