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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	dapr_dm "github.com/radius-project/radius/pkg/daprrp/datamodel"
	"github.com/radius-project/radius/pkg/daprrp/processors/pubsubbrokers"
	"github.com/radius-project/radius/pkg/daprrp/processors/secretstores"
	"github.com/radius-project/radius/pkg/daprrp/processors/statestores"
	ds_dm "github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	mongo_prc "github.com/radius-project/radius/pkg/datastoresrp/processors/mongodatabases"
	redis_prc "github.com/radius-project/radius/pkg/datastoresrp/processors/rediscaches"
	sql_prc "github.com/radius-project/radius/pkg/datastoresrp/processors/sqldatabases"
	msg_dm "github.com/radius-project/radius/pkg/messagingrp/datamodel"
	"github.com/radius-project/radius/pkg/messagingrp/processors/rabbitmqqueues"
	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/portableresources/frontend/handler"
	"github.com/radius-project/radius/pkg/portableresources/processors"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/driver"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/sdk/clients"

	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	backend_ctrl "github.com/radius-project/radius/pkg/portableresources/backend/controller"

	"github.com/radius-project/radius/pkg/ucp/secret/provider"
)

type Service struct {
	worker.Service
}

// NewService creates a new Service instance with the given options.
func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		worker.Service{
			Options:      options,
			ProviderName: handler.PortableResourcesNamespace,
		},
	}
}

// Name returns a string containing the namespace of the Resource Provider.
func (s *Service) Name() string {
	return fmt.Sprintf("%s async worker", handler.PortableResourcesNamespace)
}

// Run initializes the service and registers controllers for each resource type to handle create/update/delete operations.
func (s *Service) Run(ctx context.Context) error {
	if err := s.Init(ctx); err != nil {
		return err
	}

	client := processors.NewResourceClient(s.Options.Arm, s.Options.UCPConnection, s.KubeClient, s.KubeDiscoveryClient)
	clientOptions := sdk.NewClientOptions(s.Options.UCPConnection)

	deploymentEngineClient, err := clients.NewResourceDeploymentsClient(&clients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          s.Options.UCPConnection.Endpoint(),
		ARMClientOptions: clientOptions,
	})
	if err != nil {
		return err
	}

	configLoader := configloader.NewEnvironmentLoader(clientOptions)
	engine := engine.NewEngine(engine.Options{
		ConfigurationLoader: configLoader,
		Drivers: map[string]driver.Driver{
			recipes.TemplateKindBicep: driver.NewBicepDriver(clientOptions, deploymentEngineClient, client),
			recipes.TemplateKindTerraform: driver.NewTerraformDriver(s.Options.UCPConnection, provider.NewSecretProvider(s.Options.Config.SecretProvider),
				driver.TerraformOptions{
					Path: s.Options.Config.Terraform.Path,
				}, s.KubeClientSet),
		},
	})

	// resourceTypes is the array that holds resource types that needs async processing.
	// We use this array to register backend controllers for each resource.
	resourceTypes := []struct {
		TypeName               string
		CreatePutController    func(options ctrl.Options) (ctrl.Controller, error)
		CreateDeleteController func(options ctrl.Options) (ctrl.Controller, error)
	}{
		{
			portableresources.RabbitMQQueuesResourceType,
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &rabbitmqqueues.Processor{}
				return backend_ctrl.NewCreateOrUpdateResource[*msg_dm.RabbitMQQueue, msg_dm.RabbitMQQueue](processor, engine, client, configLoader, options)
			},
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &rabbitmqqueues.Processor{}
				return backend_ctrl.NewDeleteResource[*msg_dm.RabbitMQQueue, msg_dm.RabbitMQQueue](processor, engine, configLoader, options)
			},
		},
		{
			portableresources.DaprStateStoresResourceType,
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &statestores.Processor{Client: s.KubeClient}
				return backend_ctrl.NewCreateOrUpdateResource[*dapr_dm.DaprStateStore, dapr_dm.DaprStateStore](processor, engine, client, configLoader, options)
			},
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &statestores.Processor{Client: s.KubeClient}
				return backend_ctrl.NewDeleteResource[*dapr_dm.DaprStateStore, dapr_dm.DaprStateStore](processor, engine, configLoader, options)
			},
		},
		{
			portableresources.DaprSecretStoresResourceType,
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &secretstores.Processor{Client: s.KubeClient}
				return backend_ctrl.NewCreateOrUpdateResource[*dapr_dm.DaprSecretStore, dapr_dm.DaprSecretStore](processor, engine, client, configLoader, options)
			},
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &secretstores.Processor{Client: s.KubeClient}
				return backend_ctrl.NewDeleteResource[*dapr_dm.DaprSecretStore, dapr_dm.DaprSecretStore](processor, engine, configLoader, options)
			},
		},
		{
			portableresources.DaprPubSubBrokersResourceType,
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &pubsubbrokers.Processor{Client: s.KubeClient}
				return backend_ctrl.NewCreateOrUpdateResource[*dapr_dm.DaprPubSubBroker, dapr_dm.DaprPubSubBroker](processor, engine, client, configLoader, options)
			},
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &pubsubbrokers.Processor{Client: s.KubeClient}
				return backend_ctrl.NewDeleteResource[*dapr_dm.DaprPubSubBroker, dapr_dm.DaprPubSubBroker](processor, engine, configLoader, options)
			},
		},
		{
			portableresources.MongoDatabasesResourceType,
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &mongo_prc.Processor{}
				return backend_ctrl.NewCreateOrUpdateResource[*ds_dm.MongoDatabase, ds_dm.MongoDatabase](processor, engine, client, configLoader, options)
			},
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &mongo_prc.Processor{}
				return backend_ctrl.NewDeleteResource[*ds_dm.MongoDatabase, ds_dm.MongoDatabase](processor, engine, configLoader, options)
			},
		},
		{
			portableresources.RedisCachesResourceType,
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &redis_prc.Processor{}
				return backend_ctrl.NewCreateOrUpdateResource[*ds_dm.RedisCache, ds_dm.RedisCache](processor, engine, client, configLoader, options)
			},
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &redis_prc.Processor{}
				return backend_ctrl.NewDeleteResource[*ds_dm.RedisCache, ds_dm.RedisCache](processor, engine, configLoader, options)
			},
		},
		{
			portableresources.SqlDatabasesResourceType,
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &sql_prc.Processor{}
				return backend_ctrl.NewCreateOrUpdateResource[*ds_dm.SqlDatabase, ds_dm.SqlDatabase](processor, engine, client, configLoader, options)
			},
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &sql_prc.Processor{}
				return backend_ctrl.NewDeleteResource[*ds_dm.SqlDatabase, ds_dm.SqlDatabase](processor, engine, configLoader, options)
			},
		},
	}

	opts := ctrl.Options{
		DataProvider: s.StorageProvider,
		KubeClient:   s.KubeClient,
	}

	for _, rt := range resourceTypes {
		// Register controllers
		err = s.Controllers.Register(ctx, rt.TypeName, v1.OperationDelete, rt.CreateDeleteController, opts)
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
