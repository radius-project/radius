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
	"github.com/project-radius/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/kubeutil"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/frontend/handler"
	"github.com/project-radius/radius/pkg/linkrp/model"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/linkrp/processors/daprstatestores"
	"github.com/project-radius/radius/pkg/linkrp/processors/rediscaches"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/configloader"
	"github.com/project-radius/radius/pkg/recipes/driver"
	"github.com/project-radius/radius/pkg/recipes/engine"
	sv "github.com/project-radius/radius/pkg/rp/secretvalue"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/sdk/clients"
	"k8s.io/client-go/discovery"

	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	backend_ctrl "github.com/project-radius/radius/pkg/linkrp/backend/controller"
)

type Service struct {
	worker.Service
}

func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		worker.Service{
			Options:      options,
			ProviderName: handler.ProviderNamespaceName,
		},
	}
}

func (s *Service) Name() string {
	return fmt.Sprintf("%s async worker", handler.ProviderNamespaceName)
}

func (s *Service) Run(ctx context.Context) error {
	if err := s.Init(ctx); err != nil {
		return err
	}

	runtimeClient, err := kubeutil.NewRuntimeClient(s.Options.K8sConfig)
	if err != nil {
		return err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(s.Options.K8sConfig)
	if err != nil {
		return err
	}

	client := processors.NewResourceClient(s.Options.Arm, s.Options.UCPConnection, runtimeClient, discoveryClient)
	clientOptions := sdk.NewClientOptions(s.Options.UCPConnection)

	deploymentEngineClient, err := clients.NewResourceDeploymentsClient(&clients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          s.Options.UCPConnection.Endpoint(),
		ARMClientOptions: sdk.NewClientOptions(s.Options.UCPConnection),
	})
	if err != nil {
		return err
	}

	configLoader := configloader.NewEnvironmentLoader(clientOptions)
	engine := engine.NewEngine(engine.Options{
		ConfigurationLoader: configLoader,
		Drivers: map[string]driver.Driver{
			recipes.TemplateKindBicep: driver.NewBicepDriver(clientOptions, deploymentEngineClient),
		},
	})

	// resourceTypes is the array that holds resource types that needs async processing.
	// We use this array to register backend controllers for each resource.
	resourceTypes := []struct {
		TypeName            string
		CreatePutController func(options ctrl.Options) (ctrl.Controller, error)
	}{
		{linkrp.MongoDatabasesResourceType, backend_ctrl.NewLegacyCreateOrUpdateResource},
		{linkrp.RedisCachesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &rediscaches.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.RedisCache, datamodel.RedisCache](processor, engine, client, configLoader, options)
		}},
		{linkrp.DaprStateStoresResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &daprstatestores.Processor{Client: runtimeClient}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.DaprStateStore, datamodel.DaprStateStore](processor, engine, client, configLoader, options)
		}},
	}

	linkAppModel, err := model.NewApplicationModel(s.Options.Arm, s.KubeClient, s.Options.UCPConnection)
	if err != nil {
		return fmt.Errorf("failed to initialize application model: %w", err)
	}

	opts := ctrl.Options{
		DataProvider: s.StorageProvider,
		KubeClient:   s.KubeClient,
		GetLinkDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewDeploymentProcessor(linkAppModel, s.StorageProvider, sv.NewSecretValueClient(s.Options.Arm), s.KubeClient)
		},
	}

	for _, rt := range resourceTypes {
		// Register controllers
		err = s.Controllers.Register(ctx, rt.TypeName, v1.OperationDelete, backend_ctrl.NewDeleteResource, opts)
		if err != nil {
			return err
		}
		err = s.Controllers.Register(ctx, rt.TypeName, v1.OperationPut, rt.CreatePutController, opts)
		if err != nil {
			return err
		}
	}
	workerOpts := worker.Options{}
	if s.Options.Config.WorkerServer != nil {
		if s.Options.Config.WorkerServer.MaxOperationConcurrency != nil {
			workerOpts.MaxOperationConcurrency = *s.Options.Config.WorkerServer.MaxOperationConcurrency
		}
		if s.Options.Config.WorkerServer.MaxOperationRetryCount != nil {
			workerOpts.MaxOperationRetryCount = *s.Options.Config.WorkerServer.MaxOperationRetryCount
		}
	}

	return s.Start(ctx, workerOpts)
}
