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

package backend

import (
	"context"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	backend_ctrl "github.com/project-radius/radius/pkg/corerp/backend/controller"
	"github.com/project-radius/radius/pkg/corerp/backend/deployment"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/model"
	"github.com/project-radius/radius/pkg/corerp/processors/extenders"
	"github.com/project-radius/radius/pkg/kubeutil"
	"github.com/project-radius/radius/pkg/linkrp"
	linkrp_backend_ctrl "github.com/project-radius/radius/pkg/linkrp/backend/controller"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/configloader"
	"github.com/project-radius/radius/pkg/recipes/driver"
	"github.com/project-radius/radius/pkg/recipes/engine"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/sdk/clients"
	"github.com/project-radius/radius/pkg/ucp/secret/provider"
	"k8s.io/client-go/discovery"
)

const (
	providerName = "Applications.Core"
)

var (
	// ResourceTypeNames is the array that holds resource types that needs async processing.
	// We use this array to generate generic backend controller for each resource.
	ResourceTypeNames = []string{
		"Applications.Core/containers",
		"Applications.Core/gateways",
		"Applications.Core/httpRoutes",
		"Applications.Core/volumes",
	}
)

// Service is a service to run AsyncReqeustProcessWorker.
type Service struct {
	worker.Service
}

// # Function Explanation
//
// NewService creates a new Service instance to run AsyncReqeustProcessWorker.
func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		worker.Service{
			ProviderName: providerName,
			Options:      options,
		},
	}
}

// # Function Explanation
//
// Name returns a string containing the service name.
func (w *Service) Name() string {
	return fmt.Sprintf("%s async worker", providerName)
}

// # Function Explanation
//
// Run initializes the application model, registers controllers for different resource types, and starts the worker with
// the given options.
func (w *Service) Run(ctx context.Context) error {
	if err := w.Init(ctx); err != nil {
		return err
	}

	coreAppModel, err := model.NewApplicationModel(w.Options.Arm, w.KubeClient, w.KubeClientSet)
	if err != nil {
		return fmt.Errorf("failed to initialize application model: %w", err)
	}

	opts := ctrl.Options{
		DataProvider: w.StorageProvider,
		KubeClient:   w.KubeClient,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewDeploymentProcessor(coreAppModel, w.StorageProvider, w.KubeClient, w.KubeClientSet)
		},
	}

	for _, rt := range ResourceTypeNames {
		// Register controllers
		err = w.Controllers.Register(ctx, rt, v1.OperationPut, backend_ctrl.NewCreateOrUpdateResource, opts)
		if err != nil {
			panic(err)
		}
		err = w.Controllers.Register(ctx, rt, v1.OperationPatch, backend_ctrl.NewCreateOrUpdateResource, opts)
		if err != nil {
			panic(err)
		}
		err = w.Controllers.Register(ctx, rt, v1.OperationDelete, backend_ctrl.NewDeleteResource, opts)
		if err != nil {
			panic(err)
		}
	}

	// Setup to run backend controller for extenders.
	runtimeClient, err := kubeutil.NewRuntimeClient(w.Options.K8sConfig)
	if err != nil {
		return err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(w.Options.K8sConfig)
	if err != nil {
		return err
	}

	// Use legacy discovery client to avoid the issue of the staled GroupVersion discovery(api.ucp.dev/v1alpha3).
	// TODO: Disable UseLegacyDiscovery once https://github.com/project-radius/radius/issues/5974 is resolved.
	discoveryClient.UseLegacyDiscovery = true

	client := processors.NewResourceClient(w.Options.Arm, w.Options.UCPConnection, runtimeClient, discoveryClient)
	clientOptions := sdk.NewClientOptions(w.Options.UCPConnection)

	deploymentEngineClient, err := clients.NewResourceDeploymentsClient(&clients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          w.Options.UCPConnection.Endpoint(),
		ARMClientOptions: sdk.NewClientOptions(w.Options.UCPConnection),
	})
	if err != nil {
		return err
	}

	configLoader := configloader.NewEnvironmentLoader(clientOptions)
	engine := engine.NewEngine(engine.Options{
		ConfigurationLoader: configLoader,
		Drivers: map[string]driver.Driver{
			recipes.TemplateKindBicep: driver.NewBicepDriver(clientOptions, deploymentEngineClient, client),
			recipes.TemplateKindTerraform: driver.NewTerraformDriver(w.Options.UCPConnection, provider.NewSecretProvider(w.Options.Config.SecretProvider),
				driver.TerraformOptions{
					Path: w.Options.Config.Terraform.Path,
				}),
		},
	})

	opts.GetDeploymentProcessor = nil
	extenderCreateOrUpdateController := func(options ctrl.Options) (ctrl.Controller, error) {
		processor := &extenders.Processor{}
		return linkrp_backend_ctrl.NewCreateOrUpdateResource[*datamodel.Extender, datamodel.Extender](processor, engine, client, configLoader, options)
	}

	// Register controllers to run backend processing for extenders.
	err = w.Controllers.Register(ctx, linkrp.N_ExtendersResourceType, v1.OperationPut, extenderCreateOrUpdateController, opts)
	if err != nil {
		return err
	}
	err = w.Controllers.Register(
		ctx,
		linkrp.N_ExtendersResourceType,
		v1.OperationDelete,
		func(options ctrl.Options) (ctrl.Controller, error) {
			return linkrp_backend_ctrl.NewDeleteResource(options, engine)
		},
		opts)
	if err != nil {
		return err
	}

	workerOpts := worker.Options{}
	if w.Options.Config.WorkerServer != nil {
		if w.Options.Config.WorkerServer.MaxOperationConcurrency != nil {
			workerOpts.MaxOperationConcurrency = *w.Options.Config.WorkerServer.MaxOperationConcurrency
		}
		if w.Options.Config.WorkerServer.MaxOperationRetryCount != nil {
			workerOpts.MaxOperationRetryCount = *w.Options.Config.WorkerServer.MaxOperationRetryCount
		}
	}

	return w.Start(ctx, workerOpts)
}
