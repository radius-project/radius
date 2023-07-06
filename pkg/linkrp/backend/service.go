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
	"github.com/project-radius/radius/pkg/linkrp/frontend/handler"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/linkrp/processors/daprpubsubbrokers"
	"github.com/project-radius/radius/pkg/linkrp/processors/daprsecretstores"
	"github.com/project-radius/radius/pkg/linkrp/processors/daprstatestores"
	"github.com/project-radius/radius/pkg/linkrp/processors/extenders"
	"github.com/project-radius/radius/pkg/linkrp/processors/mongodatabases"
	"github.com/project-radius/radius/pkg/linkrp/processors/rabbitmqmessagequeues"
	"github.com/project-radius/radius/pkg/linkrp/processors/rediscaches"
	"github.com/project-radius/radius/pkg/linkrp/processors/sqldatabases"
	msg_dm "github.com/project-radius/radius/pkg/messagingrp/datamodel"
	"github.com/project-radius/radius/pkg/messagingrp/processors/rabbitmqqueues"

	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/configloader"
	"github.com/project-radius/radius/pkg/recipes/driver"
	"github.com/project-radius/radius/pkg/recipes/engine"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/sdk/clients"
	"k8s.io/client-go/discovery"

	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	backend_ctrl "github.com/project-radius/radius/pkg/linkrp/backend/controller"
)

type Service struct {
	worker.Service
}

// # Function Explanation
//
// NewService creates a new Service instance with the given options.
func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		worker.Service{
			Options:      options,
			ProviderName: handler.LinkProviderNamespace,
		},
	}
}

// # Function Explanation
//
// Name returns a string containing the namespace of the LinkProvider.
func (s *Service) Name() string {
	return fmt.Sprintf("%s async worker", handler.LinkProviderNamespace)
}

// # Function Explanation
//
// Run initializes the service and registers controllers for each resource type to handle create/update/delete operations.
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
			recipes.TemplateKindTerraform: driver.NewTerraformDriver(s.Options.UCPConnection, driver.TerraformOptions{
				Path: s.Options.Config.Terraform.Path,
			}),
		},
	})

	// resourceTypes is the array that holds resource types that needs async processing.
	// We use this array to register backend controllers for each resource.
	resourceTypes := []struct {
		TypeName            string
		CreatePutController func(options ctrl.Options) (ctrl.Controller, error)
	}{
		{linkrp.MongoDatabasesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &mongodatabases.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.MongoDatabase, datamodel.MongoDatabase](processor, engine, client, configLoader, options)
		}},
		{linkrp.RedisCachesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &rediscaches.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.RedisCache, datamodel.RedisCache](processor, engine, client, configLoader, options)
		}},
		{linkrp.SqlDatabasesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &sqldatabases.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.SqlDatabase, datamodel.SqlDatabase](processor, engine, client, configLoader, options)
		}},
		{linkrp.RabbitMQMessageQueuesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &rabbitmqmessagequeues.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.RabbitMQMessageQueue, datamodel.RabbitMQMessageQueue](processor, engine, client, configLoader, options)
		}},
		{linkrp.DaprStateStoresResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &daprstatestores.Processor{Client: runtimeClient}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.DaprStateStore, datamodel.DaprStateStore](processor, engine, client, configLoader, options)
		}},
		{linkrp.DaprSecretStoresResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &daprsecretstores.Processor{Client: runtimeClient}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.DaprSecretStore, datamodel.DaprSecretStore](processor, engine, client, configLoader, options)
		}},
		{linkrp.DaprPubSubBrokersResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &daprpubsubbrokers.Processor{Client: runtimeClient}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.DaprPubSubBroker, datamodel.DaprPubSubBroker](processor, engine, client, configLoader, options)
		}},
		{linkrp.ExtendersResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &extenders.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*datamodel.Extender, datamodel.Extender](processor, engine, client, configLoader, options)
		}},

		// Updates for Spliting Linkrp Namespace
		{linkrp.N_RabbitMQQueuesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &rabbitmqqueues.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*msg_dm.RabbitMQQueue, msg_dm.RabbitMQQueue](processor, engine, client, configLoader, options)
		}},
		/*  The following will be worked on and uncommented in upcoming PRs
		{linkrp.N_MongoDatabasesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &mongodatabases.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*ds_dm.MongoDatabase, ds_dm.MongoDatabase](processor, engine, client, configLoader, options)
		}},
		{linkrp.N_RedisCachesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &rediscaches.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*ds_dm.RedisCache, ds_dm.RedisCache](processor, engine, client, configLoader, options)
		}},
		{linkrp.N_SqlDatabasesResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &sqldatabases.Processor{}
			return backend_ctrl.NewCreateOrUpdateResource[*ds_dm.SqlDatabase, ds_dm.SqlDatabase](processor, engine, client, configLoader, options)
		}},
		{linkrp.N_DaprStateStoresResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &daprstatestores.Processor{Client: runtimeClient}
			return backend_ctrl.NewCreateOrUpdateResource[*dapr_dm.DaprStateStore, dapr_dm.DaprStateStore](processor, engine, client, configLoader, options)
		}},
		{linkrp.N_DaprSecretStoresResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &daprsecretstores.Processor{Client: runtimeClient}
			return backend_ctrl.NewCreateOrUpdateResource[*dapr_dm.DaprSecretStore, dapr_dm.DaprSecretStore](processor, engine, client, configLoader, options)
		}},
		{linkrp.N_DaprPubSubBrokersResourceType, func(options ctrl.Options) (ctrl.Controller, error) {
			processor := &daprpubsubbrokers.Processor{Client: runtimeClient}
			return backend_ctrl.NewCreateOrUpdateResource[*dapr_dm.DaprPubSubBroker, dapr_dm.DaprPubSubBroker](processor, engine, client, configLoader, options)
		}},*/
	}

	opts := ctrl.Options{
		DataProvider: s.StorageProvider,
		KubeClient:   s.KubeClient,
	}

	for _, rt := range resourceTypes {
		// Register controllers
		err = s.Controllers.Register(ctx, rt.TypeName, v1.OperationDelete, func(options ctrl.Options) (ctrl.Controller, error) {
			return backend_ctrl.NewDeleteResource(options, client)
		}, opts)
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
