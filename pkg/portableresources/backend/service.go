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
	dapr_dm "github.com/radius-project/radius/pkg/daprrp/datamodel"
	"github.com/radius-project/radius/pkg/daprrp/processors/pubsubbrokers"
	"github.com/radius-project/radius/pkg/daprrp/processors/secretstores"
	"github.com/radius-project/radius/pkg/daprrp/processors/statestores"
	ds_dm "github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	mongo_prc "github.com/radius-project/radius/pkg/datastoresrp/processors/mongodatabases"
	redis_prc "github.com/radius-project/radius/pkg/datastoresrp/processors/rediscaches"
	sql_prc "github.com/radius-project/radius/pkg/datastoresrp/processors/sqldatabases"
	"github.com/radius-project/radius/pkg/kubeutil"
	msg_dm "github.com/radius-project/radius/pkg/messagingrp/datamodel"
	"github.com/radius-project/radius/pkg/messagingrp/processors/rabbitmqqueues"
	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/portableresources/frontend/handler"

	"github.com/radius-project/radius/pkg/recipes/controllerconfig"

	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	backend_ctrl "github.com/radius-project/radius/pkg/portableresources/backend/controller"
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

	k8s, err := kubeutil.NewClients(s.Options.K8sConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize kubernetes client: %w", err)
	}

	recipeControllerConfig, err := controllerconfig.New(s.Options)
	if err != nil {
		return err
	}

	engine := recipeControllerConfig.Engine
	client := recipeControllerConfig.ResourceClient
	configLoader := recipeControllerConfig.ConfigLoader

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
				return backend_ctrl.NewCreateOrUpdateResource[*msg_dm.RabbitMQQueue, msg_dm.RabbitMQQueue](options, processor, engine, client, configLoader)
			},
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &rabbitmqqueues.Processor{}
				return backend_ctrl.NewDeleteResource[*msg_dm.RabbitMQQueue, msg_dm.RabbitMQQueue](options, processor, engine, configLoader)
			},
		},
		{
			portableresources.DaprStateStoresResourceType,
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &statestores.Processor{Client: k8s.RuntimeClient}
				return backend_ctrl.NewCreateOrUpdateResource[*dapr_dm.DaprStateStore, dapr_dm.DaprStateStore](options, processor, engine, client, configLoader)
			},
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &statestores.Processor{Client: k8s.RuntimeClient}
				return backend_ctrl.NewDeleteResource[*dapr_dm.DaprStateStore, dapr_dm.DaprStateStore](options, processor, engine, configLoader)
			},
		},
		{
			portableresources.DaprSecretStoresResourceType,
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &secretstores.Processor{Client: k8s.RuntimeClient}
				return backend_ctrl.NewCreateOrUpdateResource[*dapr_dm.DaprSecretStore, dapr_dm.DaprSecretStore](options, processor, engine, client, configLoader)
			},
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &secretstores.Processor{Client: k8s.RuntimeClient}
				return backend_ctrl.NewDeleteResource[*dapr_dm.DaprSecretStore, dapr_dm.DaprSecretStore](options, processor, engine, configLoader)
			},
		},
		{
			portableresources.DaprPubSubBrokersResourceType,
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &pubsubbrokers.Processor{Client: k8s.RuntimeClient}
				return backend_ctrl.NewCreateOrUpdateResource[*dapr_dm.DaprPubSubBroker, dapr_dm.DaprPubSubBroker](options, processor, engine, client, configLoader)
			},
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &pubsubbrokers.Processor{Client: k8s.RuntimeClient}
				return backend_ctrl.NewDeleteResource[*dapr_dm.DaprPubSubBroker, dapr_dm.DaprPubSubBroker](options, processor, engine, configLoader)
			},
		},
		{
			portableresources.MongoDatabasesResourceType,
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &mongo_prc.Processor{}
				return backend_ctrl.NewCreateOrUpdateResource[*ds_dm.MongoDatabase, ds_dm.MongoDatabase](options, processor, engine, client, configLoader)
			},
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &mongo_prc.Processor{}
				return backend_ctrl.NewDeleteResource[*ds_dm.MongoDatabase, ds_dm.MongoDatabase](options, processor, engine, configLoader)
			},
		},
		{
			portableresources.RedisCachesResourceType,
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &redis_prc.Processor{}
				return backend_ctrl.NewCreateOrUpdateResource[*ds_dm.RedisCache, ds_dm.RedisCache](options, processor, engine, client, configLoader)
			},
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &redis_prc.Processor{}
				return backend_ctrl.NewDeleteResource[*ds_dm.RedisCache, ds_dm.RedisCache](options, processor, engine, configLoader)
			},
		},
		{
			portableresources.SqlDatabasesResourceType,
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &sql_prc.Processor{}
				return backend_ctrl.NewCreateOrUpdateResource[*ds_dm.SqlDatabase, ds_dm.SqlDatabase](options, processor, engine, client, configLoader)
			},
			func(options ctrl.Options) (ctrl.Controller, error) {
				processor := &sql_prc.Processor{}
				return backend_ctrl.NewDeleteResource[*ds_dm.SqlDatabase, ds_dm.SqlDatabase](options, processor, engine, configLoader)
			},
		},
	}

	opts := ctrl.Options{
		DataProvider: s.StorageProvider,
		KubeClient:   k8s.RuntimeClient,
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
